package cmd

import (
	"runtime"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/go-faster/s3t/internal/harness"
)

// errNoConfig is returned when no config file was given. Running needs one;
// listing does not.
var errNoConfig = errors.New(
	"no config file: pass --config or set S3TEST_CONF (see s3tests.conf.SAMPLE)")

// defaultParallel bounds the worker pool. The suite is I/O bound so more
// workers help, but past roughly sixteen a single server starts returning 503
// SlowDown and tests fail for the wrong reason.
func defaultParallel() int { return min(8, runtime.NumCPU()) }

func cmdRun(sel *selection, cfgPath *string) *cobra.Command {
	var (
		parallel       int
		serial         bool
		timeout        time.Duration
		cleanupTimeout time.Duration
		stallTimeout   time.Duration
		maxLeaked      int
		verbose        bool
		color          string
		jsonPath       string
		junitPath      string
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

			workers := parallel
			if serial {
				workers = 1
			}
			if stallTimeout <= 0 {
				// The backstop only needs to be looser than any single
				// test; tying it to the per-test timeout keeps the two
				// from being configured into conflict.
				stallTimeout = 3 * timeout
			}

			out := cmd.OutOrStdout()
			rep := newReporter(out, useColor(color, out), verbose, tests)
			rep.header(tests, cfg.Endpoint, workers)

			runner := harness.Runner{
				Parallel:       workers,
				Timeout:        timeout,
				CleanupTimeout: cleanupTimeout,
				StallTimeout:   stallTimeout,
				MaxLeaked:      maxLeaked,
				Observe:        rep.result,
			}

			start := time.Now()
			results, err := runner.Run(cmd.Context(), tests)
			if err != nil {
				return err
			}

			wall := time.Since(start)
			if jsonPath != "" {
				if err := writeJSON(jsonPath, results); err != nil {
					return err
				}
			}
			if junitPath != "" {
				if err := writeJUnit(junitPath, results, wall); err != nil {
					return err
				}
			}

			summaryErr := rep.summary(results, wall)
			// An interrupted run must not look like success: it reports
			// on the tests that finished, but the ones that never ran
			// are unknown, not passing.
			if ctxErr := cmd.Context().Err(); ctxErr != nil {
				return ctxErr
			}
			return summaryErr
		},
	}

	f := cmd.Flags()
	f.IntVarP(&parallel, "parallel", "p", defaultParallel(), "number of tests to run at once")
	f.BoolVar(&serial, "serial", false, "run one test at a time, for debugging")
	f.DurationVar(&timeout, "timeout", 5*time.Minute, "per-test timeout")
	f.DurationVar(&cleanupTimeout, "cleanup-timeout", time.Minute, "per-test cleanup timeout")
	f.DurationVar(&stallTimeout, "stall-timeout", 0,
		"abort if no test finishes in this long (default: three times --timeout)")
	f.IntVar(&maxLeaked, "max-leaked", 8, "abort after this many tests are abandoned")
	f.BoolVarP(&verbose, "verbose", "v", false, "show output from passing tests too")
	f.StringVar(&color, "color", "auto", "colorize output: auto, always or never")
	f.StringVar(&jsonPath, "json", "", "write a line-delimited JSON report to this file")
	f.StringVar(&junitPath, "junit", "", "write a JUnit XML report to this file")
	return cmd
}
