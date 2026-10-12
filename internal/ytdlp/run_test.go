package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeBin points Bin at a /bin/sh script for the test's duration.
func fakeBin(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-yt-dlp")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := Bin
	Bin = p
	t.Cleanup(func() { Bin = old })
}

func TestDownloadFakeSuccess(t *testing.T) {
	fakeBin(t, `
echo "DEST /d/x.webm"
echo 'PROG av01 none {"status":"downloading","downloaded_bytes":50,"total_bytes":100}'
echo 'PROG none opus {"status":"downloading","downloaded_bytes":10,"total_bytes":40}'
echo '[Merger] Merging formats into "/d/x.mkv"'
echo "DONE /d/x.mkv"
echo "WARNING: something" >&2`)
	var got []Progress
	res, err := Download(context.Background(), Options{URL: "u", Folder: "/d"}, func(p Progress) { got = append(got, p) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "/d/x.mkv" || res.Dest != "/d/x.webm" {
		t.Errorf("res %+v", res)
	}
	var stages []Stage
	for _, p := range got {
		stages = append(stages, p.Stage)
	}
	w := []Stage{StageVideo, StageAudio, StageMerging, StageDone}
	if len(stages) != len(w) {
		t.Fatalf("stages %v want %v", stages, w)
	}
	for i := range w {
		if stages[i] != w[i] {
			t.Fatalf("stages %v want %v", stages, w)
		}
	}
	if got[0].Percent != 0.5 || got[len(got)-1].Percent != 1 {
		t.Errorf("percents %+v", got)
	}
}

func TestDownloadFakeFailure(t *testing.T) {
	fakeBin(t, `
echo "DEST /d/x.webm"
echo "WARNING: [youtube] foo" >&2
echo "ERROR: [youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser" >&2
exit 1`)
	res, err := Download(context.Background(), Options{URL: "u", Folder: "/d"}, nil)
	var ye *Error
	if !errors.As(err, &ye) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ye.LastError != "[youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser" {
		t.Errorf("LastError %q", ye.LastError)
	}
	if ye.Hint != "Set a cookies browser in Settings." || err.Error() != ye.LastError {
		t.Errorf("hint %q err %q", ye.Hint, err)
	}
	if len(ye.StderrTail) != 2 || ye.StderrTail[0] != "WARNING: [youtube] foo" {
		t.Errorf("tail %q", ye.StderrTail)
	}
	if res.Dest != "/d/x.webm" || res.Path != "" {
		t.Errorf("res %+v (Dest must survive failure)", res)
	}
}

func TestStderrTailBounded(t *testing.T) {
	fakeBin(t, `i=0; while [ $i -lt 100 ]; do echo "line $i" >&2; i=$((i+1)); done; echo "ERROR: boom" >&2; exit 2`)
	_, err := Download(context.Background(), Options{URL: "u"}, nil)
	var ye *Error
	if !errors.As(err, &ye) || len(ye.StderrTail) != tailLines || ye.StderrTail[tailLines-1] != "ERROR: boom" || ye.StderrTail[0] != "line 61" {
		t.Fatalf("tail %d first=%q err=%v", len(ye.StderrTail), ye.StderrTail[0], err)
	}
}

func TestBinMissing(t *testing.T) {
	old := Bin
	Bin = "/nonexistent/yt-dlp"
	defer func() { Bin = old }()
	_, err := Version(context.Background())
	var ye *Error
	if !errors.As(err, &ye) || !strings.Contains(err.Error(), "no such file") {
		t.Errorf("err %v", err)
	}
}

func TestVersionFake(t *testing.T) {
	fakeBin(t, `[ "$1" = "--version" ] && echo 2026.08.19`)
	v, err := Version(context.Background())
	if v != "2026.08.19" || err != nil {
		t.Errorf("%q %v", v, err)
	}
}

// cancelScenario runs a fake yt-dlp that spawns a long-lived child (like
// ffmpeg), records the child's pid, and leaves a .part file; it cancels once
// the child exists and returns the child pid, the dest path and Download's result.
func cancelScenario(t *testing.T, script string) (childPID int, dest string, elapsed time.Duration, err error) {
	t.Helper()
	dir := t.TempDir()
	dest = filepath.Join(dir, "vid [id].mp4")
	pidFile := filepath.Join(dir, "child.pid")
	fakeBin(t, script+"\n") // script references $DIR via env below
	t.Setenv("FAKE_DIR", dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if b, e := os.ReadFile(pidFile); e == nil && strings.HasSuffix(string(b), "\n") {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	start := time.Now()
	res, err := Download(ctx, Options{URL: "u", Folder: dir}, nil)
	elapsed = time.Since(start)
	wg.Wait()
	if res.Dest != dest {
		t.Fatalf("Dest %q want %q", res.Dest, dest)
	}
	b, e := os.ReadFile(pidFile)
	if e != nil {
		t.Fatal(e)
	}
	childPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	return
}

func gone(pid int) bool {
	for i := 0; i < 100; i++ {
		// a killed child may linger as a zombie until init reaps it; /proc state Z counts as gone
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil || strings.Contains(string(b), ") Z ") {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

const fakeLeader = `
echo "DEST $FAKE_DIR/vid [id].mp4"
touch "$FAKE_DIR/vid [id].f137.mp4.part" "$FAKE_DIR/vid [id].mp4.ytdl"
%s
sleep 60 &
echo $! > "$FAKE_DIR/child.pid"
wait`

// Cancel: SIGTERM reaches the child through the process group, .part stays.
func TestCancelKillsGroup(t *testing.T) {
	pid, dest, el, err := cancelScenario(t, strings.Replace(fakeLeader, "%s", "", 1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if !gone(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("child %d survived", pid)
	}
	if el > 3*time.Second {
		t.Errorf("took %v, SIGTERM path should be fast", el)
	}
	if _, err := os.Stat(strings.TrimSuffix(dest, ".mp4") + ".f137.mp4.part"); err != nil {
		t.Errorf(".part must survive cancel: %v", err)
	}
}

// A leader and child that both ignore SIGTERM are SIGKILLed after the grace period.
func TestCancelEscalatesToKill(t *testing.T) {
	old := termGrace
	termGrace = 300 * time.Millisecond
	defer func() { termGrace = old }()

	pid, _, el, err := cancelScenario(t, strings.Replace(fakeLeader, "%s", "trap '' TERM", 1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if !gone(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("TERM-ignoring child %d survived", pid)
	}
	if el < 250*time.Millisecond || el > 5*time.Second {
		t.Errorf("took %v, want ~grace", el)
	}
}

func TestRemovePartFiles(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "My Mix", "001 - Song [x].webm") // note: ext differs from final
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string {
		p := filepath.Join(dir, "My Mix", name)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rm := []string{
		mk("001 - Song [x].f401.mp4.part"), mk("001 - Song [x].f140.m4a.part"),
		mk("001 - Song [x].f251.webm.part-Frag7"), mk("001 - Song [x].f251.webm.ytdl"), mk("001 - Song [x].m4a.part"),
	}
	keep := []string{
		mk("001 - Song [x].m4a"),           // finished file of this entry
		mk("002 - Other [y].f1.mp4.part"),  // another entry's partial
		mk("001 - Song [x] copy.mp4.part"), // different stem sharing a prefix
	}
	if err := RemovePartFiles(dest); err != nil {
		t.Fatal(err)
	}
	for _, p := range rm {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be removed (%v)", filepath.Base(p), err)
		}
	}
	for _, p := range keep {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should be kept: %v", filepath.Base(p), err)
		}
	}
	if RemovePartFiles("") != nil || RemovePartFiles(filepath.Join(dir, "nope", "a.mp4")) != nil {
		t.Error("empty dest / missing folder must be a no-op")
	}
}
