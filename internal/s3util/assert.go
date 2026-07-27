// Package s3util holds assertions and helpers shared by ported tests.
package s3util

import (
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/harness"
)

// StatusAndCode re-exports the client helper so tests and assertions share one
// way of reading an S3 error.
func StatusAndCode(err error) (status int, code string) { return client.StatusAndCode(err) }

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

// EqualStrings fails the test unless got and want hold the same strings in the
// same order. Order matters: S3 listings are lexicographically sorted and the
// upstream assertions compare lists directly.
func EqualStrings(t *harness.T, got, want []string, what string) {
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

// EqualMetadata fails the test unless the two metadata maps match.
//
// S3 metadata keys are case-insensitive and SDKs normalize them differently,
// so keys are compared lowercased rather than byte for byte.
func EqualMetadata(t *harness.T, got, want map[string]string, what string) {
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
		return
	}
	lower := make(map[string]string, len(got))
	for k, v := range got {
		lower[strings.ToLower(k)] = v
	}
	for k, v := range want {
		if lower[strings.ToLower(k)] != v {
			t.Errorf("%s = %v, want %v", what, got, want)
			return
		}
	}
}

// ErrorIsOneOf fails the test unless err is an S3 error with the given code and
// one of the given statuses.
//
// Upstream uses this where implementations legitimately disagree on which
// status to return, so pinning a single one would fail a correct server.
func ErrorIsOneOf(t *harness.T, err error, statuses []int, code string) {
	if err == nil {
		t.Fatalf("expected error with code %s, got success", code)
	}
	gotStatus, gotCode := StatusAndCode(err)
	if !slices.Contains(statuses, gotStatus) || gotCode != code {
		t.Errorf("error = status %d code %s, want one of %v with code %s (%v)",
			gotStatus, gotCode, statuses, code, err)
	}
}

// ErrorStatus fails the test unless err is an S3 error with the given HTTP
// status, without checking the error code. Used where the response carries no
// body to put a code in, as with HEAD.
func ErrorStatus(t *harness.T, err error, status int) {
	if err == nil {
		t.Fatalf("expected error with status %d, got success", status)
	}
	if got, _ := StatusAndCode(err); got != status {
		t.Errorf("error = status %d, want %d (%v)", got, status, err)
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
	gotStatus, gotCode := StatusAndCode(err)
	if gotStatus != status || gotCode != code {
		t.Errorf("error = status %d code %s, want status %d code %s (%v)",
			gotStatus, gotCode, status, code, err)
	}
}

// RandomString returns n printable characters, mirroring upstream's
// _generate_random_string. The content only needs to be incompressible enough
// to make a range read meaningful, not unpredictable.
func RandomString(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.IntN(len(alphabet))] //nolint:gosec // test payload
	}
	return string(b)
}
