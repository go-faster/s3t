// Package cmd implements the s3t command line interface.
package cmd

import (
	"github.com/spf13/cobra"
)

// Root returns the top-level s3t command.
//
// Subcommands are added as the suite lands: run, list and markers all need the
// test registry, which arrives with the harness. See PLAN.md §2.
func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3t",
		Short: "S3 compatibility test suite",
		Long: "s3t runs the S3 compatibility suite against an S3-compatible server.\n\n" +
			"It is a Go port of ceph/s3-tests; see the UPSTREAM file for the commit\n" +
			"it tracks.",
		// A failing run is not a usage error, so do not print usage on it.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		cmdVersion(),
	)
	return cmd
}
