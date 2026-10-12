package main

// install.go implements `yt-dlp-gui install` and `yt-dlp-gui uninstall`. It
// mirrors ccorral's installer, minus the systemd service: install asks first
// and writes nothing until the user has confirmed a summary of exactly what
// will change; only then does it copy the binary and write a desktop entry.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	installName    = "yt-dlp-gui"
	installDesktop = "yt-dlp-gui.desktop"
)

// installDesktopTemplate is the launcher entry; %s is the Exec path.
// StartupWMClass matches the Wayland app_id set in main.go; Icon is a stock
// freedesktop theme name, so no icon file is shipped.
const installDesktopTemplate = `[Desktop Entry]
Type=Application
Name=yt-dlp-gui
Comment=Desktop front end for yt-dlp
Exec=%s
Icon=folder-download
Terminal=false
Categories=Network;AudioVideo;
StartupWMClass=yt-dlp-gui
`

// installRefresher runs update-desktop-database on dir. Tests substitute a
// fake so the real tool is never invoked.
type installRefresher func(dir string) error

// installRefresh is best-effort: a missing binary is not an error.
func installRefresh(dir string) error {
	path, err := exec.LookPath("update-desktop-database")
	if err != nil {
		return nil
	}
	return exec.Command(path, dir).Run()
}

// installEnv is everything the install/uninstall core touches, so tests can
// point all of it into temp dirs.
type installEnv struct {
	home    string // user home; ~/.local/bin lives under it
	dataDir string // user data dir ($XDG_DATA_HOME); applications/ lives under it
	self    string // path of the binary to copy (install only)
	refresh installRefresher
}

func (e installEnv) binDir() string      { return filepath.Join(e.home, ".local", "bin") }
func (e installEnv) binPath() string     { return filepath.Join(e.binDir(), installName) }
func (e installEnv) desktopDir() string  { return filepath.Join(e.dataDir, "applications") }
func (e installEnv) desktopPath() string { return filepath.Join(e.desktopDir(), installDesktop) }

// desktopEntry renders the .desktop file for this environment.
func (e installEnv) desktopEntry() string {
	return fmt.Sprintf(installDesktopTemplate, installExecArg(e.binPath()))
}

// installDefaultEnv builds the production environment.
func installDefaultEnv(needSelf bool, out io.Writer) (installEnv, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(out, "cannot determine home directory: %v\n", err)
		return installEnv{}, false
	}
	env := installEnv{home: home, dataDir: os.Getenv("XDG_DATA_HOME"), refresh: installRefresh}
	if env.dataDir == "" {
		env.dataDir = filepath.Join(home, ".local", "share")
	}
	if needSelf {
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintf(out, "cannot determine own executable: %v\n", err)
			return installEnv{}, false
		}
		if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
			self = resolved
		}
		env.self = self
	}
	return env, true
}

// installMain is `yt-dlp-gui install`. It returns the process exit code.
func installMain() int {
	env, ok := installDefaultEnv(true, os.Stdout)
	if !ok {
		return 1
	}
	return installRun(env, os.Stdin, os.Stdout)
}

// uninstallMain is `yt-dlp-gui uninstall`. It returns the process exit code.
func uninstallMain() int {
	env, ok := installDefaultEnv(false, os.Stdout)
	if !ok {
		return 1
	}
	return uninstallRun(env, os.Stdin, os.Stdout)
}

func installRun(env installEnv, in io.Reader, out io.Writer) int {
	r := bufio.NewReader(in)

	fmt.Fprintf(out, "Install yt-dlp-gui to %s.\n\n", env.binPath())

	// --- summary + confirm (nothing is written before the "yes") ---
	sameBin := false
	if ti, terr := os.Stat(env.binPath()); terr == nil {
		if si, serr := os.Stat(env.self); serr == nil && os.SameFile(ti, si) {
			sameBin = true
		}
	}
	if sameBin {
		fmt.Fprintf(out, "Note: you are running the installed copy (%s), so there is no new binary to install —\n"+
			"this only rewrites the desktop entry. To install a new build, run it from the build:\n"+
			"  ./yt-dlp-gui install\n\n", env.binPath())
	}
	fmt.Fprintln(out, "This will:")
	if sameBin {
		fmt.Fprintf(out, "  - reinstall from the installed copy (same version): %s is left as is\n", env.binPath())
	} else {
		fmt.Fprintf(out, "  - copy %s to %s\n", env.self, env.binPath())
	}
	fmt.Fprintf(out, "  - write %s\n", env.desktopPath())
	fmt.Fprintf(out, "  - run: update-desktop-database %s (if installed)\n", env.desktopDir())
	fmt.Fprintf(out, "It leaves your settings (%s) alone.\n\n", installSettingsPath())
	if yes, ok := installAsk(r, out, "Proceed? [y/N] "); !ok || !yes {
		fmt.Fprintln(out, "Aborted; nothing was written.")
		return 1
	}
	fmt.Fprintln(out)

	// --- act ---
	fail := func(what string, err error) int {
		fmt.Fprintf(out, "%s: %v\n", what, err)
		return 1
	}
	if sameBin {
		fmt.Fprintf(out, "Kept %s (same file as the running binary)\n", env.binPath())
	} else {
		if err := installCopy(env.self, env.binPath()); err != nil {
			return fail("install failed", err)
		}
		fmt.Fprintf(out, "Wrote %s\n", env.binPath())
	}
	if err := installWriteFile(env.desktopPath(), []byte(env.desktopEntry())); err != nil {
		return fail("writing desktop entry failed", err)
	}
	fmt.Fprintf(out, "Wrote %s\n", env.desktopPath())
	if err := env.refresh(env.desktopDir()); err != nil {
		fmt.Fprintf(out, "update-desktop-database failed (ignored): %v\n", err)
	}

	if !installOnPath(env.binDir()) {
		fmt.Fprintf(out, "\nNote: %s is not on your $PATH. Add it, e.g.:\n  export PATH=\"$HOME/.local/bin:$PATH\"\n", env.binDir())
	}
	fmt.Fprintln(out, "\nyt-dlp-gui is installed; start it from your launcher or run: yt-dlp-gui")
	return 0
}

func uninstallRun(env installEnv, in io.Reader, out io.Writer) int {
	r := bufio.NewReader(in)
	fmt.Fprintln(out, "Uninstall yt-dlp-gui. This will:")
	fmt.Fprintf(out, "  - remove %s\n", env.desktopPath())
	fmt.Fprintf(out, "  - remove %s\n", env.binPath())
	fmt.Fprintf(out, "It leaves your settings (%s) in place.\n\n", installSettingsPath())
	if yes, ok := installAsk(r, out, "Proceed? [y/N] "); !ok || !yes {
		fmt.Fprintln(out, "Aborted; nothing was changed.")
		return 1
	}
	fmt.Fprintln(out)

	code := 0
	warn := func(what string, err error) {
		fmt.Fprintf(out, "%s: %v\n", what, err)
		code = 1
	}
	if err := os.Remove(env.desktopPath()); err != nil && !os.IsNotExist(err) {
		warn("removing desktop entry failed", err)
	} else {
		fmt.Fprintf(out, "Removed %s\n", env.desktopPath())
	}
	if err := env.refresh(env.desktopDir()); err != nil {
		fmt.Fprintf(out, "update-desktop-database failed (ignored): %v\n", err)
	}
	if err := os.Remove(env.binPath()); err != nil && !os.IsNotExist(err) {
		warn("removing binary failed", err)
	} else {
		fmt.Fprintf(out, "Removed %s\n", env.binPath())
	}
	fmt.Fprintln(out, "\nLeft in place: your yt-dlp-gui settings.")
	return code
}

// installAsk prints prompt and reads one answer. yes is true only for y/yes
// (any case); anything else, including an empty line, is a no. ok is false on
// EOF with nothing typed.
func installAsk(r *bufio.Reader, out io.Writer, prompt string) (yes, ok bool) {
	fmt.Fprint(out, prompt)
	line, err := r.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if err != nil && ans == "" {
		fmt.Fprintln(out)
		return false, false
	}
	return ans == "y" || ans == "yes", true
}

// installSettingsPath is the settings file the app uses (display only; install
// and uninstall never touch it).
func installSettingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "~/.config"
	}
	return filepath.Join(dir, installName, "settings.json")
}

// installExecArg quotes p for a desktop-entry Exec line when it contains
// characters the spec reserves (a home directory with a space, say).
func installExecArg(p string) string {
	if !strings.ContainsAny(p, " \t\n\"'\\><~|&;$*?#()`") {
		return p
	}
	return `"` + strings.NewReplacer(`\`, `\\\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(p) + `"`
}

// installCopy copies src to target atomically with mode 0755.
func installCopy(src, target string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return installAtomic(target, in, 0o755)
}

// installWriteFile writes data to path atomically with mode 0644.
func installWriteFile(path string, data []byte) error {
	return installAtomic(path, bytes.NewReader(data), 0o644)
}

// installAtomic writes r to a temp file beside target and renames it into place.
func installAtomic(target string, r io.Reader, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".yt-dlp-gui-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// installOnPath reports whether dir is listed in $PATH.
func installOnPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		if p == dir {
			return true
		}
		if abs, err := filepath.Abs(p); err == nil && abs == dir {
			return true
		}
	}
	return false
}
