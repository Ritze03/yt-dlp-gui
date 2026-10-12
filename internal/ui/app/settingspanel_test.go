package app

import (
	"image"
	"log"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"yt-dlp-gui/internal/settings"
	"yt-dlp-gui/internal/ui/theme"
)

func TestSettingsFromFields(t *testing.T) {
	dir := t.TempDir()
	cur := settings.Settings{Dark: true, Folder: "/old", Parallel: 1, Cookies: "none", SubLang: "en"}

	got, err := settingsFromFields(cur, "  "+dir+" ", 2, 3, true, " de,fr ")
	if err != nil {
		t.Fatal(err)
	}
	want := settings.Settings{Dark: true, Folder: dir, Parallel: 3, Cookies: "brave", Subs: true, SubLang: "de,fr"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if got, _ := settingsFromFields(settings.Settings{Dark: false}, dir, 0, 0, false, ""); got.Dark || got.SubLang != "en" || got.Cookies != "none" || got.Parallel != 1 {
		t.Errorf("empty lang / Dark carry: %+v", got)
	}
	if got, _ := settingsFromFields(cur, dir, 9, 9, false, ""); got.Parallel != 4 || got.Cookies != "none" {
		t.Errorf("out-of-range indices: %+v", got)
	}

	if _, err := settingsFromFields(cur, filepath.Join(dir, "nope"), 0, 0, false, "en"); err == nil {
		t.Error("missing folder accepted")
	}
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := settingsFromFields(cur, f, 0, 0, false, "en"); err == nil {
		t.Error("file accepted as folder")
	}
	if _, err := settingsFromFields(cur, "", 0, 0, false, "en"); err == nil {
		t.Error("empty folder accepted")
	}
	if _, err := settingsFromFields(cur, dir, 0, 0, true, "en de"); err == nil {
		t.Error("lang with space accepted")
	}
}

func TestSettingsFromFieldsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "dl"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := settingsFromFields(settings.Default(), "~/dl", 0, 0, false, "en")
	if err != nil || got.Folder != filepath.Join(home, "dl") {
		t.Errorf("~/dl: %q, %v", got.Folder, err)
	}
	if got, err := settingsFromFields(settings.Default(), "~", 0, 0, false, "en"); err != nil || got.Folder != home {
		t.Errorf("~: %q, %v", got.Folder, err)
	}
}

func TestSettingsPanelOpen(t *testing.T) {
	p := newSettingsPanel()
	p.err = "stale"
	p.open(settings.Settings{Folder: "/some/dir", Parallel: 4, Cookies: "firefox", Subs: true, SubLang: "de"})
	if p.folder.Text() != "/some/dir" || p.lang.Text() != "de" || p.parallel.Selected() != 3 || p.ckies.Selected() != 1 || !p.subs.Value || p.err != "" {
		t.Errorf("open: folder=%q lang=%q par=%d cookies=%d subs=%v err=%q",
			p.folder.Text(), p.lang.Text(), p.parallel.Selected(), p.ckies.Selected(), p.subs.Value, p.err)
	}
	p.open(settings.Settings{Parallel: 0, Cookies: "bogus"}) // unnormalised input must not panic
	if p.parallel.Selected() != 0 || p.ckies.Selected() != 0 || p.subs.Value {
		t.Errorf("open defaults: par=%d cookies=%d subs=%v", p.parallel.Selected(), p.ckies.Selected(), p.subs.Value)
	}
}

// TestSettingsPanelLayout lays the panel out without a window: it must draw
// and report no events when nothing was clicked.
func TestSettingsPanelLayout(t *testing.T) {
	th := theme.New(true)
	cur := settings.Default()
	p := newSettingsPanel()
	p.open(cur)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(700, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	d, _, saved, closed := p.layout(gtx, th, cur)
	if d.Size.Y == 0 || saved || closed {
		t.Errorf("idle layout: size=%v saved=%v closed=%v", d.Size, saved, closed)
	}
}

// TestSettingsPanelDemo opens the panel in a window for screenshots:
//
//	SP_DEMO=1 go test -run TestSettingsPanelDemo ./internal/ui/app   (SP_DEMO=light for the light theme)
func TestSettingsPanelDemo(t *testing.T) {
	mode := os.Getenv("SP_DEMO")
	if mode == "" {
		t.Skip("set SP_DEMO=1 (or light) to open the demo window")
	}
	app.ID = "settings-demo"
	go func() {
		w := new(app.Window)
		w.Option(app.Title("settings panel demo"), app.Size(unit.Dp(720), unit.Dp(520)))
		th := theme.New(mode != "light")
		cur := settings.Default()
		p := newSettingsPanel()
		p.open(cur)
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				if e.Err != nil {
					log.Fatal(e.Err)
				}
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				th.Background(gtx)
				var saved, closed bool
				layout.UniformInset(20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					d, next, s, c := p.layout(gtx, th, cur)
					if s {
						cur = next
					}
					saved, closed = s, c
					return d
				})
				if saved || closed {
					log.Printf("saved=%v closed=%v cur=%+v", saved, closed, cur)
					p.open(cur)
				}
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}
