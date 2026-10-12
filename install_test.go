package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// instFixture is a temp HOME + data dir + fake binary + fake
// update-desktop-database. HOME and XDG_DATA_HOME point into the temp dir
// too, so nothing can reach the real home.
type instFixture struct {
	env     installEnv
	refresh []string // dirs update-desktop-database was run on
}

func newInstFixture(t *testing.T) *instFixture {
	t.Helper()
	f := &instFixture{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	self := filepath.Join(t.TempDir(), "yt-dlp-gui-build")
	if err := os.WriteFile(self, []byte("BINARY-V2"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.env = installEnv{
		home:    home,
		dataDir: filepath.Join(home, ".local", "share"),
		self:    self,
		refresh: func(dir string) error {
			f.refresh = append(f.refresh, dir)
			return nil
		},
	}
	return f
}

func (f *instFixture) write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *instFixture) install(stdin string) (int, string) {
	var out strings.Builder
	code := installRun(f.env, strings.NewReader(stdin), &out)
	return code, out.String()
}

func (f *instFixture) uninstall(stdin string) (int, string) {
	var out strings.Builder
	code := uninstallRun(f.env, strings.NewReader(stdin), &out)
	return code, out.String()
}

// snapshot maps every file and dir under root to its content.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			m[p] = "<dir>"
			return nil
		}
		b, err := os.ReadFile(p)
		m[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestInstallFresh(t *testing.T) {
	f := newInstFixture(t)
	t.Setenv("PATH", f.env.binDir())
	code, out := f.install("y\n")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if got := readFile(t, f.env.binPath()); got != "BINARY-V2" {
		t.Errorf("binary content %q", got)
	}
	fi, err := os.Stat(f.env.binPath())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("binary mode %v, want 0755", fi.Mode().Perm())
	}
	want := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=yt-dlp-gui\n" +
		"Comment=Desktop front end for yt-dlp\n" +
		"Exec=" + f.env.binPath() + "\n" +
		"Icon=folder-download\n" +
		"Terminal=false\n" +
		"Categories=Network;AudioVideo;\n" +
		"StartupWMClass=yt-dlp-gui\n"
	if got := readFile(t, f.env.desktopPath()); got != want {
		t.Errorf("desktop entry:\n%s\nwant:\n%s", got, want)
	}
	if fi, err := os.Stat(f.env.desktopPath()); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("desktop entry mode %v (%v), want 0644", fi.Mode().Perm(), err)
	}
	if wantPath := filepath.Join(f.env.home, ".local", "share", "applications", "yt-dlp-gui.desktop"); f.env.desktopPath() != wantPath {
		t.Errorf("desktop path %s, want %s", f.env.desktopPath(), wantPath)
	}
	// no leftover temp files
	for _, dir := range []string{f.env.binDir(), f.env.desktopDir()} {
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Errorf("%s has %d entries, want 1", dir, len(entries))
		}
	}
	if !reflect.DeepEqual(f.refresh, []string{f.env.desktopDir()}) {
		t.Errorf("update-desktop-database calls %q", f.refresh)
	}
	if strings.Contains(out, "not on your $PATH") {
		t.Errorf("PATH warning although bin dir is on PATH:\n%s", out)
	}
}

func TestInstallPathWarning(t *testing.T) {
	f := newInstFixture(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	code, out := f.install("y\n")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if want := "Note: " + f.env.binDir() + " is not on your $PATH."; !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
}

func TestInstallHonoursDataDir(t *testing.T) {
	f := newInstFixture(t)
	f.env.dataDir = filepath.Join(f.env.home, "xdg-data")
	if code, out := f.install("y\n"); code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !exists(filepath.Join(f.env.home, "xdg-data", "applications", "yt-dlp-gui.desktop")) {
		t.Error("desktop entry not under the configured data dir")
	}
}

func TestInstallDefaultEnvXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	env, ok := installDefaultEnv(false, os.Stdout)
	if !ok || env.dataDir != filepath.Join(home, ".local", "share") {
		t.Errorf("default data dir %q", env.dataDir)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	if env, _ = installDefaultEnv(false, os.Stdout); env.dataDir != filepath.Join(home, "data") {
		t.Errorf("XDG_DATA_HOME ignored: %q", env.dataDir)
	}
}

// Nothing may be written before the final "yes".
func TestInstallAbortWritesNothing(t *testing.T) {
	for _, stdin := range []string{
		"",        // EOF at the confirm
		"\n",      // default answer
		"n\n",     // explicit no
		"maybe\n", // anything but y/yes
		"yess\n",  // near miss
	} {
		f := newInstFixture(t)
		before := snapshot(t, f.env.home)
		code, out := f.install(stdin)
		if code != 1 {
			t.Errorf("stdin %q: code %d, want 1\n%s", stdin, code, out)
		}
		if after := snapshot(t, f.env.home); !reflect.DeepEqual(before, after) {
			t.Errorf("stdin %q: filesystem changed: %v", stdin, after)
		}
		if len(f.refresh) != 0 {
			t.Errorf("stdin %q: update-desktop-database ran", stdin)
		}
		if !strings.Contains(out, "nothing was written") {
			t.Errorf("stdin %q: no abort message:\n%s", stdin, out)
		}
	}
}

func TestInstallSummaryListsActions(t *testing.T) {
	f := newInstFixture(t)
	_, out := f.install("n\n")
	for _, want := range []string{
		"copy " + f.env.self + " to " + f.env.binPath(),
		"write " + f.env.desktopPath(),
		"update-desktop-database " + f.env.desktopDir(),
		"settings.json",
		"Proceed? [y/N]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

func TestInstallReinstallOverwrites(t *testing.T) {
	f := newInstFixture(t)
	f.write(t, f.env.binPath(), "BINARY-V1")
	f.write(t, f.env.desktopPath(), "stale")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(f.env.binPath(), old, old)
	code, out := f.install("y\n")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if strings.Contains(out, "Note: you are running the installed copy") {
		t.Errorf("unexpected Note:\n%s", out)
	}
	if got := readFile(t, f.env.binPath()); got != "BINARY-V2" {
		t.Errorf("binary %q, want BINARY-V2", got)
	}
	if got := readFile(t, f.env.desktopPath()); got != f.env.desktopEntry() {
		t.Errorf("desktop entry not rewritten:\n%s", got)
	}
}

// Running as the installed copy: loud Note, no copy, but the desktop entry
// is rewritten.
func TestInstallSameBinaryWarnsAndSkipsCopy(t *testing.T) {
	f := newInstFixture(t)
	f.write(t, f.env.binPath(), "BINARY-V1")
	f.write(t, f.env.desktopPath(), "stale")
	f.env.self = f.env.binPath()
	code, out := f.install("y\n")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{
		"Note: you are running the installed copy (" + f.env.binPath() + "), so there is no new binary to install —\n" +
			"this only rewrites the desktop entry. To install a new build, run it from the build:\n" +
			"  ./yt-dlp-gui install\n",
		"reinstall from the installed copy (same version)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "copy "+f.env.self+" to") {
		t.Errorf("same-binary run must not claim to copy:\n%s", out)
	}
	if got := readFile(t, f.env.binPath()); got != "BINARY-V1" {
		t.Errorf("binary %q changed", got)
	}
	if got := readFile(t, f.env.desktopPath()); got != f.env.desktopEntry() {
		t.Errorf("desktop entry not rewritten:\n%s", got)
	}
}

func TestUninstall(t *testing.T) {
	f := newInstFixture(t)
	if code, out := f.install("y\n"); code != 0 {
		t.Fatalf("install: %d\n%s", code, out)
	}
	settings := filepath.Join(f.env.home, ".config", "yt-dlp-gui", "settings.json")
	f.write(t, settings, `{"x":1}`)
	f.refresh = nil

	code, out := f.uninstall("y\n")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if exists(f.env.binPath()) {
		t.Error("binary not removed")
	}
	if exists(f.env.desktopPath()) {
		t.Error("desktop entry not removed")
	}
	if got := readFile(t, settings); got != `{"x":1}` {
		t.Errorf("settings changed: %q", got)
	}
	if !strings.Contains(out, "Left in place: your yt-dlp-gui settings.") {
		t.Errorf("output does not say settings are kept:\n%s", out)
	}
	if !reflect.DeepEqual(f.refresh, []string{f.env.desktopDir()}) {
		t.Errorf("update-desktop-database calls %q", f.refresh)
	}
}

func TestUninstallAbort(t *testing.T) {
	for _, stdin := range []string{"", "\n", "n\n"} {
		f := newInstFixture(t)
		f.write(t, f.env.binPath(), "BIN")
		f.write(t, f.env.desktopPath(), "entry")
		before := snapshot(t, f.env.home)
		if code, _ := f.uninstall(stdin); code != 1 {
			t.Errorf("stdin %q: code %d, want 1", stdin, code)
		}
		if after := snapshot(t, f.env.home); !reflect.DeepEqual(before, after) {
			t.Errorf("stdin %q: filesystem changed", stdin)
		}
	}
}

// Uninstalling when nothing is installed must not blow up on missing files.
func TestUninstallNothingInstalled(t *testing.T) {
	f := newInstFixture(t)
	if code, out := f.uninstall("y\n"); code != 0 {
		t.Errorf("code %d\n%s", code, out)
	}
}

func TestInstallExecArg(t *testing.T) {
	for in, want := range map[string]string{
		"/home/a/.local/bin/yt-dlp-gui":   "/home/a/.local/bin/yt-dlp-gui",
		"/home/a b/.local/bin/yt-dlp-gui": `"/home/a b/.local/bin/yt-dlp-gui"`,
		`/home/a"$b/x`:                    `"/home/a\"\$b/x"`,
	} {
		if got := installExecArg(in); got != want {
			t.Errorf("installExecArg(%q) = %q, want %q", in, got, want)
		}
	}
}
