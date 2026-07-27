// Package s3util holds assertions and helpers shared by ported tests.
package s3util

import (
	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/harness"
)

// NoError fails the test if err is non-nil. op names the operation, so the
// failure reads "put object: <error>".
func NoError(t *harness.T, err error, op string) {
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

// Equal fails the test if got != want.
func Equal[T comparable](t *harness.T, got, want T, what string) {
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// EqualNow is Equal, but stops the test rather than continuing with a value
// known to be wrong.
func EqualNow[T comparable](t *harness.T, got, want T, what string) {
	if got != want {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// ErrorIs fails the test unless err is an S3 error with the given HTTP status
// and error code, mirroring upstream's
//
//	status, error_code = _get_status_and_error_code(e.response)
//	assert status == 404
//	assert error_code == 'NoSuchKey'
//
// A nil error is reported as such rather than as a mismatch, since "the call
// unexpectedly succeeded" and "the call failed differently" are different
// bugs.
func ErrorIs(t *harness.T, err error, status int, code string) {
	if err == nil {
		t.Fatalf("expected error with status %d and code %s, got success", status, code)
	}
	gotStatus, gotCode := client.StatusAndCode(err)
	if gotStatus != status || gotCode != code {
		t.Errorf("error = status %d code %s, want status %d code %s (%v)",
			gotStatus, gotCode, status, code, err)
	}
}
