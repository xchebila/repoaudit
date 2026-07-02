package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"repoaudit/analyzers/secrets"
	"repoaudit/core"
	"repoaudit/output"
)

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a local repository for exposed secrets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			if strings.Contains(path, "://") {
				return fmt.Errorf("scanning a remote URL is not yet implemented, clone the repo and pass a local path instead")
			}

			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("cannot access %s: %w", path, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", path)
			}

			scanner := core.NewScanner(path, secrets.New())
			for _, w := range scanner.Warnings() {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  gitignore: %s\n", w)
			}

			findings, err := scanner.Scan()
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			score := core.ComputeCategoryScore(findings)
			output.WriteReport(cmd.OutOrStdout(), findings, score)

			if score.Value < 70 {
				os.Exit(1)
			}
			return nil
		},
	}
}
