package app

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

// rowState is the per-job widget state, kept in App.rows keyed by job ID.
type rowState struct {
	btn     widget.Clickable // cancel (queued/running) or remove (finished)
	lastPct float32          // last known percent, shown while yt-dlp reports -1
}

func active(s queue.State) bool { return s == queue.Queued || s == queue.Running }

// layout draws one queue row and handles its button. hint is the (already
// resolved) hint text for j.Err, "" when there is none.
func (r *rowState) layout(gtx theme.C, t *theme.Theme, q *queue.Queue, j queue.Job, hint string) theme.D {
	if r.btn.Clicked(gtx) {
		if active(j.State) {
			q.Cancel(j.ID)
		} else {
			q.Remove(j.ID)
		}
	}
	if p := j.Progress.Percent; p >= 0 {
		r.lastPct = float32(p)
	}
	pb := t.ProgressBar(r.lastPct)
	switch j.State {
	case queue.Queued:
		pb.Progress = 0
	case queue.Done:
		pb.Progress, pb.Color = 1, t.P.Success
	case queue.Failed:
		pb.Color = t.P.Error
	}
	ic, desc := theme.IcDelete, "Remove"
	if active(j.State) {
		ic, desc = theme.IcClose, "Cancel"
	}

	return t.Card(gtx, func(gtx theme.C) theme.D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx theme.C) theme.D {
						l := material.Body1(t.Theme, j.Title)
						l.MaxLines = 1
						l.Font.Weight = font.Medium
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx theme.C) theme.D {
						// negative inset cancels the button's own padding so the icon lines up with the card edge
						return layout.Inset{Top: -6, Bottom: -6, Right: -8}.Layout(gtx, t.IconButton(&r.btn, ic, desc).Layout)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(pb.Layout),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx theme.C) theme.D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, t.Muted(stateText(j)).Layout),
					layout.Rigid(t.Muted(rateText(j)).Layout),
				)
			}),
			layout.Rigid(func(gtx theme.C) theme.D {
				if j.State != queue.Failed || j.Err == nil {
					return theme.D{}
				}
				return layout.Inset{Top: 6}.Layout(gtx, func(gtx theme.C) theme.D {
					e := t.Muted(j.Err.Error())
					e.Color = t.P.Error
					lines := []layout.FlexChild{layout.Rigid(e.Layout)}
					if hint != "" {
						lines = append(lines, layout.Rigid(t.Muted(hint).Layout))
					}
					// phase B (row.go): expandable stderr tail and Retry button go here.
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
				})
			}),
		)
	})
}

// stateText is the left-hand status line: "Downloading audio · 42%".
func stateText(j queue.Job) string {
	if j.State != queue.Running {
		return j.State.String()
	}
	var s string
	switch j.Progress.Stage {
	case ytdlp.StageVideo:
		s = "Downloading video"
	case ytdlp.StageAudio:
		s = "Downloading audio"
	default:
		s = j.Progress.Stage.String() // Starting, Downloading, Merging, Processing
	}
	if p := j.Progress.Percent; p >= 0 && j.Progress.Stage >= ytdlp.StageDownloading && j.Progress.Stage <= ytdlp.StageAudio {
		s += fmt.Sprintf(" · %.0f%%", p*100)
	}
	return s
}

// rateText is "3.2 MB/s · 1m23s" for a running job, whatever part is known.
func rateText(j queue.Job) string {
	if j.State != queue.Running {
		return ""
	}
	var parts []string
	if v := j.Progress.Speed; v > 0 {
		if v >= 1e6 {
			parts = append(parts, fmt.Sprintf("%.1f MB/s", v/1e6))
		} else {
			parts = append(parts, fmt.Sprintf("%.0f KB/s", v/1e3))
		}
	}
	if d := j.Progress.ETA; d > 0 {
		parts = append(parts, d.Round(time.Second).String())
	}
	return strings.Join(parts, " · ")
}
