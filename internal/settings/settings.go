// Package settings persists the app's settings as JSON under
// os.UserConfigDir()/yt-dlp-gui/settings.json.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"yt-dlp-gui/internal/ytdlp"
)

// Settings is everything that survives a restart.
type Settings struct {
	Dark     bool   `json:"dark"`
	Folder   string `json:"folder"`
	Parallel int    `json:"parallel"` // 1..4
	Cookies  string `json:"cookies"`  // one of ytdlp.Browsers
	Subs     bool   `json:"subs"`
	SubLang  string `json:"sub_lang"`
	Preset   string `json:"preset"` // format dropdown choice, a stable key owned by the UI ("best" ... "advanced")
}

// Default returns the first-run settings: dark theme, ~/Downloads, one
// download at a time, no cookies, no subtitles (language "en").
func Default() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		Dark:     true,
		Folder:   filepath.Join(home, "Downloads"),
		Parallel: 1,
		Cookies:  "none",
		SubLang:  "en",
		Preset:   "best",
	}
}

// Path is the settings file location.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yt-dlp-gui", "settings.json"), nil
}

// Load reads the settings file. A missing file gives defaults and a nil error;
// an unreadable or corrupt file gives defaults and the error (callers may
// ignore it). Missing fields keep their defaults; out-of-range values are
// fixed up.
func Load() (Settings, error) {
	s := Default()
	p, err := Path()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err != nil {
		return Default(), fmt.Errorf("settings %s: %w", p, err)
	}
	return s.normalize(), nil
}

func (s Settings) normalize() Settings {
	d := Default()
	s.Parallel = min(max(s.Parallel, 1), 4)
	if !slices.Contains(ytdlp.Browsers, s.Cookies) {
		s.Cookies = "none"
	}
	if s.Folder == "" {
		s.Folder = d.Folder
	}
	if s.SubLang == "" {
		s.SubLang = d.SubLang
	}
	if s.Preset == "" { // unknown keys are the UI's to fall back on
		s.Preset = d.Preset
	}
	return s
}

// Save writes the settings atomically (temp file in the same dir, then rename).
func Save(s Settings) error {
	p, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.normalize(), "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "settings-*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// ValidFolder reports whether path is an existing, writable directory.
func ValidFolder(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", path)
	}
	f, err := os.CreateTemp(path, ".yt-dlp-gui-write-test-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", path, err)
	}
	f.Close()
	return os.Remove(f.Name())
}
