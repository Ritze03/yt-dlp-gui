// Package ytdlp drives the system yt-dlp binary as a subprocess: probe a URL,
// build download arguments, run a download with progress events, cancel it,
// classify errors, and report the yt-dlp version. No UI code lives here.
//
// Typical use by a queue:
//
//	p, err := ytdlp.ProbeURL(ctx, url, cookies)        // once per pasted URL
//	o := ytdlp.Options{URL: e.URL, Folder: dir, Preset: ytdlp.Best, Playlist: &e}
//	res, err := ytdlp.Download(ctx, o, func(pr ytdlp.Progress) { ... })
//	// cancel: cancel ctx. Download returns after the whole process group is gone.
//	// remove row: ytdlp.RemovePartFiles(res.Dest)
package ytdlp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Bin is the yt-dlp executable. It is looked up on PATH and the child inherits
// the full environment (yt-dlp needs deno on PATH for YouTube JS challenges).
// Tests may point it at a fake script.
var Bin = "yt-dlp"

// Preset is a one-click format choice, in UI order.
type Preset int

const (
	Best     Preset = iota // best video+audio
	P1080                  // up to 1080p
	P720                   // up to 720p
	AudioM4A               // audio only, m4a
	AudioMP3               // audio only, mp3
)

// Presets returns all presets in UI order.
func Presets() []Preset { return []Preset{Best, P1080, P720, AudioM4A, AudioMP3} }

// String is the UI label.
func (p Preset) String() string {
	switch p {
	case Best:
		return "Best video+audio"
	case P1080:
		return "1080p"
	case P720:
		return "720p"
	case AudioM4A:
		return "Audio only (m4a)"
	case AudioMP3:
		return "Audio only (mp3)"
	}
	return fmt.Sprintf("Preset(%d)", int(p))
}

// IsAudio reports whether the preset extracts audio only.
func (p Preset) IsAudio() bool { return p == AudioM4A || p == AudioMP3 }

// Browsers are the accepted "cookies from browser" values; "none" (or "")
// means no cookies.
var Browsers = []string{"none", "firefox", "chromium", "brave", "chrome", "zen", "floorp"}

// Options describes one download job.
type Options struct {
	URL    string // the video URL (for a playlist entry: Playlist.URL)
	Folder string // -P target folder
	Preset Preset

	// Advanced single-video format (ignored when empty). VideoOnlyFormat
	// appends "+ba" so the chosen video-only stream gets audio.
	FormatID        string
	VideoOnlyFormat bool

	Cookies string // browser from Browsers; "" or "none" = off
	Subs    bool   // embed subtitles (ignored for audio-only presets)
	SubLang string // default "en"

	// Playlist is non-nil for a playlist entry: output goes to
	// "<PlaylistTitle>/<NNN> - <title>.<ext>". Index is 1-based.
	Playlist *Entry
}

const (
	singleTemplate = "%(title)s [%(id)s].%(ext)s"
	progPrefix     = "PROG "
	destPrefix     = "DEST "
	donePrefix     = "DONE "
)

// cookieArgs returns the --cookies-from-browser args shared by probe and download.
func cookieArgs(browser string) []string {
	if browser == "" || browser == "none" {
		return nil
	}
	return []string{"--cookies-from-browser", cookieSpec(browser)}
}

// forkRoots are the profile roots (relative to $HOME, native first, then
// Flatpak) of the Firefox forks yt-dlp has no name for.
var forkRoots = map[string][]string{
	"zen":    {".zen", ".var/app/app.zen_browser.zen/.zen"},
	"floorp": {".floorp", ".var/app/one.ablaze.floorp/.floorp"},
}

// cookieSpec maps a Browsers value to yt-dlp's BROWSER[:PROFILE] spec. zen and
// floorp are Firefox-format, so they become "firefox:<profile root>"; yt-dlp
// takes the newest cookies.sqlite under that root (root, root/*/, root/Profiles/*/).
// The first existing root wins; none existing gives the native one, so yt-dlp
// reports its own "could not find firefox cookies database" error.
func cookieSpec(browser string) string {
	rels, ok := forkRoots[browser]
	if !ok {
		return browser
	}
	// ponytail: no $HOME means a relative root, which yt-dlp will fail on clearly.
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, rels[0])
	for _, r := range rels {
		if st, err := os.Stat(filepath.Join(home, r)); err == nil && st.IsDir() {
			root = filepath.Join(home, r)
			break
		}
	}
	return "firefox:" + root
}

// Args builds the yt-dlp argument list for a download (without the binary
// name). It is pure.
//
// Output contract, relied on by Download:
//   - stdout "PROG <vcodec> <acodec> <progress json>" for every progress tick
//     (codecs of the stream being fetched: video-only / audio-only / both);
//   - stdout "DEST <path>" once, before download: the intended final path
//     (extension may differ after -x), used to locate .part files;
//   - stdout "DONE <path>" once, after all post-processing: the final file.
//
// --print implies --quiet, so --no-quiet --progress bring back progress and
// the [Merger]/[ExtractAudio]/... lines used for stage detection.
func Args(o Options) []string {
	a := []string{
		"--no-quiet", "--progress", "--newline", "--progress-delta", "0.3",
		"--progress-template", "download:" + progPrefix + "%(info.vcodec)s %(info.acodec)s %(progress)j",
		"--print", "before_dl:" + destPrefix + "%(_filename)s",
		"--print", "after_move:" + donePrefix + "%(filepath)s",
		"--no-playlist", // every job is one video; entry URLs must not re-expand
		"--embed-metadata", "--embed-thumbnail", "--embed-chapters",
	}
	// ponytail: subtitles are skipped for audio presets; an audio container
	// can't embed them and --write-subs would leave stray .vtt files.
	if o.Subs && !o.Preset.IsAudio() {
		lang := o.SubLang
		if lang == "" {
			lang = "en"
		}
		a = append(a, "--write-subs", "--embed-subs", "--sub-langs", lang)
	}
	a = append(a, cookieArgs(o.Cookies)...)
	a = append(a, formatArgs(o)...)
	a = append(a, "-P", o.Folder, "-o", outputTemplate(o.Playlist), "--", o.URL)
	return a
}

// formatArgs maps FormatID (advanced) or Preset to -f/-S/-x flags (D25).
func formatArgs(o Options) []string {
	if o.FormatID != "" {
		f := o.FormatID
		if o.VideoOnlyFormat {
			f += "+ba"
		}
		return []string{"-f", f}
	}
	const sortTail = ",ext:mp4:m4a" // prefer mp4/m4a: --embed-thumbnail dislikes webm
	switch o.Preset {
	case P1080:
		return []string{"-f", "bv*+ba/b", "-S", "res:1080" + sortTail}
	case P720:
		return []string{"-f", "bv*+ba/b", "-S", "res:720" + sortTail}
	case AudioM4A:
		return []string{"-f", "ba[ext=m4a]/ba", "-x", "--audio-format", "m4a"}
	case AudioMP3:
		return []string{"-f", "ba", "-x", "--audio-format", "mp3", "--audio-quality", "0"}
	}
	return []string{"-f", "bv*+ba/b", "-S", "res" + sortTail}
}

// outputTemplate is the -o value (D21). A playlist entry is downloaded from
// its own URL, where yt-dlp has no playlist_title/playlist_index, so the
// sanitised title and zero-padded index are substituted here.
func outputTemplate(e *Entry) string {
	if e == nil {
		return singleTemplate
	}
	title := strings.ReplaceAll(sanitize(e.PlaylistTitle), "%", "%%") // % is the template escape
	return fmt.Sprintf("%s/%03d - %%(title)s.%%(ext)s", title, e.Index)
}

var badNameChars = regexp.MustCompile(`[\x00-\x1f/\\:*?"<>|]+`)

// sanitize makes s safe as one path component. ponytail: coarse replacement
// with "_", not yt-dlp's lookalike-unicode substitution.
func sanitize(s string) string {
	s = strings.Trim(strings.TrimSpace(badNameChars.ReplaceAllString(s, "_")), ". ")
	if s == "" {
		return "playlist"
	}
	return s
}
