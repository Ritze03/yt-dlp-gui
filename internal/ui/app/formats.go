package app

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"

	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

// fpListMax caps the format list height; it scrolls beyond that.
const fpListMax unit.Dp = 300

// formatPicker is the "Advanced…" panel (D6/D11/D24): the real format list of
// one probed single video in a Discloser. The wiring (I11) calls layout every
// frame while the panel is open and queues the download when picked is true.
type formatPicker struct {
	title    string
	formats  []ytdlp.Format // fpSorted
	rows     []widget.Clickable
	sel      int // index into formats, -1 = none
	list     widget.List
	disc     component.DiscloserState
	ok, quit widget.Clickable
}

// newFormatPicker is for a single video (p.IsPlaylist false); starts expanded.
func newFormatPicker(p *ytdlp.Probe) *formatPicker {
	fs := fpSorted(p.Formats)
	fp := &formatPicker{title: p.Title, formats: fs, rows: make([]widget.Clickable, len(fs)), sel: -1}
	fp.list.Axis = layout.Vertical
	// a zero Duration would make DiscloserState.Layout reset the state to collapsed
	fp.disc.Duration, fp.disc.State = 100*time.Millisecond, component.Visible
	return fp
}

// fpSorted orders formats for display: video+audio and video-only by height
// desc then tbr desc, audio-only last by tbr desc. Input is not modified.
func fpSorted(fs []ytdlp.Format) []ytdlp.Format {
	out := append([]ytdlp.Format(nil), fs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.AudioOnly() != b.AudioOnly() {
			return b.AudioOnly()
		}
		if !a.AudioOnly() && a.Height != b.Height {
			return a.Height > b.Height
		}
		return a.TBR > b.TBR
	})
	return out
}

// fpLabel renders one list row: "137 · mp4 · 1080p · 24fps · avc1 · video only · 43.2 MB".
// Unknown parts (height, fps, codec, size) are omitted.
func fpLabel(f ytdlp.Format) string {
	parts := []string{f.ID, f.Ext}
	if f.AudioOnly() {
		parts = append(parts, "audio")
		if f.ACodec != "" {
			parts = append(parts, f.ACodec)
		}
	} else {
		if f.Height > 0 {
			parts = append(parts, fmt.Sprintf("%dp", f.Height))
		}
		if f.FPS > 0 {
			parts = append(parts, fmt.Sprintf("%.0ffps", f.FPS))
		}
		if v, _, _ := strings.Cut(f.VCodec, "."); v != "" && v != "none" {
			parts = append(parts, v)
		}
		if f.VideoOnly() {
			parts = append(parts, "video only")
		}
	}
	if f.Size > 0 {
		parts = append(parts, fpSize(f.Size))
	}
	return strings.Join(parts, " · ")
}

// fpSize is a decimal size: "812 KB", "43.2 MB", "1.2 GB".
func fpSize(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1e3)
}

// layout draws the panel. picked is true once when Download is clicked (f is
// the chosen format); cancel is true once when Cancel is clicked.
func (fp *formatPicker) layout(gtx theme.C, t *theme.Theme) (d theme.D, f ytdlp.Format, picked, cancel bool) {
	if fp.sel >= 0 && fp.ok.Clicked(gtx) {
		f, picked = fp.formats[fp.sel], true
	}
	cancel = fp.quit.Clicked(gtx)
	for i := range fp.rows {
		if fp.rows[i].Clicked(gtx) {
			fp.sel = i
		}
	}

	d = t.Card(gtx, func(gtx theme.C) theme.D {
		return component.Discloser(t.Theme, &fp.disc).Layout(gtx,
			func(gtx theme.C) theme.D { // control: the whole header row is the click target
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				ic := theme.IcExpand
				if fp.disc.Visible() {
					ic = theme.IcCollapse
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx theme.C) theme.D { return theme.Icon(gtx, ic, 20, t.P.Muted) }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Flexed(1, func(gtx theme.C) theme.D {
						l := material.Body1(t.Theme, fmt.Sprintf("Advanced — formats for %s (%d)", fp.title, len(fp.formats)))
						l.MaxLines = 1
						return l.Layout(gtx)
					}),
				)
			},
			func(gtx theme.C) theme.D { return theme.D{} },
			func(gtx theme.C) theme.D { return fp.detail(gtx, t) },
		)
	})
	return d, f, picked, cancel
}

func (fp *formatPicker) detail(gtx theme.C, t *theme.Theme) theme.D {
	return layout.Inset{Top: 12}.Layout(gtx, func(gtx theme.C) theme.D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx theme.C) theme.D {
				gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(fpListMax))
				return material.List(t.Theme, &fp.list).Layout(gtx, len(fp.formats), func(gtx theme.C, i int) theme.D {
					return fp.row(gtx, t, i)
				})
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(t.Muted("Video-only formats get the best audio merged in.").Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx theme.C) theme.D { return theme.D{Size: image.Pt(gtx.Constraints.Max.X, 0)} }),
					layout.Rigid(t.Button(&fp.quit, "Cancel").Layout),
					layout.Rigid(func(gtx theme.C) theme.D {
						if fp.sel < 0 {
							return theme.D{}
						}
						return layout.Inset{Left: 8}.Layout(gtx, t.AccentButton(&fp.ok, "Download").Layout)
					}),
				)
			}),
		)
	})
}

// row is one clickable line; the selected one gets an accent tint and a check.
func (fp *formatPicker) row(gtx theme.C, t *theme.Theme, i int) theme.D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	sel := i == fp.sel
	return fp.rows[i].Layout(gtx, func(gtx theme.C) theme.D {
		return layout.Background{}.Layout(gtx,
			func(gtx theme.C) theme.D {
				var bg color.NRGBA
				switch {
				case sel:
					bg = t.P.Accent
					bg.A = 0x33
				case fp.rows[i].Hovered():
					bg = t.P.Track
				}
				rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(6))
				paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
				return theme.D{Size: gtx.Constraints.Min}
			},
			func(gtx theme.C) theme.D {
				return layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx theme.C) theme.D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx theme.C) theme.D {
							l := material.Body2(t.Theme, fpLabel(fp.formats[i]))
							l.MaxLines = 1
							if sel {
								l.Color = t.P.Accent
							}
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx theme.C) theme.D {
							if !sel {
								return theme.D{}
							}
							return theme.Icon(gtx, theme.IcCheck, 18, t.P.Accent)
						}),
					)
				})
			},
		)
	})
}
