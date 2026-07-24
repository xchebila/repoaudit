// Package cli wires Cobra commands to the core scan engine.
package cli

import "github.com/spf13/cobra"

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "reposcan",
		Short:   "Security sanity check for Git repositories",
		Version: version,
	}
	root.AddCommand(newScanCmd())
	root.AddCommand(newDiffCmd())
	return root
}
