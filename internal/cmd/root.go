// Package cmd implements the s3t command line interface.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/suite"
)

// Root returns the top-level s3t command.
func Root() *cobra.Command {
	var (
		sel     selection
		cfgPath string
	)

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

	f := cmd.PersistentFlags()
	f.StringVarP(&cfgPath, "config", "c", os.Getenv("S3TEST_CONF"),
		"path to s3tests.conf (defaults to $S3TEST_CONF)")
	sel.bind(f)

	cmd.AddCommand(
		cmdRun(&sel, &cfgPath),
		cmdList(&sel),
		cmdMarkers(&sel),
		cmdVersion(),
	)
	return cmd
}

// registryWithTimeouts builds the test registry with a real configuration and
// the given HTTP bounds.
func registryWithTimeouts(cfgPath string, t client.Timeouts) (*harness.Registry, *config.Config, *client.Factory, error) {
	if cfgPath == "" {
		return nil, nil, nil, errNoConfig
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, nil, err
	}
	clients := client.NewWithTimeouts(cfg, t)
	r, err := suite.Registry(cfg, clients)
	if err != nil {
		return nil, nil, nil, err
	}
	return r, cfg, clients, nil
}

// listRegistry builds the registry without a configuration.
//
// Test names and markers are known before any server is contacted, so listing
// works with no config file and no reachable endpoint. The zero config is
// never used: the bodies that would read it are not run.
func listRegistry() (*harness.Registry, error) {
	cfg := &config.Config{}
	return suite.Registry(cfg, client.New(cfg))
}
