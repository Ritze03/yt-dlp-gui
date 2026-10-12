package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
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

// Regression (QC H1): the footer must sit inside the card and be clickable at
// every window height, for short and long lists. A press+release at the Queue
// button's spot (bottom-right of the card, inside the 16dp padding) must queue.
func TestPlaylistFooterInsideCardAndClickable(t *testing.T) {
	th := theme.New(true)
	for _, h := range []int{520, 750, 1000} {
		for _, n := range []int{3, 205} {
			t.Run(fmt.Sprintf("h%d_n%d", h, n), func(t *testing.T) {
				p := &ytdlp.Probe{Title: "PL"}
				for i := range n {
					p.Entries = append(p.Entries, ytdlp.Entry{ID: fmt.Sprint(i), Title: "Entry", Index: i + 1, Duration: 61})
				}
				pp := newPlaylistPicker(p)
				var r input.Router
				gtx := layout.Context{
					Ops:         new(op.Ops),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Constraints{Max: image.Pt(900, h)},
					Source:      r.Source(),
				}
				frame := func() (theme.D, bool, bool) {
					gtx.Reset()
					d, q, c := pp.layout(gtx, th)
					r.Frame(gtx.Ops)
					return d, q, c
				}
				d, _, _ := frame()
				if d.Size.Y > h*6/10 {
					t.Fatalf("card height %d > 60%% of %d", d.Size.Y, h)
				}
				if n == 3 && d.Size.Y >= h*6/10 {
					t.Errorf("short list should shrink the card, got %d of max %d", d.Size.Y, h*6/10)
				}
				if n == 205 && d.Size.Y < h*6/10-2 {
					t.Errorf("long list should fill the 60%% cap, got %d of %d", d.Size.Y, h*6/10)
				}
				pos := f32.Pt(float32(d.Size.X-16-10), float32(d.Size.Y-16-10))
				r.Queue(
					pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pos},
					pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos},
					pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos},
				)
				if _, q, _ := frame(); !q {
					t.Errorf("click at %v (card %v) did not hit Queue: footer is outside the card or covered", pos, d.Size)
				}
				if got := len(pp.checked()); got != n {
					t.Errorf("click toggled entries: %d checked, want %d", got, n)
				}
			})
		}
	}
}
