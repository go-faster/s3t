// Package harness runs the S3 compatibility suite.
//
// It replaces pytest: test packages return their tests as values, the runner
// selects a subset and executes it, and results are reported in a machine
// readable form. Test names match ceph/s3-tests exactly so results from the
// two suites can be compared directly.
package harness

import (
	"fmt"
	"slices"
	"sort"
)

// Test is a single compatibility test.
type Test struct {
	// Name matches the upstream Python function name without the "test_"
	// prefix, e.g. "bucket_list_empty" for test_bucket_list_empty.
	Name string

	// Module is the upstream file the test came from, e.g.
	// "s3tests/functional/test_s3.py". It exists so pytest node IDs in an
	// allow-list can be validated rather than merely matched on name.
	Module string

	// Markers mirror the upstream pytest markers: "fails_on_aws",
	// "lifecycle", and so on. The runner selects on them but never skips on
	// them by itself; they are backend metadata, not instructions.
	Markers []string

	// Fn is the test body.
	Fn func(*T)
}

// Upstream module paths. Tests reference these instead of repeating literals,
// so a typo cannot silently create a module that no allow-list will match.
const (
	ModuleS3      = "s3tests/functional/test_s3.py"
	ModuleHeaders = "s3tests/functional/test_headers.py"
)

// NodeID returns the pytest node ID for the test, the form used in allow-list
// files: "s3tests/functional/test_s3.py::test_bucket_list_empty".
func (t Test) NodeID() string {
	return t.Module + "::test_" + t.Name
}

// Marker reports whether the test carries the given marker.
func (t Test) Marker(name string) bool {
	return slices.Contains(t.Markers, name)
}

// Registry is a validated, name-unique set of tests.
//
// Test packages return plain slices; the runner collects them into a Registry,
// which is where duplicate names and malformed entries are caught. Keeping
// this a value rather than package state means a test of the harness can build
// one in isolation, and there is no import-order dependency to reason about.
type Registry struct {
	byName map[string]Test
}

// NewRegistry validates tests and collects them.
//
// An empty field or an unknown module is a bug in the suite, as is a duplicate
// name: every one of them would otherwise surface as a test that silently
// never runs, or as two tests fighting over one result.
func NewRegistry(tests []Test) (*Registry, error) {
	r := &Registry{byName: make(map[string]Test, len(tests))}
	for _, t := range tests {
		if t.Name == "" {
			return nil, fmt.Errorf("test in %s has no name", t.Module)
		}
		if t.Fn == nil {
			return nil, fmt.Errorf("test %q has no body", t.Name)
		}
		switch t.Module {
		case ModuleS3, ModuleHeaders:
		default:
			return nil, fmt.Errorf("test %q has unknown module %q", t.Name, t.Module)
		}
		if _, ok := r.byName[t.Name]; ok {
			return nil, fmt.Errorf("duplicate test %q", t.Name)
		}
		r.byName[t.Name] = t
	}
	return r, nil
}

// Len returns the number of tests.
func (r *Registry) Len() int { return len(r.byName) }

// All returns every test, sorted by name.
func (r *Registry) All() []Test {
	out := make([]Test, 0, len(r.byName))
	for _, t := range r.byName {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup returns the test with the given name.
func (r *Registry) Lookup(name string) (Test, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// MarkerCount is a marker and the number of tests carrying it.
type MarkerCount struct {
	Name  string
	Count int
}

// Markers returns every marker in the registry with its test count, sorted by
// name.
func (r *Registry) Markers() []MarkerCount {
	counts := map[string]int{}
	for _, t := range r.byName {
		for _, m := range t.Markers {
			counts[m]++
		}
	}
	out := make([]MarkerCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, MarkerCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
