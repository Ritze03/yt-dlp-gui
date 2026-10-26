package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// Gio v0.10.3 can lose Ctrl+V for the rest of the session (its Wayland
// ReadClipboard never resets readClipClose when the clipboard has no text), so
// pasting asks the system clipboard tools instead.

const (
	clipTimeout = 2 * time.Second
	clipMax     = 8192 // bytes; a URL field has no use for more
)

// clipRunner runs argv and returns its stdout. Replaced in tests.
type clipRunner func(ctx context.Context, argv []string) ([]byte, error)

func execRunner(ctx context.Context, argv []string) ([]byte, error) {
	return exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
}

// clipCommands lists the commands to try, in order: wl-paste on Wayland,
// xclip then xsel otherwise.
func clipCommands(wayland bool) [][]string {
	if wayland {
		return [][]string{{"wl-paste", "--no-newline", "--type", "text"}}
	}
	return [][]string{
		{"xclip", "-selection", "clipboard", "-o"},
		{"xsel", "--clipboard", "--output"},
	}
}

// readClipboard returns the trimmed clipboard text. The first command that
// exists decides: its failure or an empty result is the error shown to the user.
func readClipboard(run clipRunner, wayland bool) (string, error) {
	var missing []string
	for _, argv := range clipCommands(wayland) {
		ctx, cancel := context.WithTimeout(context.Background(), clipTimeout)
		out, err := run(ctx, argv)
		cancel()
		if errors.Is(err, exec.ErrNotFound) {
			missing = append(missing, argv[0])
			continue
		}
		if err != nil {
			// wl-paste, xclip and xsel exit non-zero for an empty or non-text clipboard.
			return "", errors.New("Clipboard is empty or not text.")
		}
		if len(out) > clipMax {
			return "", errors.New("Clipboard text is too long.")
		}
		if !utf8.Valid(out) || bytes.IndexByte(out, 0) >= 0 {
			return "", errors.New("Clipboard is not text.")
		}
		s := strings.TrimSpace(string(out))
		if s == "" {
			return "", errors.New("Clipboard is empty.")
		}
		return s, nil
	}
	return "", errors.New(strings.Join(missing, " / ") + " not found. Install it to paste (wl-clipboard on Wayland, xclip or xsel on X11).")
}

func onWayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }
