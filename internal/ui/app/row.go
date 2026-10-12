package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
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

// rowAction is one button in a row's title bar.
type rowAction int

const (
	rowCancel rowAction = iota
	rowOpen
	rowShow
	rowRetry
	rowRemove
	rowNActions
)

// rowTail is how many stderr lines the expanded "Details" block shows.
const rowTail = 12

// rowActions lists the buttons a row in state s offers, left to right.
func rowActions(s queue.State) []rowAction {
	switch s {
	case queue.Done:
		return []rowAction{rowOpen, rowShow, rowRemove}
	case queue.Failed, queue.Cancelled:
		return []rowAction{rowRetry, rowRemove}
	}
	return []rowAction{rowCancel}
}

// rowOpenCmd opens the file with the desktop's default app. The path is its
// own argv element, so no shell ever sees it.
func rowOpenCmd(path string) []string { return []string{"xdg-open", path} }

// rowShowCmd opens the file's folder. ponytail: xdg-open can't select the file
// (that needs a file-manager-specific D-Bus call), so it just opens the directory.
func rowShowCmd(path string) []string { return []string{"xdg-open", filepath.Dir(path)} }

// rowRun starts argv detached and reaps it. ponytail: a missing xdg-open is ignored.
func rowRun(argv []string) {
	if cmd := exec.Command(argv[0], argv[1:]...); cmd.Start() == nil {
		go cmd.Wait()
	}
}

// rowState is the per-job widget state, kept in App.rows keyed by job ID.
type rowState struct {
	btns     [rowNActions]widget.Clickable
	details  widget.Clickable // toggles the stderr tail of a failed row
	expanded bool
	lastPct  float32 // last known percent, shown while yt-dlp reports -1
}

func active(s queue.State) bool { return s == queue.Queued || s == queue.Running }

// layout draws one queue row and handles its buttons. hint is the (already
// resolved) hint text for j.Err, "" when there is none.
func (r *rowState) layout(gtx theme.C, t *theme.Theme, q *queue.Queue, j queue.Job, hint string) theme.D {
	acts := rowActions(j.State)
	for _, a := range acts {
		if !r.btns[a].Clicked(gtx) {
			continue
		}
		switch a {
		case rowCancel:
			q.Cancel(j.ID)
		case rowRetry:
			q.Retry(j.ID)
		case rowRemove:
			q.Remove(j.ID)
		case rowOpen:
			rowRun(rowOpenCmd(j.Path))
		case rowShow:
			rowRun(rowShowCmd(j.Path))
		}
	}
	if r.details.Clicked(gtx) {
		r.expanded = !r.expanded
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
						var btns []layout.FlexChild
						for _, a := range acts {
							if (a == rowOpen || a == rowShow) && j.Path == "" {
								continue
							}
							ic, desc := rowIcon(a)
							btns = append(btns, layout.Rigid(t.IconButton(&r.btns[a], ic, desc).Layout))
						}
						// negative inset cancels the button's own padding so the icons line up with the card edge
						return layout.Inset{Top: -6, Bottom: -6, Right: -8}.Layout(gtx, func(gtx theme.C) theme.D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx, btns...)
						})
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
					return r.failure(gtx, t, j.Err, hint)
				})
			}),
		)
	})
}

func rowIcon(a rowAction) (*widget.Icon, string) {
	switch a {
	case rowOpen:
		return theme.IcOpen, "Open file"
	case rowShow:
		return theme.IcFolder, "Show in folder"
	case rowRetry:
		return theme.IcRetry, "Retry"
	case rowRemove:
		return theme.IcDelete, "Remove"
	}
	return theme.IcClose, "Cancel"
}

// failure is the error line, hint and the expandable stderr tail of a failed row.
func (r *rowState) failure(gtx theme.C, t *theme.Theme, e *ytdlp.Error, hint string) theme.D {
	msg := t.Muted(e.Error())
	msg.Color = t.P.Error
	msg.MaxLines = 2
	kids := []layout.FlexChild{layout.Rigid(msg.Layout)}
	if hint != "" {
		kids = append(kids, layout.Rigid(t.Muted(hint).Layout))
	}
	if len(e.StderrTail) > 0 {
		ic := theme.IcExpand
		if r.expanded {
			ic = theme.IcCollapse
		}
		toggle := func(gtx theme.C) theme.D {
			return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx theme.C) theme.D {
				return material.Clickable(gtx, &r.details, func(gtx theme.C) theme.D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx theme.C) theme.D { return theme.Icon(gtx, ic, 18, t.P.Muted) }),
						layout.Rigid(layout.Spacer{Width: 4}.Layout),
						layout.Rigid(t.Muted("Details").Layout),
					)
				})
			}))
		}
		kids = append(kids, layout.Rigid(layout.Spacer{Height: 4}.Layout), layout.Rigid(toggle))
		if r.expanded {
			tail := e.StderrTail
			if len(tail) > rowTail {
				tail = tail[len(tail)-rowTail:]
			}
			// ponytail: no monospace face is embedded, so the tail is plain muted text.
			kids = append(kids, layout.Rigid(layout.Spacer{Height: 4}.Layout), layout.Rigid(t.Muted(strings.Join(tail, "\n")).Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
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
