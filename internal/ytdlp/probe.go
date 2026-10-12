package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Format is one downloadable stream from a single-video probe.
type Format struct {
	ID     string
	Ext    string
	Note   string // yt-dlp format_note, e.g. "1080p", "medium"
	Height int    // 0 for audio
	FPS    float64
	VCodec string // "none" for audio-only; "" if unknown
	ACodec string // "none" for video-only; "" if unknown
	Size   int64  // filesize or filesize_approx; 0 if unknown
	TBR    float64
}

// VideoOnly reports a video stream without audio (needs "+ba" when downloaded).
func (f Format) VideoOnly() bool { return hasCodec(f.VCodec) && f.ACodec == "none" }

// AudioOnly reports an audio stream without video.
func (f Format) AudioOnly() bool { return f.VCodec == "none" && f.ACodec != "none" }

func hasCodec(c string) bool { return c != "" && c != "none" }

// Entry is one video of a playlist.
type Entry struct {
	ID            string
	URL           string
	Title         string
	Duration      float64 // seconds, 0 if unknown
	PlaylistTitle string  // title of the (sub)playlist this entry came from
	Index         int     // 1-based position within that playlist
}

// Probe is the result of ProbeURL.
type Probe struct {
	IsPlaylist bool
	ID, Title  string
	Duration   float64  // single video only
	Formats    []Format // single video only; storyboards removed
	Entries    []Entry  // playlist only; one level of nesting flattened

	subs []Entry // unresolved sub-playlist references (ProbeURL resolves them)
}

// ErrTooDeep is returned when playlists nest more than one level (D15).
var ErrTooDeep = errors.New("playlist is nested too deeply (only one level of sub-playlists is supported)")

// ProbeURL runs `yt-dlp -J --flat-playlist` (with cookies, D17) and parses the
// result. Sub-playlists (channel tabs, mixes) are expanded one level deep with
// one extra call each; deeper nesting yields ErrTooDeep. yt-dlp failures are
// returned as *Error.
func ProbeURL(ctx context.Context, url, cookies string) (*Probe, error) {
	p, err := probeOnce(ctx, url, cookies, true)
	if err != nil || !p.IsPlaylist {
		return p, err
	}
	for _, s := range p.subs {
		sp, err := probeOnce(ctx, s.URL, cookies, false)
		if err != nil {
			return nil, err
		}
		if !sp.IsPlaylist { // the "sub-playlist" turned out to be a plain video
			p.Entries = append(p.Entries, Entry{ID: sp.ID, URL: s.URL, Title: sp.Title, Duration: sp.Duration, PlaylistTitle: p.Title})
			continue
		}
		title := firstNonEmpty(sp.Title, s.Title)
		for _, e := range sp.Entries {
			e.PlaylistTitle = title
			p.Entries = append(p.Entries, e)
		}
	}
	p.subs = nil
	renumber(p.Entries)
	return p, nil
}

func probeOnce(ctx context.Context, url, cookies string, allowNested bool) (*Probe, error) {
	args := append([]string{"-J", "--flat-playlist"}, cookieArgs(cookies)...)
	args = append(args, "--", url)
	var out bytes.Buffer
	if err := run(ctx, args, &out); err != nil {
		return nil, err
	}
	return parseProbe(out.Bytes(), allowNested)
}

// renumber assigns Index 1..n within each run of equal PlaylistTitle.
func renumber(es []Entry) {
	n, prev := 0, "\x00"
	for i := range es {
		if es[i].PlaylistTitle != prev {
			n, prev = 0, es[i].PlaylistTitle
		}
		n++
		es[i].Index = n
	}
}

type rawInfo struct {
	Type       string      `json:"_type"`
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	URL        string      `json:"url"`
	WebpageURL string      `json:"webpage_url"`
	IEKey      string      `json:"ie_key"`
	Duration   float64     `json:"duration"`
	Entries    []*rawInfo  `json:"entries"` // elements may be null (unavailable videos)
	Formats    []rawFormat `json:"formats"`
}

type rawFormat struct {
	FormatID   string  `json:"format_id"`
	Ext        string  `json:"ext"`
	Note       string  `json:"format_note"`
	Height     float64 `json:"height"`
	FPS        float64 `json:"fps"`
	VCodec     string  `json:"vcodec"`
	ACodec     string  `json:"acodec"`
	FileSize   float64 `json:"filesize"`
	FileSizeAp float64 `json:"filesize_approx"`
	TBR        float64 `json:"tbr"`
}

// ParseProbe parses the JSON of `yt-dlp -J --flat-playlist`. It is pure.
// Inline nested playlists (one level) are flattened into Entries; nested
// playlists that are only referenced by URL are left for ProbeURL to resolve.
func ParseProbe(data []byte) (*Probe, error) { return parseProbe(data, true) }

// parseProbe with allowNested=false rejects any sub-playlist (used for the
// second level, so a third level is ErrTooDeep).
func parseProbe(data []byte, allowNested bool) (*Probe, error) {
	var r rawInfo
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse yt-dlp JSON: %w", err)
	}
	if r.Type != "playlist" && r.Type != "multi_video" {
		p := &Probe{ID: r.ID, Title: r.Title, Duration: r.Duration}
		for _, f := range r.Formats {
			if f.Ext == "mhtml" || f.Note == "storyboard" || (f.VCodec == "none" && f.ACodec == "none") {
				continue // storyboards and other non-media pseudo formats
			}
			size := f.FileSize
			if size == 0 {
				size = f.FileSizeAp
			}
			p.Formats = append(p.Formats, Format{
				ID: f.FormatID, Ext: f.Ext, Note: f.Note, Height: int(f.Height), FPS: f.FPS,
				VCodec: f.VCodec, ACodec: f.ACodec, Size: int64(size), TBR: f.TBR,
			})
		}
		return p, nil
	}

	p := &Probe{IsPlaylist: true, ID: r.ID, Title: r.Title}
	for _, e := range r.Entries {
		if e == nil {
			continue
		}
		if isPlaylistRef(e) {
			if !allowNested {
				return nil, ErrTooDeep
			}
			if e.Type == "playlist" || len(e.Entries) > 0 { // inline: flatten one level
				title := firstNonEmpty(e.Title, r.Title)
				for _, c := range e.Entries {
					if c == nil {
						continue
					}
					if isPlaylistRef(c) {
						return nil, ErrTooDeep
					}
					if en, ok := toEntry(c); ok {
						en.PlaylistTitle = title
						p.Entries = append(p.Entries, en)
					}
				}
				continue
			}
			// referenced by URL only: ProbeURL fetches it
			if u := firstNonEmpty(e.URL, e.WebpageURL); u != "" {
				p.subs = append(p.subs, Entry{ID: e.ID, URL: u, Title: e.Title})
			}
			continue
		}
		if en, ok := toEntry(e); ok {
			en.PlaylistTitle = r.Title
			p.Entries = append(p.Entries, en)
		}
	}
	renumber(p.Entries)
	if len(p.Entries) == 0 && len(p.subs) == 0 {
		return nil, errors.New("playlist has no downloadable entries")
	}
	return p, nil
}

// isPlaylistRef reports whether a playlist entry is itself a playlist.
// ponytail: YouTube-shaped heuristic (ie_key YoutubeTab / *Playlist); other
// sites that nest differently just get their sub-playlists treated as videos.
func isPlaylistRef(e *rawInfo) bool {
	return e.Type == "playlist" || e.Type == "multi_video" ||
		strings.HasSuffix(e.IEKey, "Tab") || strings.HasSuffix(e.IEKey, "Playlist")
}

func toEntry(e *rawInfo) (Entry, bool) {
	u := firstNonEmpty(e.URL, e.WebpageURL)
	if u == "" && e.ID != "" && strings.HasPrefix(e.IEKey, "Youtube") {
		u = "https://www.youtube.com/watch?v=" + e.ID
	}
	if e.Title == "[Private video]" || e.Title == "[Deleted video]" { // yt-dlp's placeholder titles
		return Entry{}, false
	}
	if u == "" {
		return Entry{}, false
	}
	return Entry{ID: e.ID, URL: u, Title: e.Title, Duration: e.Duration}, true
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
