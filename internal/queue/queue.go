// Package queue is the download queue: job rows with state and live progress,
// a parallel limit of 1-4, cancel / retry / remove, on top of internal/ytdlp.
// It has no UI code; the UI reads Jobs() every frame and passes w.Invalidate
// as onChange.
package queue

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"yt-dlp-gui/internal/ytdlp"
)

// State is the lifecycle of a job row.
type State int

const (
	Queued State = iota
	Running
	Done
	Failed
	Cancelled
)

func (s State) String() string {
	return [...]string{"Queued", "Running", "Done", "Failed", "Cancelled"}[s]
}

// Job is one row. Jobs() hands out copies; Err and Opts.Playlist point at
// values that are never mutated after being set, so sharing them is race-free.
type Job struct {
	ID       int
	Title    string
	Opts     ytdlp.Options
	State    State
	Progress ytdlp.Progress
	Path     string       // final file, set when Done
	Dest     string       // intended path (for .part cleanup), set as soon as known
	Err      *ytdlp.Error // set when Failed; non-yt-dlp errors are wrapped into one
	Attempts int          // runs started since the last Add/Retry (403 auto-retry)
}

// Runner is ytdlp.Download's signature; tests substitute a fake.
type Runner func(ctx context.Context, o ytdlp.Options, onProgress func(ytdlp.Progress)) (ytdlp.Result, error)

type job struct {
	Job
	cancel     context.CancelFunc // non-nil while Running
	cancelling bool               // Cancel/Remove asked for it
	removed    bool               // row already dropped from q.jobs
}

// Queue schedules jobs FIFO with at most `parallel` running at once.
type Queue struct {
	run      Runner
	onChange func()

	mu       sync.Mutex
	jobs     []*job
	nextID   int
	parallel int
	running  int // goroutines in flight, including removed rows still winding down
	closed   bool
	wg       sync.WaitGroup
	onRetry  func(*ytdlp.Options) // see SetRetryHook
}

// New returns a queue. run nil means ytdlp.Download; onChange may be nil and is
// called without any lock held after every state or progress change.
func New(run Runner, parallel int, onChange func()) *Queue {
	if run == nil {
		run = ytdlp.Download
	}
	return &Queue{run: run, onChange: onChange, parallel: clamp(parallel)}
}

func clamp(n int) int { return min(max(n, 1), 4) }

func (q *Queue) changed() {
	if q.onChange != nil {
		q.onChange()
	}
}

// Add queues one job and returns its ID.
func (q *Queue) Add(title string, o ytdlp.Options) int {
	q.mu.Lock()
	id := q.add(title, o)
	q.start()
	q.mu.Unlock()
	q.changed()
	return id
}

// AddEntries queues one job per playlist entry, inheriting base (folder,
// preset, cookies, ...). Each Opts.Playlist points at its own copy of the entry.
func (q *Queue) AddEntries(entries []ytdlp.Entry, base ytdlp.Options) []int {
	ids := make([]int, 0, len(entries))
	q.mu.Lock()
	for _, e := range entries {
		o := base
		o.URL = e.URL
		o.Playlist = &e // Go 1.22+: e is a fresh variable per iteration
		ids = append(ids, q.add(e.Title, o))
	}
	q.start()
	q.mu.Unlock()
	q.changed()
	return ids
}

func (q *Queue) add(title string, o ytdlp.Options) int {
	q.nextID++
	q.jobs = append(q.jobs, &job{Job: Job{ID: q.nextID, Title: title, Opts: o}})
	return q.nextID
}

// SetParallel sets the limit (clamped to 1..4). Raising it starts waiting jobs
// at once; lowering it never kills running ones.
func (q *Queue) SetParallel(n int) {
	q.mu.Lock()
	q.parallel = clamp(n)
	q.start()
	q.mu.Unlock()
	q.changed()
}

// Jobs returns a snapshot of all rows in insertion order.
func (q *Queue) Jobs() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, len(q.jobs))
	for i, j := range q.jobs {
		out[i] = j.Job
	}
	return out
}

// find returns the job with the given id and its index, or nil.
func (q *Queue) find(id int) (*job, int) {
	for i, j := range q.jobs {
		if j.ID == id {
			return j, i
		}
	}
	return nil, -1
}

// Cancel stops a running job (it becomes Cancelled once yt-dlp is gone, .part
// files stay so a Retry resumes) or cancels a queued one immediately.
func (q *Queue) Cancel(id int) {
	q.mu.Lock()
	if j, _ := q.find(id); j != nil {
		q.cancel(j)
	}
	q.mu.Unlock()
	q.changed()
}

func (q *Queue) cancel(j *job) {
	switch j.State {
	case Queued:
		j.State = Cancelled
	case Running:
		j.cancelling = true
		j.cancel()
	}
}

// CancelAll cancels every running and queued job.
func (q *Queue) CancelAll() {
	q.mu.Lock()
	for _, j := range q.jobs {
		q.cancel(j)
	}
	q.mu.Unlock()
	q.changed()
}

// SetRetryHook installs fn, which Retry applies to the job's Options so a retry
// can pick up settings changed since the job was queued (cookies, subtitles)
// while folder, format and URL stay. fn runs under the queue lock: it must be
// cheap and must not call back into the queue.
func (q *Queue) SetRetryHook(fn func(*ytdlp.Options)) {
	q.mu.Lock()
	q.onRetry = fn
	q.mu.Unlock()
}

// Retry requeues a Failed or Cancelled job; the kept .part files let yt-dlp
// resume. Options are unchanged unless a retry hook adjusts them.
func (q *Queue) Retry(id int) {
	q.mu.Lock()
	if j, _ := q.find(id); j != nil && (j.State == Failed || j.State == Cancelled) {
		if q.onRetry != nil {
			q.onRetry(&j.Opts)
		}
		j.State = Queued
		j.Err = nil
		j.Progress = ytdlp.Progress{}
		j.Attempts = 0
		j.cancelling = false
		q.start()
	}
	q.mu.Unlock()
	q.changed()
}

// Remove drops the row. A running job is cancelled and its .part files are
// deleted once yt-dlp has exited; Failed/Cancelled rows lose their .part files
// now; a Done row's finished file is never touched.
func (q *Queue) Remove(id int) {
	q.mu.Lock()
	j, i := q.find(id)
	if j == nil {
		q.mu.Unlock()
		return
	}
	q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
	j.removed = true
	q.cancel(j) // Running: the worker deletes the .part files when it returns
	dest := j.Dest
	cleanNow := j.State == Failed || j.State == Cancelled
	q.mu.Unlock()
	if cleanNow {
		ytdlp.RemovePartFiles(dest) // ponytail: best effort, a leftover .part is harmless
	}
	q.changed()
}

// ClearDone drops every Done row (the finished files are never touched, as in
// Remove) and returns how many it dropped.
func (q *Queue) ClearDone() int {
	q.mu.Lock()
	n := len(q.jobs)
	q.jobs = slices.DeleteFunc(q.jobs, func(j *job) bool { return j.State == Done })
	n -= len(q.jobs)
	q.mu.Unlock()
	if n > 0 {
		q.changed()
	}
	return n
}

// ClearCancelled drops every Cancelled row like Remove would, deleting each
// one's leftover .part files, and returns how many it dropped. A row is only
// Cancelled once its download has returned (or it never started and has no
// Dest), so nothing is still writing.
func (q *Queue) ClearCancelled() int {
	q.mu.Lock()
	var dests []string
	q.jobs = slices.DeleteFunc(q.jobs, func(j *job) bool {
		if j.State != Cancelled {
			return false
		}
		dests = append(dests, j.Dest)
		return true
	})
	q.mu.Unlock()
	for _, d := range dests {
		ytdlp.RemovePartFiles(d) // ponytail: best effort, as in Remove
	}
	if len(dests) > 0 {
		q.changed()
	}
	return len(dests)
}

// Close cancels everything and blocks until every download has returned.
// Call it when the app quits.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.CancelAll()
	q.wg.Wait()
}

// start launches queued jobs while there is a free slot. Caller holds q.mu.
func (q *Queue) start() {
	if q.closed {
		return
	}
	for _, j := range q.jobs {
		if q.running >= q.parallel {
			return
		}
		if j.State != Queued {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		j.State = Running
		j.cancel = cancel
		j.Progress = ytdlp.Progress{}
		q.running++
		q.wg.Add(1)
		go q.work(ctx, j)
	}
}

func (q *Queue) work(ctx context.Context, j *job) {
	defer q.wg.Done()

	q.mu.Lock()
	opts := j.Opts
	q.mu.Unlock()

	var res ytdlp.Result
	var err error
	for attempt := 1; ; attempt++ {
		q.mu.Lock()
		j.Attempts = attempt
		q.mu.Unlock()
		var r ytdlp.Result
		r, err = q.run(ctx, opts, func(p ytdlp.Progress) { q.progress(j, p) })
		if r.Dest != "" || r.Path != "" {
			res = r
			q.mu.Lock()
			j.Dest = r.Dest
			q.mu.Unlock()
		}
		// ponytail: YouTube answers HTTP 403 intermittently; one automatic
		// re-run (the .part resumes) hides most of it. A second 403 is real.
		if attempt == 1 && ctx.Err() == nil && is403(err) {
			q.mu.Lock()
			j.Progress = ytdlp.Progress{}
			q.mu.Unlock()
			q.changed()
			continue
		}
		break
	}

	q.mu.Lock()
	removed := j.removed
	cancelled := j.cancelling || errors.Is(err, context.Canceled) || ctx.Err() != nil
	j.cancel() // release the context; after reading ctx.Err()
	j.cancel = nil
	if !removed {
		switch {
		case err == nil:
			j.State, j.Path = Done, res.Path
		case cancelled:
			j.State = Cancelled
		default:
			j.State, j.Err = Failed, asError(err)
		}
	}
	q.mu.Unlock()
	if removed {
		ytdlp.RemovePartFiles(res.Dest) // ponytail: best effort
	}

	q.mu.Lock()
	q.running--
	q.start()
	q.mu.Unlock()
	q.changed()
}

// progress folds one event into the row: a new stage means a new stream so the
// percent restarts; a stage-only event (Percent -1) within a stage keeps the
// last known percent.
func (q *Queue) progress(j *job, p ytdlp.Progress) {
	q.mu.Lock()
	if p.Percent < 0 && p.Stage == j.Progress.Stage {
		p.Percent = j.Progress.Percent
	}
	j.Progress = p
	q.mu.Unlock()
	q.changed()
}

func is403(err error) bool {
	var ye *ytdlp.Error
	return errors.As(err, &ye) && strings.Contains(ye.LastError, "HTTP Error 403")
}

// asError gives the UI a single error shape.
func asError(err error) *ytdlp.Error {
	var ye *ytdlp.Error
	if errors.As(err, &ye) {
		return ye
	}
	return &ytdlp.Error{LastError: err.Error(), Err: err}
}
