// Package output renders scan results for the terminal.
package output

import (
	"fmt"
	"io"

	"github.com/xchebila/reposcan/core"
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
// overall score line. This is the only output surface for `reposcan scan`
// today — JSON/HTML come in Phase 5, and the per-category score breakdown
// vision.md's roadmap shows (Secrets, Git History, Docker...) is explicitly
// that phase's job too; for now secrets and git-history findings share one
// score, still dominated by the worst Severity present regardless of which
// category it came from.
func WriteReport(w io.Writer, findings []core.Finding, score core.Score) {
	if len(findings) == 0 {
		fmt.Fprintf(w, "%s✔️  OK%s   - No secrets found in the working tree or scanned git history\n", colorGrn, colorReset)
	} else {
		for _, f := range findings {
			ic, col := icon(f.Severity)
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			if f.CommitHash != "" {
				loc = fmt.Sprintf("%s, commit %s", loc, f.CommitHash[:7])
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

	fmt.Fprintf(w, "\nSECURITY SCORE: %d/100  (%s)\n", score.Value, score.Grade)
}
