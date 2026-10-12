package ytdlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHint(t *testing.T) {
	const cookies = "Set a cookies browser in Settings."
	tests := []struct{ line, want string }{
		// bot check (curly and straight apostrophe), real wording + yt-dlp's login hint
		{"[youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication.", cookies},
		{"[youtube] abc: Sign in to confirm you're not a bot", cookies},
		// age gate
		{"[youtube] abc: Sign in to confirm your age. This video may be inappropriate for some users.", cookies},
		{"[youtube] abc: This video is age-restricted; some formats may be missing", cookies},
		// 403
		{"unable to download video data: HTTP Error 403: Forbidden", "yt-dlp may be outdated."},
		{"HTTP Error 403: Forbidden", "yt-dlp may be outdated."},
		// geo
		{"[youtube] abc: The uploader has not made this video available in your country", "Not available in your country."},
		{"[foo] 1: This video is not available from your location due to geo restriction", "Not available in your country."},
		{"[foo] 1: Video is geo restricted", "Not available in your country."},
		// no match
		{"[youtube] abc: Video unavailable", ""},
		{"HTTP Error 404: Not Found", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := Hint(tc.line); got != tc.want {
			t.Errorf("Hint(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		local, latest string
		want          bool
	}{
		{"2026.08.19", "2026.09.01", true},
		{"2026.08.19", "2026.08.19", false},
		{"2026.08.19", "2026.08.19.1", true},
		{"2026.08.19.1", "2026.08.19", false},
		{"2026.08.19.1", "2026.08.19.2", true},
		{"2026.08.19", "2026.08.9", false}, // numeric, not lexical
		{"2026.8.9", "2026.08.19", true},
		{"2025.12.31", "2026.01.01", true},
		{"2026.08.19", "v2026.08.20", true},
		{"2026.08.19.232050", "2026.08.19", false},
		{"", "2026.08.19", true},
	}
	for _, tc := range tests {
		if got := Newer(tc.local, tc.latest); got != tc.want {
			t.Errorf("Newer(%q,%q) = %v, want %v", tc.local, tc.latest, got, tc.want)
		}
	}
}

func TestLatestTag(t *testing.T) {
	status, body := http.StatusOK, `{"tag_name":"2026.09.01","name":"x"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub requires a User-Agent")
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	old := ReleasesURL
	ReleasesURL = srv.URL
	defer func() { ReleasesURL = old }()

	if tag, ok := LatestTag(context.Background()); !ok || tag != "2026.09.01" {
		t.Errorf("ok case: %q %v", tag, ok)
	}
	status = http.StatusForbidden // rate limited
	if tag, ok := LatestTag(context.Background()); ok || tag != "" {
		t.Errorf("403: %q %v", tag, ok)
	}
	status, body = http.StatusOK, `not json`
	if _, ok := LatestTag(context.Background()); ok {
		t.Error("bad json must be ok=false")
	}
	status, body = http.StatusOK, `{"tag_name":""}`
	if _, ok := LatestTag(context.Background()); ok {
		t.Error("empty tag must be ok=false")
	}
	srv.Close()
	if _, ok := LatestTag(context.Background()); ok {
		t.Error("unreachable must be ok=false")
	}
}
