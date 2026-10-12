package queue

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yt-dlp-gui/internal/ytdlp"
)

// fake is a Runner whose runs block until released (or their ctx is
// cancelled), recording concurrency, start order and what ctx looked like.
type fake struct {
	t       *testing.T
	dir     string
	started chan string // URL of every run start, in order

	mu       sync.Mutex
	gates    map[string]chan struct{} // closed to let a run finish
	calls    map[string]int
	cur, max int
	ctxErr   map[string]error // ctx.Err() seen when the run returned
	script   func(url string, call int) error
	prog     func(url string, on func(ytdlp.Progress))
	returned atomic.Int32
}

func newFake(t *testing.T) *fake {
	return &fake{t: t, dir: t.TempDir(), started: make(chan string, 100),
		gates: map[string]chan struct{}{}, calls: map[string]int{}, ctxErr: map[string]error{}}
}

func (f *fake) gate(url string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.gates[url]
	if !ok {
		g = make(chan struct{})
		f.gates[url] = g
	}
	return g
}

func (f *fake) release(url string) { close(f.gate(url)) }

func (f *fake) callCount(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[url]
}

func (f *fake) dest(url string) string { return filepath.Join(f.dir, filepath.Base(url)+".mp4") }

func (f *fake) part(url string) string { return f.dest(url) + ".part" }

func (f *fake) run(ctx context.Context, o ytdlp.Options, on func(ytdlp.Progress)) (ytdlp.Result, error) {
	defer f.returned.Add(1)
	url := o.URL
	f.mu.Lock()
	f.calls[url]++
	call := f.calls[url]
	f.cur++
	f.max = max(f.max, f.cur)
	script, prog := f.script, f.prog
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.cur--
		f.ctxErr[url] = ctx.Err()
		f.mu.Unlock()
	}()

	res := ytdlp.Result{Dest: f.dest(url)}
	if err := os.WriteFile(f.part(url), []byte("partial"), 0o644); err != nil {
		f.t.Error(err)
	}
	f.started <- url
	if prog != nil {
		prog(url, on)
	}
	if script != nil {
		if err := script(url, call); err != nil {
			return res, err
		}
		res.Path = res.Dest
		return res, nil
	}
	select {
	case <-ctx.Done():
		return res, ctx.Err()
	case <-f.gate(url):
		os.Remove(f.part(url))
		os.WriteFile(res.Dest, []byte("final"), 0o644)
		res.Path = res.Dest
		return res, nil
	}
}

func (f *fake) waitStarted(url string) {
	f.t.Helper()
	select {
	case got := <-f.started:
		if got != url {
			f.t.Fatalf("started %q, want %q", got, url)
		}
	case <-time.After(5 * time.Second):
		f.t.Fatalf("timeout waiting for %q to start", url)
	}
}

// waitStartedAny waits for len(urls) starts in any order (goroutines race).
func (f *fake) waitStartedAny(urls ...string) {
	f.t.Helper()
	want := map[string]bool{}
	for _, u := range urls {
		want[u] = true
	}
	for range urls {
		select {
		case got := <-f.started:
			if !want[got] {
				f.t.Fatalf("started %q, want one of %v", got, urls)
			}
			delete(want, got)
		case <-time.After(5 * time.Second):
			f.t.Fatalf("timeout waiting for starts, missing %v", want)
		}
	}
}

func (f *fake) expectNoStart() {
	f.t.Helper()
	select {
	case got := <-f.started:
		f.t.Fatalf("unexpected start of %q", got)
	case <-time.After(30 * time.Millisecond):
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func state(q *Queue, id int) State {
	for _, j := range q.Jobs() {
		if j.ID == id {
			return j.State
		}
	}
	return -1
}

func waitState(t *testing.T, q *Queue, id int, want State) {
	t.Helper()
	waitFor(t, want.String(), func() bool { return state(q, id) == want })
}

func get(t *testing.T, q *Queue, id int) Job {
	t.Helper()
	for _, j := range q.Jobs() {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %d not found", id)
	return Job{}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func opts(url string) ytdlp.Options {
	return ytdlp.Options{URL: url, Folder: "/dl", Preset: ytdlp.P720}
}

func TestParallel1RunsOneAtATimeFIFO(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	ids := []int{q.Add("a", opts("a")), q.Add("b", opts("b")), q.Add("c", opts("c"))}

	for i, u := range []string{"a", "b", "c"} {
		f.waitStarted(u)
		f.expectNoStart() // nothing else may start while u runs
		if got := state(q, ids[i]); got != Running {
			t.Fatalf("%s: state %v", u, got)
		}
		f.release(u)
		waitState(t, q, ids[i], Done)
	}
	if f.max != 1 {
		t.Errorf("max concurrency %d, want 1", f.max)
	}
}

func TestParallel2RunsTwoAtOnce(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 2, nil)
	defer q.Close()
	a, b, c := q.Add("a", opts("a")), q.Add("b", opts("b")), q.Add("c", opts("c"))
	f.waitStartedAny("a", "b")
	f.expectNoStart()
	if state(q, a) != Running || state(q, b) != Running || state(q, c) != Queued {
		t.Fatalf("states %v", q.Jobs())
	}
	f.release("a")
	f.waitStarted("c")
	f.release("b")
	f.release("c")
	waitState(t, q, c, Done)
	if f.max != 2 {
		t.Errorf("max concurrency %d, want 2", f.max)
	}
}

func TestSetParallel(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	a, b := q.Add("a", opts("a")), q.Add("b", opts("b"))
	f.waitStarted("a")
	f.expectNoStart()
	q.SetParallel(2) // raising starts the waiting job at once
	f.waitStarted("b")

	c, d := q.Add("c", opts("c")), q.Add("d", opts("d"))
	q.SetParallel(1) // lowering kills nothing...
	f.expectNoStart()
	if state(q, a) != Running || state(q, b) != Running {
		t.Fatal("lowering parallel killed a running job")
	}
	f.release("a") // ...but nothing new starts until running < 1
	waitState(t, q, a, Done)
	f.expectNoStart()
	f.release("b")
	f.waitStarted("c")
	f.release("c")
	f.waitStarted("d")
	f.release("d")
	waitState(t, q, d, Done)
	_ = c

	q.SetParallel(99)
	q.SetParallel(-5)
	if q.parallel != 1 {
		t.Errorf("parallel %d, want clamped to 1", q.parallel)
	}
	if New(nil, 9, nil).parallel != 4 || New(nil, 0, nil).parallel != 1 {
		t.Error("New does not clamp")
	}
}

func TestCancelRunningKeepsPart(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	id := q.Add("a", opts("a"))
	f.waitStarted("a")
	q.Cancel(id)
	waitState(t, q, id, Cancelled)
	j := get(t, q, id)
	if j.Err != nil {
		t.Errorf("Err = %v on cancelled job", j.Err)
	}
	if f.ctxErr["a"] == nil {
		t.Error("runner never saw ctx cancelled")
	}
	if !exists(f.part("a")) {
		t.Error(".part removed by Cancel")
	}
	if j.Dest != f.dest("a") {
		t.Errorf("Dest = %q", j.Dest)
	}
}

func TestCancelQueuedAndCancelAll(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	a, b, c := q.Add("a", opts("a")), q.Add("b", opts("b")), q.Add("c", opts("c"))
	f.waitStarted("a")
	q.Cancel(b)
	if state(q, b) != Cancelled { // immediate
		t.Fatalf("queued cancel: %v", state(q, b))
	}
	q.CancelAll()
	waitState(t, q, a, Cancelled)
	waitState(t, q, c, Cancelled)
	f.expectNoStart() // cancelled queued jobs are never started
	if f.callCount("b") != 0 || f.callCount("c") != 0 {
		t.Error("cancelled queued job ran")
	}
}

func TestRetryCancelledAndFailedReusesOpts(t *testing.T) {
	f := newFake(t)
	f.script = func(url string, call int) error {
		if url == "bad" && call == 1 {
			return &ytdlp.Error{LastError: "boom"}
		}
		return nil
	}
	q := New(f.run, 1, nil)
	defer q.Close()
	o := ytdlp.Options{URL: "bad", Folder: "/dl", Preset: ytdlp.AudioMP3, Cookies: "firefox", Subs: true, SubLang: "de"}
	id := q.Add("bad", o)
	waitState(t, q, id, Failed)
	if j := get(t, q, id); j.Err == nil || j.Err.LastError != "boom" {
		t.Fatalf("Err = %v", j.Err)
	}
	q.Retry(id)
	waitState(t, q, id, Done)
	j := get(t, q, id)
	if j.Err != nil || j.Opts != o || j.Path == "" {
		t.Errorf("after retry: %+v", j)
	}
	if f.callCount("bad") != 2 {
		t.Errorf("calls = %d", f.callCount("bad"))
	}
	f.waitStarted("bad") // the two runs above
	f.waitStarted("bad")
	q.Retry(id) // Done rows are not retried
	f.expectNoStart()

	// Cancelled -> Retry with a blocking runner
	g := newFake(t)
	q2 := New(g.run, 1, nil)
	defer q2.Close()
	o2 := o
	o2.URL = "a"
	id2 := q2.Add("a", o2)
	g.waitStarted("a")
	q2.Cancel(id2)
	waitState(t, q2, id2, Cancelled)
	q2.Retry(id2)
	g.waitStarted("a")
	if j := get(t, q2, id2); j.State != Running || j.Progress != (ytdlp.Progress{}) || j.Attempts != 1 {
		t.Errorf("after retry: %+v", j)
	}
	g.release("a")
	waitState(t, q2, id2, Done)
}

func TestRemoveFailedDeletesPart(t *testing.T) {
	f := newFake(t)
	f.script = func(string, int) error { return &ytdlp.Error{LastError: "nope"} }
	q := New(f.run, 1, nil)
	defer q.Close()
	id := q.Add("a", opts("a"))
	waitState(t, q, id, Failed)
	if !exists(f.part("a")) {
		t.Fatal("failed job should keep .part")
	}
	q.Remove(id)
	if len(q.Jobs()) != 0 {
		t.Error("row still present")
	}
	if exists(f.part("a")) {
		t.Error(".part not deleted")
	}
}

func TestRemoveRunningCancelsThenDeletesPart(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	a := q.Add("a", opts("a"))
	q.Add("b", opts("b"))
	f.waitStarted("a")
	q.Remove(a)
	if len(q.Jobs()) != 1 || q.Jobs()[0].Title != "b" {
		t.Fatalf("jobs: %+v", q.Jobs())
	}
	f.waitStarted("b") // slot freed only once a's runner has returned
	if f.ctxErr["a"] == nil {
		t.Error("runner a never saw ctx cancelled")
	}
	if exists(f.part("a")) {
		t.Error(".part not deleted after runner returned")
	}
	f.release("b")
}

func TestRemoveDoneKeepsFile(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	id := q.Add("a", opts("a"))
	f.waitStarted("a")
	f.release("a")
	waitState(t, q, id, Done)
	q.Remove(id)
	if len(q.Jobs()) != 0 {
		t.Error("row still present")
	}
	if !exists(f.dest("a")) {
		t.Error("finished file deleted")
	}
	q.Remove(12345) // unknown id is a no-op
}

func TestAuto403Retry(t *testing.T) {
	e403 := &ytdlp.Error{LastError: "unable to download video data: HTTP Error 403: Forbidden"}

	t.Run("second attempt succeeds", func(t *testing.T) {
		f := newFake(t)
		f.script = func(_ string, call int) error {
			if call == 1 {
				return e403
			}
			return nil
		}
		q := New(f.run, 1, nil)
		defer q.Close()
		id := q.Add("a", opts("a"))
		waitState(t, q, id, Done)
		if n := f.callCount("a"); n != 2 {
			t.Errorf("runner called %d times, want 2", n)
		}
		if j := get(t, q, id); j.Err != nil || j.Attempts != 2 {
			t.Errorf("job %+v", j)
		}
	})
	t.Run("both fail", func(t *testing.T) {
		f := newFake(t)
		f.script = func(string, int) error { return e403 }
		q := New(f.run, 1, nil)
		defer q.Close()
		id := q.Add("a", opts("a"))
		waitState(t, q, id, Failed)
		if n := f.callCount("a"); n != 2 {
			t.Errorf("runner called %d times, want 2", n)
		}
		if j := get(t, q, id); j.Err != e403 {
			t.Errorf("Err = %v", j.Err)
		}
	})
	t.Run("other error not retried", func(t *testing.T) {
		f := newFake(t)
		f.script = func(string, int) error { return &ytdlp.Error{LastError: "Video unavailable"} }
		q := New(f.run, 1, nil)
		defer q.Close()
		id := q.Add("a", opts("a"))
		waitState(t, q, id, Failed)
		if n := f.callCount("a"); n != 1 {
			t.Errorf("runner called %d times, want 1", n)
		}
	})
}

func TestNonYtdlpErrorIsWrapped(t *testing.T) {
	f := newFake(t)
	boom := errors.New(`exec: "yt-dlp": executable file not found in $PATH`)
	f.script = func(string, int) error { return boom }
	q := New(f.run, 1, nil)
	defer q.Close()
	id := q.Add("a", opts("a"))
	waitState(t, q, id, Failed)
	j := get(t, q, id)
	if j.Err == nil || j.Err.LastError != boom.Error() || !errors.Is(j.Err, boom) {
		t.Errorf("Err = %#v", j.Err)
	}
}

func TestAddEntries(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	entries := []ytdlp.Entry{
		{ID: "1", URL: "u1", Title: "One", PlaylistTitle: "PL", Index: 1},
		{ID: "2", URL: "u2", Title: "Two", PlaylistTitle: "PL", Index: 2},
	}
	base := ytdlp.Options{URL: "ignored", Folder: "/dl", Preset: ytdlp.P1080, Cookies: "brave", Subs: true, SubLang: "fr"}
	ids := q.AddEntries(entries, base)
	if len(ids) != 2 {
		t.Fatalf("ids %v", ids)
	}
	jobs := q.Jobs()
	for i, j := range jobs {
		e := entries[i]
		if j.ID != ids[i] || j.Title != e.Title || j.Opts.URL != e.URL || j.Opts.Playlist == nil || *j.Opts.Playlist != e {
			t.Errorf("job %d: %+v", i, j)
		}
		if j.Opts.Folder != "/dl" || j.Opts.Preset != ytdlp.P1080 || j.Opts.Cookies != "brave" || !j.Opts.Subs || j.Opts.SubLang != "fr" {
			t.Errorf("job %d lost base options: %+v", i, j.Opts)
		}
	}
	if jobs[0].Opts.Playlist == jobs[1].Opts.Playlist {
		t.Error("rows share one Entry copy")
	}
	f.waitStarted("u1")
}

func TestProgressStageHandling(t *testing.T) {
	f := newFake(t)
	var seen []ytdlp.Progress
	proceed := make(chan struct{})
	reached := make(chan struct{}, 10)
	var q *Queue
	var id int
	f.prog = func(url string, on func(ytdlp.Progress)) {
		for _, p := range []ytdlp.Progress{
			{Stage: ytdlp.StageVideo, Percent: 0.4, Speed: 10},
			{Stage: ytdlp.StageVideo, Percent: -1},   // same stage: keep 0.4
			{Stage: ytdlp.StageAudio, Percent: -1},   // new stage: unknown
			{Stage: ytdlp.StageAudio, Percent: 0.25}, //
			{Stage: ytdlp.StageMerging, Percent: -1}, // new stage, unknown
		} {
			on(p)
			seen = append(seen, get(t, q, 1).Progress) // first job has ID 1
			reached <- struct{}{}
		}
		<-proceed
	}
	q = New(f.run, 1, nil)
	defer q.Close()
	id = q.Add("a", opts("a"))
	for range 5 {
		<-reached
	}
	close(proceed)
	f.release("a")
	waitState(t, q, id, Done)

	wantPct := []float64{0.4, 0.4, -1, 0.25, -1}
	wantStage := []ytdlp.Stage{ytdlp.StageVideo, ytdlp.StageVideo, ytdlp.StageAudio, ytdlp.StageAudio, ytdlp.StageMerging}
	for i, p := range seen {
		if p.Percent != wantPct[i] || p.Stage != wantStage[i] {
			t.Errorf("event %d: got %v/%v want %v/%v", i, p.Stage, p.Percent, wantStage[i], wantPct[i])
		}
	}
}

func TestCloseWaitsForRuns(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 2, nil)
	for _, u := range []string{"a", "b", "c"} {
		q.Add(u, opts(u))
	}
	f.waitStartedAny("a", "b")
	q.Close()
	if n := f.returned.Load(); n != 2 {
		t.Errorf("%d runs returned after Close, want 2", n)
	}
	for _, j := range q.Jobs() {
		if j.State != Cancelled {
			t.Errorf("job %s is %v after Close", j.Title, j.State)
		}
	}
	if f.callCount("c") != 0 {
		t.Error("queued job started during Close")
	}
	q.Add("d", opts("d")) // after Close nothing starts
	f.expectNoStart()
}

func TestOnChangeNotUnderLock(t *testing.T) {
	f := newFake(t)
	var calls atomic.Int32
	var q *Queue
	q = New(f.run, 1, func() {
		calls.Add(1)
		q.Jobs() // deadlocks if the queue called us while holding its lock
	})
	defer q.Close()
	f.prog = func(url string, on func(ytdlp.Progress)) {
		on(ytdlp.Progress{Stage: ytdlp.StageDownloading, Percent: 0.5})
	}
	done := make(chan struct{})
	go func() {
		id := q.Add("a", opts("a"))
		f.waitStarted("a")
		f.release("a")
		waitState(t, q, id, Done)
		q.Cancel(id)
		q.Remove(id)
		q.SetParallel(2)
		q.CancelAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: onChange called under lock?")
	}
	if calls.Load() < 5 {
		t.Errorf("onChange called %d times", calls.Load())
	}
}

func TestJobsIsACopy(t *testing.T) {
	f := newFake(t)
	q := New(f.run, 1, nil)
	defer q.Close()
	id := q.Add("a", opts("a"))
	f.waitStarted("a")
	js := q.Jobs()
	js[0].State = Failed
	js[0].Title = "hacked"
	if j := get(t, q, id); j.State != Running || j.Title != "a" {
		t.Errorf("mutating the snapshot changed the queue: %+v", j)
	}
}

func TestRetryHookAppliesCurrentSettings(t *testing.T) {
	f := newFake(t)
	var mu sync.Mutex
	var seen []ytdlp.Options
	f.script = func(url string, call int) error {
		if call == 1 {
			return &ytdlp.Error{LastError: "Sign in to confirm you're not a bot"}
		}
		return nil
	}
	q := New(func(ctx context.Context, o ytdlp.Options, on func(ytdlp.Progress)) (ytdlp.Result, error) {
		mu.Lock()
		seen = append(seen, o)
		mu.Unlock()
		return f.run(ctx, o, on)
	}, 1, nil)
	defer q.Close()
	q.SetRetryHook(func(o *ytdlp.Options) { o.Cookies, o.Subs = "firefox", true })
	o := ytdlp.Options{URL: "bad", Folder: "/dl", Preset: ytdlp.AudioMP3, Cookies: "none", SubLang: "de"}
	id := q.Add("bad", o)
	waitState(t, q, id, Failed)
	q.Retry(id)
	waitState(t, q, id, Done)
	mu.Lock()
	defer mu.Unlock()
	want := o
	want.Cookies, want.Subs = "firefox", true
	if len(seen) != 2 || seen[0] != o || seen[1] != want {
		t.Errorf("runs = %+v, want first %+v then %+v (folder/preset/URL kept)", seen, o, want)
	}
}

func TestClearDone(t *testing.T) {
	f := newFake(t)
	f.script = func(url string, _ int) error {
		switch url {
		case "run":
			<-f.gate("run") // stays Running until released
		case "bad":
			return &ytdlp.Error{LastError: "nope"}
		}
		return nil
	}
	q := New(f.run, 1, nil) // one slot: canc stays Queued behind run
	defer q.Close()
	if n := q.ClearDone(); n != 0 {
		t.Errorf("empty queue: cleared %d", n)
	}
	a := q.Add("a", opts("a"))
	b := q.Add("b", opts("b"))
	bad := q.Add("bad", opts("bad"))
	run := q.Add("run", opts("run"))
	waitState(t, q, a, Done)
	waitState(t, q, b, Done)
	waitState(t, q, bad, Failed)
	waitState(t, q, run, Running)
	canc := q.Add("canc", opts("canc"))
	q.Cancel(canc)
	waitState(t, q, canc, Cancelled)
	if n := q.ClearDone(); n != 2 {
		t.Errorf("cleared %d, want 2", n)
	}
	got := map[int]State{}
	for _, j := range q.Jobs() {
		got[j.ID] = j.State
	}
	if len(got) != 3 || got[bad] != Failed || got[run] != Running || got[canc] != Cancelled {
		t.Errorf("remaining = %v", got)
	}
	if n := q.ClearDone(); n != 0 {
		t.Errorf("second clear removed %d", n)
	}
	f.release("run")
}
