// yt-dlp GUI — scaffold / headless render smoke test.
package main

import (
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func main() {
	app.ID = "yt-dlp-gui" // Wayland app_id, so sway can match the window
	go func() {
		w := new(app.Window)
		w.Option(app.Title("yt-dlp GUI"), app.Size(unit.Dp(1100), unit.Dp(750)))
		if err := loop(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func loop(w *app.Window) error {
	th := material.NewTheme()
	th.Palette.Bg = color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}
	th.Palette.Fg = color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	icon, err := widget.NewIcon(icons.FileFileDownload)
	if err != nil {
		return err
	}
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff},
				clip.Rect{Max: gtx.Constraints.Max}.Op())
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return component.Surface(th).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Max.X = gtx.Dp(32)
								return icon.Layout(gtx, th.Palette.Fg)
							}),
							layout.Rigid(layout.Spacer{Width: 12}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.H4(th, "yt-dlp GUI — hello")
								l.Color = th.Palette.Fg
								return l.Layout(gtx)
							}),
						)
					})
				})
			})
			e.Frame(gtx.Ops)
		}
	}
}
