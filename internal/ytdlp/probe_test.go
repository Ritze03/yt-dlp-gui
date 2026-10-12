package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// With only a Flatpak install present, every browser reaches the probe call
// with that root (forks as firefox:<root>, chromium family as <name>:<root>).
func TestProbeURLFlatpakCookies(t *testing.T) {
	fakeBin(t, `echo "ERROR: $*" >&2; exit 1`) // echoes its argv back as the error
	for _, c := range cookieCases {
		home := cookieEnv(t)
		mkdir(t, filepath.Join(home, c.flatpak))
		_, err := ProbeURL(context.Background(), "https://youtu.be/x", c.browser)
		var ye *Error
		if !errors.As(err, &ye) || !strings.Contains(ye.LastError, "--cookies-from-browser "+c.name+":"+filepath.Join(home, c.flatpak)+" ") {
			t.Errorf("%s: %v", c.browser, err)
		}
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseProbeSingle(t *testing.T) {
	p, err := ParseProbe(fixture(t, "single.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.IsPlaylist || p.ID != "55XuQm3NZi8" || p.Title != "phem - STFU (official video)" || p.Duration != 173 {
		t.Fatalf("header: %+v", p)
	}
	var ids []string
	for _, f := range p.Formats {
		ids = append(ids, f.ID)
	}
	// storyboard "sb0" is dropped, order kept
	if w := []string{"233", "140", "251", "160", "137", "401", "18"}; !reflect.DeepEqual(ids, w) {
		t.Fatalf("format ids %v want %v", ids, w)
	}
	by := map[string]Format{}
	for _, f := range p.Formats {
		by[f.ID] = f
	}
	// video-only: vcodec set, acodec "none"
	if f := by["401"]; !f.VideoOnly() || f.AudioOnly() || f.Height != 2026 || f.Size != 121337553 || f.Ext != "mp4" || f.Note != "2160p" {
		t.Errorf("401: %+v", f)
	}
	// audio-only with a null acodec-less sibling (233) still counts as audio
	if f := by["140"]; f.VideoOnly() || !f.AudioOnly() || f.ACodec != "mp4a.40.2" || f.Size != 2802999 {
		t.Errorf("140: %+v", f)
	}
	if f := by["233"]; f.VideoOnly() || !f.AudioOnly() {
		t.Errorf("233 (null acodec): %+v", f)
	}
	// progressive format: neither; size falls back to filesize_approx
	if f := by["18"]; f.VideoOnly() || f.AudioOnly() || f.Size != 7500000 || f.Height != 360 {
		t.Errorf("18: %+v", f)
	}
}

func TestParseProbePlaylist(t *testing.T) {
	p, err := ParseProbe(fixture(t, "playlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsPlaylist || p.Title != "My Mix: 100% Best/Songs" || p.ID != "PLabc123" {
		t.Fatalf("header: %+v", p)
	}
	// private video (no url) and null entry are skipped; the one without
	// "url" falls back to a watch URL built from the id; indexes stay dense.
	w := []Entry{
		{ID: "aaaaaaaaaaa", URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Title: "First song", Duration: 120, PlaylistTitle: p.Title, Index: 1},
		{ID: "ccccccccccc", URL: "https://www.youtube.com/watch?v=ccccccccccc", Title: "Third song", Duration: 215, PlaylistTitle: p.Title, Index: 2},
		{ID: "ddddddddddd", URL: "https://www.youtube.com/watch?v=ddddddddddd", Title: "No url field", Duration: 60, PlaylistTitle: p.Title, Index: 3},
	}
	if !reflect.DeepEqual(p.Entries, w) {
		t.Errorf("entries\n got %+v\nwant %+v", p.Entries, w)
	}
}

func TestParseProbeNested(t *testing.T) {
	p, err := ParseProbe(fixture(t, "nested.json"))
	if err != nil {
		t.Fatal(err)
	}
	// inline sub-playlist is flattened with its own title and numbering;
	// the by-URL one is left in subs for ProbeURL; the direct video belongs to the top playlist.
	var got [][3]any
	for _, e := range p.Entries {
		got = append(got, [3]any{e.ID, e.PlaylistTitle, e.Index})
	}
	w := [][3]any{
		{"vvvvvvvvvv1", "Some Channel - Videos", 1},
		{"vvvvvvvvvv2", "Some Channel - Videos", 2},
		{"rrrrrrrrrr1", "Some Channel", 1},
	}
	if !reflect.DeepEqual(got, w) {
		t.Errorf("entries %v want %v", got, w)
	}
	if len(p.subs) != 1 || p.subs[0].URL != "https://www.youtube.com/@chan/shorts" {
		t.Errorf("subs %+v", p.subs)
	}
}

func TestParseProbeTooDeep(t *testing.T) {
	if _, err := ParseProbe(fixture(t, "deep.json")); !errors.Is(err, ErrTooDeep) {
		t.Errorf("deep: %v", err)
	}
	// a sub-playlist fetched as the second level may not contain playlists
	if _, err := parseProbe(fixture(t, "nested.json"), false); !errors.Is(err, ErrTooDeep) {
		t.Errorf("second level: %v", err)
	}
	if _, err := parseProbe(fixture(t, "nested_sub.json"), false); err != nil {
		t.Errorf("plain sub-playlist must parse: %v", err)
	}
}

func TestParseProbeBad(t *testing.T) {
	if _, err := ParseProbe([]byte("not json")); err == nil {
		t.Error("garbage must fail")
	}
	if _, err := ParseProbe([]byte(`{"_type":"playlist","title":"empty","entries":[]}`)); err == nil {
		t.Error("empty playlist must fail")
	}
}

// ProbeURL end to end against a fake yt-dlp: top level + one sub-playlist call.
func TestProbeURLNestedFake(t *testing.T) {
	dir := t.TempDir()
	for name, fx := range map[string]string{"top": "nested.json", "sub": "nested_sub.json"} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), fixture(t, fx), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// $@ ends with "-- <url>"; the shorts URL is the sub call, anything else the top call.
	fakeBin(t, `case "$*" in
*cookies-from-browser\ firefox*) ;; *) echo "ERROR: cookies missing" >&2; exit 1;; esac
case "$*" in *@chan/shorts) cat `+dir+`/sub.json;; *) cat `+dir+`/top.json;; esac`)

	p, err := ProbeURL(context.Background(), "https://www.youtube.com/@chan", "firefox")
	if err != nil {
		t.Fatal(err)
	}
	var got [][3]any
	for _, e := range p.Entries {
		got = append(got, [3]any{e.ID, e.PlaylistTitle, e.Index})
	}
	w := [][3]any{
		{"vvvvvvvvvv1", "Some Channel - Videos", 1},
		{"vvvvvvvvvv2", "Some Channel - Videos", 2},
		{"rrrrrrrrrr1", "Some Channel", 1},
		{"sssssssss01", "Some Channel - Shorts", 1},
		{"sssssssss02", "Some Channel - Shorts", 2},
	}
	if !reflect.DeepEqual(got, w) {
		t.Errorf("entries %v want %v", got, w)
	}

	// cookies are passed to the probe call too: without them the fake errors
	_, err = ProbeURL(context.Background(), "https://www.youtube.com/@chan", "")
	var ye *Error
	if !errors.As(err, &ye) || ye.LastError != "cookies missing" {
		t.Errorf("want *Error{cookies missing}, got %v", err)
	}
}
