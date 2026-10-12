package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"slices"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

func playlistFixture(t *testing.T) *playlistPicker {
	t.Helper()
	b, err := os.ReadFile("testdata/playlist_flat.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ytdlp.ParseProbe(b)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsPlaylist || len(p.Entries) != 5 {
		t.Fatalf("fixture: IsPlaylist=%v entries=%d", p.IsPlaylist, len(p.Entries))
	}
	return newPlaylistPicker(p)
}

func playlistIDs(es []ytdlp.Entry) []string {
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestPlaylistChecked(t *testing.T) {
	pp := playlistFixture(t)
	if got := len(pp.checked()); got != 5 {
		t.Fatalf("all checked by default, got %d", got)
	}
	pp.on[0].Value, pp.on[3].Value = false, false
	want := []string{"bbbbbbbbbbb", "ccccccccccc", "eeeeeeeeeee"}
	if got := playlistIDs(pp.checked()); !slices.Equal(got, want) {
		t.Errorf("checked = %v, want %v", got, want)
	}
	pp.setAll(false)
	if got := pp.checked(); len(got) != 0 {
		t.Errorf("setAll(false): %v", got)
	}
	pp.setAll(true)
	if got := len(pp.checked()); got != 5 {
		t.Errorf("setAll(true): %d", got)
	}
}

func TestPlaylistDur(t *testing.T) {
	for in, want := range map[float64]string{0: "", -1: "", 59: "0:59", 125: "2:05", 3725: "1:02:05"} {
		if got := playlistDur(in); got != want {
			t.Errorf("playlistDur(%v) = %q, want %q", in, got, want)
		}
	}
}

// Acceptance: a fixture -J --flat-playlist JSON, picker, queue only the checked
// entries, each with the D21 playlist template.
func TestPlaylistQueuesCheckedWithTemplate(t *testing.T) {
	pp := playlistFixture(t)
	pp.on[1].Value, pp.on[4].Value = false, false // keep 1st, 3rd, 4th

	runs := make(chan ytdlp.Options, 10)
	q := queue.New(func(_ context.Context, o ytdlp.Options, _ func(ytdlp.Progress)) (ytdlp.Result, error) {
		runs <- o
		return ytdlp.Result{}, nil
	}, 1, nil)
	base := ytdlp.Options{Folder: t.TempDir(), Preset: ytdlp.AudioM4A}
	q.AddEntries(pp.checked(), base)

	// Wait for all three runs before Close, which would cancel unstarted ones.
	var got []ytdlp.Options
	for range 3 {
		select {
		case o := <-runs:
			got = append(got, o)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out after %d runs", len(got))
		}
	}
	q.Close()
	if len(runs) != 0 {
		t.Fatalf("more runs than checked entries: %d extra", len(runs))
	}

	wantURL := []string{
		"https://www.youtube.com/watch?v=aaaaaaaaaaa",
		"https://www.youtube.com/watch?v=ccccccccccc",
		"https://www.youtube.com/watch?v=ddddddddddd",
	}
	wantIdx := []int{1, 3, 4}
	for i, o := range got {
		if o.URL != wantURL[i] {
			t.Errorf("run %d URL = %q, want %q", i, o.URL, wantURL[i])
		}
		if o.Preset != ytdlp.AudioM4A || o.Folder != base.Folder {
			t.Errorf("run %d did not inherit base options: %+v", i, o)
		}
		// "Rock/Pop: Best 100%" -> sanitised path component, % escaped for yt-dlp.
		want := fmt.Sprintf("Rock_Pop_ Best 100%%%%/%03d - %%(title)s.%%(ext)s", wantIdx[i])
		args := ytdlp.Args(o)
		k := slices.Index(args, "-o")
		if k < 0 || k+1 >= len(args) || args[k+1] != want {
			t.Errorf("run %d: -o not %q in %q", i, want, args)
		}
	}
}

// Layout smoke test: the card stays within 60% of the window and is non-empty
// for a long list; an empty-selection frame must not panic either.
func TestPlaylistLayout(t *testing.T) {
	pp := playlistFixture(t)
	for i := 0; i < 200; i++ {
		pp.entries = append(pp.entries, ytdlp.Entry{ID: fmt.Sprint(i), Title: "Extra", Index: 6 + i})
	}
	pp.on = append(pp.on, make([]widget.Bool, 200)...)
	th := theme.New(true)
	frame := func() (theme.D, bool, bool) {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(900, 700)),
		}
		return pp.layout(gtx, th)
	}
	d, q, c := frame()
	if q || c {
		t.Errorf("no click yet, got queue=%v cancel=%v", q, c)
	}
	if d.Size.Y == 0 || d.Size.Y > 700*6/10 {
		t.Errorf("height = %d, want 1..420", d.Size.Y)
	}
	pp.setAll(false)
	if d, _, _ := frame(); d.Size.Y == 0 {
		t.Error("empty selection collapsed the card")
	}
}
