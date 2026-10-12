package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReleasesURL is the GitHub "latest release" endpoint; a var so tests can
// point it at httptest.
var ReleasesURL = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"

// Version returns `yt-dlp --version`, e.g. "2026.08.19". Never runs -U.
func Version(ctx context.Context) (string, error) {
	var out bytes.Buffer
	if err := run(ctx, []string{"--version"}, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// LatestTag fetches the latest release tag (e.g. "2026.08.19"). ok is false on
// any failure (offline, rate limit, bad JSON): callers silently skip the hint (D22).
func LatestTag(ctx context.Context) (tag string, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ReleasesURL, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "yt-dlp-gui")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var v struct {
		TagName string `json:"tag_name"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&v) != nil || v.TagName == "" {
		return "", false
	}
	return v.TagName, true
}

// Newer reports whether latest is a newer version than local. Both are
// compared numerically per dot-separated part ("2026.08.19" < "2026.08.19.1" <
// "2026.09.01"); a leading "v" is ignored and non-numeric parts count as 0.
func Newer(local, latest string) bool {
	a, b := versionParts(local), versionParts(latest)
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return y > x
		}
	}
	return false
}

func versionParts(v string) []int {
	var ps []int
	for _, s := range strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".") {
		n, _ := strconv.Atoi(s) // non-numeric -> 0
		ps = append(ps, n)
	}
	return ps
}
