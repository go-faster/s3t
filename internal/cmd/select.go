package cmd

import (
	"os"
	"regexp"

	"github.com/go-faster/errors"
	"github.com/spf13/pflag"

	"github.com/go-faster/s3t/internal/harness"
)

// selection holds the flags that narrow a run, shared by every subcommand so
// `s3t list -m lifecycle` previews exactly what `s3t run -m lifecycle` runs.
type selection struct {
	run       string
	markers   string
	allowList string
}

func (s *selection) bind(f *pflag.FlagSet) {
	f.StringVarP(&s.run, "run", "k", "", "run only tests whose name matches this regular expression")
	f.StringVarP(&s.markers, "markers", "m", "",
		`run only tests matching a marker expression, e.g. 'lifecycle and not fails_on_aws'`)
	f.StringVar(&s.allowList, "allow-list", "",
		"file of pytest node IDs to run, one per line; '#' comments are ignored")
}

// resolve turns the flags into a list of tests.
func (s *selection) resolve(r *harness.Registry) ([]harness.Test, error) {
	var sel harness.Selection
	var err error

	if s.run != "" {
		if sel.Run, err = regexp.Compile(s.run); err != nil {
			return nil, errors.Wrap(err, "compile --run")
		}
	}
	if sel.Markers, err = harness.ParseMarkerExpr(s.markers); err != nil {
		return nil, errors.Wrap(err, "parse --markers")
	}
	if s.allowList != "" {
		if sel.AllowList, err = readAllowList(s.allowList); err != nil {
			return nil, err
		}
	}
	return r.Select(sel)
}

func readAllowList(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // caller-supplied path
	if err != nil {
		return nil, errors.Wrap(err, "open allow-list")
	}
	defer func() { _ = f.Close() }()

	ids, err := harness.ParseAllowList(f)
	if err != nil {
		return nil, errors.Wrap(err, "parse allow-list")
	}
	return ids, nil
}

// readKnownFailures loads a known-failures file.
func readKnownFailures(path string, r *harness.Registry) (*harness.KnownFailures, error) {
	f, err := os.Open(path) //nolint:gosec // caller-supplied path
	if err != nil {
		return nil, errors.Wrap(err, "open known-failures")
	}
	defer func() { _ = f.Close() }()

	known, err := harness.ParseKnownFailures(f, r)
	if err != nil {
		return nil, errors.Wrap(err, "parse known-failures")
	}
	return known, nil
}
