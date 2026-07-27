package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/config"
)

// testConfig returns a config with credentials set. They must be non-empty or
// the SDK fails before it ever reaches the network, which would make the
// timeout tests below pass without exercising a timeout at all.
func testConfig(endpoint string) *config.Config {
	return &config.Config{
		Endpoint: endpoint,
		Main:     config.User{AccessKey: "test", SecretKey: "secret"},
	}
}

// A server that accepts the connection and never answers is the failure mode
// http.Client.Timeout handles worst, and the one a broken S3 server is most
// likely to exhibit. ResponseHeaderTimeout is what catches it.
func TestResponseHeaderTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-block // accept, then never respond
	}))
	// Defers run last-in-first-out, so the handler must be released before
	// Close, which waits for outstanding handlers to return.
	defer srv.Close()
	defer close(block)

	f := NewWithTimeouts(testConfig(srv.URL), Timeouts{
		// Request is left long on purpose: the assertion is that the
		// response-header bound fires first.
		Request:        time.Minute,
		Dial:           time.Second,
		TLSHandshake:   time.Second,
		ResponseHeader: 50 * time.Millisecond,
	})

	start := time.Now()
	_, err := f.Main().ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err == nil {
		t.Fatal("request succeeded against a server that never responded")
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("request took %s; the response-header timeout did not fire", elapsed)
	}
	// Without a lower bound this test would pass on any instant failure,
	// including one that never reached the server.
	if elapsed < 50*time.Millisecond {
		t.Errorf("request failed after %s, before the timeout could fire: %v", elapsed, err)
	}
}

// The remaining bounds are asserted structurally rather than behaviorally.
// Provoking a real dial timeout needs an address that blackholes packets, and
// reserved ranges are reset by local networks often enough to make such a test
// flaky in CI -- which is worse than not having it.
func TestTimeoutsAreWired(t *testing.T) {
	want := Timeouts{
		Request:        11 * time.Second,
		Dial:           12 * time.Second,
		TLSHandshake:   13 * time.Second,
		ResponseHeader: 14 * time.Second,
	}
	f := NewWithTimeouts(testConfig("http://example.invalid"), want)

	hc := f.http
	if hc.Timeout != want.Request {
		t.Errorf("Timeout = %s, want %s", hc.Timeout, want.Request)
	}

	tr, ok := hc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", hc.Transport)
	}
	if tr.TLSHandshakeTimeout != want.TLSHandshake {
		t.Errorf("TLSHandshakeTimeout = %s, want %s", tr.TLSHandshakeTimeout, want.TLSHandshake)
	}
	if tr.ResponseHeaderTimeout != want.ResponseHeader {
		t.Errorf("ResponseHeaderTimeout = %s, want %s", tr.ResponseHeaderTimeout, want.ResponseHeader)
	}
	// The default of 2 would serialize concurrent tests onto two sockets and
	// silently cap throughput at any --parallel.
	if tr.MaxIdleConnsPerHost < 16 {
		t.Errorf("MaxIdleConnsPerHost = %d, too low for the worker pool", tr.MaxIdleConnsPerHost)
	}
}

// Retries must stay off: the suite asserts on 5xx and throttling responses, so
// a retry would turn an assertion into a delay or a silent pass.
func TestNoRetries(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	f := New(testConfig(srv.URL))
	if _, err := f.Main().ListBuckets(context.Background(), &s3.ListBucketsInput{}); err == nil {
		t.Fatal("a 503 was not reported as an error")
	}
	if attempts != 1 {
		t.Errorf("server saw %d attempts, want 1", attempts)
	}
}

// The suite asserts on status codes for successful calls too, which the SDK
// does not otherwise expose.
func TestStatusCaptured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets></Buckets></ListAllMyBucketsResult>`))
	}))
	defer srv.Close()

	f := New(testConfig(srv.URL))
	out, err := f.Main().ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if got := Status(out.ResultMetadata); got != 200 {
		t.Errorf("Status = %d, want 200", got)
	}
}

func TestStatusAndCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchBucket</Code><Message>nope</Message></Error>`))
	}))
	defer srv.Close()

	f := New(testConfig(srv.URL))
	_, err := f.Main().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("b"),
		Key:    aws.String("k"),
	})
	if err == nil {
		t.Fatal("HeadObject succeeded against a 404")
	}
	status, code := StatusAndCode(err)
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
	if code == "" {
		t.Error("no error code extracted")
	}
}

// A nil error must not look like a plausible zero value, or a test that
// expected a failure and got none compares against something that could match.
func TestStatusAndCodeNil(t *testing.T) {
	if status, code := StatusAndCode(nil); status != 0 || code != "" {
		t.Errorf("StatusAndCode(nil) = (%d, %q), want (0, \"\")", status, code)
	}
}
