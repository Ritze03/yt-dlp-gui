# yt-dlp-gui

A lean personal desktop GUI for [yt-dlp](https://github.com/yt-dlp/yt-dlp), written in Go with [Gio](https://gioui.org) (immediate-mode UI, native Wayland).

Paste a video or playlist URL, pick a format preset, and watch the download queue. The app drives the system `yt-dlp` binary as a subprocess: it probes the URL with `-J --flat-playlist` and reads per-stream progress from `--progress-template` JSON. Each row shows its stage: video, audio, merging, processing.

Developed and tested on Linux (Wayland, wlroots compositor) only. Other platforms are untested.

## Features

- Format presets: Best video+audio, 1080p, 720p, Audio only (m4a), Audio only (mp3). Video presets prefer mp4/m4a. The chosen preset is remembered.
- "Advanced...": lists the video's real formats. A video-only pick gets the best audio merged in, which may produce `.mkv`.
- Playlists: a checkbox picker with select all/none. Only checked entries are queued, saved as `<Folder>/<playlist title>/<NNN> - <title>.<ext>`. Single videos are saved as `<title> [<id>].<ext>`.
- Queue with 1-4 parallel downloads (default 1), per-row progress, speed and ETA, and a "Clear completed" button.
  - Cancel keeps the `.part` files, so Retry resumes. Retry uses the current cookie and subtitle settings.
  - Remove deletes leftover partial files, never the finished file.
  - Failed rows show yt-dlp's error, a hint, and expandable details.
  - Finished rows have Open file and Show in folder (via `xdg-open`).
- Metadata, thumbnail and chapters are always embedded.
- Optional subtitles in a chosen language (default `en`): manual subs, falling back to YouTube auto-captions, embedded in the file. Video presets only.
- Cookies from browser: none, Firefox, Chromium, Brave, Chrome, Zen or Floorp. Native and Flatpak installs both work; Zen and Floorp use yt-dlp's Firefox cookie reader. Useful for age-restricted or bot-checked videos.
- Footer shows the `yt-dlp --version`. At startup the app checks GitHub's latest yt-dlp release once and shows "update available (<tag>)" if yours is older. It never updates yt-dlp itself.

## Requirements

- `yt-dlp` on `PATH`. Update it with your package manager; the app never runs `yt-dlp -U`.
- `ffmpeg` and `ffprobe`, for merging and embedding.
- A JS runtime such as `deno` is recommended for YouTube (yt-dlp uses it for JS challenges).
- `xdg-open`, for Open file / Show in folder.
- To build: Go 1.27+ and Gio's Linux dependencies (Wayland and X11 dev libraries, libxkbcommon, EGL/Vulkan headers). See https://gioui.org/doc/install/linux.

## Build and run

```
go run .
# or
go build -o yt-dlp-gui . && ./yt-dlp-gui
```

## Settings

Open the gear icon. Settings are saved as JSON in `~/.config/yt-dlp-gui/settings.json` (`os.UserConfigDir()/yt-dlp-gui/settings.json`).

- Download folder (default `~/Downloads`; validated as an existing, writable directory)
- Theme: Dark or Light
- Parallel downloads: 1-4
- Cookies from browser
- Subtitles on/off and language

## Development

```
go test ./...
YTDLP_LIVE=1 go test -run Live ./internal/ytdlp/ -v   # opt-in, hits YouTube
```

`scripts/headless.sh` runs the app inside a headless sway session, separate from your real Wayland session, for screenshots and UI checks. Subcommands: `start`, `run <cmd>`, `shot <out.png>`, `type`, `key`, `click X Y`, `msg`, `env`, `stop`. It needs sway, grim and wtype.

Layout: `internal/ytdlp` (subprocess, args, progress, errors), `internal/queue` (job queue), `internal/settings` (JSON persistence), `internal/ui` (Gio UI and theme).

The Inter font is embedded (`internal/ui/theme/fonts/`), under the SIL Open Font License; see `OFL.txt` there.

## License

No license specified yet.
