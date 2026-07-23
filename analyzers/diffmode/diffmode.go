// Package diffmode implements RepoAudit's Security Diff Mode: instead of a
// static score for the whole repo, it shows exactly what a change (two git
// refs) introduces or fixes. This is what makes RepoAudit useful as a
// recurring PR check, not just a one-off scan (vision.md, Phase 3).
//
// It reuses the existing per-file analyzers (secrets, docker, cicd)
// unchanged — the only new logic here is reading a ref's tree without
// touching the working directory, and diffing two Finding sets. The
// Dependency Scanner's network calls are deliberately not part of this by
// default, for the same reason they're opt-in in `scan` (ADR 0004).
package diffmode

import (
	"errors"
	"sort"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"repoaudit/analyzers/cicd"
	"repoaudit/analyzers/docker"
	"repoaudit/analyzers/secrets"
	"repoaudit/core"
)

// ErrNotAGitRepo mirrors githistory's error: Security Diff Mode is
// inherently git-native (it compares two refs), so a plain directory with
// no .git is a hard error here, not a silent skip.
var ErrNotAGitRepo = errors.New("not a git repository")

// Status marks whether a Finding was introduced or resolved between the
// two refs compared. Findings present on both sides aren't reported at
// all — Security Diff Mode shows the delta, not the whole repo's state.
type Status string

const (
	New   Status = "NEW"
	Fixed Status = "FIXED"
)

// DiffFinding is a core.Finding plus which side of the diff it belongs to.
type DiffFinding struct {
	core.Finding
	Status Status
}

// Diff compares refA against refB (e.g. "main" against "feature-branch")
// and returns every Finding that appears on exactly one side.
func Diff(repoPath, refA, refB string) ([]DiffFinding, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, ErrNotAGitRepo
	}

	treeA, err := resolveTree(repo, refA)
	if err != nil {
		return nil, err
	}
	treeB, err := resolveTree(repo, refB)
	if err != nil {
		return nil, err
	}

	findingsA, err := scanTree(treeA)
	if err != nil {
		return nil, err
	}
	findingsB, err := scanTree(treeB)
	if err != nil {
		return nil, err
	}

	return diffFindings(findingsA, findingsB), nil
}

func resolveTree(repo *git.Repository, ref string) (*object.Tree, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}

// scanTree runs every working-tree analyzer against a ref's tree exactly
// as core.Scanner runs them against the actual working tree — same
// per-file guards (size, binary, vendored paths), same analyzers, just
// reading blobs from git instead of the filesystem so no checkout is
// needed.
func scanTree(tree *object.Tree) ([]core.Finding, error) {
	analyzerList := []core.Analyzer{secrets.New(), docker.New(), cicd.New()}

	var findings []core.Finding
	err := tree.Files().ForEach(func(f *object.File) error {
		if core.IsVendoredPath(f.Name) {
			return nil
		}
		if f.Size > core.MaxFileSize {
			return nil
		}
		content, err := f.Contents()
		if err != nil {
			return nil
		}
		if core.IsBinary([]byte(content)) {
			return nil
		}

		fc := core.FileContext{Path: f.Name, Content: []byte(content)}
		for _, a := range analyzerList {
			findings = append(findings, a.Run(fc)...)
		}
		return nil
	})
	return findings, err
}

// findingKey identifies "the same finding" across two refs. Line is
// deliberately excluded: an unrelated line added earlier in an unchanged
// file would shift every later line number, and comparing by exact Line
// would misreport a still-present secret as both FIXED (old line) and NEW
// (new line) for no real reason. Known trade-off: a renamed-but-unmodified
// file reports as FIXED (old path) + NEW (new path) rather than
// unchanged — no rename detection, consistent with vision.md's Non-Goals
// (no deep static analysis).
type findingKey struct {
	File     string
	ID       string
	Category string
}

func keyOf(f core.Finding) findingKey {
	return findingKey{File: f.File, ID: f.ID, Category: f.Category}
}

// diffFindings pairs findings by key and reports only the surplus on
// either side — if refA has two matches for the same key and refB has
// one, exactly one is reported FIXED (the pair that still exists on both
// sides is not "the same finding twice", it's genuinely one instance
// resolved).
func diffFindings(before, after []core.Finding) []DiffFinding {
	beforeByKey := groupByKey(before)
	afterByKey := groupByKey(after)

	keys := make(map[findingKey]bool, len(beforeByKey)+len(afterByKey))
	for k := range beforeByKey {
		keys[k] = true
	}
	for k := range afterByKey {
		keys[k] = true
	}

	var result []DiffFinding
	for k := range keys {
		b, a := beforeByKey[k], afterByKey[k]
		n := len(b)
		if len(a) < n {
			n = len(a)
		}
		for _, f := range a[n:] {
			result = append(result, DiffFinding{Finding: f, Status: New})
		}
		for _, f := range b[n:] {
			result = append(result, DiffFinding{Finding: f, Status: Fixed})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].File != result[j].File {
			return result[i].File < result[j].File
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Status < result[j].Status
	})
	return result
}

func groupByKey(findings []core.Finding) map[findingKey][]core.Finding {
	m := make(map[findingKey][]core.Finding, len(findings))
	for _, f := range findings {
		k := keyOf(f)
		m[k] = append(m[k], f)
	}
	return m
}
