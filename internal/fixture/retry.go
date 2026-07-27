package fixture

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/go-faster/s3t/internal/client"
)

// retryBudget bounds how long setup or teardown will keep retrying. Short
// enough that a genuinely broken server fails the test quickly rather than
// stretching the run.
const retryBudget = 30 * time.Second

// throttled reports whether an error is the server asking for less load.
func throttled(err error) bool {
	status, code := client.StatusAndCode(err)
	return status == 503 || code == "SlowDown" || code == "ServiceUnavailable"
}

// retryThrottled runs fn until it succeeds, fails for a reason other than
// throttling, or the budget runs out.
//
// Only fixture work uses this -- creating and removing buckets. The tests
// themselves never retry, because the suite asserts on 503 and throttling
// responses and a retry would turn those assertions into silent passes. That
// distinction is the whole reason this lives here rather than in the client's
// retryer.
func retryThrottled(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(retryBudget)
	delay := 50 * time.Millisecond

	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil || !throttled(err) || time.Now().After(deadline) {
			return err
		}

		// Exponential backoff with jitter: without it, workers that were
		// throttled together retry together and throttle each other again.
		wait := delay + rand.N(delay) //nolint:gosec // jitter, not secrecy
		if delay < 4*time.Second {
			delay *= 2
		}

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return err
		}
	}
}
