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

	advancedLabel = "Advanced…" // dropdown option after the presets (D6)
	advancedNote  = "Advanced… is for single videos — playlist entries use Best."
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
	gearBt  widget.Clickable
	list    widget.List
	rows    map[int]*rowState // by job ID, pruned every frame
	focused bool              // URL field focused once at startup

	// Panels between the status line and the queue; at most one is shown, settings first.
	settings *settingsPanel
	showSet  bool
	cur      *pendingPick // picker being shown, UI goroutine only

	// mu guards what background goroutines (probes, version check) write.
	// Everything else is touched by the UI goroutine only.
	mu       sync.Mutex
	probing  int           // probes in flight (ponytail: several may run at once, one status line)
	probeErr string        // last probe error + hint, "" when none
	note     string        // muted status note, "" when none
	pending  []pendingPick // probes waiting for a picker, FIFO (ponytail: one picker at a time)
	clearURL string        // URL of a probe that succeeded; the UI goroutine clears the field if it still matches
	version  string        // yt-dlp --version
	verErr   bool          // yt-dlp could not be run
	latest   string        // latest release tag, "" until known / on failure
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
		rows: map[int]*rowState{},

		settings: newSettingsPanel(),
	}
	a.list.Axis = layout.Vertical
	a.url.SingleLine, a.url.Submit = true, true
	a.presets = ytdlp.Presets()
	labels := make([]string, len(a.presets), len(a.presets)+1)
	for i, p := range a.presets {
		labels[i] = p.String()
	}
	labels = append(labels, advancedLabel)
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
	if a.gearBt.Clicked(gtx) {
		if a.showSet = !a.showSet; a.showSet {
			a.settings.open(a.s)
		}
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
		layout.Rigid(a.panels),
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
			layout.Rigid(t.IconButton(&a.gearBt, theme.IcSettings, "Settings").Layout),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
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
	probing, perr, note := a.probing, a.probeErr, a.note
	a.mu.Unlock()
	var l material.LabelStyle
	switch {
	case probing > 0:
		l = a.th.Muted("Fetching info…")
	case perr != "":
		l = a.th.Muted(perr)
		l.Color = a.th.P.Error
	case note != "":
		l = a.th.Muted(note)
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
	return material.List(a.th.Theme, &a.list).Layout(gtx, len(jobs), func(gtx theme.C, i int) theme.D {
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
	txt, warn, bad := footerText(v, latest, verErr)
	if txt == "" {
		return theme.D{}
	}
	l := a.th.Muted(txt)
	switch {
	case bad:
		l.Color = a.th.P.Error
	case warn:
		l.Color = a.th.P.Warning
	}
	return layout.Inset{Top: 8, Bottom: 12, Left: pad, Right: pad}.Layout(gtx, l.Layout)
}

// footerText is the D22 footer: the local yt-dlp version, a warning when the
// latest release tag is newer, an error when yt-dlp could not be run.
func footerText(version, latest string, verErr bool) (text string, warn, bad bool) {
	switch {
	case verErr:
		return "yt-dlp not found — install it and restart", false, true
	case version == "":
		return "", false, false
	case ytdlp.Newer(version, latest):
		return "yt-dlp " + version + " — update available (" + latest + ")", true, false
	}
	return "yt-dlp " + version, false, false
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

// optsFor builds the options shared by every job of one submit from the
// settings at that moment (D7, D17, D18) and the dropdown index. The
// Advanced… index yields Best here; its format id is set by the picker (D24).
func optsFor(s settings.Settings, choice int) ytdlp.Options {
	o := ytdlp.Options{Folder: s.Folder, Preset: ytdlp.Best, Cookies: s.Cookies, Subs: s.Subs, SubLang: s.SubLang}
	if ps := ytdlp.Presets(); choice < len(ps) {
		o.Preset = ps[choice]
	}
	return o
}

func isAdvanced(choice int) bool { return choice >= len(ytdlp.Presets()) }

// pendingPick is a probe result waiting for the user: a playlist checkbox list
// or, for a single video on Advanced…, the format list.
type pendingPick struct {
	p    *ytdlp.Probe
	base ytdlp.Options // URL and settings as of submit time
	pl   *playlistPicker
	fp   *formatPicker
}

// submit probes the URL field in the background and queues the result.
func (a *App) submit() {
	url := strings.TrimSpace(a.url.Text())
	if url == "" {
		return
	}
	s, choice := a.s, a.preset.Selected() // settings and preset as of now
	a.mu.Lock()
	a.probing++
	a.probeErr, a.note = "", ""
	a.mu.Unlock()
	// ponytail: no cancel for a running probe; ProbeURL ends on its own and Close() does not wait for it.
	go a.probe(url, s, choice)
}

func (a *App) invalidate() {
	if a.w != nil {
		a.w.Invalidate()
	}
}

// probe runs the -J call and either queues straight away (single video, normal
// preset) or hands the result to the UI goroutine through a.pending.
func (a *App) probe(url string, s settings.Settings, choice int) {
	p, err := ytdlp.ProbeURL(context.Background(), url, s.Cookies)
	msg, note, ok := "", "", true
	var ye *ytdlp.Error
	switch {
	case err == nil:
		base := optsFor(s, choice)
		base.URL = url
		if p.IsPlaylist || isAdvanced(choice) {
			if p.IsPlaylist && isAdvanced(choice) {
				note = advancedNote
			}
			a.mu.Lock()
			a.pending = append(a.pending, pendingPick{p: p, base: base})
			a.mu.Unlock()
		} else {
			a.q.Add(p.Title, base)
		}
	case errors.As(err, &ye):
		// ponytail: an unprobeable URL is queued anyway; the download fails with yt-dlp's own
		// ERROR: line and the row offers Details + Retry (D16), at the cost of one extra yt-dlp run.
		base := optsFor(s, choice)
		base.URL = url
		a.q.Add(url, base)
	default: // ErrTooDeep, bad JSON: no row
		msg, ok = err.Error(), false
	}
	a.mu.Lock()
	a.probing--
	a.probeErr, a.note = msg, note
	if ok {
		a.clearURL = url
	}
	a.mu.Unlock()
	a.invalidate()
}

// panels shows the settings panel, else the current picker (taking the next
// pending probe if none is open). Queued / cancelled pickers close at once.
func (a *App) panels(gtx theme.C) theme.D {
	t := a.th
	if a.cur == nil {
		a.mu.Lock()
		if len(a.pending) > 0 {
			pk := a.pending[0]
			a.pending = a.pending[1:]
			a.cur = &pk
			if pk.p.IsPlaylist {
				pk.pl = newPlaylistPicker(pk.p)
			} else {
				pk.fp = newFormatPicker(pk.p)
			}
		}
		a.mu.Unlock()
	}
	var w layout.Widget
	switch {
	case a.showSet:
		w = func(gtx theme.C) theme.D {
			d, next, saved, closed := a.settings.layout(gtx, t, a.s)
			if saved {
				a.s = next
				a.save()
				a.q.SetParallel(next.Parallel)
			}
			if closed {
				a.showSet = false
			}
			return d
		}
	case a.cur != nil && a.cur.pl != nil:
		w = func(gtx theme.C) theme.D {
			d, queue, cancel := a.cur.pl.layout(gtx, t)
			if queue {
				a.q.AddEntries(a.cur.pl.checked(), a.cur.base)
			}
			if queue || cancel {
				a.cur = nil
				a.invalidate()
			}
			return d
		}
	case a.cur != nil:
		w = func(gtx theme.C) theme.D {
			d, f, picked, cancel := a.cur.fp.layout(gtx, t)
			if picked {
				o := a.cur.base
				o.FormatID, o.VideoOnlyFormat = f.ID, f.VideoOnly()
				a.q.Add(a.cur.p.Title, o)
			}
			if picked || cancel {
				a.cur = nil
				a.invalidate()
			}
			return d
		}
	default:
		return theme.D{}
	}
	return layout.Inset{Top: 12, Left: pad, Right: pad}.Layout(gtx, w)
}
