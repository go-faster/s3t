package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Status is the outcome of a test.
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
	// StatusTimeout is distinct from StatusFailed: a test that ran out of
	// time tells you something different about the server than one that
	// asserted and lost, and the two are triaged differently.
	StatusTimeout Status = "timeout"
)

// failNow and skipNow unwind a test body via panic, the way testing.T does.
// They are recovered by the runner and never escape.
type failNow struct{}

type skipNow struct{}

// T is the handle a test body uses to report results, mirroring the subset of
// testing.T that the ported suite needs.
//
// Every method is safe to call from multiple goroutines: the atomic-write
// tests race concurrent readers and writers against one key and report from
// all of them.
type T struct {
	name string
	ctx  context.Context

	mu       sync.Mutex
	logs     []string
	failed   bool
	skipped  bool
	skipMsg  string
	cleanups []func()
}

func newT(ctx context.Context, name string) *T {
	return &T{name: name, ctx: ctx}
}

// Name returns the test name, e.g. "bucket_list_empty".
func (t *T) Name() string { return t.name }

// Ctx returns the test context. It is canceled when the test's deadline
// expires or the run is interrupted, so every request made through it is
// bounded without the test having to think about it.
func (t *T) Ctx() context.Context { return t.ctx }

// Logf records a message shown only if the test fails, unless the runner is
// in verbose mode.
func (t *T) Logf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs = append(t.logs, fmt.Sprintf(format, args...))
}

// Errorf marks the test failed and continues.
func (t *T) Errorf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.logs = append(t.logs, fmt.Sprintf(format, args...))
}

// Fatalf marks the test failed and stops it immediately.
//
// It must be called from the goroutine running the test body; from any other
// goroutine it would unwind the wrong stack, so use Errorf there.
func (t *T) Fatalf(format string, args ...any) {
	t.Errorf(format, args...)
	panic(failNow{})
}

// Skipf stops the test and records it as skipped. Used where upstream skips
// on unconfigured optional features, not to paper over failures.
func (t *T) Skipf(format string, args ...any) {
	t.mu.Lock()
	t.skipped = true
	t.skipMsg = fmt.Sprintf(format, args...)
	t.mu.Unlock()
	panic(skipNow{})
}

// Cleanup registers a function to run when the test ends, in reverse order of
// registration. Cleanups run whether the test passed, failed, or skipped.
func (t *T) Cleanup(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanups = append(t.cleanups, fn)
}

// Failed reports whether the test has been marked failed.
func (t *T) Failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}

// output returns the accumulated log lines as a single string.
func (t *T) output() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.logs, "\n")
}

// runCleanups runs registered cleanups in LIFO order, isolating each from the
// others: one panicking cleanup must not strand the rest, or a single bad
// teardown leaks every bucket after it.
func (t *T) runCleanups() {
	t.mu.Lock()
	fns := t.cleanups
	t.cleanups = nil
	t.mu.Unlock()

	for i := len(fns) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("cleanup panicked: %v", r)
				}
			}()
			fns[i]()
		}()
	}
}
