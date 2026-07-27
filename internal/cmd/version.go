package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is set by goreleaser via -ldflags; for `go install` and local builds
// it stays empty and the module version from the build info is used instead.
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}

// vcsInfo returns the revision and dirty state stamped into the binary by the
// Go toolchain, empty if the build had no VCS information.
func vcsInfo() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "s3t %s %s/%s %s\n",
				buildVersion(), runtime.GOOS, runtime.GOARCH, runtime.Version()); err != nil {
				return err
			}
			if rev, modified := vcsInfo(); rev != "" {
				suffix := ""
				if modified {
					suffix = " (dirty)"
				}
				if _, err := fmt.Fprintf(out, "revision %s%s\n", rev, suffix); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
