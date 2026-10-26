package theme

import (
	"image/color"
	"math"
	"testing"
)

func lum(c color.NRGBA) float64 {
	f := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func contrast(a, b color.NRGBA) float64 {
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

func TestPaletteContrast(t *testing.T) {
	for name, p := range map[string]Palette{"dark": Dark, "light": Light} {
		for _, bg := range []color.NRGBA{p.Bg, p.Surface} {
			for cn, c := range map[string]color.NRGBA{"Fg": p.Fg, "Muted": p.Muted, "Error": p.Error, "Warning": p.Warning, "Success": p.Success, "Accent": p.Accent} {
				if r := contrast(c, bg); r < 4.5 {
					t.Errorf("%s %s on %v: %.2f < 4.5", name, cn, bg, r)
				}
			}
		}
		if r := contrast(p.OnAccent, p.Accent); r < 4.5 {
			t.Errorf("%s OnAccent/Accent %.2f < 4.5", name, r)
		}
	}
}

func TestNew(t *testing.T) {
	th := New(true)
	if th.Face != "Inter" || th.P != Dark {
		t.Fatalf("face %q, dark palette %v", th.Face, th.P == Dark)
	}
	th.SetDark(false)
	if th.P != Light || th.Palette.ContrastBg != Light.Accent {
		t.Fatal("SetDark(false) did not swap palette")
	}
}
