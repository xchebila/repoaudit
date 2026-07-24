package diffmode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Split across two literals so this file's own committed source never
// contains the contiguous matching substring -- RepoScan's own CI runs
// `reposcan diff` against this repo (.github/workflows/reposcan-self-check.yml),
// and a plain literal here would be indistinguishable from a real
// hardcoded key. See the longer explanation in analyzers/secrets/secrets_test.go.
const (
	fixtureAWSKey  = "AKIA" + "ABCDEFGHIJKLMNOP"
	fixtureAWSKey2 = "AKIA" + "ZYXWVUTSRQPONMLK"
)

// newRepo creates a real, throwaway git repository in t.TempDir() -- no
// clone, no /tmp corpus dependency (this project has lost that corpus once
// already, per docs/testing.md), deterministic and fast.
func newRepo(t *testing.T) (dir string, repo *git.Repository) {
	t.Helper()
	dir = t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git.PlainInit: %v", err)
	}
	return dir, repo
}

// commit writes files (relative path -> content) into the worktree,
// removes any path in remove, stages everything (including deletions), and
// commits. Returns the new commit hash as a ref string usable directly with
// diffmode.Diff -- no branch bookkeeping needed.
func commit(t *testing.T, repo *git.Repository, dir string, files map[string]string, remove []string) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	for _, name := range remove {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatalf("removing %s: %v", name, err)
		}
	}
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := wt.Commit("test commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return hash.String()
}

func TestDiff_NewSecret(t *testing.T) {
	dir, repo := newRepo(t)
	refA := commit(t, repo, dir, map[string]string{"main.py": "print(1)\n"}, nil)
	refB := commit(t, repo, dir, map[string]string{"main.py": "AWS_KEY=" + fixtureAWSKey + "\n"}, nil)

	result, err := Diff(dir, refA, refB)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d diff findings, want 1: %+v", len(result), result)
	}
	if result[0].Status != New || result[0].ID != "secrets.aws_access_key" {
		t.Errorf("got %+v, want Status=NEW ID=secrets.aws_access_key", result[0])
	}
}

func TestDiff_FixedSecret(t *testing.T) {
	dir, repo := newRepo(t)
	refA := commit(t, repo, dir, map[string]string{"main.py": "AWS_KEY=" + fixtureAWSKey + "\n"}, nil)
	refB := commit(t, repo, dir, map[string]string{"main.py": "print(1)\n"}, nil)

	result, err := Diff(dir, refA, refB)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d diff findings, want 1: %+v", len(result), result)
	}
	if result[0].Status != Fixed || result[0].ID != "secrets.aws_access_key" {
		t.Errorf("got %+v, want Status=FIXED ID=secrets.aws_access_key", result[0])
	}
}

// TestDiff_PreexistingIssueIsNotReported confirms Security Diff Mode shows
// only the delta: a secret present on both refs, untouched by the change
// between them, must not appear at all -- not as NEW, not as FIXED.
func TestDiff_PreexistingIssueIsNotReported(t *testing.T) {
	dir, repo := newRepo(t)
	refA := commit(t, repo, dir, map[string]string{
		"main.py":      "AWS_KEY=" + fixtureAWSKey + "\n",
		"unrelated.py": "x = 1\n",
	}, nil)
	refB := commit(t, repo, dir, map[string]string{
		"unrelated.py": "x = 2\n", // touches a different file; main.py untouched
	}, nil)

	result, err := Diff(dir, refA, refB)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d diff findings, want 0 (pre-existing, untouched issue): %+v", len(result), result)
	}
}

// TestDiff_LineShiftDoesNotFalselyReport confirms Line is excluded from the
// finding-matching key: adding unrelated lines above an unchanged secret
// shifts its line number but must not produce a false FIXED+NEW pair.
func TestDiff_LineShiftDoesNotFalselyReport(t *testing.T) {
	dir, repo := newRepo(t)
	refA := commit(t, repo, dir, map[string]string{
		"main.py": "AWS_KEY=" + fixtureAWSKey + "\n",
	}, nil)
	refB := commit(t, repo, dir, map[string]string{
		"main.py": "# a comment\n# another comment\nAWS_KEY=" + fixtureAWSKey + "\n",
	}, nil)

	result, err := Diff(dir, refA, refB)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d diff findings, want 0 (same secret, shifted line): %+v", len(result), result)
	}
}

// TestDiff_CountAwarePairing confirms two same-key findings in one file,
// with one occurrence removed, report exactly one FIXED -- not zero, not
// two -- per the count-based slicing in diffFindings.
func TestDiff_CountAwarePairing(t *testing.T) {
	dir, repo := newRepo(t)
	refA := commit(t, repo, dir, map[string]string{
		"main.py": "AWS_KEY=" + fixtureAWSKey + "\nAWS_KEY2=" + fixtureAWSKey2 + "\n",
	}, nil)
	refB := commit(t, repo, dir, map[string]string{
		"main.py": "AWS_KEY=" + fixtureAWSKey + "\n",
	}, nil)

	result, err := Diff(dir, refA, refB)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d diff findings, want exactly 1 FIXED: %+v", len(result), result)
	}
	if result[0].Status != Fixed {
		t.Errorf("got Status=%s, want FIXED", result[0].Status)
	}
}

func TestDiff_NotAGitRepo(t *testing.T) {
	dir := t.TempDir() // no git init at all
	_, err := Diff(dir, "HEAD", "HEAD")
	if err != ErrNotAGitRepo {
		t.Errorf("got err=%v, want ErrNotAGitRepo", err)
	}
}
