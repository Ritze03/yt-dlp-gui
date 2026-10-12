package ytdlp

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseProgressLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Progress
	}{
		{"exact total, null speed/eta",
			`PROG av01.0.12M.08 none {"status": "downloading", "downloaded_bytes": 25, "total_bytes": 100, "eta": null, "speed": null}`,
			Progress{Stage: StageVideo, Percent: 0.25, Downloaded: 25, Total: 100}},
		{"null total_bytes -> estimate",
			`PROG none mp4a.40.2 {"status": "downloading", "downloaded_bytes": 50, "total_bytes": null, "total_bytes_estimate": 200.5, "speed": 1024.5, "eta": 12}`,
			Progress{Stage: StageAudio, Percent: 50 / 200.5, Speed: 1024.5, ETA: 12 * time.Second, Downloaded: 50, Total: 200}},
		{"fragments only",
			`PROG avc1.64001F mp4a.40.2 {"status": "downloading", "downloaded_bytes": 10, "total_bytes": null, "total_bytes_estimate": null, "fragment_index": 3, "fragment_count": 12}`,
			Progress{Stage: StageDownloading, Percent: 0.25, Downloaded: 10}},
		{"nothing known",
			`PROG NA NA {"status": "downloading", "downloaded_bytes": 10}`,
			Progress{Stage: StageDownloading, Percent: -1, Downloaded: 10}},
		{"finished without totals",
			`PROG none opus {"status": "finished", "downloaded_bytes": null}`,
			Progress{Stage: StageAudio, Percent: 1}},
		{"over 100% clamps",
			`PROG NA NA {"status": "downloading", "downloaded_bytes": 120, "total_bytes_estimate": 100}`,
			Progress{Stage: StageDownloading, Percent: 1, Downloaded: 120, Total: 100}},
		{"zero total ignored",
			`PROG NA NA {"status": "downloading", "downloaded_bytes": 5, "total_bytes": 0}`,
			Progress{Stage: StageDownloading, Percent: -1, Downloaded: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseProgressLine(tc.line)
			if !ok || got != tc.want {
				t.Errorf("ok=%v\n got %+v\nwant %+v", ok, got, tc.want)
			}
		})
	}
	for _, bad := range []string{"", "[download] 10%", "PROG", "PROG a b not-json", `DONE /x`} {
		if _, ok := ParseProgressLine(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

// best_run.txt is a recorded `Best` run (stdout of a real yt-dlp 2026.08.19
// call with Args), trimmed to every 25th progress tick. Feeding it through the
// tracker must give Video -> Audio -> Merging -> Processing without the stage
// ever moving backwards, and capture the DEST/DONE paths.
func TestTrackerRecordedBestRun(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "best_run.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var tr tracker
	var events []Progress
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if p, ok := tr.feed(sc.Text()); ok {
			events = append(events, p)
		}
	}
	if len(events) == 0 {
		t.Fatal("no events")
	}
	var order []Stage
	last := Progress{Stage: StageStarting}
	for _, e := range events {
		if e.Stage < last.Stage {
			t.Errorf("stage went backwards: %v after %v", e.Stage, last.Stage)
		}
		if len(order) == 0 || order[len(order)-1] != e.Stage {
			order = append(order, e.Stage)
		}
		last = e
	}
	wantOrder := []Stage{StageVideo, StageAudio, StageMerging, StageProcessing}
	if len(order) != len(wantOrder) {
		t.Fatalf("stage order %v want %v", order, wantOrder)
	}
	for i := range order {
		if order[i] != wantOrder[i] {
			t.Fatalf("stage order %v want %v", order, wantOrder)
		}
	}
	// per-stream percent runs 0..1 in both download stages (the "reset" the stages explain)
	var vMax, aMax float64
	for _, e := range events {
		switch e.Stage {
		case StageVideo:
			vMax = max(vMax, e.Percent)
		case StageAudio:
			aMax = max(aMax, e.Percent)
		case StageMerging, StageProcessing:
			if e.Percent != -1 {
				t.Errorf("%v percent = %v, want -1", e.Stage, e.Percent)
			}
		}
	}
	if vMax != 1 || aMax != 1 {
		t.Errorf("video max %v audio max %v, want 1/1", vMax, aMax)
	}
	if tr.dest == "" || tr.path == "" || tr.path != "/tmp/dl/phem - STFU (official video) [55XuQm3NZi8].mp4" {
		t.Errorf("dest=%q path=%q", tr.dest, tr.path)
	}
}

func TestTrackerStageLines(t *testing.T) {
	var tr tracker
	feed := func(l string) (Stage, bool) { p, ok := tr.feed(l); return p.Stage, ok }

	// audio-only preset: audio stream, then ExtractAudio etc. collapse into one Processing event
	if s, ok := feed(`PROG none mp4a.40.2 {"status":"downloading","downloaded_bytes":1,"total_bytes":2}`); !ok || s != StageAudio {
		t.Fatalf("audio: %v %v", s, ok)
	}
	if s, ok := feed(`[ExtractAudio] Not converting audio /x.m4a; file is already in target format m4a`); !ok || s != StageProcessing {
		t.Fatalf("extract: %v %v", s, ok)
	}
	for _, l := range []string{`[Metadata] Adding metadata to "/x.m4a"`, `[EmbedThumbnail] ffmpeg: Adding thumbnail to "/x.m4a"`, `[Fixup] foo`} {
		if _, ok := feed(l); ok {
			t.Errorf("%q should not re-emit", l)
		}
	}
	// noise is ignored
	for _, l := range []string{`[info] Downloading 1 format(s): 140`, `[download] Destination: /x`, `Deleting original file /x`, ``} {
		if _, ok := feed(l); ok {
			t.Errorf("%q should be ignored", l)
		}
	}
	feed("DEST /d/x.webm")
	feed("DONE /d/x.m4a")
	if tr.dest != "/d/x.webm" || tr.path != "/d/x.m4a" {
		t.Errorf("dest=%q path=%q", tr.dest, tr.path)
	}
}
