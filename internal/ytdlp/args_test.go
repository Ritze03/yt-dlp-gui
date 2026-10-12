package ytdlp

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// base is the part of Args shared by every download, before the format flags.
var base = []string{
	"--no-quiet", "--progress", "--newline", "--progress-delta", "0.3",
	"--progress-template", "download:PROG %(info.vcodec)s %(info.acodec)s %(progress)j",
	"--print", "before_dl:DEST %(_filename)s",
	"--print", "after_move:DONE %(filepath)s",
	"--no-playlist",
	"--embed-metadata", "--embed-thumbnail", "--embed-chapters",
}

func want(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func tailArgs(folder, tmpl, url string) []string {
	return []string{"-P", folder, "-o", tmpl, "--", url}
}

func TestArgsPresets(t *testing.T) {
	const url, dir = "https://youtu.be/x", "/dl"
	single := tailArgs(dir, "%(title)s [%(id)s].%(ext)s", url)
	tests := []struct {
		name   string
		preset Preset
		format []string
	}{
		{"best", Best, []string{"-f", "bv*+ba/b", "-S", "res,ext:mp4:m4a"}},
		{"1080p", P1080, []string{"-f", "bv*+ba/b", "-S", "res:1080,ext:mp4:m4a"}},
		{"720p", P720, []string{"-f", "bv*+ba/b", "-S", "res:720,ext:mp4:m4a"}},
		{"m4a", AudioM4A, []string{"-f", "ba[ext=m4a]/ba", "-x", "--audio-format", "m4a"}},
		{"mp3", AudioMP3, []string{"-f", "ba", "-x", "--audio-format", "mp3", "--audio-quality", "0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Args(Options{URL: url, Folder: dir, Preset: tc.preset})
			if w := want(base, tc.format, single); !reflect.DeepEqual(got, w) {
				t.Errorf("got  %q\nwant %q", got, w)
			}
		})
	}
}

func TestArgsAdvanced(t *testing.T) {
	single := tailArgs("/dl", "%(title)s [%(id)s].%(ext)s", "u")
	got := Args(Options{URL: "u", Folder: "/dl", Preset: P720, FormatID: "137", VideoOnlyFormat: true})
	if w := want(base, []string{"-f", "137+ba"}, single); !reflect.DeepEqual(got, w) {
		t.Errorf("video-only: got %q want %q", got, w)
	}
	got = Args(Options{URL: "u", Folder: "/dl", FormatID: "18"})
	if w := want(base, []string{"-f", "18"}, single); !reflect.DeepEqual(got, w) {
		t.Errorf("combined: got %q want %q", got, w)
	}
	// the advanced format replaces the preset's -S entirely
	if slices.Contains(got, "-S") {
		t.Errorf("advanced args must not carry -S: %q", got)
	}
}

func TestArgsEmbedAlways(t *testing.T) {
	for _, p := range Presets() {
		for _, subs := range []bool{false, true} {
			a := Args(Options{URL: "u", Folder: "/d", Preset: p, Subs: subs})
			for _, f := range []string{"--embed-metadata", "--embed-thumbnail", "--embed-chapters", "--no-playlist"} {
				if !slices.Contains(a, f) {
					t.Errorf("preset %v subs=%v missing %s", p, subs, f)
				}
			}
		}
	}
}

func TestArgsSubs(t *testing.T) {
	has := func(a []string) bool { return slices.Contains(a, "--write-subs") }
	a := Args(Options{URL: "u", Folder: "/d", Subs: true})
	i := slices.Index(a, "--sub-langs")
	if !has(a) || !slices.Contains(a, "--embed-subs") || i < 0 || a[i+1] != "en" {
		t.Errorf("default lang: %q", a)
	}
	a = Args(Options{URL: "u", Folder: "/d", Subs: true, SubLang: "de"})
	if i := slices.Index(a, "--sub-langs"); i < 0 || a[i+1] != "de" {
		t.Errorf("lang de: %q", a)
	}
	if !slices.Contains(a, "--write-auto-subs") {
		t.Errorf("subs must also fetch auto captions: %q", a)
	}
	exact := Args(Options{URL: "u", Folder: "/d", Preset: P720, Subs: true})
	if w := want(base, []string{"--write-subs", "--write-auto-subs", "--embed-subs", "--compat-options", "no-keep-subs", "--sub-langs", "en"},
		[]string{"-f", "bv*+ba/b", "-S", "res:720,ext:mp4:m4a"}, tailArgs("/d", "%(title)s [%(id)s].%(ext)s", "u")); !reflect.DeepEqual(exact, w) {
		t.Errorf("720p+subs: got %q want %q", exact, w)
	}
	if has(Args(Options{URL: "u", Folder: "/d", SubLang: "de"})) {
		t.Error("subs off must not add --write-subs")
	}
	for _, p := range []Preset{AudioM4A, AudioMP3} {
		if a := Args(Options{URL: "u", Folder: "/d", Preset: p, Subs: true}); has(a) || slices.Contains(a, "--write-auto-subs") {
			t.Errorf("audio preset %v must skip subs", p)
		}
	}
}

// cookieCases: per browser, the yt-dlp name, native root, Flatpak root (both
// relative to the temp $HOME; XDG_CONFIG_HOME unset so config = $HOME/.config).
// Firefox is absent: yt-dlp's own default already searches Flatpak/snap/XDG.
var cookieCases = []struct{ browser, name, native, flatpak string }{
	{"chromium", "chromium", ".config/chromium", ".var/app/org.chromium.Chromium/config/chromium"},
	{"chrome", "chrome", ".config/google-chrome", ".var/app/com.google.Chrome/config/google-chrome"},
	{"brave", "brave", ".config/BraveSoftware/Brave-Browser", ".var/app/com.brave.Browser/config/BraveSoftware/Brave-Browser"},
	{"zen", "firefox", ".zen", ".var/app/app.zen_browser.zen/.zen"},
	{"floorp", "firefox", ".floorp", ".var/app/one.ablaze.floorp/.floorp"},
}

// cookieEnv gives a clean temp $HOME and returns it.
func cookieEnv(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCookieSpec(t *testing.T) {
	for _, c := range cookieCases {
		home := cookieEnv(t)
		native, flatpak := filepath.Join(home, c.native), filepath.Join(home, c.flatpak)
		fork := c.name != c.browser
		// nothing present: yt-dlp's default (forks: the native root, for its own error)
		nothing := c.browser
		if fork {
			nothing = c.name + ":" + native
		}
		if got := cookieSpec(c.browser); got != nothing {
			t.Errorf("%s nothing present: %q want %q", c.browser, got, nothing)
		}
		mkdir(t, flatpak)
		if got, w := cookieSpec(c.browser), c.name+":"+flatpak; got != w {
			t.Errorf("%s flatpak only: %q want %q", c.browser, got, w)
		}
		mkdir(t, native)
		if got := cookieSpec(c.browser); got != nothing { // both: native wins
			t.Errorf("%s both: %q want %q", c.browser, got, nothing)
		}
		if err := os.Remove(flatpak); err != nil {
			t.Fatal(err)
		}
		if got := cookieSpec(c.browser); got != nothing { // native only
			t.Errorf("%s native only: %q want %q", c.browser, got, nothing)
		}
	}
	cookieEnv(t)
	// firefox: yt-dlp already searches XDG, ~/.mozilla, Flatpak and snap itself
	if got := cookieSpec("firefox"); got != "firefox" {
		t.Errorf("firefox: %q", got)
	}
}

// XDG_CONFIG_HOME moves the native Chromium-family root, as in yt-dlp.
func TestCookieSpecXDG(t *testing.T) {
	home := cookieEnv(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	mkdir(t, filepath.Join(home, ".var/app/com.google.Chrome/config/google-chrome"))
	mkdir(t, filepath.Join(xdg, "google-chrome"))
	if got := cookieSpec("chrome"); got != "chrome" {
		t.Errorf("native under XDG_CONFIG_HOME: %q", got)
	}
}

func TestArgsCookies(t *testing.T) {
	for _, b := range []string{"", "none"} {
		if a := Args(Options{URL: "u", Folder: "/d", Cookies: b}); slices.Contains(a, "--cookies-from-browser") {
			t.Errorf("cookies %q must add nothing: %q", b, a)
		}
		if cookieArgs(b) != nil {
			t.Errorf("%q must give no flag", b)
		}
	}
	for _, c := range cookieCases { // Flatpak only: every case must reach the download argv
		home := cookieEnv(t)
		mkdir(t, filepath.Join(home, c.flatpak))
		a := Args(Options{URL: "u", Folder: "/d", Cookies: c.browser})
		if i := slices.Index(a, "--cookies-from-browser"); i < 0 || a[i+1] != c.name+":"+filepath.Join(home, c.flatpak) {
			t.Errorf("%s download args: %q", c.browser, a)
		}
	}
	cookieEnv(t)
	a := Args(Options{URL: "u", Folder: "/d", Cookies: "firefox"})
	if i := slices.Index(a, "--cookies-from-browser"); i < 0 || a[i+1] != "firefox" {
		t.Errorf("download args: %q", a)
	}
}

func TestArgsPlaylistTemplate(t *testing.T) {
	e := &Entry{PlaylistTitle: "My Mix: 100% Best/Songs", Index: 7}
	a := Args(Options{URL: "u", Folder: "/d", Playlist: e})
	// ':' and '/' sanitised, '%' escaped for the output template, index padded
	if got, w := a[slices.Index(a, "-o")+1], "My Mix_ 100%% Best_Songs/007 - %(title)s.%(ext)s"; got != w {
		t.Errorf("template %q want %q", got, w)
	}
	e = &Entry{PlaylistTitle: " .. ", Index: 1234}
	a = Args(Options{URL: "u", Folder: "/d", Playlist: e})
	if got, w := a[slices.Index(a, "-o")+1], "playlist/1234 - %(title)s.%(ext)s"; got != w {
		t.Errorf("template %q want %q", got, w)
	}
	// URL goes last, after "--", so a URL starting with '-' is not an option
	if n := len(a); a[n-2] != "--" || a[n-1] != "u" {
		t.Errorf("tail %q", a[len(a)-2:])
	}
}

func TestPresetStrings(t *testing.T) {
	var labels []string
	for _, p := range Presets() {
		labels = append(labels, p.String())
	}
	if got := strings.Join(labels, "|"); got != "Best video+audio|1080p|720p|Audio only (m4a)|Audio only (mp3)" {
		t.Error(got)
	}
}
