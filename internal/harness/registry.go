// Package harness runs the S3 compatibility suite.
//
// It replaces pytest: tests register themselves at init time, the runner
// selects a subset and executes it, and results are reported in a machine
// readable form. Test names match ceph/s3-tests exactly so results from the
// two suites can be compared directly.
package harness

import (
	"fmt"
	"sort"
	"sync"
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

var (
	mu       sync.Mutex
	registry = map[string]Test{}
)

// Register adds a test to the global registry.
//
// It panics on a duplicate name, an empty field, or an unknown module: these
// are all programming errors in the suite itself, and every one of them would
// otherwise show up as a test that silently never runs.
func Register(t Test) {
	if t.Name == "" {
		panic("harness: test has no name")
	}
	if t.Fn == nil {
		panic(fmt.Sprintf("harness: test %q has no body", t.Name))
	}
	switch t.Module {
	case ModuleS3, ModuleHeaders:
	default:
		panic(fmt.Sprintf("harness: test %q has unknown module %q", t.Name, t.Module))
	}

	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[t.Name]; ok {
		panic(fmt.Sprintf("harness: duplicate test %q", t.Name))
	}
	registry[t.Name] = t
}

// All returns every registered test, sorted by name.
func All() []Test {
	mu.Lock()
	defer mu.Unlock()

	out := make([]Test, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// NodeID returns the pytest node ID for the test, the form used in allow-list
// files: "s3tests/functional/test_s3.py::test_bucket_list_empty".
func (t Test) NodeID() string {
	return t.Module + "::test_" + t.Name
}

// Markers returns every registered marker with the number of tests carrying
// it, sorted by name.
func Markers() []struct {
	Name  string
	Count int
} {
	counts := map[string]int{}
	for _, t := range All() {
		for _, m := range t.Markers {
			counts[m]++
		}
	}

	out := make([]struct {
		Name  string
		Count int
	}, 0, len(counts))
	for name, count := range counts {
		out = append(out, struct {
			Name  string
			Count int
		}{name, count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
