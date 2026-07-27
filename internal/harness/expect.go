package harness

import (
	"io"

	"github.com/go-faster/errors"
)

// KnownFailures is a set of tests expected to fail, read from a file of pytest
// node IDs.
//
// It inverts an allow-list: the whole suite runs, anything outside the set must
// pass, and anything inside it must fail. The second half is what keeps the
// file honest — a fixed test shows up as an unexpected pass on the change that
// fixed it, so the list can only shrink.
type KnownFailures struct {
	names map[string]bool

	// unknown counts entries naming tests this binary does not have. The
	// port is a subset of upstream, so these are expected; they are counted
	// rather than rejected, and reported so the number cannot drift
	// unnoticed.
	unknown int
}

// ParseKnownFailures reads node IDs, one per line, ignoring blank lines and
// '#' comments.
func ParseKnownFailures(r io.Reader, reg *Registry) (*KnownFailures, error) {
	ids, err := parseNodeIDs(r)
	if err != nil {
		return nil, err
	}

	kf := &KnownFailures{names: make(map[string]bool, len(ids))}
	for _, id := range ids {
		name, ok := nodeIDName(id)
		if !ok {
			return nil, errors.Errorf("known-failures entry %q is not a pytest node ID", id)
		}
		if _, found := reg.Lookup(name); !found {
			kf.unknown++
			continue
		}
		kf.names[name] = true
	}
	return kf, nil
}

// Expected reports whether a test is expected to fail.
func (k *KnownFailures) Expected(name string) bool {
	if k == nil {
		return false
	}
	return k.names[name]
}

// Len returns how many expected failures name a test this binary has.
func (k *KnownFailures) Len() int {
	if k == nil {
		return 0
	}
	return len(k.names)
}

// Unknown returns how many entries named a test this binary does not have.
func (k *KnownFailures) Unknown() int {
	if k == nil {
		return 0
	}
	return k.unknown
}

// Classify converts a raw result into its reported status given the expected
// failures.
//
// A failure that was expected becomes StatusExpectedFailure and does not fail
// the run. A pass that was expected to fail becomes StatusUnexpectedPass and
// does: the fix and the list entry belong in the same change.
//
// Skips are left alone. A test that skipped did not demonstrate anything, so
// calling it an unexpected pass would be wrong.
func (k *KnownFailures) Classify(res Result) Result {
	if !k.Expected(res.Test.Name) {
		return res
	}
	switch res.Status {
	case StatusFailed, StatusTimeout:
		res.Status = StatusExpectedFailure
	case StatusPassed:
		res.Status = StatusUnexpectedPass
	case StatusSkipped, StatusExpectedFailure, StatusUnexpectedPass:
	}
	return res
}
