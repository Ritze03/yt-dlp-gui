package app

import (
	"image"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

func TestRowCmds(t *testing.T) {
	p := "/tmp/dl/Ängie - Dope [aIb43qCLguo].m4a"
	if got, want := rowOpenCmd(p), []string{"xdg-open", p}; !reflect.DeepEqual(got, want) {
		t.Errorf("rowOpenCmd = %q, want %q", got, want)
	}
	if got, want := rowShowCmd(p), []string{"xdg-open", "/tmp/dl"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rowShowCmd = %q, want %q", got, want)
	}
}

func TestRowActions(t *testing.T) {
	for s, want := range map[queue.State][]rowAction{
		queue.Queued:    {rowCancel},
		queue.Running:   {rowCancel},
		queue.Done:      {rowOpen, rowShow, rowRemove},
		queue.Failed:    {rowRetry, rowRemove},
		queue.Cancelled: {rowRetry, rowRemove},
	} {
		if got := rowActions(s); !reflect.DeepEqual(got, want) {
			t.Errorf("rowActions(%v) = %v, want %v", s, got, want)
		}
	}
}

// TestRowLayout is a smoke test: every state, with the Details block expanded, lays out.
func TestRowLayout(t *testing.T) {
	th := theme.New(true)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 800)),
	}
	e := &ytdlp.Error{LastError: "ERROR: boom", StderrTail: make([]string, 20)}
	for _, s := range []queue.State{queue.Queued, queue.Running, queue.Done, queue.Failed, queue.Cancelled} {
		r := &rowState{expanded: true}
		j := queue.Job{State: s, Title: "t", Path: "/tmp/x.m4a", Err: e}
		if d := r.layout(gtx, th, nil, j, "hint"); d.Size.Y == 0 {
			t.Errorf("%v: empty layout", s)
		}
	}
}
