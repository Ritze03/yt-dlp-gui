package theme

import (
	"log"
	"os"
	"testing"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TestDemo opens a showcase window for screenshots:
//
//	THEME_DEMO=1 go test -run TestDemo ./internal/ui/theme   (THEME_DEMO=light starts in light mode)
//
// The header icon toggles the theme.
func TestDemo(t *testing.T) {
	mode := os.Getenv("THEME_DEMO")
	if mode == "" {
		t.Skip("set THEME_DEMO=1 (or light) to open the demo window")
	}
	app.ID = "theme-demo"
	go func() {
		w := new(app.Window)
		w.Option(app.Title("theme demo"), app.Size(unit.Dp(720), unit.Dp(560)))
		if err := demoLoop(w, mode != "light"); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func demoLoop(w *app.Window, dark bool) error {
	th := New(dark)
	dd := NewDropdown([]string{"Best video+audio", "1080p", "720p", "Audio only (m4a)", "Audio only (mp3)"})
	var toggle, add, plain, cancel, retry widget.Clickable
	var url widget.Editor
	url.SingleLine = true
	var check widget.Bool
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if toggle.Clicked(gtx) {
				th.SetDark(!th.Dark)
			}
			dd.Changed()
			th.Background(gtx)
			layout.UniformInset(20).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D { return Icon(gtx, IcDownload, 28, th.P.Accent) }),
							layout.Rigid(layout.Spacer{Width: 10}.Layout),
							layout.Flexed(1, th.Title("yt-dlp GUI").Layout),
							layout.Rigid(func(gtx C) D {
								ic := IcSun
								if !th.Dark {
									ic = IcMoon
								}
								return th.IconButton(&toggle, ic, "Toggle theme").Layout(gtx)
							}),
							layout.Rigid(func(gtx C) D { return th.IconButton(&cancel, IcSettings, "Settings").Layout(gtx) }),
						)
					}),
					layout.Rigid(layout.Spacer{Height: 16}.Layout),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D { return th.Input(gtx, &url, "Paste a video or playlist URL") }),
							layout.Rigid(layout.Spacer{Width: 8}.Layout),
							layout.Rigid(func(gtx C) D { return th.AccentButton(&add, "Add").Layout(gtx) }),
						)
					}),
					layout.Rigid(layout.Spacer{Height: 12}.Layout),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(th.Muted("Format").Layout),
							layout.Rigid(layout.Spacer{Width: 10}.Layout),
							layout.Rigid(func(gtx C) D { return dd.Layout(gtx, th) }),
							layout.Rigid(layout.Spacer{Width: 10}.Layout),
							layout.Rigid(func(gtx C) D { return th.Button(&plain, "Advanced").Layout(gtx) }),
							layout.Rigid(layout.Spacer{Width: 10}.Layout),
							layout.Rigid(func(gtx C) D { return material.CheckBox(th.Theme, &check, "Playlist").Layout(gtx) }),
						)
					}),
					layout.Rigid(layout.Spacer{Height: 16}.Layout),
					layout.Rigid(func(gtx C) D {
						return demoRow(gtx, th, 0.6, "Some Video Title That Is Fairly Long.mp4", "Downloading", &cancel, &retry)
					}),
					layout.Rigid(layout.Spacer{Height: 10}.Layout),
					layout.Rigid(func(gtx C) D { return demoRow(gtx, th, 1, "Finished item", "Done", &cancel, &retry) }),
					layout.Rigid(layout.Spacer{Height: 10}.Layout),
					layout.Rigid(func(gtx C) D {
						return demoRow(gtx, th, 0.2, "Broken item", "Error: HTTP 403 Forbidden", &cancel, &retry)
					}),
					layout.Flexed(1, func(gtx C) D { return D{Size: gtx.Constraints.Max} }),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, th.Muted("yt-dlp 2026.10.01").Layout),
							layout.Rigid(func(gtx C) D {
								l := th.Muted("Update available: 2026.10.09")
								l.Color = th.P.Warning
								return l.Layout(gtx)
							}),
						)
					}),
				)
			})
			e.Frame(gtx.Ops)
		}
	}
}

func demoRow(gtx C, th *Theme, p float32, title, stage string, cancel, retry *widget.Clickable) D {
	return th.Card(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(material.Body1(th.Theme, title).Layout),
					layout.Rigid(layout.Spacer{Height: 8}.Layout),
					layout.Rigid(func(gtx C) D {
						pb := th.ProgressBar(p)
						if p == 1 {
							pb.Color = th.P.Success
						}
						return pb.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: 6}.Layout),
					layout.Rigid(func(gtx C) D {
						l := th.Muted(stage + "  -  4.2 MiB/s  -  ETA 00:41")
						if p == 0.2 {
							l = th.Muted(stage)
							l.Color = th.P.Error
						}
						return l.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(func(gtx C) D { return th.IconButton(retry, IcRetry, "Retry").Layout(gtx) }),
			layout.Rigid(func(gtx C) D { return th.IconButton(cancel, IcClose, "Cancel").Layout(gtx) }),
		)
	})
}
