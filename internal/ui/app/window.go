// Package app is the main window: header, URL bar, download queue and footer,
// wired to internal/queue and internal/ytdlp.
package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/settings"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

const (
	outdatedHint = "yt-dlp may be outdated."
	blockedHint  = "YouTube blocked this request — retry, or set a cookies browser in Settings."
	pad          = 20 // dp, window edge padding
)

// App is the main window's state. Run it on the goroutine that owns the window.
type App struct {
	w       *app.Window
	th      *theme.Theme
	s       settings.Settings
	q       *queue.Queue
	presets []ytdlp.Preset

	url     widget.Editor
	preset  *theme.Dropdown
	addBtn  widget.Clickable
	themeBt widget.Clickable
	list    layout.List
	rows    map[int]*rowState // by job ID, pruned every frame
	focused bool              // URL field focused once at startup

	// mu guards what background goroutines (probes, version check) write.
	// Everything else is touched by the UI goroutine only.
	mu       sync.Mutex
	probing  int    // probes in flight (ponytail: several may run at once, one status line)
	probeErr string // last probe error + hint, "" when none
	clearURL string // URL of a probe that succeeded; the UI goroutine clears the field if it still matches
	version  string // yt-dlp --version
	verErr   bool   // yt-dlp could not be run
	latest   string // latest release tag, "" until known / on failure
}

// New builds the window state, loads settings and starts the version check.
func New(w *app.Window) *App {
	s, err := settings.Load()
	if err != nil {
		log.Printf("settings: %v", err) // defaults are in s; carry on
	}
	a := &App{
		w:    w,
		th:   theme.New(s.Dark),
		s:    s,
		q:    queue.New(nil, s.Parallel, w.Invalidate),
		list: layout.List{Axis: layout.Vertical},
		rows: map[int]*rowState{},
	}
	a.url.SingleLine, a.url.Submit = true, true
	a.presets = ytdlp.Presets()
	labels := make([]string, len(a.presets))
	for i, p := range a.presets {
		labels[i] = p.String()
	}
	a.preset = theme.NewDropdown(labels)
	go a.checkVersion()
	return a
}

// checkVersion fills the footer: local version, then the latest GitHub tag (D22).
func (a *App) checkVersion() {
	v, err := ytdlp.Version(context.Background())
	a.mu.Lock()
	a.version, a.verErr = v, err != nil
	a.mu.Unlock()
	a.w.Invalidate()
	if err != nil {
		return
	}
	if tag, ok := ytdlp.LatestTag(context.Background()); ok {
		a.mu.Lock()
		a.latest = tag
		a.mu.Unlock()
		a.w.Invalidate()
	}
}

// Run is the event loop. On window close it cancels every download and waits
// for the yt-dlp children to exit (D14) before returning.
func (a *App) Run() error {
	var ops op.Ops
	for {
		switch e := a.w.Event().(type) {
		case app.DestroyEvent:
			a.q.Close()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.frame(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) frame(gtx theme.C) {
	if !a.focused {
		a.focused = true
		gtx.Execute(key.FocusCmd{Tag: &a.url})
	}
	// Drain editor events before Layout does (it discards them).
	for {
		ev, ok := a.url.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.submit()
		}
	}
	if a.addBtn.Clicked(gtx) {
		a.submit()
	}
	if a.themeBt.Clicked(gtx) {
		a.th.SetDark(!a.th.Dark)
		a.s.Dark = a.th.Dark
		a.save()
	}
	a.mu.Lock()
	if a.clearURL != "" {
		if strings.TrimSpace(a.url.Text()) == a.clearURL {
			a.url.SetText("")
		}
		a.clearURL = ""
	}
	a.mu.Unlock()

	t := a.th
	t.Background(gtx)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.header),
		layout.Rigid(a.inputCard),
		layout.Rigid(a.status),
		layout.Flexed(1, a.queueList),
		layout.Rigid(a.footer),
	)
}

func (a *App) save() {
	if err := settings.Save(a.s); err != nil {
		log.Printf("settings: %v", err)
	}
}

func (a *App) header(gtx theme.C) theme.D {
	t := a.th
	ic, desc := theme.IcSun, "Switch to light theme"
	if !t.Dark {
		ic, desc = theme.IcMoon, "Switch to dark theme"
	}
	return layout.Inset{Top: 16, Left: pad, Right: pad, Bottom: 16}.Layout(gtx, func(gtx theme.C) theme.D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx theme.C) theme.D { return theme.Icon(gtx, theme.IcDownload, 24, t.P.Accent) }),
			layout.Rigid(layout.Spacer{Width: 10}.Layout),
			layout.Flexed(1, t.Title("yt-dlp GUI").Layout),
			layout.Rigid(t.IconButton(&a.themeBt, ic, desc).Layout),
		)
	})
}

func (a *App) inputCard(gtx theme.C) theme.D {
	t := a.th
	return layout.Inset{Left: pad, Right: pad}.Layout(gtx, func(gtx theme.C) theme.D {
		return t.Card(gtx, func(gtx theme.C) theme.D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx theme.C) theme.D { return t.Input(gtx, &a.url, "Paste a video or playlist URL") }),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(func(gtx theme.C) theme.D { return a.preset.Layout(gtx, t) }),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(a.addButton),
			)
		})
	})
}

func (a *App) addButton(gtx theme.C) theme.D {
	t := a.th
	return material.ButtonLayoutStyle{Background: t.P.Accent, CornerRadius: 8, Button: &a.addBtn}.Layout(gtx, func(gtx theme.C) theme.D {
		return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 16}.Layout(gtx, func(gtx theme.C) theme.D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx theme.C) theme.D { return theme.Icon(gtx, theme.IcAdd, 18, t.P.OnAccent) }),
				layout.Rigid(layout.Spacer{Width: 6}.Layout),
				layout.Rigid(func(gtx theme.C) theme.D {
					l := material.Body2(t.Theme, "Add")
					l.Color = t.P.OnAccent
					l.Font.Weight = 600
					return l.Layout(gtx)
				}),
			)
		})
	})
}

// status is the line under the input card: "Fetching info…" or the last probe error.
func (a *App) status(gtx theme.C) theme.D {
	a.mu.Lock()
	probing, perr := a.probing, a.probeErr
	a.mu.Unlock()
	var l material.LabelStyle
	switch {
	case probing > 0:
		l = a.th.Muted("Fetching info…")
	case perr != "":
		l = a.th.Muted(perr)
		l.Color = a.th.P.Error
	default:
		return theme.D{}
	}
	return layout.Inset{Top: 10, Left: pad + 4, Right: pad}.Layout(gtx, l.Layout)
}

func (a *App) queueList(gtx theme.C) theme.D {
	jobs := a.q.Jobs()
	live := make(map[int]bool, len(jobs))
	for _, j := range jobs {
		live[j.ID] = true
		if a.rows[j.ID] == nil {
			a.rows[j.ID] = &rowState{}
		}
	}
	for id := range a.rows {
		if !live[id] {
			delete(a.rows, id)
		}
	}
	if len(jobs) == 0 {
		return layout.Center.Layout(gtx, a.th.Muted("No downloads yet — paste a link above.").Layout)
	}
	// Rows carry their own side padding so card shadows are not clipped by the list.
	return a.list.Layout(gtx, len(jobs), func(gtx theme.C, i int) theme.D {
		j := jobs[i]
		in := layout.Inset{Left: pad, Right: pad, Bottom: 12}
		if i == 0 {
			in.Top = 16
		}
		return in.Layout(gtx, func(gtx theme.C) theme.D {
			return a.rows[j.ID].layout(gtx, a.th, a.q, j, a.hintFor(j.Err))
		})
	})
}

func (a *App) footer(gtx theme.C) theme.D {
	a.mu.Lock()
	v, verErr, latest := a.version, a.verErr, a.latest
	a.mu.Unlock()
	t := a.th
	var l material.LabelStyle
	switch {
	case verErr:
		l = t.Muted("yt-dlp not found — install it and restart")
		l.Color = t.P.Error
	case v == "":
		return theme.D{}
	case ytdlp.Newer(v, latest):
		l = t.Muted("yt-dlp " + v + " — update available (" + latest + ")")
		l.Color = t.P.Warning
	default:
		l = t.Muted("yt-dlp " + v)
	}
	return layout.Inset{Top: 8, Bottom: 12, Left: pad, Right: pad}.Layout(gtx, l.Layout)
}

// hintFor is the one-line suggestion for a failed job or probe (D16). Same as
// err.Hint except for HTTP 403: "outdated" only if the D22 check found a newer release.
func (a *App) hintFor(err *ytdlp.Error) string {
	if err == nil {
		return ""
	}
	if err.Hint != "" && err.Hint == ytdlp.Hint("HTTP Error 403") {
		a.mu.Lock()
		newer := ytdlp.Newer(a.version, a.latest)
		a.mu.Unlock()
		if newer {
			return outdatedHint
		}
		return blockedHint
	}
	return err.Hint
}

// submit probes the URL field in the background and queues the result.
func (a *App) submit() {
	url := strings.TrimSpace(a.url.Text())
	if url == "" {
		return
	}
	base := ytdlp.Options{
		Folder:  a.s.Folder,
		Preset:  a.presets[a.preset.Selected()],
		Cookies: a.s.Cookies,
		Subs:    a.s.Subs,
		SubLang: a.s.SubLang,
	}
	a.mu.Lock()
	a.probing++
	a.probeErr = ""
	a.mu.Unlock()
	go func() {
		// ponytail: no cancel for a running probe; ProbeURL ends on its own and Close() does not wait for it.
		p, err := ytdlp.ProbeURL(context.Background(), url, base.Cookies)
		msg := ""
		if err == nil {
			base.URL = url
			if p.IsPlaylist {
				a.q.AddEntries(p.Entries, base)
			} else {
				a.q.Add(p.Title, base)
			}
		} else {
			msg = err.Error()
			var ye *ytdlp.Error
			if errors.As(err, &ye) {
				msg = ye.Error()
				if h := a.hintFor(ye); h != "" {
					msg += " — " + h
				}
			}
		}
		a.mu.Lock()
		a.probing--
		a.probeErr = msg
		if err == nil {
			a.clearURL = url
		}
		a.mu.Unlock()
		a.w.Invalidate()
	}()
}
