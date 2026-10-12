package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Result is a finished download.
type Result struct {
	Path string // final file (yt-dlp's after_move filepath)
	// Dest is the intended output path announced before downloading (the
	// extension may differ from Path, e.g. after -x). It is set even when the
	// download fails or is cancelled; keep it to call RemovePartFiles later.
	Dest string
}

// Download runs one download and calls onProgress (may be nil) from yt-dlp's
// stdout goroutine for every progress/stage event; the queue should coalesce
// if it redraws. It blocks until yt-dlp and its children are gone.
//
// Cancel by cancelling ctx: the whole process group gets SIGTERM, then SIGKILL
// after 5s. .part files are left in place so a retry resumes; the returned
// error is ctx.Err() (context.Canceled) and Result.Dest is still filled in.
// Other failures are *Error (use errors.As) carrying LastError, Hint and the
// stderr tail. On success a final Progress{Stage: StageDone, Percent: 1} is sent.
func Download(ctx context.Context, o Options, onProgress func(Progress)) (Result, error) {
	var t tracker
	lw := &lineWriter{fn: func(line string) {
		if p, ok := t.feed(line); ok && onProgress != nil {
			onProgress(p)
		}
	}}
	err := run(ctx, Args(o), lw)
	lw.flush()
	res := Result{Path: t.path, Dest: t.dest}
	if err != nil {
		return res, err
	}
	if res.Path == "" {
		return res, &Error{Err: errors.New("yt-dlp finished without reporting an output file")}
	}
	if onProgress != nil {
		onProgress(Progress{Stage: StageDone, Percent: 1})
	}
	return res, nil
}

// RemovePartFiles deletes the leftovers of an unfinished download: every
// file in dest's folder named "<stem>.<rest>" where rest contains ".part"
// (streams "<stem>.f401.mp4.part", fragments ".part-Frag12") or ends in ".ytdl",
// or is a sidecar: a thumbnail "<stem>.webp|jpg|jpeg|png" or subtitles
// "<stem>.<lang>.vtt|srt|ass|ttml|srv3|json3".
// dest is Result.Dest, whose stem is exactly what yt-dlp used for the temp
// files (the extension may differ, only the stem matters). An empty dest, or a
// missing folder, is a no-op. A finished media file (.mp4, .m4a, ...), another
// stem's files and subfolders are never touched.
func RemovePartFiles(dest string) error {
	if dest == "" {
		return nil
	}
	dir, base := filepath.Split(dest)
	if dir == "" {
		dir = "."
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base)) + "."
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var errs []error
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, stem) {
			continue
		}
		if isLeftover(n[len(stem):]) {
			if err := os.Remove(filepath.Join(dir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

var (
	thumbExts = map[string]bool{"webp": true, "jpg": true, "jpeg": true, "png": true}
	subExts   = map[string]bool{"vtt": true, "srt": true, "ass": true, "ttml": true, "srv3": true, "json3": true}
)

// isLeftover reports whether rest (a file name after "<stem>.") is a partial
// or sidecar file that is not part of a finished download.
func isLeftover(rest string) bool {
	if strings.Contains(rest, ".part") || strings.HasSuffix(rest, ".ytdl") {
		return true
	}
	i := strings.LastIndex(rest, ".")
	if i < 0 {
		return thumbExts[rest]
	}
	// "<lang>.<ext>"; ponytail: lang is any non-empty token (a file name has no "/").
	return i > 0 && subExts[rest[i+1:]]
}
