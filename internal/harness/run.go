package harness

import (
	"context"
	"time"
)

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
// It is sequential: the worker pool, watchdog and stall detection come with
// the concurrency work, and the shape below is what they will wrap.
type Runner struct {
	// Timeout bounds a single test. Zero means no bound.
	Timeout time.Duration

	// Observe, if set, is called as each test finishes, so a caller can
	// report progress without waiting for the whole run.
	Observe func(Result)
}

// Run executes tests in order and returns their results. It stops early if ctx
// is done, returning the results collected so far.
func (r Runner) Run(ctx context.Context, tests []Test) []Result {
	results := make([]Result, 0, len(tests))
	for _, test := range tests {
		if ctx.Err() != nil {
			break
		}
		res := r.runOne(ctx, test)
		if r.Observe != nil {
			r.Observe(res)
		}
		results = append(results, res)
	}
	return results
}

func (r Runner) runOne(ctx context.Context, test Test) Result {
	ctx, cancel := context.WithCancel(ctx)
	if r.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
	}
	defer cancel()

	t := newT(ctx, test.Name)
	start := time.Now()
	status := invoke(t, test.Fn)
	// Cleanups run before the duration is taken: emptying a bucket is part
	// of what a test costs, and hiding it makes slow teardowns invisible.
	t.runCleanups()

	// A test that failed only because its deadline expired is reported as a
	// timeout, since "the server never answered" and "the server answered
	// wrongly" are triaged differently.
	if status == StatusFailed && ctx.Err() != nil {
		status = StatusTimeout
	}
	// Cleanup can fail a test that had otherwise passed.
	if status == StatusPassed && t.Failed() {
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
			t.Errorf("panic: %v", r)
			status = StatusFailed
		}
	}()
	fn(t)
	return StatusPassed
}
