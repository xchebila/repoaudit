// Package cicd implements RepoAudit's Phase 3 GitHub Actions detection
// rules: overly broad workflow permissions, actions pinned to a mutable
// branch ref instead of a version/SHA, and secrets echoed into build logs.
// Hardcoded secret values in a workflow file need no rule here — a
// workflow is just YAML text, so analyzers/secrets already scans it via
// the same core.Scanner pass, tagged Category "secrets".
package cicd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"repoaudit/core"
)

type Analyzer struct{}

func New() *Analyzer { return &Analyzer{} }

func (a *Analyzer) Name() string { return "cicd" }

func (a *Analyzer) Run(file core.FileContext) []core.Finding {
	if !isWorkflowFile(file.Path) {
		return nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(file.Content, &doc); err != nil {
		// Not valid YAML (or empty) — nothing to analyze, not a scan error.
		return nil
	}
	if len(doc.Content) == 0 {
		return nil
	}

	var findings []core.Finding
	walk(doc.Content[0], file.Path, &findings)
	return findings
}

func isWorkflowFile(path string) bool {
	dir := filepath.ToSlash(filepath.Dir(path))
	ext := strings.ToLower(filepath.Ext(path))
	return strings.HasSuffix(dir, ".github/workflows") && (ext == ".yml" || ext == ".yaml")
}

// walk recurses through the YAML tree looking for specific key names
// (permissions, uses, run) wherever they appear, rather than modeling the
// full workflow/job/step schema. None of the three checks need to know
// whether a match is at the workflow level or nested in a specific job —
// "permissions: write-all" is exactly as risky either place — so a schema
// walk would add structure this analyzer doesn't use.
func walk(node *yaml.Node, path string, findings *[]core.Finding) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]

			switch key.Value {
			case "permissions":
				if f := checkPermissions(value, path); f != nil {
					*findings = append(*findings, *f)
				}
			case "uses":
				if f := checkUses(value, path); f != nil {
					*findings = append(*findings, *f)
				}
			case "run":
				*findings = append(*findings, checkRun(value, path)...)
			}
			walk(value, path, findings)
		}
		return
	}
	for _, child := range node.Content {
		walk(child, path, findings)
	}
}

// checkPermissions flags the literal "write-all" shorthand. A per-scope
// map (e.g. `permissions: {contents: write}`) is deliberately not
// evaluated here — judging which scopes are "too broad" for a given
// workflow's actual needs is exactly the kind of ambiguous, context-
// dependent call vision.md's Non-Goals rule out ("si une analyse est...
// ambiguë, elle est hors scope").
func checkPermissions(value *yaml.Node, path string) *core.Finding {
	if value.Kind != yaml.ScalarNode || value.Value != "write-all" {
		return nil
	}
	f := finding(writeAllRule, path, value.Line)
	return &f
}

var mutableRefPattern = regexp.MustCompile(`^([^@\s]+)@(main|master)$`)

// checkUses flags an action pinned to a mutable branch ref instead of a
// version tag or commit SHA — the maintainer (or anyone who compromises
// their account) can change what code runs in your CI, with access to
// your secrets, without you ever changing a line in your own repo.
//
// False positive this rule doesn't try to exclude: a first-party action
// from your own organization, referencing your own trusted main branch —
// technically still "unpinned", but the supply-chain risk this rule cares
// about doesn't apply the same way to code you already control. Not
// distinguished here: there's no reliable signal in a single workflow
// file for "this uses: is same-org" versus "this is a third party".
func checkUses(value *yaml.Node, path string) *core.Finding {
	if value.Kind != yaml.ScalarNode {
		return nil
	}
	if !mutableRefPattern.MatchString(value.Value) {
		return nil
	}
	f := finding(unpinnedActionRule, path, value.Line)
	return &f
}

var echoSecretPattern = regexp.MustCompile(`(?i)^\s*(echo|print|printf|cat)\b.*\$\{\{\s*secrets\.[A-Za-z0-9_]+\s*\}\}`)

// checkRun flags shell commands that print a secret directly to the build
// log. GitHub does mask registered secret values in logs, but that
// masking has known bypass patterns (transformed/encoded output, secrets
// containing characters that break the matcher) and doesn't retroactively
// scrub logs already made public before a secret was registered.
//
// The pattern requires "}}" immediately after the secret name, so a
// boolean presence check like `echo "token set: ${{ secrets.TOKEN != ”
// }}"` does NOT match — confirmed with a synthetic fixture, not assumed.
// A real, undetected gap instead: assigning the secret to a shell
// variable on one line and echoing that variable on another (`TOKEN="${{
// secrets.TOKEN }}"` then `echo $TOKEN`) — this line-based check has no
// notion of shell variable flow across lines, so it won't connect the two.
func checkRun(value *yaml.Node, path string) []core.Finding {
	if value.Kind != yaml.ScalarNode {
		return nil
	}
	var findings []core.Finding
	for i, line := range strings.Split(value.Value, "\n") {
		if echoSecretPattern.MatchString(line) {
			findings = append(findings, finding(echoSecretRule, path, value.Line+i))
		}
	}
	return findings
}

type rule struct {
	id       string
	severity core.Severity
	title    string
	message  string
	fix      string
}

var (
	writeAllRule = rule{
		id:       "cicd.permissions_write_all",
		severity: core.High,
		title:    "Workflow grants write-all permissions",
		message:  "This workflow's GITHUB_TOKEN has write access to every scope (contents, packages, deployments, etc.). A compromised action or a malicious PR that reaches this workflow can use that token to push code, publish packages, or modify releases.",
		fix:      "Replace write-all with the specific scopes this workflow actually needs (e.g. contents: read, pull-requests: write) — GitHub Actions defaults every unlisted scope to none.",
	}
	unpinnedActionRule = rule{
		id:       "cicd.unpinned_action",
		severity: core.Medium,
		title:    "Action pinned to a mutable branch ref",
		message:  "This action is referenced by branch name (@main/@master) instead of a version tag or commit SHA. Whoever controls that branch — the maintainer, or anyone who compromises their account — can change what code runs in this workflow, with access to its secrets, at any time.",
		fix:      "Pin to a release tag (@v4) or, for maximum safety, a full commit SHA (@a1b2c3...).",
	}
	echoSecretRule = rule{
		id:       "cicd.secret_echoed",
		severity: core.Medium,
		title:    "Secret printed to the build log",
		message:  "This step echoes a secrets.* value directly. GitHub Actions masks known secret values in logs, but that masking has documented bypass patterns and won't scrub a log that was already public before the secret was registered.",
		fix:      "Remove the secret from command output; if you need to verify it's set, check its presence without printing the value (e.g. `if [ -z \"$TOKEN\" ]`).",
	}
	noDependabotRule = rule{
		id:       "cicd.no_dependabot",
		severity: core.Low,
		title:    "No Dependabot configuration",
		message:  "This repo uses GitHub Actions but has no .github/dependabot.yml. Dependencies (and the actions themselves) won't get automated update PRs when a known vulnerability is fixed upstream.",
		fix:      "Add a .github/dependabot.yml enabling updates for your package ecosystem(s) and for github-actions.",
	}
)

func finding(r rule, path string, line int) core.Finding {
	return core.Finding{
		ID:       r.id,
		Severity: r.severity,
		Title:    r.title,
		Message:  r.message,
		Fix:      r.fix,
		File:     path,
		Line:     line,
		Category: "cicd",
	}
}

// CheckDependabot is a repo-level check, not a per-file Analyzer rule:
// "no Dependabot config" is about a file's absence across the whole repo,
// which the per-file Run() callback has no way to express. Mirrors how
// analyzers/githistory is invoked directly from cli/scan.go rather than
// through the core.Analyzer interface, for the same reason.
func CheckDependabot(repoRoot string) []core.Finding {
	workflowsDir := filepath.Join(repoRoot, ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return nil // no workflows dir at all: not a GitHub Actions user, nothing to flag
	}
	hasWorkflow := false
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == ".yml" || ext == ".yaml") {
			hasWorkflow = true
			break
		}
	}
	if !hasWorkflow {
		return nil
	}

	for _, name := range []string{"dependabot.yml", "dependabot.yaml"} {
		if _, err := os.Stat(filepath.Join(repoRoot, ".github", name)); err == nil {
			return nil
		}
	}

	return []core.Finding{finding(noDependabotRule, ".github/dependabot.yml", 0)}
}
