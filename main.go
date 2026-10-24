// yt-dlp-gui: a small desktop front end for yt-dlp.
package main

import (
	"fmt"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"

	ui "yt-dlp-gui/internal/ui/app"
)

const usage = `yt-dlp-gui - desktop front end for yt-dlp

usage:
  yt-dlp-gui                    open the GUI`

func main() {
	// Subcommands are handled before any Gio call, so they never open a window.
	if len(os.Args) > 1 {
		switch cmd := os.Args[1]; {
		case cmd == "-h" || cmd == "--help" || cmd == "help":
			fmt.Println(usage)
			return
		default:
			fmt.Println(usage)
			os.Exit(2)
		}
	}

	app.ID = "yt-dlp-gui" // Wayland app_id, so sway can match the window
	go func() {
		w := new(app.Window)
		w.Option(app.Title("yt-dlp-gui"), app.Size(unit.Dp(900), unit.Dp(650)), app.MinSize(unit.Dp(560), unit.Dp(400)))
		if err := ui.New(w).Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
