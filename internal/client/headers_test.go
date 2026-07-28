package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// captureHeaders runs one PutObject and returns the headers the server saw.
func captureHeaders(t *testing.T, opts ...func(*s3.Options)) http.Header {
	t.Helper()

	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := New(testConfig(srv.URL))
	_, err := f.Main().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String("b"),
		Key:    aws.String("k"),
		Body:   strings.NewReader("body"),
	}, opts...)
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	return got
}

func TestWithHeaders(t *testing.T) {
	got := captureHeaders(t, WithHeaders(map[string]string{
		"x-amz-server-side-encryption-customer-algorithm": "AES256",
		"Content-Type": "text/plain",
	}))

	if v := got.Get("x-amz-server-side-encryption-customer-algorithm"); v != "AES256" {
		t.Errorf("sse-c algorithm header = %q, want AES256", v)
	}
	if v := got.Get("Content-Type"); v != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", v)
	}
}

// The signature has to cover an injected header, as it does in botocore, where
// before-call runs against the request dict and signing happens after. A
// header added afterwards would turn every test that sends a deliberately bad
// value into a signature mismatch.
func TestWithHeadersSigned(t *testing.T) {
	got := captureHeaders(t, WithHeaders(map[string]string{"x-amz-meta-injected": "v"}))

	if got.Get("x-amz-meta-injected") != "v" {
		t.Fatal("header was not sent")
	}
	if signed := got.Get("Authorization"); !strings.Contains(signed, "x-amz-meta-injected") {
		t.Errorf("SignedHeaders omits a header added before signing: %q", signed)
	}
}

func TestWithoutHeader(t *testing.T) {
	got := captureHeaders(t, WithoutHeader("Content-Type"))
	if v := got.Get("Content-Type"); v != "" {
		t.Errorf("Content-Type = %q, want it removed", v)
	}
}

// Options are per-call: a client used without them must be unaffected.
func TestHeaderOptionsDoNotLeak(t *testing.T) {
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(testConfig(srv.URL)).Main()
	in := &s3.PutObjectInput{Bucket: aws.String("b"), Key: aws.String("k")}

	if _, err := c.PutObject(context.Background(), in,
		WithHeaders(map[string]string{"x-amz-meta-once": "v"})); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if _, err := c.PutObject(context.Background(), in); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(seen))
	}
	if seen[1].Get("x-amz-meta-once") != "" {
		t.Error("a per-call header leaked into a later request")
	}
}

// The bucket name enters the path during endpoint resolution, which runs after
// the Build step. A rewrite placed there is a silent no-op, which would make
// every bucket-naming test pass vacuously by creating a validly-named bucket
// instead of the invalid one under test.
func TestWithPathReplace(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := New(testConfig(srv.URL))
	if _, err := f.Main().CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String("placeholder-name"),
	}, WithPathReplace("placeholder-name", "a")); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if gotPath != "/a" {
		t.Errorf("path = %q, want /a", gotPath)
	}
}
