package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func redirect(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func write(t *testing.T, content string) {
	t.Helper()
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	redirect(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Dark || s.Parallel != 1 || s.Cookies != "none" || s.SubLang != "en" || s.Subs {
		t.Errorf("bad defaults: %+v", s)
	}
	if filepath.Base(s.Folder) != "Downloads" {
		t.Errorf("Folder = %q, want .../Downloads", s.Folder)
	}
}

func TestSaveLoadRoundTripAndThemePersists(t *testing.T) {
	dir := redirect(t)
	want := Settings{Dark: false, Folder: "/tmp/x", Parallel: 3, Cookies: "firefox", Subs: true, SubLang: "de", Preset: "m4a"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "yt-dlp-gui", "settings.json")); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Dark {
		t.Error("light theme did not survive a restart")
	}
	// no temp files left behind
	ents, _ := os.ReadDir(filepath.Join(dir, "yt-dlp-gui"))
	if len(ents) != 1 {
		t.Errorf("config dir has %d entries, want 1", len(ents))
	}
}

func TestCorruptJSON(t *testing.T) {
	redirect(t)
	write(t, "{not json")
	s, err := Load()
	if err == nil {
		t.Fatal("want error")
	}
	if s != Default() {
		t.Errorf("got %+v, want defaults", s)
	}
}

func TestOutOfRangeFixedUp(t *testing.T) {
	for in, want := range map[string]int{`{"parallel":0}`: 1, `{"parallel":-3}`: 1, `{"parallel":9}`: 4, `{"parallel":2}`: 2} {
		redirect(t)
		write(t, in)
		s, err := Load()
		if err != nil || s.Parallel != want {
			t.Errorf("%s: parallel=%d err=%v, want %d", in, s.Parallel, err, want)
		}
	}
	redirect(t)
	write(t, `{"cookies":"netscape"}`)
	if s, _ := Load(); s.Cookies != "none" {
		t.Errorf("Cookies = %q, want none", s.Cookies)
	}
}

func TestPresetDefaultsToBest(t *testing.T) {
	for _, in := range []string{`{}`, `{"preset":""}`} {
		redirect(t)
		write(t, in)
		if s, err := Load(); err != nil || s.Preset != "best" {
			t.Errorf("%s: preset=%q err=%v, want best", in, s.Preset, err)
		}
	}
}

func TestPartialJSONKeepsDefaults(t *testing.T) {
	redirect(t)
	write(t, `{"dark":false,"unknown":1}`)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	d := Default()
	d.Dark = false
	if s != d {
		t.Errorf("got %+v, want %+v", s, d)
	}
}

func TestValidFolder(t *testing.T) {
	dir := t.TempDir()
	if err := ValidFolder(dir); err != nil {
		t.Error(err)
	}
	if err := ValidFolder(filepath.Join(dir, "nope")); err == nil {
		t.Error("missing folder accepted")
	}
	f := filepath.Join(dir, "file")
	os.WriteFile(f, nil, 0o644)
	if err := ValidFolder(f); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("file accepted: %v", err)
	}
}
