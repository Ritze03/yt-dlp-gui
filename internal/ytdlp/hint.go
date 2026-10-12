package ytdlp

import "strings"

// Hint maps a yt-dlp error line (Error.LastError) to a one-line suggestion,
// or "" when nothing matches (D16). Matching is case-insensitive substring;
// the wordings come from yt-dlp's own messages:
//
//   - bot check / age gate: YouTube's playability reason surfaced verbatim by
//     extractor/youtube/_video.py, "Sign in to confirm you’re not a bot" and
//     "Sign in to confirm your age" (plus its "Use --cookies-from-browser or
//     --cookies for the authentication" login hint and "age-restricted" notes);
//   - HTTP 403: networking/exceptions.py HTTPError -> "HTTP Error 403: Forbidden";
//   - geo-block: GeoRestrictedError via raise_geo_restricted() in
//     extractor/common.py ("This video is not available from your location due
//     to geo restriction") and YouTube's "The uploader has not made this video
//     available in your country".
func Hint(errLine string) string {
	l := strings.ToLower(errLine)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("not a bot", "sign in to confirm", "confirm your age", "age-restricted", "inappropriate for some users", "--cookies-from-browser"):
		return "Set a cookies browser in Settings."
	case has("http error 403"):
		return "yt-dlp may be outdated."
	case has("available in your country", "not available from your location", "geo restriction", "geo-restricted", "geo restricted"):
		return "Not available in your country."
	}
	return ""
}
