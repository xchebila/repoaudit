package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"repoaudit/analyzers/githistory"
	"repoaudit/analyzers/secrets"
	"repoaudit/core"
	"repoaudit/output"
)

func newScanCmd() *cobra.Command {
	var fullHistory bool
	var noHistory bool

	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a local repository for exposed secrets, in the working tree and in git history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fullHistory && noHistory {
				return fmt.Errorf("--full-history and --no-history are mutually exclusive")
			}

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

			if !noHistory {
				opts := githistory.Options{FullHistory: fullHistory}
				if fullHistory {
					opts.OnProgress = func(n, total int) {
						if total > 0 {
							fmt.Fprintf(cmd.ErrOrStderr(), "   ... scanned %d/%d commits so far\n", n, total)
						} else {
							fmt.Fprintf(cmd.ErrOrStderr(), "   ... scanned %d commits so far\n", n)
						}
					}
				}
				result, err := githistory.Scan(path, opts)
				switch {
				case errors.Is(err, githistory.ErrNotAGitRepo):
					// Not every scan target is a git checkout; working-tree
					// scanning alone is still fully valid.
				case err != nil:
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  git history scan failed: %v\n", err)
				default:
					findings = append(findings, result.Findings...)
					if result.Truncated {
						fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  git history scan stopped after %d commits (time budget) — some older history was not checked. Use --full-history for an exhaustive scan.\n", result.CommitsScanned)
					}
				}
			}

			score := core.ComputeCategoryScore(findings)
			output.WriteReport(cmd.OutOrStdout(), findings, score)

			if score.Value < 70 {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&fullHistory, "full-history", false, "scan the entire reachable git history plus dangling commits, no time budget")
	cmd.Flags().BoolVar(&noHistory, "no-history", false, "skip git history scanning, working tree only")

	return cmd
}
