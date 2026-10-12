package app

import (
	"context"
	"image"
	"os"
	"slices"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

func fpFixture(t *testing.T) *ytdlp.Probe {
	t.Helper()
	b, err := os.ReadFile("../../ytdlp/testdata/single.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ytdlp.ParseProbe(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fpIDs(fs []ytdlp.Format) []string {
	var ids []string
	for _, f := range fs {
		ids = append(ids, f.ID)
	}
	return ids
}

func TestFormatSorted(t *testing.T) {
	p := fpFixture(t)
	before := fpIDs(p.Formats)
	got := fpIDs(fpSorted(p.Formats))
	// video(+audio) by height desc, then audio-only by tbr desc (233 has no tbr: last)
	if want := []string{"401", "137", "18", "160", "251", "140", "233"}; !slices.Equal(got, want) {
		t.Errorf("sorted %v want %v", got, want)
	}
	if !slices.Equal(fpIDs(p.Formats), before) {
		t.Error("fpSorted modified its input")
	}
	// same height: higher tbr first
	same := fpSorted([]ytdlp.Format{
		{ID: "a", Height: 720, TBR: 1, VCodec: "avc1", ACodec: "none"},
		{ID: "b", Height: 720, TBR: 2, VCodec: "vp9", ACodec: "none"},
	})
	if same[0].ID != "b" {
		t.Errorf("tbr tiebreak: %v", fpIDs(same))
	}
}

func TestFormatLabel(t *testing.T) {
	by := map[string]ytdlp.Format{}
	for _, f := range fpFixture(t).Formats {
		by[f.ID] = f
	}
	for _, c := range []struct{ id, want string }{
		{"137", "137 · mp4 · 1012p · 24fps · avc1 · video only · 43.2 MB"}, // video-only (fixture height is 1012)
		{"140", "140 · m4a · audio · mp4a.40.2 · 2.8 MB"},                  // audio-only
		{"18", "18 · mp4 · 360p · 24fps · avc1 · 7.5 MB"},                  // combined, size from filesize_approx
		{"233", "233 · mp4 · audio"},                                       // unknown size and codec
	} {
		if got := fpLabel(by[c.id]); got != c.want {
			t.Errorf("%s: %q want %q", c.id, got, c.want)
		}
	}
	if got := fpLabel(ytdlp.Format{ID: "9", Ext: "webm", VCodec: "vp9", ACodec: "none"}); got != "9 · webm · vp9 · video only" {
		t.Errorf("unknown height/fps/size: %q", got)
	}
	for n, want := range map[int64]string{812: "1 KB", 812000: "812 KB", 43172767: "43.2 MB", 1.2e9: "1.2 GB"} {
		if got := fpSize(n); got != want {
			t.Errorf("fpSize(%d) = %q want %q", n, got, want)
		}
	}
}

// fpFrame lays fp out once on a fresh op list.
func fpFrame(fp *formatPicker, th *theme.Theme) (f ytdlp.Format, picked, cancel bool) {
	gtx := theme.C{Ops: new(op.Ops), Now: time.Now(), Constraints: layout.Exact(image.Pt(900, 700))}
	_, f, picked, cancel = fp.layout(gtx, th)
	return
}

func TestFormatPickerFlow(t *testing.T) {
	p := fpFixture(t)
	th := theme.New(false)
	fp := newFormatPicker(p)
	if !fp.disc.Visible() {
		t.Fatal("discloser should start expanded")
	}
	if _, picked, cancel := fpFrame(fp, th); picked || cancel {
		t.Fatal("nothing happens without input")
	}
	// Download without a selection does nothing
	fp.ok.Click()
	if _, picked, _ := fpFrame(fp, th); picked {
		t.Fatal("picked without a selection")
	}

	u := "https://youtu.be/55XuQm3NZi8"
	for _, c := range []struct {
		row       int // index in the sorted list
		id        string
		videoOnly bool
	}{{1, "137", true}, {2, "18", false}} {
		fp.rows[c.row].Click()
		fpFrame(fp, th) // processes the row click
		fp.ok.Click()
		f, picked, _ := fpFrame(fp, th)
		if !picked || f.ID != c.id || f.VideoOnly() != c.videoOnly {
			t.Fatalf("row %d: picked=%v f=%+v", c.row, picked, f)
		}
		if _, again, _ := fpFrame(fp, th); again {
			t.Error("picked must fire once")
		}
		args := ytdlp.Args(ytdlp.Options{URL: u, Folder: t.TempDir(), FormatID: f.ID, VideoOnlyFormat: f.VideoOnly()})
		i := slices.Index(args, "-f")
		if i < 0 || i+1 >= len(args) {
			t.Fatalf("no -f in %v", args)
		}
		want := c.id
		if c.videoOnly {
			want += "+ba"
		}
		if args[i+1] != want {
			t.Errorf("-f %q want %q", args[i+1], want)
		}
	}

	fp.quit.Click()
	if _, _, cancel := fpFrame(fp, th); !cancel {
		t.Error("cancel not reported")
	}
	if _, _, cancel := fpFrame(fp, th); cancel {
		t.Error("cancel must fire once")
	}
}

// Live sanity check against a D10 URL; needs network and yt-dlp.
func TestFormatPickerLive(t *testing.T) {
	if os.Getenv("YTDLP_LIVE") != "1" {
		t.Skip("set YTDLP_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := ytdlp.ProbeURL(ctx, "https://youtu.be/55XuQm3NZi8", "")
	if err != nil {
		t.Fatal(err)
	}
	fp := newFormatPicker(p)
	if len(fp.formats) == 0 || fp.formats[0].AudioOnly() {
		t.Fatalf("formats: %d", len(fp.formats))
	}
	fpFrame(fp, theme.New(false))
}
