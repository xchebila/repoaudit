package dependencies

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseGoSum_SkipsGoModOnlyLines is the regression test for a real bug
// found while investigating issue #13 (vulnerable transitive deps): a
// go.sum line ending in "/go.mod" records only that version's go.mod file
// hash, read to resolve the dependency graph -- its actual module content
// was never fetched or compiled in. `go mod tidy` does not remove these
// lines; they're legitimate bookkeeping for versions a transitive
// dependency's own go.mod once required, superseded elsewhere by MVS. The
// previous version of parseGoSum treated every such line the same as a
// real content line, reporting versions that were never actually built as
// if they were pinned dependencies -- confirmed in practice:
// golang.org/x/text@v0.3.6 and gopkg.in/yaml.v2@v2.2.2 (the latter not
// even reachable from this project's import graph at all, per `go mod why`)
// both kept showing up as vulnerable long after the real, selected
// versions were upgraded.
func TestParseGoSum_SkipsGoModOnlyLines(t *testing.T) {
	dir := t.TempDir()
	goSum := `golang.org/x/text v0.3.6/go.mod h1:5Zoc/QRtKVWzQhOtBMvqHzDpF6irO9z98xDceosuGiQ=
golang.org/x/text v0.40.0 h1:Ub2Z6/xjgF1WrYQz2nuITOEegKFtiIy+rieRJ5lHZKs=
golang.org/x/text v0.40.0/go.mod h1:hpnzDAfGV753zIKo+wk3u1bVKCGPbrnF7+7LBF/UHVY=
gopkg.in/yaml.v2 v2.2.2/go.mod h1:hI93XBmqTisBFMUTm0b8Fm+jr3Dg1NNxqwp+5A1VGuI=
`
	path := filepath.Join(dir, "go.sum")
	if err := os.WriteFile(path, []byte(goSum), 0o644); err != nil {
		t.Fatal(err)
	}

	deps := parseGoSum(path, "go.sum")

	if len(deps) != 1 {
		t.Fatalf("got %d dependencies, want 1 (only the real content line): %+v", len(deps), deps)
	}
	got := deps[0]
	if got.Name != "golang.org/x/text" || got.Version != "v0.40.0" {
		t.Errorf("got %+v, want golang.org/x/text@v0.40.0 (the only line with real module content, not a go.mod-only line)", got)
	}

	for _, d := range deps {
		if d.Version == "v0.3.6" {
			t.Errorf("x/text@v0.3.6 was reported, but its go.sum entry is go.mod-only -- that version was never actually built")
		}
		if d.Name == "gopkg.in/yaml.v2" {
			t.Errorf("yaml.v2 was reported, but it has no real content line and isn't reachable from this project's import graph")
		}
	}
}

func TestParseGoSum_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.sum")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if deps := parseGoSum(path, "go.sum"); deps != nil {
		t.Errorf("got %v, want nil for an empty go.sum", deps)
	}
}

func TestParseGoSum_MissingFile(t *testing.T) {
	if deps := parseGoSum("/does/not/exist/go.sum", "go.sum"); deps != nil {
		t.Errorf("got %v, want nil for a missing file", deps)
	}
}
