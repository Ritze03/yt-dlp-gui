package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// termGrace is how long a cancelled yt-dlp gets after SIGTERM before Go
// SIGKILLs it (and we SIGKILL the whole group).
var termGrace = 5 * time.Second

const tailLines = 40

// Error is a failed yt-dlp run (probe or download).
type Error struct {
	LastError  string   // last "ERROR:" line from stderr, prefix stripped
	Hint       string   // see Hint; "" when no pattern matched
	StderrTail []string // last ~40 stderr lines
	Err        error    // underlying exec error (exit status, or start failure)
}

func (e *Error) Error() string {
	switch {
	case e.LastError != "":
		return e.LastError
	case e.Err != nil:
		return "yt-dlp: " + e.Err.Error()
	}
	return "yt-dlp failed"
}

func (e *Error) Unwrap() error { return e.Err }

// run starts Bin with args in its own process group, writing stdout to out
// (may be nil). stderr is kept as a bounded tail. Cancelling ctx sends SIGTERM to the whole
// group (yt-dlp + ffmpeg children), then SIGKILL; run returns ctx.Err() then.
// Other failures are *Error.
func run(ctx context.Context, args []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, Bin, args...)
	// Own process group so signals reach ffmpeg children. Env is inherited
	// (cmd.Env nil) so deno is found on PATH.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = termGrace // then Go SIGKILLs the leader and abandons the pipes

	cmd.Stdout = out
	tail := &tailBuffer{}
	errw := &lineWriter{fn: tail.add}
	cmd.Stderr = errw

	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
		// Go only kills the leader; sweep stragglers (ffmpeg ignoring SIGTERM).
		// ESRCH when the group is already gone is expected and ignored.
		if ctx.Err() != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	errw.flush()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		err = fmt.Errorf("%q not found on PATH: %w", Bin, err)
	}
	last := tail.lastError()
	return &Error{LastError: last, Hint: Hint(last), StderrTail: tail.lines(), Err: err}
}

// lineWriter splits written bytes into lines and calls fn for each.
type lineWriter struct {
	fn  func(string)
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.fn(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.fn(strings.TrimRight(string(w.buf), "\r\n"))
		w.buf = nil
	}
}

// tailBuffer keeps the last tailLines stderr lines.
type tailBuffer struct {
	mu sync.Mutex
	ls []string
}

func (t *tailBuffer) add(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ls = append(t.ls, line)
	if len(t.ls) > tailLines {
		t.ls = t.ls[len(t.ls)-tailLines:]
	}
}

func (t *tailBuffer) lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.ls...)
}

// lastError returns the last "ERROR: ..." line without its prefix.
func (t *tailBuffer) lastError() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := len(t.ls) - 1; i >= 0; i-- {
		if s, ok := strings.CutPrefix(t.ls[i], "ERROR: "); ok {
			return s
		}
	}
	return ""
}
