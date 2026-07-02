// Package cli wires Cobra commands to the core scan engine.
package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "repoaudit",
		Short: "Security sanity check for Git repositories",
	}
	root.AddCommand(newScanCmd())
	return root
}
