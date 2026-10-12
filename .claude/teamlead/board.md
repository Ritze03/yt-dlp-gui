# Board

<!-- GENERATED from .state/board.json — do not hand-edit; use the board tools -->

| ✓ | ID | Task | Agent | Owns | State | Branch |
|:-:|:--:|------|-------|------|-------|--------|
|   | 1 | Scaffold: go.mod (gio + gio-x v0.10.3, x/exp/shiny), hello-window main.go, scripts/headless.sh; prove Gio renders under headless sway — I1 | `tl-sonnet-high` | `go.mod`, `go.sum`, `main.go`, `scripts` | returned | — |
|   | 2 | internal/ytdlp: probe, download+JSON progress+stages, preset/flag args, pgid cancel, error hints, version + GitHub check; unit tests — I2 | `tl-sonnet-high` | `internal/ytdlp` | blocked-by 1 | — |
|   | 3 | internal/ui/theme: dark/light palettes (indigo accent), Inter font, icons, card helper, dropdown component — I3 | `tl-sonnet-high` | `internal/ui/theme` | blocked-by 1 | — |
|   | 4 | internal/queue + internal/settings: job states, parallel 1-4, retry/remove/cancel-all, playlist expansion; settings JSON; fake-runner tests — I4 | `tl-sonnet-high` | `internal/queue`, `internal/settings` | blocked-by 2 | — |
|   | 5 | internal/ui/app main window: header theme toggle, URL+Add, preset dropdown, queue rows w/ progress/stage/cancel, footer version; wire main.go — I5 | `tl-sonnet-high` | `internal/ui/app`, `main.go` | blocked-by 3,4 | — |
|   | 6 | QC phase A: review vs D1-D27, build/vet/test, headless sway smoke run with a D10 URL into temp dir — I6 | `tl-sonnet-high` | *(read-only)* | blocked-by 5 | — |
