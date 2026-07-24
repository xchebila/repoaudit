// Package analyzers provides the single list of built-in analyzers shared
// between scan mode (cli/scan.go) and Security Diff Mode
// (analyzers/diffmode/diffmode.go). It exists purely to avoid the two call
// sites hand-duplicating the same []core.Analyzer{secrets.New(), ...}
// literal, which had no compiler or test to catch one of them drifting out
// of sync with the other.
//
// This can't live in core itself: secrets/docker/cicd all import core (for
// core.Finding, core.Analyzer, ...), so core importing them back would be
// an import cycle. This package sits above both core and the individual
// analyzer packages, which is also exactly where cli and diffmode already
// sit -- neither gains a new dependency it didn't already have.
package analyzers

import (
	"github.com/xchebila/reposcan/analyzers/cicd"
	"github.com/xchebila/reposcan/analyzers/docker"
	"github.com/xchebila/reposcan/analyzers/secrets"
	"github.com/xchebila/reposcan/core"
)

// BuiltinAnalyzers returns a fresh slice of the built-in analyzers, in the
// order scan mode and diff mode have always run them. A fresh slice per
// call, not a shared package-level var, since core.Scanner and diffmode's
// scanTree both just read from it and neither mutates it, but a shared
// backing array would make that assumption fragile against a future caller
// that does.
func BuiltinAnalyzers() []core.Analyzer {
	return []core.Analyzer{secrets.New(), docker.New(), cicd.New()}
}
