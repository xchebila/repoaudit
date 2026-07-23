package core

import (
	"fmt"
	"os"
	"time"
)

// AnalyzerTimeout bounds how long RunAnalyzer waits for a single
// Analyzer.Run call on one file. Built-in analyzers (regex/YAML parsing)
// have never come close to this in benchmarking (docs/benchmarks.md) --
// it's a structural guard against a future analyzer (or a bug in one)
// that hangs, not a limit anything is expected to hit today. Matches the
// plugin protocol's own per-file allowance (analyzers/plugin/plugin.go)
// for consistency. A var, not a const, so tests can shrink it instead of
// waiting out the real value.
var AnalyzerTimeout = 5 * time.Second

// RunAnalyzer calls a.Run(file) with a soft per-file timeout, shared by
// core.Scanner and Security Diff Mode (analyzers/diffmode) so this guard
// exists in exactly one place rather than being reimplemented twice --
// the same reasoning as analyzers.BuiltinAnalyzers().
//
// Go has no way to forcibly stop a running goroutine: if a.Run doesn't
// return in time, the goroutine below keeps running in the background,
// but the caller never blocks on it -- a warning is logged and this file
// is skipped for that analyzer, the same trade-off the plugin protocol
// makes for a stuck subprocess (there, the OS process can actually be
// killed; here, only the wait is abandoned).
func RunAnalyzer(a Analyzer, file FileContext) []Finding {
	type result struct{ findings []Finding }
	ch := make(chan result, 1)
	go func() {
		ch <- result{a.Run(file)}
	}()

	select {
	case res := <-ch:
		return res.findings
	case <-time.After(AnalyzerTimeout):
		fmt.Fprintf(os.Stderr, "⚠️  analyzer %q did not finish on %s within %s — skipped for this file\n", a.Name(), file.Path, AnalyzerTimeout)
		return nil
	}
}
