#!/usr/bin/env bash
# Headless sway harness for GUI tests. Never touches the user's real Wayland session.
#   headless.sh start            start headless sway (1100x750), write state file
#   headless.sh run <cmd...>     run a command inside the session (background, pid recorded)
#   headless.sh shot <out.png>   grim screenshot of the headless output
#   headless.sh type <text>      wtype text into the focused window
#   headless.sh key <key...>     wtype key presses, e.g. `key ctrl+l` `key Return` (modifier+key)
#   headless.sh click X Y        left-click at output pixel X Y (1100x750); `click X Y move` only moves the pointer
#   headless.sh msg <swaymsg..>  swaymsg against the headless instance (e.g. msg -t get_tree)
#   headless.sh env              print `export` lines for the session (eval "$(headless.sh env)")
#   headless.sh stop             kill apps + sway, remove state (idempotent)
# State file: ${TMPDIR:-/tmp}/ytdlp-gui-headless.env. App log (kept after stop, reset on start): ${state}.app.log
# Renderer: Gio picks EGL (Mesa llvmpipe/swrast) here; LIBGL_ALWAYS_SOFTWARE=1 forces it.
set -eu
ST="${TMPDIR:-/tmp}/ytdlp-gui-headless.env"
RT="${TMPDIR:-/tmp}/ytdlp-gui-headless-rt"
load() { [ -f "$ST" ] || { echo "no headless session; run start" >&2; exit 1; }; . "$ST"
  export XDG_RUNTIME_DIR="$RT" WAYLAND_DISPLAY SWAYSOCK LIBGL_ALWAYS_SOFTWARE=1; }

case "${1:-}" in
start)
  "$0" stop
  rm -rf "$RT" "$ST.app.log"; mkdir -p "$RT"; chmod 700 "$RT"
  cat > "$RT/sway.conf" <<CONF
output HEADLESS-1 resolution 1100x750
default_border none
CONF
  # env -u strips the real session's vars so nothing leaks into / out of this instance
  env -u WAYLAND_DISPLAY -u SWAYSOCK -u DISPLAY -u I3SOCK \
    XDG_RUNTIME_DIR="$RT" WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman \
    sway -c "$RT/sway.conf" >"$RT/sway.log" 2>&1 &
  pid=$!
  for _ in $(seq 100); do
    s=$(ls "$RT"/wayland-* 2>/dev/null | grep -v '\.lock$' | head -1 || true)
    k=$(ls "$RT"/sway-ipc.* 2>/dev/null | head -1 || true)
    [ -n "$s" ] && [ -n "$k" ] && break
    kill -0 $pid 2>/dev/null || { cat "$RT/sway.log" >&2; exit 1; }
    sleep 0.1
  done
  [ -n "${s:-}" ] && [ -n "${k:-}" ] || { echo "sway socket timeout" >&2; kill $pid; exit 1; }
  printf 'WAYLAND_DISPLAY=%s\nSWAYSOCK=%s\nSWAY_PID=%s\n' "$(basename "$s")" "$k" "$pid" > "$ST"
  echo "headless sway up: $(cat "$ST" | tr '\n' ' ')" ;;
run)
  shift; load
  setsid "$@" >>"$ST.app.log" 2>&1 &
  echo "$!" >> "$ST.apps"; echo "pid $!" ;;
shot)
  load; grim "${2:?usage: shot out.png}" ;;
type)
  # the first key of every wtype run is swallowed by the compositor; a throwaway Shift_L takes the hit
  shift; load; wtype -k Shift_L "$@" ;;
key)
  # "ctrl+l" -> -M ctrl -k l -m ctrl ; plain "Return" -> -k Return
  shift; load
  for k in "$@"; do
    a=(-k Shift_L); mods=(); key="${k##*+}"
    [ "$key" != "$k" ] && for m in $(echo "${k%+*}" | tr + ' '); do mods+=("$m"); done
    for m in "${mods[@]}"; do a+=(-M "$m"); done
    a+=(-k "$key")
    for m in "${mods[@]}"; do a+=(-m "$m"); done
    wtype "${a[@]}"
  done ;;
click)
  # headless sway has no input devices; scripts/vptr/vptr.c drives a wlr-virtual-pointer. Built on demand.
  shift; load
  D="$(dirname "$0")/vptr"; B="${TMPDIR:-/tmp}/ytdlp-gui-vptr"
  if [ ! -x "$B/vptr" ] || [ "$D/vptr.c" -nt "$B/vptr" ]; then
    mkdir -p "$B"
    X="$D/wlr-virtual-pointer-unstable-v1.xml"
    wayland-scanner client-header "$X" "$B/vp.h"; wayland-scanner private-code "$X" "$B/vp.c"
    cc -o "$B/vptr" -I"$B" "$D/vptr.c" "$B/vp.c" -lwayland-client
  fi
  [ $# -ge 2 ] || { echo "usage: click X Y [move]" >&2; exit 2; }
  "$B/vptr" "$1" "$2" $([ "${3:-}" = move ] || echo click) ;;
msg)
  shift; load; swaymsg "$@" ;;
env)
  load; printf 'export XDG_RUNTIME_DIR=%q WAYLAND_DISPLAY=%q SWAYSOCK=%q LIBGL_ALWAYS_SOFTWARE=1\n' "$RT" "$WAYLAND_DISPLAY" "$SWAYSOCK" ;;
stop)
  if [ -f "$ST.apps" ]; then
    while read -r p; do kill -- -"$p" 2>/dev/null || kill "$p" 2>/dev/null || true; done < "$ST.apps"
  fi
  if [ -f "$ST" ]; then
    . "$ST"; kill "$SWAY_PID" 2>/dev/null || true
    for _ in $(seq 30); do kill -0 "$SWAY_PID" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$SWAY_PID" 2>/dev/null || true
  fi
  rm -rf "$ST" "$ST.apps" "$RT" ;;
*) sed -n '2,13p' "$0"; exit 2 ;;
esac
