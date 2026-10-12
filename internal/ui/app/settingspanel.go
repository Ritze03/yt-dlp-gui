package app

import (
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"yt-dlp-gui/internal/settings"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

// settingsPanel is the settings card (D7, D17, D18, D27). Open it with open(),
// then call layout every frame while it is visible.
type settingsPanel struct {
	folder, lang             widget.Editor
	themeDD, parallel, ckies *theme.Dropdown
	subs                     widget.Bool
	save, cancel             widget.Clickable
	closeBtn                 widget.Clickable
	err                      string        // inline folder/language error, "" when none
	scrim, card              gesture.Click // modal: a scrim click closes, a card click is swallowed
}

func newSettingsPanel() *settingsPanel {
	p := &settingsPanel{}
	p.folder.SingleLine, p.folder.Submit = true, true
	p.lang.SingleLine, p.lang.Submit = true, true
	p.themeDD = theme.NewDropdown([]string{"Dark", "Light"})
	p.themeDD.Width = 120
	p.parallel = theme.NewDropdown([]string{"1", "2", "3", "4"})
	p.parallel.Width = 90
	labels := make([]string, len(ytdlp.Browsers))
	for i, b := range ytdlp.Browsers {
		labels[i] = strings.ToUpper(b[:1]) + b[1:]
	}
	p.ckies = theme.NewDropdown(labels)
	p.ckies.Width = 160
	return p
}

// open loads s into the widgets and clears any error.
func (p *settingsPanel) open(s settings.Settings) {
	p.folder.SetText(s.Folder)
	p.lang.SetText(s.SubLang)
	p.themeDD.SetSelected(map[bool]int{true: 0, false: 1}[s.Dark])
	p.parallel.SetSelected(min(max(s.Parallel, 1), 4) - 1)
	p.ckies.SetSelected(max(slices.Index(ytdlp.Browsers, s.Cookies), 0))
	p.subs.Value = s.Subs
	p.err = ""
}

// settingsFromFields validates the raw field values and builds the new
// settings. themeIdx 0 is Dark, 1 is Light; any other value keeps
// cur.Dark. The folder is trimmed, a leading
// "~" expanded to the home directory, and checked with settings.ValidFolder;
// an empty language means "en", whitespace inside it is rejected.
func settingsFromFields(cur settings.Settings, folder string, themeIdx, parallelIdx, cookiesIdx int, subs bool, lang string) (settings.Settings, error) {
	folder = strings.TrimSpace(folder)
	if folder == "~" || strings.HasPrefix(folder, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return cur, err
		}
		folder = filepath.Join(home, folder[1:])
	}
	if err := settings.ValidFolder(folder); err != nil {
		return cur, err
	}
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "en"
	}
	if strings.ContainsAny(lang, " \t\n") {
		return cur, errors.New("subtitle language must not contain spaces (use commas: en,de)")
	}
	if cookiesIdx < 0 || cookiesIdx >= len(ytdlp.Browsers) {
		cookiesIdx = 0
	}
	if themeIdx == 0 || themeIdx == 1 {
		cur.Dark = themeIdx == 0
	}
	cur.Folder = folder
	cur.Parallel = min(max(parallelIdx+1, 1), 4)
	cur.Cookies = ytdlp.Browsers[cookiesIdx]
	cur.Subs = subs
	cur.SubLang = lang
	return cur, nil
}

// layout draws the panel as a card and handles its buttons. saved is true once
// per valid Save click (next holds the new settings); closed is true when the
// panel should be hidden (close icon, Cancel, or a successful Save).
func (p *settingsPanel) layout(gtx theme.C, t *theme.Theme, cur settings.Settings) (d theme.D, next settings.Settings, saved, closed bool) {
	// Drain editor events before Layout discards them; Enter saves.
	submit := p.save.Clicked(gtx)
	for _, ed := range []*widget.Editor{&p.folder, &p.lang} {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				submit = true
			}
		}
	}
	if submit {
		n, err := settingsFromFields(cur, p.folder.Text(), p.themeDD.Selected(), p.parallel.Selected(), p.ckies.Selected(), p.subs.Value, p.lang.Text())
		if err != nil {
			p.err = err.Error()
		} else {
			p.err, next, saved, closed = "", n, true, true
		}
	}
	if p.cancel.Clicked(gtx) || p.closeBtn.Clicked(gtx) {
		closed = true
	}

	// label stacks a muted caption over a field.
	label := func(txt string, w layout.Widget) layout.Widget {
		return func(gtx theme.C) theme.D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(t.Muted(txt).Layout),
				layout.Rigid(layout.Spacer{Height: 6}.Layout),
				layout.Rigid(w),
			)
		}
	}
	gap := layout.Rigid(layout.Spacer{Height: 18}.Layout)
	d = t.Card(gtx, func(gtx theme.C) theme.D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, t.Title("Settings").Layout),
					layout.Rigid(func(gtx theme.C) theme.D {
						// negative inset cancels the button's own padding (as in row.go)
						return layout.Inset{Top: -6, Bottom: -6, Right: -8}.Layout(gtx, t.IconButton(&p.closeBtn, theme.IcClose, "Close").Layout)
					}),
				)
			}),
			gap,
			layout.Rigid(label("Download folder", func(gtx theme.C) theme.D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx theme.C) theme.D { return t.Input(gtx, &p.folder, "~/Downloads") }),
					layout.Rigid(func(gtx theme.C) theme.D {
						if p.err == "" {
							return theme.D{}
						}
						l := t.Muted(p.err)
						l.Color = t.P.Error
						return layout.Inset{Top: 6}.Layout(gtx, l.Layout)
					}),
				)
			})),
			gap,
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(label("Theme", func(gtx theme.C) theme.D { return p.themeDD.Layout(gtx, t) })),
					layout.Rigid(layout.Spacer{Width: 24}.Layout),
					layout.Rigid(label("Parallel downloads", func(gtx theme.C) theme.D { return p.parallel.Layout(gtx, t) })),
					layout.Rigid(layout.Spacer{Width: 24}.Layout),
					layout.Rigid(label("Cookies from browser", func(gtx theme.C) theme.D { return p.ckies.Layout(gtx, t) })),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Rigid(t.Muted("Cookies are used for age-restricted or bot-checked videos.").Layout),
			gap,
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(material.CheckBox(t.Theme, &p.subs, "Download subtitles").Layout),
					layout.Flexed(1, func(gtx theme.C) theme.D {
						return layout.E.Layout(gtx, func(gtx theme.C) theme.D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(t.Muted("Language").Layout),
								layout.Rigid(layout.Spacer{Width: 8}.Layout),
								layout.Rigid(func(gtx theme.C) theme.D {
									gtx.Constraints.Max.X = gtx.Dp(120)
									return t.Input(gtx, &p.lang, "en")
								}),
							)
						})
					}),
				)
			}),
			gap,
			layout.Rigid(func(gtx theme.C) theme.D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X // Flex resets cross-axis Min; SpaceStart needs the full width
				return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
					layout.Rigid(t.Button(&p.cancel, "Cancel").Layout),
					layout.Rigid(layout.Spacer{Width: 10}.Layout),
					layout.Rigid(t.AccentButton(&p.save, "Save").Layout),
				)
			}),
		)
	})
	return d, next, saved, closed
}

// modal draws the dimming scrim over the whole window and the settings card
// centred on it. The scrim swallows all pointer input, so the page underneath
// is inert; a click on the scrim or Escape closes without saving.
// ponytail: the card is not scrollable; at the minimum window height (400dp)
// it just fits. A scroll list would clip the dropdown menus that overhang it.
func (p *settingsPanel) modal(gtx theme.C, t *theme.Theme, cur settings.Settings) (next settings.Settings, saved, closed bool) {
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			closed = true
		}
	}
	for {
		e, ok := p.scrim.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			closed = true
		}
	}
	p.card.Update(gtx.Source) // drained; the card only has to take the clicks away from the scrim

	size := gtx.Constraints.Max
	st := clip.Rect{Max: size}.Push(gtx.Ops)
	p.scrim.Add(gtx.Ops)
	a := uint8(140) // ~55%
	if !t.Dark {
		a = 100 // ~40%
	}
	paint.Fill(gtx.Ops, color.NRGBA{A: a})
	st.Pop()

	layout.Center.Layout(gtx, func(gtx theme.C) theme.D {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X-2*gtx.Dp(pad), gtx.Dp(560))
		m := op.Record(gtx.Ops)
		d, n, sv, cl := p.layout(gtx, t, cur)
		call := m.Stop()
		// The card's click area covers the card only; menus overhanging it stay on top of the scrim.
		cs := clip.Rect(image.Rectangle{Max: d.Size}).Push(gtx.Ops)
		p.card.Add(gtx.Ops)
		cs.Pop()
		call.Add(gtx.Ops)
		next, saved, closed = n, sv, closed || cl
		return d
	})
	return next, saved, closed
}
