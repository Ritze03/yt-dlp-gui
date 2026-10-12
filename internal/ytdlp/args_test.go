package ytdlp

import (
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
	if has(Args(Options{URL: "u", Folder: "/d", SubLang: "de"})) {
		t.Error("subs off must not add --write-subs")
	}
	for _, p := range []Preset{AudioM4A, AudioMP3} {
		if has(Args(Options{URL: "u", Folder: "/d", Preset: p, Subs: true})) {
			t.Errorf("audio preset %v must skip subs", p)
		}
	}
}

func TestArgsCookies(t *testing.T) {
	for _, b := range []string{"", "none"} {
		if a := Args(Options{URL: "u", Folder: "/d", Cookies: b}); slices.Contains(a, "--cookies-from-browser") {
			t.Errorf("cookies %q must add nothing: %q", b, a)
		}
	}
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
