#!/bin/sh
# ritz-get-spec: 1
# install.sh for a program managed by ritz-get.  Spec: https://github.com/Ritze03/ritz-get/blob/main/SPEC.md
# ritz-get runs it from the clone root, stdin /dev/null:
#     ./install.sh --install --yes     first install AND every update/repair (idempotent, safe while running)
#     ./install.sh --remove  --yes     never touches config or user data
# By hand:  ./install.sh --install | --remove
# Copy this file to the repo root and edit ONLY the per-program section. If this file and SPEC.md disagree, SPEC.md wins.
set -eu

# Available everywhere below: $dir (the clone, absolute, from pwd -P), $bin (~/.local/bin), $cfg (XDG config), $data (XDG data),
# $first (non-empty on a first install), $yes (1 under --yes).
dir=$(cd "$(dirname "$0")" && pwd -P)
cd "$dir"
bin=$HOME/.local/bin
cfg=${XDG_CONFIG_HOME:-$HOME/.config}
data=${XDG_DATA_HOME:-$HOME/.local/share}

# ---- per-program section (edit) --------------------------------------------------------
NAME=yt-dlp-gui                 # the manifest NAME; the command will be ~/.local/bin/NAME
TARGET=build/$NAME              # the executable inside the clone that ~/.local/bin/NAME points to
ROOT_LINK=no                    # yes: also link /usr/local/bin/NAME (needs root once, first install only; SPEC rules R32-R36)

# A C header that cgo needs but no pkg-config file reliably announces (vulkan.pc ships with the
# loader on some distros and the headers with another package): preprocess an #include instead.
need_header() {                                                  # need_header NAME HEADER pacman:PKG dnf:PKG apt:PKG
	n=$1; h=$2; shift 2
	command -v gcc >/dev/null 2>&1 || return 0                   # a missing gcc is already reported
	printf '#include <%s>\n' "$h" | gcc -E -x c - >/dev/null 2>&1 || miss "$n" "$*"
}

# Gio needs cgo: gcc, pkg-config and the Wayland/X11/EGL/Vulkan dev files (https://gioui.org/doc/install/linux).
check_tools() {
	need go pacman:go dnf:golang apt:golang-go
	need gcc pacman:base-devel dnf:gcc apt:gcc
	need_pc wayland-client pacman:wayland dnf:wayland-devel apt:libwayland-dev
	need_pc wayland-cursor pacman:wayland dnf:wayland-devel apt:libwayland-dev
	need_pc wayland-egl pacman:wayland dnf:wayland-devel apt:libwayland-dev
	need_pc x11 pacman:libx11 dnf:libX11-devel apt:libx11-dev
	need_pc x11-xcb pacman:libx11 dnf:libX11-devel apt:libx11-xcb-dev
	need_pc xkbcommon-x11 pacman:libxkbcommon-x11 dnf:libxkbcommon-x11-devel apt:libxkbcommon-x11-dev
	need_pc xcursor pacman:libxcursor dnf:libXcursor-devel apt:libxcursor-dev
	need_pc xfixes pacman:libxfixes dnf:libXfixes-devel apt:libxfixes-dev
	need_pc egl pacman:mesa dnf:mesa-libEGL-devel apt:libegl1-mesa-dev
	need_pc glesv2 pacman:mesa dnf:mesa-libGLES-devel apt:libgles2-mesa-dev
	need_header vulkan vulkan/vulkan.h pacman:vulkan-headers dnf:vulkan-headers apt:libvulkan-dev
}

# Build into build/ under a temporary name, then mv it over the final name (atomic: a running
# program keeps its old file, and a failed build never reaches the mv, because of set -e).
build() {
	GOTOOLCHAIN=auto CGO_ENABLED=1 go build -o "build/.$NAME.new" .
	mv -f "build/.$NAME.new" "build/$NAME"
}

# Everything created outside the clone is made with link TARGET LINK (a symlink into the clone).
# The start-menu shortcut comes from yt-dlp-gui.desktop and is made by ritz-get, not here.
install_files() {
	link "$dir/$TARGET" "$bin/$NAME"
	command -v yt-dlp >/dev/null 2>&1 || echo "note: yt-dlp is not on PATH; $NAME needs it (and ffmpeg) at run time, install both with your package manager"
}

# Remove exactly what install_files created, with unlink_ours. Must work with the build tools gone.
remove_files() {
	unlink_ours "$bin/$NAME"
}
# ---- end per-program section -----------------------------------------------------------

# ---- ritz-get plumbing: do not edit below ----------------------------------------------
usage() { echo "usage: $0 --install | --remove [--yes]" >&2; exit 2; }
mode= yes=
for a; do
	case $a in
	--install|--remove) [ -z "$mode" ] || usage; mode=$a ;;
	--yes) yes=1 ;;
	*) usage ;;
	esac
done
[ -n "$mode" ] || usage
[ "$(id -u)" -ne 0 ] || { echo "$NAME: do not run install.sh as root" >&2; exit 1; }

first=; [ -L "$bin/$NAME" ] || first=1      # first install = no ~/.local/bin/NAME link yet (decided before anything changes)
stable=$HOME/.local/share/ritz-get/links/$NAME    # fixed path (not XDG): the system link points here, so it never needs root again
sys=/usr/local/bin/$NAME
need_root= missing= pcdone=
# $yes is 1 under --yes: if you ever ask a question, ask only when it is empty. With --yes never read stdin.

link() { mkdir -p "$(dirname "$2")"; ln -sfn "$1" "$2"; }       # link TARGET LINK  (absolute TARGET)
unlink_ours() {                                                  # delete LINK only if it points into this clone
	case $(readlink "$1" 2>/dev/null || true) in "$dir"/*) rm -f "$1"; echo "removed $1" ;; esac
}
miss() { echo "missing: $1 (try: $2)" >&2; missing=1; }
need() {
	t=$1; shift
	command -v "$t" >/dev/null 2>&1 || miss "$t" "$*"
}
need_pc() {
	if [ -z "$pcdone" ]; then pcdone=1; need pkg-config pacman:pkgconf dnf:pkgconf-pkg-config apt:pkg-config; fi
	command -v pkg-config >/dev/null 2>&1 || return 0
	l=$1; shift
	pkg-config --exists "$l" 2>/dev/null || miss "$l" "$*"
}

# The only root: one call, ln -sfn / rm -f, values as arguments (pkexec drops the environment).
run_root() {                                                     # run_root 'SCRIPT using "$1" "$2"' ARG...
	r=0; s=$1; shift
	$RITZ_GET_SUDO /bin/sh -c "$s" sh "$@" || r=$?
	case $r in
	0) ;;
	126) echo "cancelled: no root permission was given" >&2; exit 1 ;;
	127) echo "needs root: no password agent running; run ritz-get ${mode#--} $NAME in a terminal" >&2; exit 4 ;;
	*) echo "$NAME: the root step failed (exit $r)" >&2; exit 1 ;;
	esac
}
root_check() {                                                   # before the build: exit 1 / 4 now, nothing changed yet
	[ "$ROOT_LINK" = yes ] || return 0
	cur=$(readlink "$sys" 2>/dev/null || true)
	[ "$cur" = "$stable" ] && return 0
	if [ -e "$sys" ] || [ -L "$sys" ]; then
		# only a dead ritz-get link may be replaced; a user file, a distro file or a live link is never ours
		if [ -L "$sys" ] && [ ! -e "$sys" ]; then
			case $cur in */ritz-get/links/"$NAME") ;; *) cur= ;; esac
		else cur=; fi
		[ -n "$cur" ] || { echo "$NAME: $sys exists and is not ours; not replacing it" >&2; exit 1; }
	fi
	need_root=1
	[ -n "${RITZ_GET_SUDO:-}" ] || { echo "needs root: run ritz-get install $NAME" >&2; exit 4; }
}
root_link() {                                                    # after the build
	[ "$ROOT_LINK" = yes ] || return 0
	link "$dir/$TARGET" "$stable"                                # user level: follows the clone wherever it moves
	[ -n "$need_root" ] || return 0
	run_root 'ln -sfn "$1" "$2"' "$stable" "$sys"
}
root_unlink() {                                                  # first thing in --remove
	[ "$ROOT_LINK" = yes ] || return 0
	[ "$(readlink "$sys" 2>/dev/null || true)" = "$stable" ] || return 0
	[ -n "${RITZ_GET_SUDO:-}" ] || { echo "needs root: run ritz-get remove $NAME" >&2; exit 4; }
	run_root 'rm -f "$1"' "$sys"
}

case $mode in
--install)
	check_tools
	[ -z "$missing" ] || exit 3
	root_check
	mkdir -p build
	build
	install_files
	root_link
	echo "$NAME installed"
	case ":$PATH:" in *":$bin:"*) ;; *) echo "note: $bin is not in your PATH" ;; esac
	;;
--remove)
	root_unlink
	remove_files
	if [ "$ROOT_LINK" = yes ]; then unlink_ours "$stable"; fi
	echo "$NAME removed (config and user data left alone)"
	;;
esac
