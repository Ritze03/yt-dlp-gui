package app

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

// playlistPicker is the checkbox list shown after a pasted URL probes as a
// playlist (D20). The caller queues checked() with the preset from the main
// dropdown (D24) when layout reports queue.
type playlistPicker struct {
	title   string
	entries []ytdlp.Entry
	on      []widget.Bool // parallel to entries
	list    widget.List
	all     widget.Clickable
	none    widget.Clickable
	ok      widget.Clickable
	cancel  widget.Clickable
}

// newPlaylistPicker starts with every entry checked.
func newPlaylistPicker(p *ytdlp.Probe) *playlistPicker {
	pp := &playlistPicker{title: p.Title, entries: p.Entries, on: make([]widget.Bool, len(p.Entries))}
	pp.list.Axis = layout.Vertical
	pp.setAll(true)
	return pp
}

func (pp *playlistPicker) setAll(on bool) {
	for i := range pp.on {
		pp.on[i].Value = on
	}
}

// checked returns the ticked entries in playlist order.
func (pp *playlistPicker) checked() []ytdlp.Entry {
	var out []ytdlp.Entry
	for i, e := range pp.entries {
		if pp.on[i].Value {
			out = append(out, e)
		}
	}
	return out
}

// playlistDur formats seconds as m:ss (h:mm:ss from an hour up); "" if unknown.
func playlistDur(sec float64) string {
	s := int(sec)
	switch {
	case s <= 0:
		return ""
	case s >= 3600:
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// layout draws the picker card. queue / cancel are true for the one frame in
// which Queue N / Cancel was clicked; Queue is ignored while nothing is checked.
// The card is capped at 60% of the available height; the list scrolls inside.
func (pp *playlistPicker) layout(gtx theme.C, t *theme.Theme) (d theme.D, queue, cancel bool) {
	for i := range pp.on { // fold in this frame's clicks so the count and buttons are current
		pp.on[i].Update(gtx)
	}
	if pp.all.Clicked(gtx) {
		pp.setAll(true)
	}
	if pp.none.Clicked(gtx) {
		pp.setAll(false)
	}
	n := len(pp.checked())
	queue = pp.ok.Clicked(gtx) && n > 0
	cancel = pp.cancel.Clicked(gtx)

	gtx.Constraints.Max.Y = gtx.Constraints.Max.Y * 6 / 10
	d = t.Card(gtx, func(gtx theme.C) theme.D {
		// Header and footer are measured first so the list gets exactly the height
		// left over (and shrinks to fit a short list); replaying them keeps their clicks.
		// ponytail: record/replay instead of a Flexed list, which would always fill the cap.
		rec := op.Record(gtx.Ops)
		hd := pp.header(gtx, t, n)
		header := rec.Stop()
		rec = op.Record(gtx.Ops)
		fd := pp.footer(gtx, t, n)
		footer := rec.Stop()
		const gap = 12
		listMax := gtx.Constraints.Max.Y - hd.Size.Y - fd.Size.Y - gtx.Dp(gap)

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx theme.C) theme.D {
				header.Add(gtx.Ops)
				return hd
			}),
			layout.Rigid(func(gtx theme.C) theme.D {
				gtx.Constraints.Max.Y = max(listMax, 0)
				return material.List(t.Theme, &pp.list).Layout(gtx, len(pp.entries), func(gtx theme.C, i int) theme.D {
					return pp.row(gtx, t, i)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(gap)}.Layout),
			layout.Rigid(func(gtx theme.C) theme.D {
				footer.Add(gtx.Ops)
				return fd
			}),
		)
	})
	return d, queue, cancel
}

// row is one entry: checkbox labelled "<index>. <title>" and its duration.
// ponytail: a long title wraps rather than truncating (CheckBox has no MaxLines).
func (pp *playlistPicker) row(gtx theme.C, t *theme.Theme, i int) theme.D {
	e := pp.entries[i]
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, material.CheckBox(t.Theme, &pp.on[i], fmt.Sprintf("%d. %s", e.Index, e.Title)).Layout),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(t.Muted(playlistDur(e.Duration)).Layout),
	)
}

// header is the title, the "N of M selected" row with Select all / none, and the gap above the list.
func (pp *playlistPicker) header(gtx theme.C, t *theme.Theme, n int) theme.D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(t.Title(pp.title).Layout),
		layout.Rigid(layout.Spacer{Height: 4}.Layout),
		layout.Rigid(func(gtx theme.C) theme.D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, t.Muted(fmt.Sprintf("%d of %d selected", n, len(pp.entries))).Layout),
				layout.Rigid(t.Button(&pp.all, "Select all").Layout),
				layout.Rigid(layout.Spacer{Width: 8}.Layout),
				layout.Rigid(t.Button(&pp.none, "Select none").Layout),
			)
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
	)
}

func (pp *playlistPicker) footer(gtx theme.C, t *theme.Theme, n int) theme.D {
	queue := t.AccentButton(&pp.ok, fmt.Sprintf("Queue %d", n))
	if n == 0 {
		queue = t.Button(&pp.ok, "Queue 0") // quiet look = disabled; the click is ignored
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, t.Muted("All selected videos use the chosen format preset.").Layout),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(t.Button(&pp.cancel, "Cancel").Layout),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(queue.Layout),
	)
}
