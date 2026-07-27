package harness

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-faster/errors"
)

// MarkerSerial pins a test to the serial phase.
//
// It is not an upstream pytest marker. The atomic read/write tests race
// concurrent readers against writers on one key and assume no other load, and
// the lifecycle tests sleep on wall-clock intervals; running either alongside
// a full worker pool makes them flaky, which is worse than making them slow.
const MarkerSerial = "serial"

// Result is the outcome of one test.
type Result struct {
	Test     Test
	Status   Status
	Duration time.Duration

	// Output is everything the test logged, shown on failure.
	Output string

	// Skip is the reason a skipped test gave.
	Skip string
}

// Runner executes tests.
//
// Every wait it performs is bounded, and the bound is enforced from outside
// the code doing the waiting: a conformance suite points at servers that may
// be broken, and the harness must not be the thing that hangs.
type Runner struct {
	// Parallel is the number of tests executed at once. Zero or one runs
	// sequentially.
	Parallel int

	// Timeout bounds a single test body. Zero means no bound.
	Timeout time.Duration

	// CleanupTimeout bounds the cleanups of one test. Zero means no bound.
	CleanupTimeout time.Duration

	// StallTimeout aborts the run if no test finishes within it. It is the
	// backstop for a bug in the scheduler itself. Zero disables it.
	StallTimeout time.Duration

	// MaxLeaked aborts the run once this many test goroutines have been
	// abandoned. Zero disables the check.
	MaxLeaked int

	// WatchdogGrace is how long past its deadline a test is given before
	// its goroutine is abandoned. Zero means defaultWatchdogGrace.
	WatchdogGrace time.Duration

	// Observe, if set, is called as each test finishes, from a single
	// goroutine, so a caller can report progress as the run proceeds.
	Observe func(Result)
}

// Run executes tests and returns their results in the order given.
//
// It returns an error only when the run itself broke down: too many abandoned
// goroutines, or a stall. Ordinary test failures are reported in the results.
func (r Runner) Run(ctx context.Context, tests []Test) ([]Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := &scheduler{
		runner:  r,
		results: make([]Result, len(tests)),
		done:    make([]bool, len(tests)),
	}
	s.progress.Store(time.Now().UnixNano())

	if r.StallTimeout > 0 {
		stop := s.watchStall(ctx, cancel)
		defer stop()
	}

	// Serial tests run first, alone, before the pool starts.
	serial, parallel := partition(tests)
	s.runIndexes(ctx, tests, serial, 1)
	if ctx.Err() == nil {
		s.runIndexes(ctx, tests, parallel, r.Parallel)
	}

	return s.collect(), s.err
}

// partition splits test indexes into the serial phase and the parallel one.
func partition(tests []Test) (serial, parallel []int) {
	for i, t := range tests {
		if t.Marker(MarkerSerial) {
			serial = append(serial, i)
		} else {
			parallel = append(parallel, i)
		}
	}
	return serial, parallel
}

type scheduler struct {
	runner  Runner
	results []Result
	done    []bool

	mu sync.Mutex

	// progress is the UnixNano of the last finished test, read by the stall
	// watchdog.
	progress atomic.Int64

	// leaked counts test goroutines abandoned after their watchdog fired.
	leaked atomic.Int64

	errOnce sync.Once
	err     error
}

// runIndexes runs the given tests with at most workers running at once.
func (s *scheduler) runIndexes(ctx context.Context, tests []Test, idx []int, workers int) {
	if len(idx) == 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(idx) {
		workers = len(idx)
	}

	// A channel of work rather than a goroutine per test: this bounds the
	// number of in-flight HTTP connections to one server.
	queue := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				s.finish(i, s.runner.runOne(ctx, tests[i]))
			}
		}()
	}

	// Feeding and waiting both watch ctx: if every worker is wedged, a send
	// blocks forever and so does the wait. The stall detector and interrupt
	// handling can only end a run if the scheduler is willing to abandon its
	// own workers -- the harness must not be the thing that hangs.
feed:
	for _, i := range idx {
		select {
		case queue <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(queue)

	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-ctx.Done():
	}
}

// finish records a result and reports it. Observe is called under the lock so
// a caller sees one test at a time without needing its own synchronization.
func (s *scheduler) finish(i int, res Result) {
	s.progress.Store(time.Now().UnixNano())
	if res.Status == StatusTimeout {
		s.checkLeaked()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[i] = res
	s.done[i] = true
	if s.runner.Observe != nil {
		s.runner.Observe(res)
	}
}

// checkLeaked aborts the run once too many goroutines have been abandoned.
// Go cannot kill a goroutine, so each one that ignored its deadline holds its
// memory and its connection until the process exits.
func (s *scheduler) checkLeaked() {
	n := s.leaked.Add(1)
	if s.runner.MaxLeaked > 0 && n >= int64(s.runner.MaxLeaked) {
		s.fail(errors.Errorf("abandoned %d test goroutines, aborting", n))
	}
}

// watchStall aborts the run if no test finishes for StallTimeout.
//
// Per-test watchdogs already bound each test, so a stall means the scheduler
// itself is stuck. This is the difference between a CI job that fails with a
// stack trace and one that burns its whole time limit.
func (s *scheduler) watchStall(ctx context.Context, cancel context.CancelFunc) (stop func()) {
	interval := min(30*time.Second, s.runner.StallTimeout)
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				last := time.Unix(0, s.progress.Load())
				if time.Since(last) < s.runner.StallTimeout {
					continue
				}
				s.fail(errors.Errorf("no test finished in %s, aborting:\n%s",
					s.runner.StallTimeout, goroutineDump()))
				cancel()
				return
			}
		}
	}()
	return func() { close(done) }
}

// fail records the first breakdown; later ones are consequences of it.
func (s *scheduler) fail(err error) {
	s.errOnce.Do(func() { s.err = err })
}

// collect returns the results of tests that actually ran, in input order.
func (s *scheduler) collect() []Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Result, 0, len(s.results))
	for i, ok := range s.done {
		if ok {
			out = append(out, s.results[i])
		}
	}
	return out
}

// defaultWatchdogGrace is how long past its deadline a test is given before
// its goroutine is abandoned. A test that respects its context unwinds well
// within this; one that does not was never going to.
const defaultWatchdogGrace = 5 * time.Second

func (r Runner) grace() time.Duration {
	if r.WatchdogGrace > 0 {
		return r.WatchdogGrace
	}
	return defaultWatchdogGrace
}

func (r Runner) runOne(ctx context.Context, test Test) Result {
	ctx, cancel := context.WithCancel(ctx)
	if r.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
	}
	defer cancel()

	t := newT(ctx, test.Name)
	start := time.Now()

	status, abandoned := r.runBody(t, test)
	// Cleanups run even for an abandoned test: it is usually stuck on one
	// request rather than truly wedged, and its buckets still need removing.
	r.runCleanups(t)

	switch {
	case abandoned:
		status = StatusTimeout
	case status == StatusFailed && ctx.Err() != nil:
		// A test that failed only because its deadline expired is a
		// timeout: the server never answering and the server answering
		// wrongly are triaged differently.
		status = StatusTimeout
	case status == StatusPassed && t.Failed():
		// Cleanup can fail a test that had otherwise passed.
		status = StatusFailed
	}

	return Result{
		Test:     test,
		Status:   status,
		Duration: time.Since(start),
		Output:   t.output(),
		Skip:     t.skipMsg,
	}
}

// runBody runs a test under a watchdog.
//
// Context cancellation only works if the test respects it; one blocked on an
// unbounded read or a WaitGroup that never completes will ignore its deadline.
// So the worker does not wait for the body: past the grace period it reports
// the timeout and moves on, leaving the goroutine to finish whenever it can.
func (r Runner) runBody(t *T, test Test) (status Status, abandoned bool) {
	if r.Timeout <= 0 {
		return invoke(t, test.Fn), false
	}

	done := make(chan Status, 1)
	go func() { done <- invoke(t, test.Fn) }()

	timer := time.NewTimer(r.Timeout + r.grace())
	defer timer.Stop()

	select {
	case status := <-done:
		return status, false
	case <-timer.C:
		t.Errorf("test did not return %s after its deadline; abandoning its goroutine\n%s",
			r.grace(), goroutineDump())
		return StatusTimeout, true
	}
}

// runCleanups runs a test's cleanups under their own deadline, so a teardown
// that hangs cannot stall the worker that a timed-out test just freed.
func (r Runner) runCleanups(t *T) {
	if r.CleanupTimeout <= 0 {
		t.runCleanups()
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		t.runCleanups()
	}()

	timer := time.NewTimer(r.CleanupTimeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		t.Errorf("cleanups did not finish within %s", r.CleanupTimeout)
	}
}

// invoke runs a test body and converts how it ended into a status.
//
// A panic that is not Fatalf or Skipf is a bug in the test rather than a
// finding about the server, but it still has to be reported as a failure and
// must not take the run down.
func invoke(t *T, fn func(*T)) (status Status) {
	defer func() {
		switch r := recover(); r.(type) {
		case nil:
			if t.Failed() {
				status = StatusFailed
			} else {
				status = StatusPassed
			}
		case failNow:
			status = StatusFailed
		case skipNow:
			status = StatusSkipped
		default:
			t.Errorf("panic: %v\n%s", r, stack())
			status = StatusFailed
		}
	}()
	fn(t)
	return StatusPassed
}

// maxDumpLen caps a goroutine dump. Enough to see where things are stuck,
// little enough to stay readable in a CI log.
const maxDumpLen = 16 << 10

func goroutineDump() string { return dump(true) }

func stack() string { return dump(false) }

func dump(all bool) string {
	buf := make([]byte, maxDumpLen)
	return string(buf[:runtime.Stack(buf, all)])
}
