package fixture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-faster/errors"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
)

func TestRetryThrottledSucceedsAfterSlowDown(t *testing.T) {
	var calls int
	err := retryThrottled(context.Background(), func() error {
		calls++
		if calls < 3 {
			return slowDownError(t)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryThrottled: %v", err)
	}
	if calls != 3 {
		t.Errorf("fn called %d times, want 3", calls)
	}
}

// Anything that is not throttling is returned at once: retrying a real failure
// would only delay the report.
func TestRetryThrottledDoesNotRetryOtherErrors(t *testing.T) {
	var calls int
	want := errors.New("no such bucket")
	err := retryThrottled(context.Background(), func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

func TestRetryThrottledStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	if err := retryThrottled(ctx, func() error {
		calls++
		return slowDownError(t)
	}); err == nil {
		t.Fatal("retryThrottled returned nil for a canceled context")
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

func TestThrottledDetection(t *testing.T) {
	if throttled(nil) {
		t.Error("throttled(nil) = true")
	}
	if throttled(errors.New("boom")) {
		t.Error("a plain error was treated as throttling")
	}
	if !throttled(slowDownError(t)) {
		t.Error("a 503 SlowDown was not treated as throttling")
	}
}

// slowDownError produces a real SDK error rather than a hand-built one, so the
// detection is tested against what the client actually returns.
func slowDownError(t *testing.T) error {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<Error><Code>SlowDown</Code><Message>slow down</Message></Error>`))
	}))
	defer srv.Close()

	f := client.New(&config.Config{
		Endpoint: srv.URL,
		Main:     config.User{AccessKey: "test", SecretKey: "secret"},
	})
	_, err := f.Main().ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err == nil {
		t.Fatal("expected an error from a 503 response")
	}
	return err
}
