package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-faster/s3t/internal/harness"
)

// ANSI escapes. Written out rather than pulled from a library: this is the
// whole of what the reporter needs.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiGray   = "\033[90m"
)

// reporter prints run progress.
type reporter struct {
	w       io.Writer
	color   bool
	verbose bool

	// nameWidth is how much room test names get, so durations line up.
	nameWidth int
}

func newReporter(w io.Writer, color, verbose bool, tests []harness.Test) *reporter {
	width := 0
	for _, t := range tests {
		width = max(width, len(t.Name))
	}
	return &reporter{w: w, color: color, verbose: verbose, nameWidth: min(width, 60)}
}

func (r *reporter) paint(s, color string) string {
	if !r.color || color == "" {
		return s
	}
	return color + s + ansiReset
}

func (r *reporter) printf(format string, args ...any) {
	// A failed write to a terminal is not actionable and must not mask a
	// test result.
	_, _ = fmt.Fprintf(r.w, format, args...)
}

// header announces what the run is about to do.
func (r *reporter) header(tests []harness.Test, endpoint string, workers int) {
	mode := fmt.Sprintf("%d workers", workers)
	if workers <= 1 {
		mode = "sequential"
	}
	r.printf("\n  %s  %s  %s\n\n",
		r.paint("s3t", ansiBold+ansiBlue),
		r.paint(fmt.Sprintf("%d tests", len(tests)), ansiBold),
		r.paint(endpoint+"  ·  "+mode, ansiDim))
}

// symbol and color for each status.
func (r *reporter) mark(s harness.Status) (symbol, color string) {
	switch s {
	case harness.StatusPassed:
		return "✔", ansiGreen
	case harness.StatusFailed:
		return "✖", ansiRed
	case harness.StatusTimeout:
		return "⏱", ansiYellow
	case harness.StatusSkipped:
		return "○", ansiGray
	default:
		return "?", ""
	}
}

// result prints one finished test.
func (r *reporter) result(res harness.Result) {
	symbol, color := r.mark(res.Status)
	r.printf("  %s %-*s %s\n",
		r.paint(symbol, color),
		r.nameWidth, res.Test.Name,
		r.paint(duration(res.Duration), ansiDim))

	if res.Status == harness.StatusSkipped && res.Skip != "" {
		r.printf("      %s\n", r.paint(res.Skip, ansiGray))
	}
	// Output is buffered and shown only when it matters, so a passing run
	// stays readable.
	failed := res.Status == harness.StatusFailed || res.Status == harness.StatusTimeout
	if res.Output != "" && (r.verbose || failed) {
		r.printf("%s\n", r.paint(indent(res.Output, "      "), ansiDim))
	}
}

// summary prints the totals and returns an error if anything failed.
func (r *reporter) summary(results []harness.Result, wall time.Duration) error {
	counts := map[harness.Status]int{}
	for _, res := range results {
		counts[res.Status]++
	}

	// Only non-zero counts are shown, so a clean run reads as one line
	// rather than a row of zeroes to scan past.
	var parts []string
	for _, s := range []harness.Status{
		harness.StatusPassed, harness.StatusFailed,
		harness.StatusTimeout, harness.StatusSkipped,
	} {
		n := counts[s]
		if n == 0 && s != harness.StatusPassed {
			continue
		}
		_, color := r.mark(s)
		parts = append(parts, r.paint(fmt.Sprintf("%d %s", n, s), color))
	}

	r.printf("\n  %s  %s\n\n",
		strings.Join(parts, r.paint("  ·  ", ansiDim)),
		r.paint("in "+duration(wall), ansiDim))

	if failed := counts[harness.StatusFailed] + counts[harness.StatusTimeout]; failed > 0 {
		r.listFailures(results)
		return errNotAllPassed{n: failed}
	}
	return nil
}

// listFailures repeats the failures at the end, so they are not lost in the
// scroll of a long run.
func (r *reporter) listFailures(results []harness.Result) {
	r.printf("  %s\n", r.paint("failed:", ansiBold+ansiRed))
	for _, res := range results {
		if res.Status == harness.StatusFailed || res.Status == harness.StatusTimeout {
			r.printf("    %s %s\n", r.paint("·", ansiRed), res.Test.NodeID())
		}
	}
	r.printf("\n")
}

// errNotAllPassed reports test failures without the "s3t:" error prefix a
// broken invocation gets: the run worked, the server did not.
type errNotAllPassed struct{ n int }

func (e errNotAllPassed) Error() string {
	return fmt.Sprintf("%d tests failed", e.n)
}

// duration formats a duration compactly: milliseconds are what most tests
// take, seconds are what a slow one takes, and neither needs six digits.
func duration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return d.Round(time.Second).String()
	}
}

func indent(s, pad string) string {
	return pad + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n"+pad)
}

// useColor decides whether to emit escapes.
//
// Honors the NO_COLOR convention and checks for a terminal, so piping to a
// file or a CI log yields plain text without anyone having to pass a flag.
func useColor(mode string, w io.Writer) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
