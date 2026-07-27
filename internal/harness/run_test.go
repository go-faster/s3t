package harness

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func run(t *testing.T, r Runner, fn func(*T)) Result {
	t.Helper()
	results, err := r.Run(context.Background(), []Test{{Name: "probe", Module: ModuleS3, Fn: fn}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	return results[0]
}

func TestRunStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(*T)
		want Status
	}{
		{"pass", func(*T) {}, StatusPassed},
		{"errorf", func(t *T) { t.Errorf("bad") }, StatusFailed},
		{"fatalf", func(t *T) { t.Fatalf("bad") }, StatusFailed},
		{"skip", func(t *T) { t.Skipf("not configured") }, StatusSkipped},
		// A panic is a bug in the test, but it must be reported rather
		// than take the run down.
		{"panic", func(*T) { panic("boom") }, StatusFailed},
		// A cleanup can fail a test that had otherwise passed.
		{"cleanup fails", func(t *T) { t.Cleanup(func() { t.Errorf("teardown") }) }, StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, Runner{}, tc.fn).Status; got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunFatalfStopsBody(t *testing.T) {
	reached := false
	run(t, Runner{}, func(t *T) {
		t.Fatalf("stop here")
		reached = true
	})
	if reached {
		t.Error("Fatalf did not stop the test body")
	}
}

func TestRunCleanupsAreLIFO(t *testing.T) {
	var order []string
	run(t, Runner{}, func(t *T) {
		t.Cleanup(func() { order = append(order, "first") })
		t.Cleanup(func() { order = append(order, "second") })
	})
	if strings.Join(order, ",") != "second,first" {
		t.Errorf("cleanup order = %v, want second,first", order)
	}
}

// One panicking cleanup must not strand the others, or a single bad teardown
// leaks every bucket registered before it.
func TestRunCleanupPanicDoesNotStrandOthers(t *testing.T) {
	ran := false
	res := run(t, Runner{}, func(t *T) {
		t.Cleanup(func() { ran = true })
		t.Cleanup(func() { panic("teardown boom") })
	})
	if !ran {
		t.Error("a panicking cleanup stopped the remaining cleanups")
	}
	if res.Status != StatusFailed {
		t.Errorf("status = %q, want %q", res.Status, StatusFailed)
	}
}

func TestRunSkipReason(t *testing.T) {
	res := run(t, Runner{}, func(t *T) { t.Skipf("no kms configured") })
	if res.Skip != "no kms configured" {
		t.Errorf("skip reason = %q", res.Skip)
	}
}

func TestRunStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	body := func(*T) { t.Error("test ran despite a canceled context") }
	results, err := Runner{}.Run(ctx, []Test{
		{Name: "a", Module: ModuleS3, Fn: body},
		{Name: "b", Module: ModuleS3, Fn: body},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestRunObserve(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	r := Runner{Observe: func(res Result) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, res.Test.Name)
	}}
	if _, err := r.Run(context.Background(), []Test{
		{Name: "a", Module: ModuleS3, Fn: func(*T) {}},
		{Name: "b", Module: ModuleS3, Fn: func(*T) {}},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Join(seen, ",") != "a,b" {
		t.Errorf("observed %v, want a,b", seen)
	}
}

// A test that respects its context is unwound by the deadline and reported as
// a timeout, not a failure.
func TestRunTimeout(t *testing.T) {
	res := run(t, Runner{Timeout: 10 * time.Millisecond}, func(t *T) {
		<-t.Ctx().Done()
		t.Fatalf("context expired")
	})
	if res.Status != StatusTimeout {
		t.Errorf("status = %q, want %q", res.Status, StatusTimeout)
	}
}

// The fault that matters: a test that ignores its context entirely. The worker
// must not wait for it, because Go cannot kill a goroutine and blocking here
// would let one wedged test stall the whole run.
func TestRunAbandonsWedgedTest(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	res := run(t, Runner{
		Timeout:       10 * time.Millisecond,
		WatchdogGrace: 20 * time.Millisecond,
	}, func(*T) {
		<-release // never returns while the test runs
	})

	if res.Status != StatusTimeout {
		t.Errorf("status = %q, want %q", res.Status, StatusTimeout)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("run took %s; the worker waited for the wedged goroutine", elapsed)
	}
	if !strings.Contains(res.Output, "abandoning its goroutine") {
		t.Errorf("output does not explain the abandonment: %q", res.Output)
	}
	// The dump is what makes a hung test diagnosable from a CI log.
	if !strings.Contains(res.Output, "goroutine") {
		t.Error("output carries no goroutine dump")
	}
}

// A wedged test must not stop the ones after it.
func TestRunContinuesAfterWedgedTest(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	results, err := Runner{
		Timeout:       10 * time.Millisecond,
		WatchdogGrace: 20 * time.Millisecond,
	}.Run(context.Background(), []Test{
		{Name: "wedged", Module: ModuleS3, Fn: func(*T) { <-release }},
		{Name: "healthy", Module: ModuleS3, Fn: func(*T) {}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[1].Status != StatusPassed {
		t.Errorf("test after the wedged one = %q, want %q", results[1].Status, StatusPassed)
	}
}

// Abandoned goroutines hold memory and a connection until the process exits,
// so past a threshold the run gives up rather than leaking without bound.
func TestRunAbortsOnTooManyLeaks(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	wedged := func(*T) { <-release }
	_, err := Runner{
		Timeout:       10 * time.Millisecond,
		WatchdogGrace: 10 * time.Millisecond,
		MaxLeaked:     2,
	}.Run(context.Background(), []Test{
		{Name: "a", Module: ModuleS3, Fn: wedged},
		{Name: "b", Module: ModuleS3, Fn: wedged},
		{Name: "c", Module: ModuleS3, Fn: wedged},
	})
	if err == nil {
		t.Fatal("Run did not abort after exceeding MaxLeaked")
	}
	if !strings.Contains(err.Error(), "abandoned") {
		t.Errorf("error = %v, want it to mention abandoned goroutines", err)
	}
}

// Cleanups still run for an abandoned test: it is usually stuck on one request
// rather than truly wedged, and its buckets still need removing.
func TestRunCleansUpAbandonedTest(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	cleaned := make(chan struct{})
	run(t, Runner{
		Timeout:       10 * time.Millisecond,
		WatchdogGrace: 10 * time.Millisecond,
	}, func(t *T) {
		t.Cleanup(func() { close(cleaned) })
		<-release
	})

	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Error("cleanups did not run for an abandoned test")
	}
}

// A teardown that hangs must not stall the worker the timed-out test freed.
func TestRunCleanupTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	res := run(t, Runner{CleanupTimeout: 20 * time.Millisecond}, func(t *T) {
		t.Cleanup(func() { <-release })
	})

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("run took %s; the worker waited for the hung cleanup", elapsed)
	}
	if !strings.Contains(res.Output, "cleanups did not finish") {
		t.Errorf("output does not report the hung cleanup: %q", res.Output)
	}
}

// The backstop for a bug in the scheduler itself: without it, a wedged run
// burns the whole CI time limit instead of failing with a stack trace.
func TestRunStallDetector(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	_, err := Runner{
		StallTimeout: 20 * time.Millisecond,
		// No per-test timeout, so only the stall detector can end this.
	}.Run(context.Background(), []Test{
		{Name: "wedged", Module: ModuleS3, Fn: func(*T) { <-release }},
	})
	if err == nil {
		t.Fatal("the stall detector did not fire")
	}
	if !strings.Contains(err.Error(), "no test finished") {
		t.Errorf("error = %v, want it to report a stall", err)
	}
}

func TestRunParallel(t *testing.T) {
	const n = 8

	var inFlight, peak atomic.Int64
	tests := make([]Test, n)
	for i := range tests {
		tests[i] = Test{Name: string(rune('a' + i)), Module: ModuleS3, Fn: func(*T) {
			cur := inFlight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
		}}
	}

	results, err := Runner{Parallel: n}.Run(context.Background(), tests)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != n {
		t.Fatalf("got %d results, want %d", len(results), n)
	}
	if peak.Load() < 2 {
		t.Errorf("peak concurrency = %d; tests did not run in parallel", peak.Load())
	}
}

// The pool bounds in-flight work: against a single server, unbounded
// concurrency turns into throttling and failures for the wrong reason.
func TestRunParallelIsBounded(t *testing.T) {
	const workers = 2

	var inFlight, peak atomic.Int64
	tests := make([]Test, 8)
	for i := range tests {
		tests[i] = Test{Name: string(rune('a' + i)), Module: ModuleS3, Fn: func(*T) {
			cur := inFlight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			inFlight.Add(-1)
		}}
	}

	if _, err := (Runner{Parallel: workers}).Run(context.Background(), tests); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if peak.Load() > workers {
		t.Errorf("peak concurrency = %d, want at most %d", peak.Load(), workers)
	}
}

// Serial tests assume no other load, so they must not overlap anything.
func TestRunSerialTestsRunAlone(t *testing.T) {
	var inFlight atomic.Int64
	var overlapped atomic.Bool

	body := func(*T) {
		if inFlight.Add(1) > 1 {
			overlapped.Store(true)
		}
		time.Sleep(10 * time.Millisecond)
		inFlight.Add(-1)
	}

	tests := []Test{
		{Name: "serial_a", Module: ModuleS3, Markers: []string{MarkerSerial}, Fn: body},
		{Name: "serial_b", Module: ModuleS3, Markers: []string{MarkerSerial}, Fn: body},
	}
	if _, err := (Runner{Parallel: 8}).Run(context.Background(), tests); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if overlapped.Load() {
		t.Error("serial tests ran concurrently")
	}
}

// Results follow the input order regardless of the order they finished in, so
// a report is deterministic.
func TestRunResultsKeepInputOrder(t *testing.T) {
	tests := []Test{
		{Name: "slow", Module: ModuleS3, Fn: func(*T) { time.Sleep(30 * time.Millisecond) }},
		{Name: "fast", Module: ModuleS3, Fn: func(*T) {}},
	}
	results, err := Runner{Parallel: 2}.Run(context.Background(), tests)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 || results[0].Test.Name != "slow" || results[1].Test.Name != "fast" {
		t.Errorf("results out of order: %v", results)
	}
}
