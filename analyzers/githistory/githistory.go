// Package githistory finds secrets that were committed and later removed —
// the one thing a working-tree scan can never catch. It does not define its
// own detection rules: every commit's changed files are fed through the
// existing analyzers/secrets rules, so the two stay in sync by construction.
package githistory

import (
	"errors"
	"io"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/xchebila/repoaudit/analyzers/secrets"
	"github.com/xchebila/repoaudit/core"
)

// DefaultBudget bounds the default (no-flag) scan by wall-clock time, not
// commit count: benchmarking showed the per-commit cost of object.DiffTree
// varies heavily and unpredictably by repo — not just with tree size (~9ms
// avg/commit on prometheus's 1.6k files vs ~0.6ms on cobra's few dozen) but
// also with per-commit delta-chain depth in the pack file, which made two
// back-to-back runs on the same repo process anywhere from 6 to 48 commits
// in a similar time window. A time budget adapts to both sources of
// variance; a fixed commit count adapts to neither. Set below the naive 2s
// first guess to leave real headroom for the working-tree scan (~1.3-1.5s
// on prometheus) within the <5s MVP budget, since the budget check happens
// between commits and can overshoot by one commit's cost in the worst case
// — see docs/decisions/0002-git-history-depth.md.
const DefaultBudget = 1500 * time.Millisecond

// hardCommitCeiling is a secondary, defensive cap independent of the time
// budget: guards against pathological cases (e.g. a huge number of
// vanishingly cheap commits) where per-commit cost alone wouldn't trip the
// budget check often enough to bound total time tightly.
const hardCommitCeiling = 20000

// Options controls how much history is walked.
type Options struct {
	// FullHistory disables both the time budget and the hard commit
	// ceiling on reachable history, AND adds a sweep of dangling commits
	// (unreachable from any ref, e.g. a deleted branch not yet
	// garbage-collected). All opt-in: none has a bounded worst case on
	// large/old repos.
	FullHistory bool
	// Budget overrides DefaultBudget. Not exposed as a CLI flag (vision.md:
	// no config for its own sake) — exists so benchmarks can sweep budgets
	// without recompiling a different constant each time.
	Budget time.Duration
	// OnProgress, if set, is called periodically during a FullHistory scan
	// with the running commit count and the total reachable commit count
	// (0 if that count isn't available). FullHistory has no time budget by
	// design (e.g. a repo with vendored dependencies can hit individual
	// commits that take seconds to diff), so without some signal — and a
	// denominator, not just a counter — a long scan is indistinguishable
	// from a hang. Ignored outside FullHistory, where the default budget
	// already keeps runs short enough not to need it.
	OnProgress func(commitsScanned, total int)
}

// progressInterval is how often (in commits) OnProgress fires during a
// FullHistory scan.
const progressInterval = 200

// Result carries findings plus enough context to be honest about coverage:
// Truncated tells the caller (and, ultimately, the user) whether the report
// reflects the whole reachable history or was cut short by the time budget
// — "Explicable" applies to what wasn't scanned too, not just to findings.
type Result struct {
	Findings       []core.Finding
	CommitsScanned int
	Truncated      bool
}

// Scan walks commit history at repoPath and returns Findings for secrets
// rule matches in any version of any changed file, tagged with the commit
// that introduced or removed them. repoPath must be a local git repository;
// ErrNotAGitRepo is returned (not wrapped) so callers can treat "just a
// plain directory, no history to scan" as a non-fatal, silent skip.
func Scan(repoPath string, opts Options) (Result, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return Result{}, ErrNotAGitRepo
	}

	analyzer := secrets.New()
	visited := map[plumbing.Hash]bool{}
	result := Result{}

	// Counting reachable commits is a plain walk of commit objects with no
	// tree diffing or blob reads — orders of magnitude cheaper than the
	// scan itself — so FullHistory can report "N/total" instead of a bare
	// counter that gives no way to tell slow-but-progressing apart from
	// stuck. Skipped outside FullHistory: the default budget already keeps
	// runs short enough that a progress readout isn't needed.
	totalCommits := 0
	if opts.FullHistory && opts.OnProgress != nil {
		totalCommits, err = countReachableCommits(repo)
		if err != nil {
			totalCommits = 0
		}
	}

	logIter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return Result{}, err
	}
	defer logIter.Close()

	budget := opts.Budget
	if budget == 0 {
		budget = DefaultBudget
	}
	start := time.Now()

	for {
		if !opts.FullHistory && (result.CommitsScanned >= hardCommitCeiling || time.Since(start) >= budget) {
			result.Truncated = true
			break
		}
		c, err := logIter.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, err
		}
		visited[c.Hash] = true
		// The first commit walked is HEAD (Log defaults From to HEAD): its
		// "after" tree is exactly the current working tree, already fully
		// covered by core.Scanner. Scanning it again here would double-count
		// every currently-present secret — once as "secrets", once as
		// "git-history" for the same file — inflating both the finding list
		// and the score's severity penalty for a single real issue. Its
		// "before" tree (whatever HEAD's commit deleted) is still genuinely
		// historical and stays in scope.
		skipTo := result.CommitsScanned == 0
		result.CommitsScanned++
		result.Findings = append(result.Findings, scanCommit(analyzer, c, skipTo)...)

		if opts.FullHistory && opts.OnProgress != nil && result.CommitsScanned%progressInterval == 0 {
			opts.OnProgress(result.CommitsScanned, totalCommits)
		}
	}

	if opts.FullHistory {
		result.Findings = append(result.Findings, scanDangling(analyzer, repo, visited)...)
	}

	return result, nil
}

// ErrNotAGitRepo signals repoPath has no .git to walk.
var ErrNotAGitRepo = errors.New("not a git repository")

// countReachableCommits walks commit objects only — no Tree(), no diffing,
// no blob reads — so it's cheap even on repos where the real scan is slow.
func countReachableCommits(repo *git.Repository) (int, error) {
	iter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	n := 0
	err = iter.ForEach(func(*object.Commit) error {
		n++
		return nil
	})
	return n, err
}

// scanDangling sweeps every commit object physically present in the repo's
// object store, regardless of ref reachability, and scans whichever ones
// the reachable-history walk didn't already cover. This is how a deleted
// branch's commits — still on disk until `git gc` prunes them — get caught,
// with no reflog parsing required.
func scanDangling(analyzer *secrets.Analyzer, repo *git.Repository, visited map[plumbing.Hash]bool) []core.Finding {
	var findings []core.Finding

	allIter, err := repo.CommitObjects()
	if err != nil {
		return nil
	}
	defer allIter.Close()

	_ = allIter.ForEach(func(c *object.Commit) error {
		if visited[c.Hash] {
			return nil
		}
		visited[c.Hash] = true
		findings = append(findings, scanCommit(analyzer, c, false)...)
		return nil
	})

	return findings
}

// scanCommit diffs c against its first parent (root commits diff against an
// empty tree) and runs the secrets analyzer over every changed file's
// content on both sides of the change — the "before" version so a secret
// that gets deleted in this commit is still caught, the "after" version so
// one that gets introduced is too. Merge commits are diffed against their
// first parent only: diffing against every parent multiplies cost on
// merge-heavy repos for content that's almost always already covered by
// the linear history of at least one side.
//
// skipTo omits the "after" side of the diff — used for HEAD, whose "after"
// tree is the current working tree and already covered by core.Scanner.
func scanCommit(analyzer *secrets.Analyzer, c *object.Commit, skipTo bool) []core.Finding {
	tree, err := c.Tree()
	if err != nil {
		return nil
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parent(0)
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return nil
	}

	var findings []core.Finding
	for _, change := range changes {
		// Checked before Files() (which decompresses blobs) so a vendor
		// bump touching thousands of third-party files costs nothing
		// beyond the tree diff itself — see core.IsVendoredPath.
		if core.IsVendoredPath(change.To.Name) || core.IsVendoredPath(change.From.Name) {
			continue
		}

		from, to, err := change.Files()
		if err != nil {
			continue
		}
		if skipTo {
			to = nil
		}
		// A pure rename or mode change (same path pair, unchanged content)
		// leaves from/to pointing at the same blob — scanning both would
		// report the same match twice for one commit. Content is what
		// matters here, so comparing blob hashes is the correct de-dup key.
		// Two distinct files that happen to share a basename (e.g.
		// examples/a/server.key and examples/b/server.key deleted in the
		// same commit) have different hashes and are correctly kept as two
		// findings — see docs/decisions/0002-git-history-depth.md.
		if from != nil && to != nil && from.Hash == to.Hash {
			to = nil
		}
		// change.From.Name / change.To.Name (not File.Name, which
		// TreeEntryFile leaves as just the tree-local basename) carry the
		// full repo-relative path — using File.Name here made two
		// same-named files in different directories print identically.
		if from != nil {
			findings = append(findings, scanBlob(analyzer, from, change.From.Name, c.Hash.String())...)
		}
		if to != nil {
			findings = append(findings, scanBlob(analyzer, to, change.To.Name, c.Hash.String())...)
		}
	}
	return findings
}

func scanBlob(analyzer *secrets.Analyzer, f *object.File, path, commitHash string) []core.Finding {
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

	fc := core.FileContext{Path: path, Content: []byte(content)}
	results := analyzer.Run(fc)
	for i := range results {
		results[i].Category = "git-history"
		results[i].CommitHash = commitHash
	}
	return results
}
