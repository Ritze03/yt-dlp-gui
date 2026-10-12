package ytdlp

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Stage is the phase of a download job, for the UI label.
type Stage int

const (
	StageStarting    Stage = iota // before the first progress line
	StageDownloading              // single stream (video and audio together)
	StageVideo                    // video-only stream of a video+audio job
	StageAudio                    // audio-only stream (second stream, or audio presets)
	StageMerging                  // ffmpeg merging video and audio
	StageProcessing               // metadata, thumbnail, subtitles, audio extraction, ...
	StageDone
)

func (s Stage) String() string {
	return [...]string{"Starting", "Downloading", "Video", "Audio", "Merging", "Processing", "Done"}[s]
}

// Progress is one progress event. Percent is for the current stream only, so
// with video+audio it runs 0..1 per stage: show the Stage next to the bar.
type Progress struct {
	Stage      Stage
	Percent    float64       // 0..1; -1 when unknown (merging, processing, no totals)
	Speed      float64       // bytes/s; 0 when unknown
	ETA        time.Duration // 0 when unknown
	Downloaded int64         // bytes of the current stream
	Total      int64         // bytes of the current stream (estimate if exact is unknown); 0 when unknown
}

// rawProgress is yt-dlp's %(progress)j; every number can be null.
type rawProgress struct {
	Status        string   `json:"status"`
	Downloaded    *float64 `json:"downloaded_bytes"`
	Total         *float64 `json:"total_bytes"`
	TotalEstimate *float64 `json:"total_bytes_estimate"`
	Speed         *float64 `json:"speed"`
	ETA           *float64 `json:"eta"`
	FragIndex     *float64 `json:"fragment_index"`
	FragCount     *float64 `json:"fragment_count"`
}

// ParseProgressLine parses a "PROG <vcodec> <acodec> <json>" stdout line (see
// Args). ok is false for any other line. It is pure; the stage comes from the
// codecs of the stream being fetched, so no state is needed.
func ParseProgressLine(line string) (Progress, bool) {
	rest, ok := strings.CutPrefix(line, progPrefix)
	if !ok {
		return Progress{}, false
	}
	vcodec, rest, _ := strings.Cut(rest, " ")
	acodec, js, _ := strings.Cut(rest, " ")
	var r rawProgress
	if json.Unmarshal([]byte(js), &r) != nil {
		return Progress{}, false
	}

	p := Progress{Stage: streamStage(vcodec, acodec), Percent: -1}
	if r.Downloaded != nil {
		p.Downloaded = int64(*r.Downloaded)
	}
	total := 0.0
	if r.Total != nil && *r.Total > 0 {
		total = *r.Total
	} else if r.TotalEstimate != nil && *r.TotalEstimate > 0 {
		total = *r.TotalEstimate
	}
	p.Total = int64(total)
	switch {
	case total > 0 && r.Downloaded != nil:
		p.Percent = clamp01(*r.Downloaded / total)
	case r.FragCount != nil && *r.FragCount > 0 && r.FragIndex != nil:
		p.Percent = clamp01(*r.FragIndex / *r.FragCount)
	case r.Status == "finished":
		p.Percent = 1
	}
	if r.Speed != nil && *r.Speed > 0 {
		p.Speed = *r.Speed
	}
	if r.ETA != nil && *r.ETA > 0 {
		p.ETA = time.Duration(*r.ETA * float64(time.Second))
	}
	return p, true
}

// streamStage: video-only -> Video, audio-only -> Audio, anything else
// (combined, unknown "NA") -> Downloading.
// ponytail: with unknown codecs a video+audio job shows one "Downloading"
// stage and the bar resets once; acceptable for the rare extractor that omits codecs.
func streamStage(vcodec, acodec string) Stage {
	v, a := hasCodec(vcodec) && vcodec != "NA", hasCodec(acodec) && acodec != "NA"
	switch {
	case v && acodec == "none":
		return StageVideo
	case vcodec == "none" && a:
		return StageAudio
	}
	return StageDownloading
}

func clamp01(f float64) float64 {
	return min(max(f, 0), 1)
}

var processingRE = regexp.MustCompile(`^\[(ExtractAudio|Metadata|EmbedThumbnail|ThumbnailsConvertor|EmbedSubtitle|SubtitlesConvertor|Fixup\w*|VideoConvertor|VideoRemuxer|ModifyChapters)\]`)

// tracker turns the stdout line stream into events and remembers the
// DEST/DONE paths.
type tracker struct {
	stage Stage
	dest  string // intended final path from before_dl (for .part cleanup)
	path  string // final file from after_move
}

// feed consumes one stdout line. ok reports that p should be emitted.
func (t *tracker) feed(line string) (p Progress, ok bool) {
	switch {
	case strings.HasPrefix(line, progPrefix):
		if p, ok = ParseProgressLine(line); ok {
			t.stage = p.Stage
		}
		return p, ok
	case strings.HasPrefix(line, destPrefix):
		t.dest = strings.TrimPrefix(line, destPrefix)
	case strings.HasPrefix(line, donePrefix):
		t.path = strings.TrimPrefix(line, donePrefix)
	case strings.HasPrefix(line, "[Merger]"):
		return t.enter(StageMerging)
	case processingRE.MatchString(line):
		return t.enter(StageProcessing)
	}
	return Progress{}, false
}

// enter emits a no-percent event only when the stage actually changes
// (several [Metadata]/[EmbedThumbnail] lines follow each other).
func (t *tracker) enter(s Stage) (Progress, bool) {
	if t.stage == s || t.stage > s { // never go backwards
		return Progress{}, false
	}
	t.stage = s
	return Progress{Stage: s, Percent: -1}, true
}
