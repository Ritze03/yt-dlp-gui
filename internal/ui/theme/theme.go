// Package theme holds the app's palettes, a configured material theme with an
// embedded Inter font, and small styled-widget helpers (card, buttons, input).
package theme

import (
	_ "embed"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

// Palette is every colour the UI uses. Text colours (Fg, Muted, Error, Warning,
// Success) are AA (>= 4.5:1) against Bg and Surface.
type Palette struct {
	Bg, Surface, Fg, Muted, Accent, OnAccent, Divider, Track, Error, Warning, Success color.NRGBA
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

var (
	// Dark uses the Graphite theme colours: Bg=PANEL, Surface=BTN, Fg=TEXT, Muted=DIM,
	// Accent=ACCENT, OnAccent=PRIMARY_TEXT, Divider=BORDER, Track=steel(40).
	Dark = Palette{
		Bg: rgb(0x1e2125), Surface: rgb(0x262a30), Fg: rgb(0xe7e9ec), Muted: rgb(0x969ca6),
		Accent: rgb(0x5b8bf0), OnAccent: rgb(0x0b1222), Divider: rgb(0x2c3036), Track: rgb(0x24282f),
		Error: rgb(0xe15554), Warning: rgb(0xd8b455), Success: rgb(0x6cc551),
	}
	Light = Palette{
		Bg: rgb(0xfafafa), Surface: rgb(0xffffff), Fg: rgb(0x1a1a1e), Muted: rgb(0x5b5b66),
		Accent: rgb(0x4f46e5), OnAccent: rgb(0xffffff), Divider: rgb(0xd8d8de), Track: rgb(0xe6e6ea),
		Error: rgb(0xb91c1c), Warning: rgb(0xa16207), Success: rgb(0x15803d),
	}
)

//go:embed fonts/Inter-Regular.ttf
var interRegular []byte

//go:embed fonts/Inter-SemiBold.ttf
var interSemiBold []byte

// Theme is a material.Theme (Inter font, palette-driven colours) plus the
// current Palette. Not safe to share between windows (owns a text shaper).
type Theme struct {
	*material.Theme
	P    Palette
	Dark bool
}

// New builds a fresh theme and text shaper. Use one per window.
func New(dark bool) *Theme {
	var faces []font.FontFace
	for _, src := range [][]byte{interRegular, interSemiBold} {
		f, err := opentype.ParseCollection(src) // also reads typeface name + weight from the file
		if err != nil {
			panic(err) // embedded data; cannot fail at runtime
		}
		faces = append(faces, f...)
	}
	m := material.NewTheme()
	m.Shaper = text.NewShaper(text.WithCollection(faces))
	m.Face = "Inter"
	t := &Theme{Theme: m}
	t.SetDark(dark)
	return t
}

// SetDark swaps the palette in place; takes effect on the next frame because
// styles are built per frame.
func (t *Theme) SetDark(dark bool) {
	t.Dark = dark
	t.P = Light
	if dark {
		t.P = Dark
	}
	t.Palette = material.Palette{Bg: t.P.Bg, Fg: t.P.Fg, ContrastBg: t.P.Accent, ContrastFg: t.P.OnAccent}
}

// Background fills the whole window with the page colour; call first each frame.
func (t *Theme) Background(gtx C) {
	paint.Fill(gtx.Ops, t.P.Bg)
}

// Card lays w out on a rounded, softly shadowed surface padded 16dp,
// spanning the available width.
func (t *Theme) Card(gtx C, w layout.Widget) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	s := component.Surface(t.Theme)
	s.Fill = t.P.Surface
	s.ShadowStyle = component.Shadow(12, 2)
	return s.Layout(gtx, func(gtx C) D {
		dims := layout.UniformInset(16).Layout(gtx, w)
		border(gtx, dims.Size, 12, t.P.Divider)
		return dims
	})
}

// border strokes a 1dp rounded outline just inside size.
func border(gtx C, size image.Point, radius unit.Dp, col color.NRGBA) {
	rr := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
}

// Button is a quiet secondary button (track-coloured, normal text).
func (t *Theme) Button(btn *widget.Clickable, txt string) material.ButtonStyle {
	b := material.Button(t.Theme, btn, txt)
	b.Background, b.Color = t.P.Track, t.P.Fg
	return t.tune(b)
}

// AccentButton is the primary call-to-action button.
func (t *Theme) AccentButton(btn *widget.Clickable, txt string) material.ButtonStyle {
	return t.tune(material.Button(t.Theme, btn, txt)) // defaults already are Accent/OnAccent
}

func (t *Theme) tune(b material.ButtonStyle) material.ButtonStyle {
	b.CornerRadius = 8
	b.TextSize = 14
	b.Font.Weight = font.SemiBold
	b.Inset = layout.Inset{Top: 10, Bottom: 10, Left: 16, Right: 16}
	return b
}

// IconButton is a flat, transparent 36dp icon button (hover tint comes from Gio).
func (t *Theme) IconButton(btn *widget.Clickable, ic *widget.Icon, desc string) material.IconButtonStyle {
	b := material.IconButton(t.Theme, btn, ic, desc)
	b.Background = color.NRGBA{}
	b.Color = t.P.Muted
	b.Size = 20
	b.Inset = layout.UniformInset(8)
	return b
}

// ProgressBar is a thin rounded accent bar on the Track colour. Override
// .Color (e.g. t.P.Success) for finished rows.
func (t *Theme) ProgressBar(p float32) material.ProgressBarStyle {
	b := material.ProgressBar(t.Theme, p)
	b.Color, b.TrackColor = t.P.Accent, t.P.Track
	b.Height, b.Radius = 6, 3
	return b
}

// Muted is small secondary text; set .Color to t.P.Warning etc. for variants.
func (t *Theme) Muted(txt string) material.LabelStyle {
	l := material.Body2(t.Theme, txt)
	l.Color = t.P.Muted
	return l
}

// Title is semi-bold heading text.
func (t *Theme) Title(txt string) material.LabelStyle {
	l := material.H6(t.Theme, txt)
	l.Font.Weight = font.SemiBold
	return l
}

// Input lays out a bordered, rounded single-field editor; the border turns
// accent-coloured while the editor has focus.
func (t *Theme) Input(gtx C, ed *widget.Editor, hint string) D {
	e := material.Editor(t.Theme, ed, hint)
	e.HintColor = t.P.Muted
	e.TextSize = 14
	width := gtx.Constraints.Max.X
	col := t.P.Divider
	if gtx.Focused(ed) {
		col = t.P.Accent
	}
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			gtx.Constraints.Min.X = width // Stack zeroes Min for its children
			return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, e.Layout)
		}),
		layout.Expanded(func(gtx C) D {
			border(gtx, gtx.Constraints.Min, 8, col)
			return D{Size: gtx.Constraints.Min}
		}),
	)
}

// Icon draws ic at size dp in col. widget.Icon sizes itself from
// gtx.Constraints.Min.X (not Max); a Max-only cap leaves it at the 24dp default.
func Icon(gtx C, ic *widget.Icon, size unit.Dp, col color.NRGBA) D {
	px := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	return ic.Layout(gtx, col)
}
