package app

import (
	"context"
	"errors"
	"image"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"yt-dlp-gui/internal/ui/theme"
)

func fakeClip(out string, err error, seen *[][]string) clipRunner {
	return func(_ context.Context, argv []string) ([]byte, error) {
		if seen != nil {
			*seen = append(*seen, argv)
		}
		return []byte(out), err
	}
}

func TestClipCommands(t *testing.T) {
	var seen [][]string
	if s, err := readClipboard(fakeClip("  https://e/v \n", nil, &seen), true); err != nil || s != "https://e/v" {
		t.Errorf("wayland: %q, %v", s, err)
	}
	if len(seen) != 1 || seen[0][0] != "wl-paste" || !slices.Contains(seen[0], "--no-newline") {
		t.Errorf("wayland argv = %v", seen)
	}

	// X11: xclip missing falls back to xsel.
	seen = nil
	run := func(_ context.Context, argv []string) ([]byte, error) {
		seen = append(seen, argv)
		if argv[0] == "xclip" {
			return nil, exec.ErrNotFound
		}
		return []byte("https://e/x\n"), nil
	}
	if s, err := readClipboard(run, false); err != nil || s != "https://e/x" {
		t.Errorf("x11: %q, %v", s, err)
	}
	if len(seen) != 2 || seen[0][0] != "xclip" || seen[1][0] != "xsel" {
		t.Errorf("x11 argv = %v", seen)
	}
}

func TestClipErrors(t *testing.T) {
	for _, c := range []struct {
		name    string
		run     clipRunner
		wayland bool
		want    string
	}{
		{"missing wl-paste", fakeClip("", exec.ErrNotFound, nil), true, "wl-paste not found"},
		{"missing x11", fakeClip("", exec.ErrNotFound, nil), false, "xclip / xsel not found"},
		{"blank", fakeClip(" \n\t", nil, nil), true, "empty"},
		{"nonzero exit", fakeClip("", errors.New("exit status 1"), nil), true, "empty or not text"},
		{"binary", fakeClip("\x89PNG\x00\x01", nil, nil), false, "not text"},
		{"huge", fakeClip(strings.Repeat("a", clipMax+1), nil, nil), true, "too long"},
	} {
		_, err := readClipboard(c.run, c.wayland)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

// pasteApp is an App with a themed URL field and a fake clipboard. frame runs
// one frame like App.frame: queue evs, run hook (where pasteKey sits), lay out
// the field (whose Update drains key events), then close the frame. The first
// frames focus the field and register its key filters.
func pasteApp(clip string) (a *App, frame func(hook func(layout.Context), evs ...key.Event) layout.Context) {
	a = winApp()
	a.th = theme.New(true)
	a.url.SingleLine = true
	a.clipRun = fakeClip(clip, nil, nil)
	var r input.Router
	var ops op.Ops
	frame = func(hook func(layout.Context), evs ...key.Event) layout.Context {
		for _, e := range evs {
			r.Queue(e)
		}
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(400, 40))}
		if hook != nil {
			hook(gtx)
		}
		a.th.Input(gtx, &a.url, "hint")
		r.Frame(&ops)
		return gtx
	}
	gtx := frame(nil)
	gtx.Execute(key.FocusCmd{Tag: &a.url})
	frame(nil)
	frame(nil)
	return
}

func waitPaste(a *App) {
	for i := 0; i < 200; i++ {
		a.mu.Lock()
		done := a.paste != nil
		a.mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var ctrlV = key.Event{Name: "V", Modifiers: key.ModShortcut, State: key.Press}

func vFilter(a *App) key.Filter {
	return key.Filter{Focus: &a.url, Name: "V", Required: key.ModShortcut}
}

// Ctrl+V on the focused URL field is taken by pasteKey before the editor sees
// it, and pastes at the caret, replacing the selection.
func TestCtrlVUsesExternalClipboard(t *testing.T) {
	a, frame := pasteApp("  https://e/v\n")
	a.url.SetText("abcdef")
	a.url.SetCaret(1, 3) // selects "bc"
	var gtx layout.Context
	frame(func(g layout.Context) {
		gtx = g
		a.pasteKey(g)
		// The editor's own V filter finds nothing left: it never gets to issue clipboard.ReadCmd.
		if ev, ok := g.Event(vFilter(a)); ok {
			t.Errorf("Ctrl+V still queued for the editor: %v", ev)
		}
	}, ctrlV)
	waitPaste(a)
	a.applyPaste(gtx)
	if got := a.url.Text(); got != "ahttps://e/vdef" {
		t.Errorf("text = %q, want insert at caret over the selection", got)
	}
}

// Control: without pasteKey the Ctrl+V is queued for the editor's filter, so
// the test above would notice if pasteKey stopped consuming it.
func TestCtrlVReachesEditorWithoutPasteKey(t *testing.T) {
	a, frame := pasteApp("x")
	got := false
	frame(func(g layout.Context) { _, got = g.Event(vFilter(a)) }, ctrlV)
	if !got {
		t.Fatal("Ctrl+V never reached the editor's filter; the routing test proves nothing")
	}
}

// The Paste button replaces the whole field and puts the caret at the end.
func TestPasteButtonReplaces(t *testing.T) {
	a, frame := pasteApp("https://e/new\n")
	a.url.SetText("old text")
	gtx := frame(nil)
	a.startPaste(false)
	waitPaste(a)
	a.applyPaste(gtx)
	if got := a.url.Text(); got != "https://e/new" {
		t.Errorf("text = %q", got)
	}
	if s, e := a.url.Selection(); s != 13 || e != 13 {
		t.Errorf("caret = %d,%d, want at the end", s, e)
	}
}

func TestPasteErrorShownOnStatus(t *testing.T) {
	a, frame := pasteApp("")
	a.clipRun = fakeClip("", exec.ErrNotFound, nil)
	a.url.SetText("keep")
	gtx := frame(nil)
	a.startPaste(false)
	waitPaste(a)
	a.applyPaste(gtx)
	a.mu.Lock()
	defer a.mu.Unlock()
	if !strings.Contains(a.probeErr, "not found") || a.url.Text() != "keep" {
		t.Errorf("probeErr = %q, text = %q", a.probeErr, a.url.Text())
	}
}
