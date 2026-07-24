package output

import (
	"flag"
	"os"
	"testing"

	"github.com/xchebila/reposcan/core"
)

// update regenerates every golden file in this package from the current
// output instead of comparing against it. Run: go test ./output/... -update
var update = flag.Bool("update", false, "update golden files instead of comparing against them")

// goldenFindings covers every core.Finding field at least once (including
// CommitHash and Context together, per docs/testing.md's own reference
// fixture) across the analyzers' real Category values, so both wire
// formats (ADR 0009, ADR 0010) are exercised on realistic content, not a
// single-field toy fixture.
var goldenFindings = []core.Finding{
	{
		ID:       "secrets.aws_access_key",
		Severity: core.Critical,
		Title:    "AWS Access Key ID exposed",
		Message:  "An AWS access key ID was found hardcoded in this file.",
		Fix:      "Revoke this key in the AWS IAM console.",
		File:     "config.py",
		Line:     1,
		Category: "secrets",
	},
	{
		ID:         "secrets.github_token",
		Severity:   core.Critical,
		Title:      "GitHub token exposed",
		Message:    "A GitHub token was found in this commit's history.",
		Fix:        "Revoke the token at github.com/settings/tokens.",
		File:       "deploy.sh",
		Line:       12,
		CommitHash: "a83f1c2",
		Category:   "git-history",
	},
	{
		ID:       "docker.no_nonroot_user",
		Severity: core.Medium,
		Title:    "No non-root USER set",
		Message:  "This Dockerfile never switches away from the default root user.",
		Fix:      "Add a USER instruction before the final CMD/ENTRYPOINT.",
		File:     "Dockerfile",
		Line:     1,
		Category: "docker",
	},
	{
		ID:       "dependencies.osv_go",
		Severity: core.Low,
		Title:    "GO-2026-1: known vulnerability in example.com/pkg@v1.0.0",
		Message:  "Infinite loop on invalid input.",
		Fix:      "Upgrade example.com/pkg past the vulnerable range.",
		File:     "go.sum",
		Category: "dependencies",
		Context:  "no severity information available from OSV.dev for this vulnerability — defaulted to Medium",
	},
}

var goldenScore = core.Score{Value: 40, Grade: "F"}

func goldenCompare(t *testing.T, path string, got []byte, normalize func([]byte) []byte) {
	t.Helper()
	if normalize != nil {
		got = normalize(got)
	}
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("writing golden file %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file %s: %v (run with -update to create it)", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("output for %s does not match golden file.\nRun `go test ./output/... -update` after confirming the new output is correct.\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
