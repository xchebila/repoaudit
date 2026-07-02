// Package output renders scan results for the terminal.
package output

import (
	"fmt"
	"io"

	"repoaudit/core"
)

const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorYel   = "\033[33m"
	colorGrn   = "\033[32m"
	colorGray  = "\033[90m"
)

func icon(sev core.Severity) (string, string) {
	switch sev {
	case core.Critical:
		return "❌", colorRed
	case core.High:
		return "❌", colorRed
	case core.Medium:
		return "⚠️", colorYel
	default:
		return "⚠️", colorGray
	}
}

// WriteReport prints findings grouped by severity (worst first), then the
// category score line. This is the only output surface for `repoaudit scan`
// today — JSON/HTML come in Phase 5.
func WriteReport(w io.Writer, findings []core.Finding, score core.Score) {
	if len(findings) == 0 {
		fmt.Fprintf(w, "%s✔️  OK%s   - No secrets found in working tree\n", colorGrn, colorReset)
	} else {
		for _, f := range findings {
			ic, col := icon(f.Severity)
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			fmt.Fprintf(w, "%s %s%-8s%s - %s (%s)\n", ic, col, f.Severity, colorReset, f.Title, loc)
			fmt.Fprintf(w, "   %s%s%s\n", colorGray, f.Message, colorReset)
			fmt.Fprintf(w, "   fix: %s\n", f.Fix)
			if f.Context != "" {
				fmt.Fprintf(w, "   %scontext: %s%s\n", colorYel, f.Context, colorReset)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "\nSECRETS SCORE: %d/100  (%s)\n", score.Value, score.Grade)
}
