package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"yt-dlp-gui/internal/queue"
	"yt-dlp-gui/internal/settings"
	"yt-dlp-gui/internal/ui/theme"
	"yt-dlp-gui/internal/ytdlp"
)

func TestWinFooterText(t *testing.T) {
	cases := []struct {
		v, latest string
		verErr    bool
		text      string
		warn, bad bool
	}{
		{"2026.08.19", "", false, "yt-dlp 2026.08.19", false, false},
		{"2026.08.19", "2026.08.19", false, "yt-dlp 2026.08.19", false, false},
		{"2026.08.19", "2026.09.01", false, "yt-dlp 2026.08.19 — update available (2026.09.01)", true, false},
		{"2026.09.01", "2026.08.19", false, "yt-dlp 2026.09.01", false, false},
		{"", "", false, "", false, false},
		{"", "", true, "yt-dlp not found — install it and restart", false, true},
	}
	for _, c := range cases {
		text, warn, bad := footerText(c.v, c.latest, c.verErr)
		if text != c.text || warn != c.warn || bad != c.bad {
			t.Errorf("footerText(%q,%q,%v) = %q,%v,%v", c.v, c.latest, c.verErr, text, warn, bad)
		}
	}
}

func TestWinFooterMockedRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"2099.01.01"}`))
	}))
	defer srv.Close()
	old := ytdlp.ReleasesURL
	ytdlp.ReleasesURL = srv.URL
	defer func() { ytdlp.ReleasesURL = old }()
	tag, ok := ytdlp.LatestTag(context.Background())
	if !ok {
		t.Fatal("LatestTag failed")
	}
	if text, warn, _ := footerText("2026.08.19", tag, false); !warn || !strings.Contains(text, "update available (2099.01.01)") {
		t.Errorf("footer = %q warn=%v", text, warn)
	}
}

func TestWinOptsFor(t *testing.T) {
	s := settings.Settings{Folder: "/dl", Cookies: "firefox", Subs: true, SubLang: "de"}
	o := optsFor(s, 2)
	if o.Preset != ytdlp.Presets()[2] || o.Folder != "/dl" || !o.Subs || o.SubLang != "de" {
		t.Errorf("opts = %+v", o)
	}
	o.URL = "u"
	args := ytdlp.Args(o)
	if i := slices.Index(args, "--cookies-from-browser"); i < 0 || args[i+1] != "firefox" {
		t.Errorf("download args lack cookies: %v", args)
	}
	adv := len(ytdlp.Presets())
	if !isAdvanced(adv) || isAdvanced(adv-1) || optsFor(s, adv).Preset != ytdlp.Best {
		t.Error("Advanced… is the index after the presets and maps to Best")
	}
}

// winFake points ytdlp.Bin at a script that records its argv to a file
// (one arg per line, appended) and then runs body. It returns that file's path.
func winFake(t *testing.T, body string) (argv string) {
	t.Helper()
	dir := t.TempDir()
	argv = filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> '" + argv + "'\n" + body + "\n"
	bin := filepath.Join(dir, "fake-yt-dlp")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ytdlp.Bin
	ytdlp.Bin = bin
	t.Cleanup(func() { ytdlp.Bin = old })
	return argv
}

const (
	winVideoJSON = `echo '{"_type":"video","id":"x","title":"t","formats":[]}'`
	winListJSON  = `echo '{"_type":"playlist","id":"p","title":"pl","entries":[{"id":"a","title":"A","url":"https://e/a"},{"id":"b","title":"B","url":"https://e/b"}]}'`
)

func TestWinProbeCookies(t *testing.T) {
	argv := winFake(t, winVideoJSON)
	if _, err := ytdlp.ProbeURL(context.Background(), "https://e/v", "firefox"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(argv)
	if !strings.Contains(string(b), "--cookies-from-browser\nfirefox\n") {
		t.Errorf("probe argv lacks cookies:\n%s", b)
	}
}

// winApp has a queue whose downloads succeed instantly (no yt-dlp).
func winApp() *App {
	run := func(context.Context, ytdlp.Options, func(ytdlp.Progress)) (ytdlp.Result, error) {
		return ytdlp.Result{}, nil
	}
	return &App{q: queue.New(run, 1, nil)}
}

func TestWinProbeRouting(t *testing.T) {
	s := settings.Settings{Folder: "/dl", Cookies: "firefox"}
	adv := len(ytdlp.Presets())

	// single video, normal preset: queued straight away
	winFake(t, winVideoJSON)
	a := winApp()
	a.probe("https://e/v", s, 0)
	if jobs := a.q.Jobs(); len(jobs) != 1 || jobs[0].Title != "t" || jobs[0].Opts.Cookies != "firefox" || len(a.pending) != 0 {
		t.Errorf("single: jobs=%+v pending=%d", jobs, len(a.pending))
	}
	// single video, Advanced…: waits for the format picker
	a = winApp()
	a.probe("https://e/v", s, adv)
	if len(a.q.Jobs()) != 0 || len(a.pending) != 1 || a.pending[0].p.IsPlaylist || a.pending[0].base.URL != "https://e/v" {
		t.Errorf("advanced: jobs=%d pending=%+v", len(a.q.Jobs()), a.pending)
	}
	// playlist: waits for the checkbox list; only checked entries are queued with the base options
	winFake(t, winListJSON)
	a = winApp()
	a.probe("https://e/l", s, 2)
	if len(a.q.Jobs()) != 0 || len(a.pending) != 1 || !a.pending[0].p.IsPlaylist {
		t.Fatalf("playlist: jobs=%d pending=%+v", len(a.q.Jobs()), a.pending)
	}
	pk := a.pending[0]
	pl := newPlaylistPicker(pk.p)
	pl.on[0].Value = false
	a.q.AddEntries(pl.checked(), pk.base)
	if jobs := a.q.Jobs(); len(jobs) != 1 || jobs[0].Title != "B" || jobs[0].Opts.Preset != ytdlp.Presets()[2] || jobs[0].Opts.Cookies != "firefox" {
		t.Errorf("playlist queue: %+v", jobs)
	}
	// playlist + Advanced…: Best, with a note
	a = winApp()
	a.probe("https://e/l", s, adv)
	if a.pending[0].base.Preset != ytdlp.Best || a.note != advancedNote {
		t.Errorf("playlist+advanced: %+v note=%q", a.pending[0].base, a.note)
	}
	// yt-dlp failure: queued anyway as a single job titled with the URL (D16)
	winFake(t, "echo 'ERROR: nope' >&2; exit 1")
	a = winApp()
	a.probe("notaurl", s, 0)
	if jobs := a.q.Jobs(); len(jobs) != 1 || jobs[0].Title != "notaurl" || jobs[0].Opts.URL != "notaurl" || a.probeErr != "" || a.clearURL != "notaurl" {
		t.Errorf("failure: jobs=%+v err=%q", jobs, a.probeErr)
	}
}

func TestWinPresetKeys(t *testing.T) {
	if len(presetKeys) != len(ytdlp.Presets())+1 {
		t.Fatalf("%d keys for %d presets + Advanced", len(presetKeys), len(ytdlp.Presets()))
	}
	for i, k := range presetKeys {
		dd := newPresetDropdown(k)
		if dd.Selected() != i || len(dd.Options) != len(presetKeys) {
			t.Errorf("key %q: selected %d (want %d), %d options", k, dd.Selected(), i, len(dd.Options))
		}
	}
	if presetKeys[len(presetKeys)-1] != "advanced" || !isAdvanced(len(presetKeys)-1) {
		t.Error("last key must be the Advanced option")
	}
	if got := newPresetDropdown("bogus").Selected(); got != 0 {
		t.Errorf("unknown key selected %d, want 0", got)
	}
}

func TestWinPresetStartupSelection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for key, label := range map[string]string{"advanced": advancedLabel, "m4a": ytdlp.AudioM4A.String()} {
		s := settings.Default()
		s.Preset = key
		if err := settings.Save(s); err != nil {
			t.Fatal(err)
		}
		loaded, _ := settings.Load()
		dd := newPresetDropdown(loaded.Preset)
		if got := dd.Options[dd.Selected()]; got != label {
			t.Errorf("saved %q: dropdown shows %q, want %q", key, got, label)
		}
	}
}

func TestWinApplySettingsKeepsPreset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := winApp()
	a.th = theme.New(true)
	a.s = settings.Default()
	a.s.Preset = "mp3"
	next, err := settingsFromFields(a.s, a.s.Folder, 0, 1, 0, false, "en") // what the panel's Save builds
	if err != nil {
		t.Fatal(err)
	}
	a.applySettings(next)
	if got, _ := settings.Load(); a.s.Preset != "mp3" || got.Preset != "mp3" {
		t.Errorf("Preset after settings Save: app=%q file=%q, want mp3", a.s.Preset, got.Preset)
	}
}

func TestWinApplySettingsTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := winApp()
	a.th = theme.New(true)
	a.s = settings.Default()
	next := a.s
	next.Dark, next.Parallel = false, 2
	a.applySettings(next)
	if a.th.Dark || a.s.Dark {
		t.Errorf("after save Dark=false: th=%v s=%v", a.th.Dark, a.s.Dark)
	}
	if got, _ := settings.Load(); got.Dark {
		t.Errorf("persisted Dark = true, want false")
	}
}
