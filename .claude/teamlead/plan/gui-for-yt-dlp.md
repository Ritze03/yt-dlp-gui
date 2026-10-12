# GUI for yt-dlp

> **Stage 5** — building user note: queue bar buttons "Cancel all", "Clear cancelled", "Clear completed" · started 2026-10-12
> Phase A passed agent and user testing.

## Goal
A lean, good-looking personal Go + Gio desktop app for yt-dlp. Paste a URL or playlist, pick a preset or an exact format, and watch a download queue with progress, in dark or light mode.

## Context
Empty project directory (greenfield, not a git repo). Machine scout, 2026-10-12:

- **yt-dlp** 2026.08.19 via pacman (`/usr/bin/yt-dlp`), plus `yt-dlp-ejs` 0.8.0. Importable as `import yt_dlp` from system Python 3.14.7. No yt-dlp config file anywhere.
- **ffmpeg/ffprobe** n9.0.2. JS runtimes: deno 2.9.7, node 24.21, bun 1.3.14.
- **Python GUI libs already importable:** PySide6 6.12, PyQt6 6.11, GTK4 + libadwaita 1.10 via PyGObject, tkinter. Not present: wx, flet, nicegui, textual.
- **Python tooling:** pip, uv, poetry (no pipx).
- **Other toolchains:** Rust 1.99 (no tauri-cli; webkit2gtk-4.1 present), Go 1.27, .NET 8, Java 21, qt6-webengine.
- **Desktop:** `mango` wlroots tiling compositor on Wayland, so no KDE or GNOME pulls toward Qt or GTK.
- **No existing yt-dlp GUI is installed** (no parabolic, tartube, etc.).
- **yt-dlp reference:** https://github.com/yt-dlp/yt-dlp. Workers check flags there, not from memory. *Your note 4.*
- **Wayland test tooling:** sway 1.12, grim, wtype, ydotool; wayland 1.26, libxkbcommon 1.13, vulkan-headers and mesa 26.2 are installed (Gio's Linux build deps). GOPATH is `~/go`.
- **Zero-install fit:** Python + PySide6 calling `yt_dlp` in-process. Runs natively on Wayland.

**Gio styling research** (read from the v0.10.3 module source, 2026-10-12):
- `gioui.org` and `gioui.org/x` are both at **v0.10.3** (2026-09-28). They are versioned in lockstep, so pin both to the same tag. https://proxy.golang.org/gioui.org/@latest
- `material.NewTheme()` takes no args. `th.Palette` is `{Bg, Fg, ContrastBg, ContrastFg}`; swapping it takes effect on the next frame as long as widget styles are built every frame. `Theme.WithPalette(p)` returns a copy.
- Styling knobs: `ButtonStyle{Background, CornerRadius, Inset,…}`, `ProgressBarStyle{Color, TrackColor, Height, Radius}`, `EditorStyle{HintColor, SelectionColor,…}`. Cards come from `component.Surface(th)` (a rounded fill plus `component.Shadow`); dividers from `component.Divider(th)`.
- **No Select/Dropdown widget exists.** The pattern is a `component.ContextArea` (`Activation: pointer.ButtonPrimary`, `AbsolutePosition: true`) wrapped around a button, with a `component.Menu` of `MenuItem`s inside. `component.Discloser` is an expand/collapse widget, a fit for "Advanced…".
- Fonts: `th.Shaper = text.NewShaper(text.WithCollection(faces))`, where `opentype.ParseCollection` loads a TTF. A shaper must not be shared across windows. Icons: `widget.NewIcon(icons.X)` from `golang.org/x/exp/shiny/materialdesign/icons`.
- **System dark-mode detection is not available on Linux.** `x/pref/theme.IsDarkMode()` is a stub, and the xdg-portal `color-scheme` setting is not served by xdg-desktop-portal-wlr (sway/wlroots). https://bbs.archlinux.org/viewtopic.php?id=285066
- Headless: `gioui.org/gpu/headless` renders frames to an `image.RGBA` (EGL or Vulkan). A real Wayland window tries EGL, then Vulkan. Running Gio under `WLR_BACKENDS=headless` sway is **unconfirmed**; it probably needs Mesa llvmpipe (mesa is installed). There is no `GIORENDERER` env var.
- Gio reads the text scale via `gsettings`, falling back to 1.0.

**yt-dlp facts** (brainstorm pass, checked against `--help` and one live `-J` call):
- Progress: `--newline --progress-template "download:PROG %(progress)j"` prints one JSON object per line, with keys `status, downloaded_bytes, total_bytes, total_bytes_estimate, speed, eta, fragment_index, fragment_count`. `speed`, `eta` and `total_bytes` are often `null`. Avoid `_percent_str` (padded, may contain ANSI codes). bestvideo+bestaudio runs 0→100% **twice**, then merges. Use `--print "after_move:DONE %(filepath)s"` for the final path. https://github.com/yt-dlp/yt-dlp#output-template
- `-J --flat-playlist` on a single video gives `_type: "video"` plus full `formats[]` (47 entries for a test URL). On a playlist it gives `_type: "playlist"` + `entries[]` of `{_type:"url", id, url, title, duration}` with **no formats**. Channel and mix URLs can nest playlists. Read JSON from stdout only; errors go to stderr.
- `-U` refuses on pacman installs. YouTube JS challenges are handled by yt-dlp-ejs + deno, so the child process must inherit `PATH`.
- Go side: use `exec.CommandContext` with `cmd.Cancel` and `cmd.WaitDelay`, or the child outlives the app. Chromium-family `--cookies-from-browser` decrypts through the system keyring. `--embed-thumbnail` has quirks with webm, so prefer mp4/m4a containers (see D25).
- Gio v0.10.3 and gio-x v0.10.3 source is unpacked under the session scratchpad (`…/scratchpad/g/gioui.org@v0.10.3`, `…/scratchpad/x/gioui.org/x@v0.10.3`). Workers can read real APIs there instead of guessing.

## Decisions
- **D1** Personal desktop app: paste a URL or playlist, pick a format or preset, and watch a download queue with progress. *Your answer to Q1.*

- **D2** Not written in Python. *Your answer to Q1.*

- **D3** Drive the system `yt-dlp` binary as a subprocess: `-J --flat-playlist` for metadata and formats, and `--newline --progress-template` for progress lines the app can parse. *This follows from D2: the `yt_dlp` module can only be called in-process from Python. Using the pacman binary also means it updates with the system.*

- **D4** Written in Go (go1.27.1 is installed). *Your preference: lightweight. Answers the language half of Q2.*

- **D5** UI built with **Gio** (`gioui.org`, immediate mode, native Wayland) using its `widget/material` theme as the base. Widgets Gio doesn't ship, such as a dropdown, come from `gioui.org/x` or are hand-built. *Your answer to Q5: leanest option; polish costs effort, which you accepted.*

- **D6** Format picking: a preset dropdown (Best video+audio, 1080p, 720p, Audio only m4a, Audio only mp3) plus an "Advanced…" option that lists that video's real formats from `-J`. *Your answer to Q3. Presets cover most downloads, and the list is there when one doesn't fit; mp3 uses the installed ffmpeg.*

- **D7** Queue: one download at a time by default, with a settings field for parallel downloads (1–4). The download folder defaults to `~/Downloads` and can be changed. The queue is not saved across restarts. *Your answer to Q4. Persisting the queue is deferred until it's missed.*

- **D8** Dark and light mode via a toggle in the header, defaulting to dark. The choice is saved, and there is no system detection: Linux has no API for it, and the portal isn't served on wlroots. Palettes: dark bg `#121212` with accent **indigo `#818cf8`** (6.3:1, near-black text on the accent); light bg `#fafafa` with accent **`#4f46e5`** (6.0:1, white text on the accent). Both pass WCAG AA. Rounded `Surface` cards, a subtle shadow, Material icons, and an embedded Inter font for a cleaner look than the Go fonts. No gradients, no animation beyond the progress bars. *Your note 1, with the accent left to me; indigo is calm and passes AA in both modes. Contrast ratios are from the research pass.*

- **D9** Testing runs in a headless sway session: `WLR_BACKENDS=headless sway`, screenshots with `grim`, input with `wtype` and `swaymsg`. *Your note 2. sway 1.12, grim, wtype and ydotool are all installed.*

- **D10** Download tests use exactly these URLs: `https://youtu.be/55XuQm3NZi8`, `https://www.youtube.com/watch?v=aIb43qCLguo`, `https://www.youtube.com/watch?v=Nw-5zhpU5lk`, `https://www.youtube.com/watch?v=u9RJbcbHkQo`, `https://www.youtube.com/watch?v=Lv3f5sEe1Hw`, `https://www.youtube.com/watch?v=KRJO1RJTloU`, `https://youtu.be/TQTypAaLLkI`. *Your note 3.*

- **D11** Pin `gioui.org` and `gioui.org/x` both to **v0.10.3**. The format dropdown is `ContextArea` + `Menu`; "Advanced…" is a `Discloser`. *Follows from the research: the two modules move in lockstep and there is no Select widget.*

- **D12** Settings (theme, download folder, parallel count, plus anything the open questions add) are saved as JSON at `os.UserConfigDir()/yt-dlp-gui/settings.json`. *Follows from D7 + D8: a folder or theme choice that resets on every start would defeat the setting.*

- **D13** Progress parsing: the JSON `--progress-template`, null-safe fields, and percent = `downloaded_bytes / (total_bytes ?? total_bytes_estimate)`, falling back to fragments. Video+audio jobs show "video → audio → merging" stages so the bar doesn't appear to reset. The final path comes from `--print after_move:`. The child inherits the full environment (deno on `PATH`). Never call `-U`. *Follows from D3 + the yt-dlp facts in Context.*

- **D14** Cancel kills the whole process group (`Setpgid`, SIGTERM, then SIGKILL after a delay) so ffmpeg children don't survive. It keeps the `.part` files so a retry resumes. Removing a row from the queue deletes its `.part` files. Quitting the app cancels everything. *Correctness, not a feature: otherwise orphaned processes keep downloading.*

- **D15** A pasted playlist expands into one queue row per entry, and each row gets its own process. Nested playlists (channel and mix URLs) go one level deep; anything deeper is rejected with a message. *Follows from D7 (per-item progress and cancel) + the `-J --flat-playlist` shape.*

- **D16** A failed row shows the last `ERROR:` line, with an expandable stderr tail. Three patterns get a hint: bot check / age gate → "set a cookies browser in Settings"; HTTP 403 → "yt-dlp may be outdated"; geo-block → "not available in your country". *Your answer to Q6.*

- **D17** Settings has a "Cookies from browser" dropdown (none / firefox / chromium / brave / chrome). It is passed as `--cookies-from-browser` to both the `-J` call and the download call. *Your answer to Q7.*

- **D18** Every download always gets `--embed-metadata --embed-thumbnail --embed-chapters`, with no UI. A "Subtitles" checkbox plus a language field (default `en`) adds `--write-subs --embed-subs --sub-langs <lang>`. Both are saved in settings. *Your answer to Q8.*

- **D19** Finished rows get "Open file" and "Show in folder" (via `xdg-open` on the file / its directory); failed and cancelled rows get "Retry", which reuses the row's URL and options. *Your answer to Q9.*

- **D20** A pasted playlist first shows a checkbox list of its entries (select all / none), and only the checked ones are queued. There is no download archive. *Your answer to Q10.*

- **D21** The filename is hard-coded with `-P <folder>`: `-o "%(title)s [%(id)s].%(ext)s"` for single videos, and `-o "%(playlist_title)s/%(playlist_index)03d - %(title)s.%(ext)s"` for playlist entries. *Your answer to Q11.*

- **D22** The footer shows `yt-dlp --version`. One GitHub request at startup (`api.github.com/repos/yt-dlp/yt-dlp/releases/latest`) compares versions; if yours is older, the footer text switches to a warning colour with "update available (<tag>)". There is no banner and no install command, and a failed request is silently ignored. *Your answer to Q12: version check yes, pacman banner no.*

- **D23** No section downloads (start/end time). *Your answer to Q13; out of scope.*

- **D24** "Advanced…" (the real format list) is offered for single videos only. Checked playlist entries all get the one preset chosen in the picker. *Follows from the facts: `--flat-playlist` entries carry no `formats[]`, so per-entry formats would mean one `-J` call per entry.*

- **D25** Preset → yt-dlp args. Best: `-f "bv*+ba/b" -S "res,ext:mp4:m4a"`. 1080p: same with `-S "res:1080,ext:mp4:m4a"`. 720p: `-S "res:720,ext:mp4:m4a"`. Audio m4a: `-f "ba[ext=m4a]/ba" -x --audio-format m4a`. Audio mp3: `-f ba -x --audio-format mp3 --audio-quality 0`. Advanced: `-f <id>`, plus `+ba` when the format is video-only. The worker verifies each against `yt-dlp --help`. *Follows from D6 + D18: mp4/m4a preference avoids the webm thumbnail quirk.*

- **D26** Automated download tests write to a temp directory, never `~/Downloads`, and use only the D10 URLs. None of the D10 URLs is a playlist, so the playlist picker is agent-tested against a fixture `-J` JSON and live-tested by you. *Follows from D10.*

- **D27** The download folder is an editable path field (validated: it must exist and be writable), not a native folder dialog. *Lean: Gio has no built-in native folder picker, and a path field covers it.*

## Done when
**Phase A — single-video core, both themes**
- [x] `go build ./...`, `go vet ./...` and `go test ./...` all pass — *verified by: agent* — ran `go build ./... && go vet ./... && go test -race -count=1 ./...; gofmt -l .` → all 5 packages ok, gofmt empty
- [x] Unit tests cover progress parsing (null `total_bytes`/`speed`/`eta`, the two-stage video+audio run), the D25 preset args, the D17/D18 flags and the three D16 error patterns — *verified by: agent* — ran `go test -race ./internal/ytdlp/...` → TestParseProgressLine, TestTrackerRecordedBestRun, TestArgsPresets/Advanced/Cookies/Subs/EmbedAlways, TestProbeURLNestedFake, TestHint all pass
- [x] Queue unit test with a fake runner: parallel=1 runs jobs strictly one after another, parallel=2 runs two at once, and Retry and Remove behave per D14 — *verified by: agent* — ran `go test -race ./internal/queue/...` → TestParallel1RunsOneAtATimeFIFO (max 1), TestParallel2RunsTwoAtOnce (max 2), Retry/Remove/Cancel tests pass
- [x] In headless sway, the app downloads `https://youtu.be/55XuQm3NZi8` with "Audio only m4a" into a temp dir; `ffprobe` shows an audio stream plus an attached cover picture — *verified by: agent* — ran `ffprobe -show_entries stream=codec_type,codec_name:stream_disposition=attached_pic` on the UI download → aac audio + png attached_pic=1; all 7 D10 URLs likewise Done
- [x] In headless sway, a "Best" download of a D10 URL completes through its stages, and `ffprobe` shows video and audio streams plus embedded metadata — *verified by: agent* — ran `ffprobe -show_streams -show_format` on Best of 55XuQm3NZi8 → av1 video + aac audio + png cover, title/artist/date tags; aIb43qCLguo showed Video→Audio→Merging→Processing→Done
- [x] Cancelling mid-download leaves no `yt-dlp`/`ffmpeg` process (`pgrep`), and the `.part` file remains — *verified by: agent* — ran `pgrep -a yt-dlp; pgrep -a ffmpeg` after row cancel of Lv3f5sEe1Hw → empty, `.f398.mp4.part` kept; window close mid-download → no procs at T+7s
- [x] The theme choice survives a restart (`settings.json` written and read back) — *verified by: agent* — ran `cat settings.json` after the toggle → `"dark": false`; relaunch with same XDG_CONFIG_HOME → started light
- [x] `grim` screenshots of the main window in dark and light mode, saved for review — *verified by: agent* — ran `scripts/headless.sh shot` → `scratchpad/shots/08-queue-all-done-dark.png`, `09-queue-light-tall.png`, `12-best-stages-montage-light.png`
- [x] It looks stylish but restrained in both themes, and the accent works — *verified by: user*
- [x] Downloading your own URLs through it feels right — *verified by: user*

**Phase B — playlists, formats, settings, row actions**
- [x] The playlist picker, fed a fixture `-J --flat-playlist` JSON, queues only the checked entries with the D21 playlist template — *verified by: agent* — ran `go test -race -run 'Playlist|WinProbeRouting' -v ./internal/ui/app/` → pass; fixture with entries 2,5 unchecked runs only 1,3,4 with `-o "Rock_Pop_ Best 100%%/00N - %(title)s.%(ext)s"`; picker footer fix verified by TestPlaylistFooterInsideCardAndClickable
- [x] "Advanced…" lists the real formats for a D10 URL, and picking a video-only format id downloads it with audio merged (`ffprobe`) — *verified by: agent* — ran `ffprobe -show_entries stream=codec_type,codec_name` on the Advanced 135 (480p video-only) download of KRJO1RJTloU → h264 video + opus audio (.mkv); list had 80 formats matching `yt-dlp -F`
- [x] The Subtitles checkbox on a D10 URL that has `en` subs produces a subtitle stream (`ffprobe`); if none of them has subs, the report says so — *verified by: agent* — ran `yt-dlp --list-subs --skip-download` on all 7 D10 URLs → none has manual subtitles (5 have only auto-captions); after adding `--write-auto-subs` (user-approved), ran `YTDLP_LIVE=1 go test -run TestLiveSubs ./internal/ytdlp/` → KRJO1RJTloU 720p has a `mov_text` eng subtitle stream, no stray `.vtt`
- [x] Arg building puts `--cookies-from-browser firefox` on both the `-J` and the download command when set — *verified by: agent* — ran `go test -race -run 'WinOptsFor|WinProbeCookies|ArgsCookies' -v ./internal/...` → pass; live `/proc/<pid>/cmdline` showed the flag on both the `-J` probe and the download
- [x] An invalid URL ends as a failed row showing its `ERROR:` line; Retry re-runs it — *verified by: agent* — ran `notaurl` through the UI → Failed row "[generic] 'notaurl' is not a valid URL", Details shows the `ERROR:` line; Retry → Starting → Failed again
- [x] Open file / Show in folder build the right `xdg-open` command (unit test) — *verified by: agent* — ran `go test -race -run RowCmds -v ./internal/ui/app/` → pass, argv `["xdg-open", path]` / `["xdg-open", dir]` for a unicode path
- [x] The footer shows the yt-dlp version; with a mocked newer release tag it shows the warning — *verified by: agent* — ran `go test -race -run WinFooter -v ./internal/ui/app/` → pass, mocked tag 2099.01.01 gives "update available (2099.01.01)" with warn; live footer shows `yt-dlp 2026.08.19`
- [x] Settings panel (folder, parallel, cookies, subtitles) values persist across a restart — *verified by: agent* — ran `cat settings.json` after Save + relaunch → `{"folder":"/tmp/i12dl","parallel":2,"cookies":"firefox","subs":true,"sub_lang":"en",…}`, reopened panel shows the same values
- [ ] A real playlist URL of yours goes through the picker as expected — *verified by: user*
- [ ] Open file / Show in folder work on your desktop, and the settings panel and picker look right — *verified by: user*

## Implementation plan
*Built from D1–D27 · decisions:f3bd*

Module root is the project dir. Layout: `main.go`, `internal/ytdlp/` (all yt-dlp process logic), `internal/queue/` + `internal/settings/`, `internal/ui/theme/` (palettes, font, dropdown, card), `internal/ui/app/` (windows and panels), `scripts/` (headless sway harness). Not a git repo, so parallel workers are kept apart by `Owns`.

| Wave | ID | Task | Agent | Owns | After |
|:----:|:--:|------|-------|------|-------|
| — | **A** | **Single-video core, both themes** | | | |
| 1 | I1 | Scaffold: `go.mod` with gioui.org + gioui.org/x pinned to v0.10.3 and x/exp/shiny, a hello-window `main.go`, and `scripts/headless.sh` (headless sway, launch the app, `grim` screenshot, `wtype` input). This proves Gio renders under headless sway, and it is the D9 risk spike — **D2**, **D4**, **D5**, **D9**, **D11** | `tl-sonnet-high` | `go.mod`, `go.sum`, `main.go`, `scripts/` | — |
| 2 | I2 | `internal/ytdlp`: `-J --flat-playlist` probe with video/playlist parsing; download with JSON progress, stages and the final path; preset/advanced args; embed, subtitle and cookie flags; fixed filename templates; process-group cancel; error classification + hints; `--version` and the GitHub latest-tag check. Unit tests, plus a live test gated by env var — **D3**, **D13**, **D14**, **D16**, **D17**, **D18**, **D21**, **D22**, **D25** | `tl-sonnet-high` | `internal/ytdlp/` | I1 |
| 2 | I3 | `internal/ui/theme`: dark/light palettes with the indigo accent, embedded Inter font, icons, a card/Surface helper, and the dropdown component (ContextArea + Menu) — **D8**, **D11** | `tl-sonnet-high` | `internal/ui/theme/` | I1 |
| 3 | I4 | `internal/queue` + `internal/settings`: job states, parallel limit 1–4, retry/remove/cancel-all, playlist entries expanded into rows; settings JSON in `os.UserConfigDir()` holding every field (theme, folder, parallel, cookies, subtitles + lang). Unit tests with a fake runner — **D7**, **D12**, **D14**, **D15** | `tl-sonnet-high` | `internal/queue/`, `internal/settings/` | I2 |
| 4 | I5 | `internal/ui/app` main window: header with the theme toggle, URL field + Add, preset dropdown, queue rows (title, progress bar, stage, speed/ETA, cancel) and a footer with the yt-dlp version; wired into `main.go` — **D1**, **D6**, **D8** | `tl-sonnet-high` | `internal/ui/app/`, `main.go` | I3, I4 |
| 5 | I6 | QC phase A: review against D1–D27, run the build/vet/test checks, then one headless sway smoke run with a D10 URL into a temp dir — **D10**, **D26** | `tl-sonnet-high` | *(read-only)* | I5 |
| — | **B** | **Playlists, formats, settings, row actions** | | | |
| 6 | I7 | Settings panel: folder path field with validation, parallel 1–4, cookies browser, subtitles + lang — **D17**, **D18**, **D27** | `tl-sonnet-high` | `internal/ui/app/settingspanel.go` | I6 |
| 6 | I8 | Playlist picker: entry checkbox list, select all/none, one preset for all — **D15**, **D20**, **D24** | `tl-sonnet-high` | `internal/ui/app/playlist.go` | I6 |
| 6 | I9 | "Advanced…" Discloser with the real format list for single videos, and no section fields — **D6**, **D23**, **D24** | `tl-sonnet-high` | `internal/ui/app/formats.go` | I6 |
| 6 | I10 | Row actions: the error line + hint + expandable stderr, Retry, Open file / Show in folder via `xdg-open` — **D16**, **D19** | `tl-sonnet-high` | `internal/ui/app/row.go` | I6 |
| 7 | I11 | Wire I7–I10 into the main window, and add the footer update warning from the GitHub check — **D22** | `tl-sonnet-high` | `internal/ui/app/window.go`, `main.go` | I7, I8, I9, I10 |
| 8 | I12 | QC phase B: review + build/vet/test checks + headless smoke run — **D10**, **D26** | `tl-sonnet-high` | *(read-only)* | I11 |

## Open questions

### Answered
- ~~1. What do you want out of this GUI?~~ → personal queue app, not Python → **D1**, **D2**
- ~~2. Which language and toolkit?~~ → Go; the toolkit choice is now Q5 → **D4**
- ~~3. What does "pick a format or preset" mean?~~ → presets + Advanced format list → **D6**
- ~~4. Queue behaviour?~~ → sequential by default, 1–4 parallel, ~/Downloads, no persistence → **D7**
- ~~5. Which Go UI toolkit?~~ → Gio → **D5**
- ~~6. Show failures with a fix hint?~~ → yes → **D16**
- ~~7. Cookies from browser?~~ → yes → **D17**
- ~~8. Embedding and subtitles?~~ → always embed + subtitles checkbox → **D18**
- ~~9. Open / Show in folder / Retry?~~ → yes → **D19**
- ~~10. Playlist item picker?~~ → yes, no archive → **D20**
- ~~11. Filename template?~~ → hard-coded → **D21**
- ~~12. Outdated warning?~~ → footer version + check, no pacman banner → **D22**
- ~~13. Section downloads?~~ → no → **D23**

## Research request

## Brainstorm request

## Notes from me

