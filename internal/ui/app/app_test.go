package app

import (
	"testing"
	"time"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/ytdlp"
)

func TestHintFor403(t *testing.T) {
	e := &ytdlp.Error{LastError: "unable to download: HTTP Error 403: Forbidden", Hint: ytdlp.Hint("HTTP Error 403: Forbidden")}
	a := &App{version: "2026.08.19", latest: "2026.08.19"}
	if got := a.hintFor(e); got != blockedHint {
		t.Errorf("up to date: %q", got)
	}
	a.latest = "2026.09.01"
	if got := a.hintFor(e); got != outdatedHint {
		t.Errorf("newer release: %q", got)
	}
	a.latest = "" // check failed
	if got := a.hintFor(e); got != blockedHint {
		t.Errorf("no latest: %q", got)
	}
	geo := &ytdlp.Error{LastError: "geo restriction", Hint: ytdlp.Hint("geo restriction")}
	if got := a.hintFor(geo); got != geo.Hint || got == "" {
		t.Errorf("other hints pass through: %q", got)
	}
	if a.hintFor(nil) != "" {
		t.Error("nil error has no hint")
	}
}

func TestRowText(t *testing.T) {
	j := queue.Job{State: queue.Running, Progress: ytdlp.Progress{Stage: ytdlp.StageAudio, Percent: 0.42, Speed: 3.26e6, ETA: 83 * time.Second}}
	if got := stateText(j); got != "Downloading audio · 42%" {
		t.Errorf("stateText = %q", got)
	}
	if got := rateText(j); got != "3.3 MB/s · 1m23s" {
		t.Errorf("rateText = %q", got)
	}
	j.Progress = ytdlp.Progress{Stage: ytdlp.StageMerging, Percent: -1}
	if got := stateText(j); got != "Merging" || rateText(j) != "" {
		t.Errorf("merging: %q %q", got, rateText(j))
	}
	if got := stateText(queue.Job{State: queue.Failed}); got != "Failed" {
		t.Errorf("failed: %q", got)
	}
}
