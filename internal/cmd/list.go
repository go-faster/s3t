package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/go-faster/s3t/internal/harness"
)

func cmdList(sel *selection) *cobra.Command {
	var nodeIDs bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print the names of the selected tests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := listRegistry()
			if err != nil {
				return err
			}
			tests, err := sel.resolve(r)
			if err != nil {
				return err
			}
			for _, t := range tests {
				name := t.Name
				if nodeIDs {
					name = t.NodeID()
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&nodeIDs, "node-ids", false,
		"print pytest node IDs instead of names, the form allow-list files use")
	return cmd
}

func cmdMarkers(sel *selection) *cobra.Command {
	return &cobra.Command{
		Use:   "markers",
		Short: "Print the markers carried by the selected tests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := listRegistry()
			if err != nil {
				return err
			}
			tests, err := sel.resolve(r)
			if err != nil {
				return err
			}
			// Count over the selection, not the whole registry, so
			// `s3t markers -k '^bucket_list'` describes that subset.
			selected, err := harness.NewRegistry(tests)
			if err != nil {
				return err
			}
			for _, m := range selected.Markers() {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-40s %d\n", m.Name, m.Count); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
