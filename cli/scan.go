package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"repoaudit/analyzers/cicd"
	"repoaudit/analyzers/dependencies"
	"repoaudit/analyzers/docker"
	"repoaudit/analyzers/githistory"
	"repoaudit/analyzers/secrets"
	"repoaudit/core"
	"repoaudit/output"
)

func newScanCmd() *cobra.Command {
	var fullHistory bool
	var noHistory bool
	var checkDeps bool

	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a local repository for exposed secrets, Dockerfile, and CI/CD misconfigurations",
		Long: `Scan a local repository for exposed secrets, Dockerfile, and CI/CD misconfigurations, in the working tree and in git history.

By default, git history scanning is bounded by a short time budget so the
scan stays fast. --full-history removes that bound and can take several
minutes on repos with a large history (observed: ~18 minutes on a repo with
~18k commits) — if you use it in CI, set a generous timeout, or it will
look like a hung job.

Dependency vulnerability checking (go.sum, requirements.txt via OSV.dev)
is off by default too, for a different reason: it's the only check here
that needs the network. --deps enables it explicitly.`,
		Args: cobra.MaximumNArgs(1),
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

			scanner := core.NewScanner(path, secrets.New(), docker.New(), cicd.New())
			for _, w := range scanner.Warnings() {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  gitignore: %s\n", w)
			}

			findings, err := scanner.Scan()
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}
			findings = append(findings, cicd.CheckDependabot(path)...)

			deps := dependencies.Discover(path)
			switch {
			case len(deps) == 0:
				// Nothing to check either way — stay silent, not every
				// repo has a Go or Python manifest.
			case checkDeps:
				result := dependencies.CheckVulnerabilities(deps)
				if result.Warning != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  %s\n", result.Warning)
				}
				findings = append(findings, result.Findings...)
			default:
				noun := "dependencies"
				if len(deps) == 1 {
					noun = "dependency"
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "ℹ️  Found %d %s — run with --deps to check them against known vulnerabilities (requires network).\n", len(deps), noun)
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

	cmd.Flags().BoolVar(&fullHistory, "full-history", false, "scan entire reachable history + dangling commits, no time budget — can take several minutes on large repos, avoid in CI without a generous timeout")
	cmd.Flags().BoolVar(&noHistory, "no-history", false, "skip git history scanning, working tree only")
	cmd.Flags().BoolVar(&checkDeps, "deps", false, "check go.sum/requirements.txt dependencies against known vulnerabilities via OSV.dev — requires network, off by default")

	return cmd
}
