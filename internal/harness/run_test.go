package harness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, r Runner, fn func(*T)) Result {
	t.Helper()
	results := r.Run(context.Background(), []Test{{Name: "probe", Module: ModuleS3, Fn: fn}})
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

func TestRunTimeout(t *testing.T) {
	res := run(t, Runner{Timeout: 10 * time.Millisecond}, func(t *T) {
		<-t.Ctx().Done()
		t.Fatalf("context expired")
	})
	if res.Status != StatusTimeout {
		t.Errorf("status = %q, want %q", res.Status, StatusTimeout)
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

	body := func(*T) { t.Fatal("test ran despite a canceled context") }
	results := Runner{}.Run(ctx, []Test{
		{Name: "a", Module: ModuleS3, Fn: body},
		{Name: "b", Module: ModuleS3, Fn: body},
	})
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestRunObserve(t *testing.T) {
	var seen []string
	r := Runner{Observe: func(res Result) { seen = append(seen, res.Test.Name) }}
	r.Run(context.Background(), []Test{
		{Name: "a", Module: ModuleS3, Fn: func(*T) {}},
		{Name: "b", Module: ModuleS3, Fn: func(*T) {}},
	})
	if strings.Join(seen, ",") != "a,b" {
		t.Errorf("observed %v, want a,b", seen)
	}
}
