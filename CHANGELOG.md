# Changelog

All notable user-facing changes, newest first. Categories:

**Added** (new features),
**Fixed** (bug and behaviour fixes),
**Removed** (things taken out),
**Info** (notes worth knowing).

## 2026-10-26

### Added
- **Paste from clipboard**: Ctrl+V pastes a URL into the input, reading the clipboard via
  `wl-paste`, `xclip` or `xsel`, whichever is installed.
- **Header icon**: the app icon now shows in the window header.

## 2026-10-24

### Added
- **ritz-get install**: the app can be installed with ritz-get or `./install.sh`, which
  builds it and links it into `~/.local/bin`, and ships a desktop entry for shortcuts.

### Removed
- **Built-in install/uninstall commands**: the `install` and `uninstall` subcommands are
  gone; use ritz-get or `./install.sh` instead.

## 2026-10-13

### Added
- **Cancel all**: the queue bar has a Cancel all button, and a Clear cancelled button that
  also deletes the leftovers of cancelled downloads. Both are greyed out when they would
  do nothing, and no longer show a click ripple while greyed out.
- **Install commands**: `install` and `uninstall` subcommands put the binary in
  `~/.local/bin` together with a desktop entry.
- **README**: the project now has a README with screenshots and a full feature rundown.

### Fixed
- **Clear completed button**: the queue bar now reserves room for it so it is never
  clipped.

### Info
- **Name**: the app is now called yt-dlp-gui in the window.

## 2026-10-12

### Added
- **First release**: a lean desktop GUI for yt-dlp. Paste a video or playlist URL, pick a
  format preset (Best, 1080p, 720p, Audio only m4a or mp3) and watch the download queue.
- **Advanced formats**: an Advanced list shows the video's real formats; a video-only
  pick gets the best audio merged in.
- **Playlists**: a checkbox picker with select all/none queues only the entries you tick.
- **Queue**: 1-4 parallel downloads with live progress, speed and ETA; rows offer Retry,
  Open file, Show in folder and expandable error details.
- **Settings**: a settings modal for theme (light or dark), parallel downloads, cookies
  from browser and subtitles.
- **Cookies from browser**: Firefox, Chromium, Brave, Chrome, Zen and Floorp, for both
  system and Flatpak installs.
- **Subtitles**: also fetch YouTube auto-captions when no manual subtitles exist.
- **Remembered preset**: the chosen format preset is kept across launches.
- **Clear completed**: removes finished rows from the queue and keeps the files.

### Fixed
- **Playlist picker footer**: it no longer overflows its card or becomes unclickable.
- **Leftover files**: removing a download also deletes its partial, thumbnail and
  subtitle files.
- **Retry**: a retried download now uses the current cookie and subtitle settings.
