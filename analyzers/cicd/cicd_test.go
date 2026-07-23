package cicd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xchebila/repoaudit/core"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		wantIDs []string
	}{
		{
			name:    "not a workflow file is ignored entirely",
			path:    "config.yml",
			content: "permissions: write-all\n",
			wantIDs: nil,
		},
		{
			name:    "write-all permissions",
			path:    ".github/workflows/deploy.yml",
			content: "permissions: write-all\njobs:\n  build:\n    runs-on: ubuntu-latest\n",
			wantIDs: []string{"cicd.permissions_write_all"},
		},
		{
			name:    "scoped permissions map is not flagged",
			path:    ".github/workflows/deploy.yml",
			content: "permissions:\n  contents: write\n  pull-requests: write\n",
			wantIDs: nil,
		},
		{
			name:    "action pinned to @main",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - uses: some-org/some-action@main\n",
			wantIDs: []string{"cicd.unpinned_action"},
		},
		{
			name:    "action pinned to @master",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - uses: some-org/some-action@master\n",
			wantIDs: []string{"cicd.unpinned_action"},
		},
		{
			name:    "action pinned to a version tag is clean",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n",
			wantIDs: nil,
		},
		{
			name:    "action pinned to a commit SHA is clean",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2\n",
			wantIDs: nil,
		},
		{
			name:    "branches: [main] is not an action ref and must not be flagged",
			path:    ".github/workflows/deploy.yml",
			content: "on:\n  push:\n    branches: [main]\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n",
			wantIDs: nil,
		},
		{
			name:    "secret echoed to the build log",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - run: echo ${{ secrets.TOKEN }}\n",
			wantIDs: []string{"cicd.secret_echoed"},
		},
		{
			name:    "boolean presence check on a secret is not flagged (documented gap)",
			path:    ".github/workflows/deploy.yml",
			content: "jobs:\n  build:\n    steps:\n      - run: echo \"token set ${{ secrets.TOKEN != '' }}\"\n",
			wantIDs: nil,
		},
		{
			name:    "invalid YAML does not crash, just yields no findings",
			path:    ".github/workflows/deploy.yml",
			content: "not: [valid: yaml: at: all\n",
			wantIDs: nil,
		},
		{
			name:    "empty file yields no findings",
			path:    ".github/workflows/deploy.yml",
			content: "",
			wantIDs: nil,
		},
	}

	a := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := a.Run(core.FileContext{Path: tt.path, Content: []byte(tt.content)})
			gotIDs := make([]string, len(findings))
			for i, f := range findings {
				gotIDs[i] = f.ID
				if f.Category != "cicd" {
					t.Errorf("finding %s: Category = %q, want \"cicd\"", f.ID, f.Category)
				}
			}
			assertSameIDs(t, gotIDs, tt.wantIDs)
		})
	}
}

func TestCheckDependabot(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, root string)
		wantFinding bool
	}{
		{
			name:        "no .github/workflows dir at all: not a GitHub Actions user",
			setup:       func(t *testing.T, root string) {},
			wantFinding: false,
		},
		{
			name: "workflows dir exists but has no .yml/.yaml files",
			setup: func(t *testing.T, root string) {
				mustMkdirAll(t, filepath.Join(root, ".github", "workflows"))
				mustWriteFile(t, filepath.Join(root, ".github", "workflows", "README.md"), "not a workflow")
			},
			wantFinding: false,
		},
		{
			name: "has a workflow and dependabot.yml: clean",
			setup: func(t *testing.T, root string) {
				mustMkdirAll(t, filepath.Join(root, ".github", "workflows"))
				mustWriteFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "jobs: {}")
				mustWriteFile(t, filepath.Join(root, ".github", "dependabot.yml"), "version: 2")
			},
			wantFinding: false,
		},
		{
			name: "has a workflow, no dependabot.yml: flagged",
			setup: func(t *testing.T, root string) {
				mustMkdirAll(t, filepath.Join(root, ".github", "workflows"))
				mustWriteFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "jobs: {}")
			},
			wantFinding: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			findings := CheckDependabot(root)
			if tt.wantFinding && len(findings) != 1 {
				t.Fatalf("got %d findings, want 1", len(findings))
			}
			if !tt.wantFinding && len(findings) != 0 {
				t.Fatalf("got %d findings, want 0: %v", len(findings), findings)
			}
			if tt.wantFinding && findings[0].ID != "cicd.no_dependabot" {
				t.Errorf("finding ID = %q, want cicd.no_dependabot", findings[0].ID)
			}
		})
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSameIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("finding IDs = %v, want %v", got, want)
	}
	wantSet := map[string]int{}
	for _, id := range want {
		wantSet[id]++
	}
	for _, id := range got {
		wantSet[id]--
	}
	for id, n := range wantSet {
		if n != 0 {
			t.Errorf("finding IDs = %v, want %v (mismatch on %q)", got, want, id)
			return
		}
	}
}
