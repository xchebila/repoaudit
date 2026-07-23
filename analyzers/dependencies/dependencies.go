// Package dependencies checks a repo's Go and Python manifests for known
// vulnerabilities via OSV.dev. Unlike every other analyzer, this one needs
// the network — see docs/decisions/0004-dependency-scanner-network.md for
// why that makes it opt-in (--deps) rather than on by default.
//
// Discovery (finding and parsing manifests) is local and always safe to
// run; only CheckVulnerabilities makes network calls. Splitting these lets
// the default scan mention "N dependencies found" without ever touching
// the network itself.
package dependencies

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xchebila/repoaudit/core"
)

// Dependency is one package+version pinned by a manifest file.
type Dependency struct {
	Name      string
	Version   string
	Ecosystem string // OSV ecosystem name: "Go" or "PyPI"
	Manifest  string // repo-relative path to the manifest that pinned it
}

// Discover walks repoRoot for go.sum and requirements.txt files and parses
// out exact-pinned dependencies. Unpinned requirements (no ==version) are
// skipped rather than queried without a version — OSV would return every
// vulnerability ever reported for that package regardless of whether the
// installed version is actually affected, which is exactly the kind of
// noisy, ambiguous result vision.md's Non-Goals rule out.
func Discover(repoRoot string) []Dependency {
	var deps []Dependency

	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}

		if d.IsDir() {
			if d.Name() == ".git" || core.IsVendoredPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		switch d.Name() {
		case "go.sum":
			deps = append(deps, parseGoSum(path, rel)...)
		case "requirements.txt":
			deps = append(deps, parseRequirementsTxt(path, rel)...)
		}
		return nil
	})

	return deps
}

func parseGoSum(path, manifestRel string) []Dependency {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// go.sum lists both a module hash line and a go.mod hash line per
	// version ("v1.2.3" and "v1.2.3/go.mod") — deduping on module@version
	// avoids querying (and, if vulnerable, reporting) the same dependency
	// twice.
	seen := map[string]bool{}
	var deps []Dependency
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		module, version := fields[0], strings.TrimSuffix(fields[1], "/go.mod")
		key := module + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		deps = append(deps, Dependency{Name: module, Version: version, Ecosystem: "Go", Manifest: manifestRel})
	}
	return deps
}

// requirementLine matches "package==1.2.3" and "package[extra]==1.2.3",
// ignoring the extra. Anything else (unpinned ">=", a git/URL install, a
// -r include) is deliberately not matched — see Discover's doc comment.
var requirementLine = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.\-]*)\s*(\[[^\]]*\])?\s*==\s*([A-Za-z0-9_.\-]+)`)

func parseRequirementsTxt(path, manifestRel string) []Dependency {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var deps []Dependency
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if idx := strings.Index(line, ";"); idx != -1 {
			line = strings.TrimSpace(line[:idx]) // strip environment markers
		}
		m := requirementLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		deps = append(deps, Dependency{Name: m[1], Version: m[3], Ecosystem: "PyPI", Manifest: manifestRel})
	}
	return deps
}
