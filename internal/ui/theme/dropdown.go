package theme

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
)

// Dropdown is a select box: a button showing the selected option that opens a
// menu (ContextArea + Menu) below it. Keep one per field, lay it out every frame.
type Dropdown struct {
	Options []string
	Width   unit.Dp // button width; defaults to 200

	sel     int
	changed bool
	btn     widget.Clickable
	items   []widget.Clickable
	area    component.ContextArea
	menu    component.MenuState
	size    image.Point // button size from the last frame, used to place/size the menu
}

func NewDropdown(opts []string) *Dropdown {
	return &Dropdown{Options: opts, Width: 200, items: make([]widget.Clickable, len(opts))}
}

// Selected is the index of the chosen option.
func (d *Dropdown) Selected() int { return d.sel }

// SetSelected sets the choice without reporting Changed.
func (d *Dropdown) SetSelected(i int) { d.sel = i }

// Changed reports (once) that the user picked an option since the last call.
func (d *Dropdown) Changed() bool {
	c := d.changed
	d.changed = false
	return c
}

func (d *Dropdown) Layout(gtx C, t *Theme) D {
	for i := range d.items {
		if d.items[i].Clicked(gtx) {
			d.changed, d.sel = d.sel != i, i
			d.area.Dismiss()
		}
	}
	d.btn.Clicked(gtx) // drained; opening is done by the ContextArea
	d.area.Activation = pointer.ButtonPrimary
	d.area.AbsolutePosition = true
	// ponytail: mouse only; keyboard focus/Enter does not open the menu (ContextArea has no Activate()).

	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(d.Width))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			dims := d.button(gtx, t)
			d.size = dims.Size
			return dims
		}),
		layout.Expanded(func(gtx C) D {
			return d.area.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min = image.Point{}
				return d.menuBox(gtx, t)
			})
		}),
	)
}

func (d *Dropdown) button(gtx C, t *Theme) D {
	return material.ButtonLayoutStyle{Background: t.P.Surface, CornerRadius: 8, Button: &d.btn}.Layout(gtx, func(gtx C) D {
		dims := layout.Inset{Top: 9, Bottom: 9, Left: 14, Right: 6}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					l := material.Body2(t.Theme, d.Options[d.sel])
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D { return Icon(gtx, IcDropDown, 20, t.P.Muted) }),
			)
		})
		border(gtx, dims.Size, 8, t.P.Divider)
		return dims
	})
}

func (d *Dropdown) menuBox(gtx C, t *Theme) D {
	d.menu.Options = d.menu.Options[:0]
	for i, o := range d.Options {
		it := component.MenuItem(t.Theme, &d.items[i], o)
		it.Label.TextSize = 14
		it.HoverColor = t.P.Track
		if i == d.sel {
			it.Label.Color = t.P.Accent
			it.Label.Font.Weight = font.SemiBold
		}
		d.menu.Options = append(d.menu.Options, func(gtx C) D {
			gtx.Constraints.Min.X = d.size.X // menu at least as wide as the button
			return it.Layout(gtx)
		})
	}
	m := component.Menu(t.Theme, &d.menu)
	m.Fill = t.P.Surface
	m.ShadowStyle = component.Shadow(8, 6)
	return layout.Inset{Top: unit.Dp(float32(d.size.Y+gtx.Dp(4)) / gtx.Metric.PxPerDp)}.Layout(gtx, func(gtx C) D {
		dims := m.Layout(gtx)
		border(gtx, dims.Size, 8, t.P.Divider)
		return dims
	})
}
