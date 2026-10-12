package ytdlp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Live tests talk to YouTube with the real yt-dlp. Run with
//
//	YTDLP_LIVE=1 go test -run Live ./internal/ytdlp/ -v
//
// Only the D10/D26 test URLs; downloads go to t.TempDir(), never ~/Downloads.
const liveURL = "https://youtu.be/55XuQm3NZi8"

// subsURL has auto-captions (en) but no manual subtitles; 52 s long.
const subsURL = "https://www.youtube.com/watch?v=KRJO1RJTloU"

// cancelURL is a larger video, so the cancel lands mid-download.
const cancelURL = "https://www.youtube.com/watch?v=aIb43qCLguo"

func liveOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("YTDLP_LIVE") != "1" {
		t.Skip("set YTDLP_LIVE=1 to run")
	}
}

func TestLiveVersion(t *testing.T) {
	liveOnly(t)
	v, err := Version(context.Background())
	if err != nil || v == "" {
		t.Fatalf("version %q %v", v, err)
	}
	tag, ok := LatestTag(context.Background())
	t.Logf("local %s, latest %q (ok=%v), newer=%v", v, tag, ok, ok && Newer(v, tag))
}

func TestLiveProbeAndDownloadAudio(t *testing.T) {
	liveOnly(t)
	ctx := context.Background()
	p, err := ProbeURL(ctx, liveURL, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.IsPlaylist || p.ID != "55XuQm3NZi8" || len(p.Formats) == 0 {
		t.Fatalf("probe: %+v", p)
	}
	var videoOnly int
	for _, f := range p.Formats {
		if f.VideoOnly() {
			videoOnly++
		}
	}
	t.Logf("%q: %d formats, %d video-only", p.Title, len(p.Formats), videoOnly)

	dir := t.TempDir()
	var stages []Stage
	res, err := Download(ctx, Options{URL: liveURL, Folder: dir, Preset: AudioM4A}, func(pr Progress) {
		if len(stages) == 0 || stages[len(stages)-1] != pr.Stage {
			stages = append(stages, pr.Stage)
		}
	})
	var ye *Error
	if errors.As(err, &ye) {
		t.Fatalf("%v\nhint: %q\nstderr tail:\n%s", err, ye.Hint, strings.Join(ye.StderrTail, "\n"))
	} else if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(res.Path); err != nil || st.Size() == 0 || filepath.Ext(res.Path) != ".m4a" {
		t.Fatalf("output %q: %v", res.Path, err)
	}
	t.Logf("path=%s dest=%s stages=%v", res.Path, res.Dest, stages)
}

// Cancel a Best download ~2s after its first progress tick (extraction takes a
// few seconds, so a fixed delay from the start could precede any download):
// the group must be gone and .part kept.
func TestLiveCancel(t *testing.T) {
	liveOnly(t)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var last Progress
	var once sync.Once
	start := time.Now()
	res, err := Download(ctx, Options{URL: cancelURL, Folder: dir, Preset: Best}, func(p Progress) {
		last = p
		once.Do(func() { time.AfterFunc(2*time.Second, cancel) })
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v (last progress %+v)", err, last)
	}
	t.Logf("cancelled after %v, last progress %+v, dest=%s", time.Since(start), last, res.Dest)

	// Every process whose command line mentions our temp dir (yt-dlp, ffmpeg) must be gone.
	if out, _ := exec.Command("pgrep", "-af", dir).Output(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("processes survived:\n%s", out)
	}
	parts, _ := filepath.Glob(filepath.Join(dir, "*.part"))
	if len(parts) == 0 {
		t.Errorf(".part file missing after cancel; dir has %v", ls(dir))
	}
	before := ls(dir)
	t.Logf("dir before RemovePartFiles: %q", before)
	if th, _ := filepath.Glob(filepath.Join(dir, "*.webp")); len(th) == 0 {
		t.Logf("note: no .webp thumbnail present before removal")
	}
	// and RemovePartFiles finds exactly those
	if err := RemovePartFiles(res.Dest); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.part*")); len(left) != 0 {
		t.Errorf("RemovePartFiles left %v", left)
	}
	if left := ls(dir); len(left) != 0 {
		t.Errorf("dir not empty after RemovePartFiles: %q", left)
	}
}

// Subs on a video preset must embed a subtitle stream from the auto-captions,
// and leave no .vtt behind.
func TestLiveSubs(t *testing.T) {
	liveOnly(t)
	dir := t.TempDir()
	res, err := Download(context.Background(), Options{URL: subsURL, Folder: dir, Preset: P720, Subs: true, SubLang: "en"}, nil)
	var ye *Error
	if errors.As(err, &ye) {
		t.Fatalf("%v\nhint: %q\nstderr tail:\n%s", err, ye.Hint, strings.Join(ye.StderrTail, "\n"))
	} else if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=index,codec_type,codec_name:stream_tags=language", "-of", "compact", res.Path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe: %v\n%s", err, out)
	}
	t.Logf("ffprobe:\n%s", out)
	if !strings.Contains(string(out), "codec_type=subtitle") {
		t.Errorf("no subtitle stream in %s", res.Path)
	}
	t.Logf("dir: %q", ls(dir))
	if v, _ := filepath.Glob(filepath.Join(dir, "*.vtt")); len(v) != 0 {
		t.Errorf("stray subtitle files: %v", v)
	}
}

func ls(dir string) []string {
	es, _ := os.ReadDir(dir)
	var n []string
	for _, e := range es {
		n = append(n, e.Name())
	}
	return n
}
