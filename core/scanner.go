package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxFileSize skips large files (binaries, lockfiles, vendored bundles) —
// they are never where a hand-typed secret lives, and reading them is the
// single biggest threat to the "scan in < 5s" MVP exit criterion. Exported
// so other repo-level scanners (e.g. git-history) apply the same guard.
const MaxFileSize = 2 << 20 // 2 MiB

// alwaysSkipDirs are directories no analyzer needs to look inside for
// Phase 1. .git is handled separately since git history has its own
// analyzer in Phase 2.
var alwaysSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// IsVendoredPath reports whether any path segment names a directory this
// project treats as third-party, not developer-authored: vendor/,
// node_modules/. Exported so git-history scanning applies the same
// exclusion the working-tree Scanner already does via alwaysSkipDirs —
// without it, a single "vendor bump" commit touching thousands of
// third-party files both tanks scan speed and surfaces the third party's
// own test/placeholder credentials (e.g. Google's and AWS's publicly
// documented example service-account keys) as if they were this repo's.
func IsVendoredPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "node_modules" || part == "vendor" {
			return true
		}
	}
	return false
}

// Scanner walks a repository's working tree and runs every registered
// Analyzer against each eligible file.
type Scanner struct {
	Root      string
	Analyzers []Analyzer
	ignore    *gitignore
}

func NewScanner(root string, an ...Analyzer) *Scanner {
	return &Scanner{
		Root:      root,
		Analyzers: an,
		ignore:    loadGitignore(root),
	}
}

// Warnings surfaces .gitignore patterns this minimal matcher can't honor
// (negation, **), so the limitation is visible in the CLI instead of
// silently under- or over-scanning.
func (s *Scanner) Warnings() []string {
	return s.ignore.warnings
}

func (s *Scanner) Scan() ([]Finding, error) {
	var findings []Finding

	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(s.Root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			if alwaysSkipDirs[d.Name()] || s.ignore.matches(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if s.ignore.matches(rel, false) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxFileSize {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			// Unreadable file (permissions, race with deletion): skip it
			// rather than failing the whole scan.
			return nil
		}
		if IsBinary(content) {
			return nil
		}

		ctx := FileContext{Path: rel, Content: content}
		for _, a := range s.Analyzers {
			findings = append(findings, RunAnalyzer(a, ctx)...)
		}
		return nil
	})

	return findings, err
}

// IsBinary uses the same heuristic as git: a NUL byte in the first chunk
// means "not text". Cheap and good enough — RepoAudit doesn't need to be
// exact here, just fast and non-noisy on binary assets.
func IsBinary(content []byte) bool {
	n := len(content)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(content[:n], 0) != -1
}

// gitignore is a deliberately minimal matcher: exact path/dir names and
// simple glob patterns via filepath.Match. It does not implement the full
// gitignore spec (no negation, no ** double-star, no anchoring nuances).
// That's a conscious MVP trade-off — respecting the common case keeps
// noise down without adding a parsing dependency (vision.md: stdlib first).
type gitignore struct {
	patterns []string
	warnings []string
}

func loadGitignore(root string) *gitignore {
	g := &gitignore{}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return g
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			g.warnings = append(g.warnings, fmt.Sprintf("negation pattern %q is not supported and was ignored — the file(s) it re-includes may be skipped by the broader pattern above it", line))
			continue
		}
		if strings.Contains(line, "**") {
			g.warnings = append(g.warnings, fmt.Sprintf("double-star pattern %q is not supported and was ignored — matching falls back to the surrounding rules only", line))
			continue
		}
		g.patterns = append(g.patterns, strings.Trim(line, "/"))
	}
	return g
}

func (g *gitignore) matches(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	for _, p := range g.patterns {
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
		if isDir && p == base {
			return true
		}
	}
	return false
}
