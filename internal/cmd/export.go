package cmd

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"time"

	"github.com/go-faster/errors"

	"github.com/go-faster/s3t/internal/harness"
)

// jsonResult is one line of the --json report.
//
// Keyed by node ID rather than name so a report can be joined directly against
// a pytest --json-report run, which is how the port is verified.
type jsonResult struct {
	NodeID   string   `json:"node_id"`
	Name     string   `json:"name"`
	Module   string   `json:"module"`
	Markers  []string `json:"markers,omitempty"`
	Status   string   `json:"status"`
	Duration float64  `json:"duration_seconds"`
	Output   string   `json:"output,omitempty"`
	Skip     string   `json:"skip_reason,omitempty"`
}

// writeJSON writes one JSON object per line.
//
// Line-delimited rather than one array: a comparison script can stream it, and
// a truncated file from an aborted run still parses up to the last complete
// line.
func writeJSON(path string, results []harness.Result) error {
	f, err := os.Create(path) //nolint:gosec // caller-supplied path
	if err != nil {
		return errors.Wrap(err, "create json report")
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, res := range results {
		if err := enc.Encode(jsonResult{
			NodeID:   res.Test.NodeID(),
			Name:     res.Test.Name,
			Module:   res.Test.Module,
			Markers:  res.Test.Markers,
			Status:   string(res.Status),
			Duration: res.Duration.Seconds(),
			Output:   res.Output,
			Skip:     res.Skip,
		}); err != nil {
			return errors.Wrap(err, "write json report")
		}
	}
	// Wrap only inside a non-nil check: errors.Wrap(nil, ...) returns a
	// non-nil error, which would report a phantom failure on success.
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close json report")
	}
	return nil
}

// JUnit XML, the subset CI systems actually read.
type junitSuites struct {
	XMLName  xml.Name    `xml:"testsuites"`
	Suites   []junitCase `xml:"testsuite>testcase"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     float64     `xml:"time,attr"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// writeJUnit writes a JUnit XML report.
//
// go-faster/fs uploads this from its weekly informational run and promotes
// newly-passing tests into the allow-list from it, so the classname and name
// together have to reconstruct the pytest node ID.
func writeJUnit(path string, results []harness.Result, wall time.Duration) error {
	report := junitSuites{Name: "s3t", Time: wall.Seconds()}
	for _, res := range results {
		c := junitCase{
			Name:      "test_" + res.Test.Name,
			Classname: res.Test.Module,
			Time:      res.Duration.Seconds(),
		}
		switch res.Status {
		case harness.StatusFailed, harness.StatusTimeout:
			c.Failure = &junitFailure{Message: string(res.Status), Body: res.Output}
			report.Failures++
		case harness.StatusSkipped:
			c.Skipped = &junitSkipped{Message: res.Skip}
			report.Skipped++
		case harness.StatusPassed:
		}
		report.Tests++
		report.Suites = append(report.Suites, c)
	}

	f, err := os.Create(path) //nolint:gosec // caller-supplied path
	if err != nil {
		return errors.Wrap(err, "create junit report")
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(xml.Header); err != nil {
		return errors.Wrap(err, "write junit report")
	}
	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")
	if err := enc.Encode(report); err != nil {
		return errors.Wrap(err, "encode junit report")
	}
	// Wrap only inside a non-nil check: errors.Wrap(nil, ...) returns a
	// non-nil error, which would report a phantom failure on success.
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close junit report")
	}
	return nil
}
