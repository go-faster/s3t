package harness

import (
	"bufio"
	"io"
	"regexp"
	"strings"

	"github.com/go-faster/errors"
)

// Selection narrows a registry down to the tests a run should execute. A zero
// Selection selects everything.
type Selection struct {
	// Run matches against test names, like pytest's -k.
	Run *regexp.Regexp

	// Markers is a compiled -m expression.
	Markers MarkerExpr

	// AllowList is a list of pytest node IDs. When non-empty only these
	// tests are selected, and every entry must resolve.
	AllowList []string
}

// Select returns the matching tests, sorted by name.
//
// The three criteria intersect rather than override each other, so
// `--allow-list x.txt -m 'not fails_on_aws'` means what it looks like.
func (r *Registry) Select(s Selection) ([]Test, error) {
	candidates := r.All()

	if len(s.AllowList) > 0 {
		var err error
		if candidates, err = r.resolve(s.AllowList); err != nil {
			return nil, err
		}
	}

	out := make([]Test, 0, len(candidates))
	for _, t := range candidates {
		if s.Run != nil && !s.Run.MatchString(t.Name) {
			continue
		}
		if !s.Markers.Match(t.Markers) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// resolve maps pytest node IDs onto registered tests.
//
// An entry that does not resolve is an error, not a silent skip: an allow-list
// is a gate, and a gate that quietly stops covering a renamed test is worse
// than one that fails loudly.
func (r *Registry) resolve(nodeIDs []string) ([]Test, error) {
	out := make([]Test, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		module, name, ok := strings.Cut(id, "::")
		if !ok {
			return nil, errors.Errorf("allow-list entry %q is not a pytest node ID", id)
		}
		t, found := r.Lookup(strings.TrimPrefix(name, "test_"))
		if !found {
			return nil, errors.Errorf("allow-list entry %q names no known test", id)
		}
		if t.Module != module {
			return nil, errors.Errorf("allow-list entry %q is in %s, not %s", id, t.Module, module)
		}
		out = append(out, t)
	}
	return out, nil
}

// ParseAllowList reads pytest node IDs, one per line.
//
// Blank lines and '#' comments are ignored, and a trailing comment is stripped
// from a line, matching how go-faster/fs feeds this file to pytest today.
func ParseAllowList(r io.Reader) ([]string, error) {
	ids, err := parseNodeIDs(r)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("allow-list is empty")
	}
	return ids, nil
}

// parseNodeIDs reads node IDs, one per line, ignoring blanks and comments.
func parseNodeIDs(r io.Reader) ([]string, error) {
	var ids []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			ids = append(ids, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, errors.Wrap(err, "read node IDs")
	}
	return ids, nil
}

// nodeIDName extracts the test name from a pytest node ID.
func nodeIDName(id string) (string, bool) {
	_, name, ok := strings.Cut(id, "::")
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(name, "test_"), true
}
