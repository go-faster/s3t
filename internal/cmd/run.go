package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/go-faster/s3t/internal/harness"
)

// errNoConfig is returned when no config file was given. Running needs one;
// listing does not.
var errNoConfig = errors.New(
	"no config file: pass --config or set S3TEST_CONF (see s3tests.conf.SAMPLE)")

func cmdRun(sel *selection, cfgPath *string) *cobra.Command {
	var (
		timeout time.Duration
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the selected tests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, cfg, _, err := registry(*cfgPath)
			if err != nil {
				return err
			}
			tests, err := sel.resolve(r)
			if err != nil {
				return err
			}
			if len(tests) == 0 {
				return errors.New("no tests selected")
			}

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "running %d tests against %s\n",
				len(tests), cfg.Endpoint); err != nil {
				return err
			}

			runner := harness.Runner{
				Timeout: timeout,
				Observe: func(res harness.Result) { report(out, res, verbose) },
			}
			results, err := runner.Run(cmd.Context(), tests)
			if err != nil {
				return err
			}
			return summarize(out, results)
		},
	}

	f := cmd.Flags()
	f.DurationVar(&timeout, "timeout", 5*time.Minute, "per-test timeout")
	f.BoolVarP(&verbose, "verbose", "v", false, "show output from passing tests too")
	return cmd
}

// report prints one finished test.
func report(w io.Writer, res harness.Result, verbose bool) {
	_, _ = fmt.Fprintf(w, "%-8s %-50s %6dms\n", res.Status, res.Test.Name, res.Duration.Milliseconds())
	if res.Status == harness.StatusSkipped && res.Skip != "" {
		_, _ = fmt.Fprintf(w, "         %s\n", res.Skip)
	}
	// Output is buffered and shown only when it matters, so a passing run
	// stays readable.
	if res.Output != "" && (verbose || res.Status == harness.StatusFailed || res.Status == harness.StatusTimeout) {
		_, _ = fmt.Fprintf(w, "%s\n", indent(res.Output))
	}
}

func summarize(w io.Writer, results []harness.Result) error {
	counts := map[harness.Status]int{}
	var total time.Duration
	for _, res := range results {
		counts[res.Status]++
		total += res.Duration
	}

	_, _ = fmt.Fprintf(w, "\n%d passed, %d failed, %d timed out, %d skipped in %s\n",
		counts[harness.StatusPassed], counts[harness.StatusFailed],
		counts[harness.StatusTimeout], counts[harness.StatusSkipped],
		total.Round(time.Millisecond))

	if failed := counts[harness.StatusFailed] + counts[harness.StatusTimeout]; failed > 0 {
		return errors.Errorf("%d tests failed", failed)
	}
	return nil
}

func indent(s string) string {
	const pad = "         "
	out := pad
	for _, r := range s {
		out += string(r)
		if r == '\n' {
			out += pad
		}
	}
	return out
}
