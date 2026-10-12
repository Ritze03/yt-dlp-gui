package theme

import (
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

// Icons are immutable, so one shared instance each is fine. Draw them with Icon().
var (
	IcDownload = mustIcon(icons.FileFileDownload)
	IcAdd      = mustIcon(icons.ContentAdd)
	IcClose    = mustIcon(icons.NavigationClose)
	IcSettings = mustIcon(icons.ActionSettings)
	IcSun      = mustIcon(icons.ImageWBSunny)     // shown in dark mode (click -> light)
	IcMoon     = mustIcon(icons.ImageBrightness2) // shown in light mode (click -> dark)
	IcFolder   = mustIcon(icons.FileFolderOpen)
	IcOpen     = mustIcon(icons.ActionOpenInNew)
	IcRetry    = mustIcon(icons.NavigationRefresh)
	IcDelete   = mustIcon(icons.ActionDelete)
	IcExpand   = mustIcon(icons.NavigationExpandMore)
	IcCollapse = mustIcon(icons.NavigationExpandLess)
	IcDropDown = mustIcon(icons.NavigationArrowDropDown)
	IcError    = mustIcon(icons.AlertError)
	IcWarning  = mustIcon(icons.AlertWarning)
	IcCheck    = mustIcon(icons.NavigationCheck)
)
