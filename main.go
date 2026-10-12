// yt-dlp GUI: a small desktop front end for yt-dlp.
package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"

	ui "yt-dlp-gui/internal/ui/app"
)

func main() {
	app.ID = "yt-dlp-gui" // Wayland app_id, so sway can match the window
	go func() {
		w := new(app.Window)
		w.Option(app.Title("yt-dlp GUI"), app.Size(unit.Dp(900), unit.Dp(650)), app.MinSize(unit.Dp(560), unit.Dp(400)))
		if err := ui.New(w).Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
