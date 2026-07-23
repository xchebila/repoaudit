package output

import (
	"fmt"
	"io"

	"github.com/xchebila/repoaudit/analyzers/diffmode"
)

// WriteDiffReport prints only what changed between two refs — vision.md's
// Security Diff Mode format (❌ NEW / ✔️ FIXED), not a full score. A repo
// scan has one number to summarize; a PR diff doesn't need one; what
// matters is whether anything NEW showed up at all.
func WriteDiffReport(w io.Writer, findings []diffmode.DiffFinding) {
	if len(findings) == 0 {
		fmt.Fprintf(w, "%s✔️  OK%s   - No security-relevant changes between these two refs\n", colorGrn, colorReset)
		return
	}

	for _, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}

		if f.Status == diffmode.Fixed {
			fmt.Fprintf(w, "%s✔️  FIXED%s  - %s (%s)\n", colorGrn, colorReset, f.Title, loc)
			continue
		}

		ic, col := icon(f.Severity)
		fmt.Fprintf(w, "%s %sNEW %-8s%s - %s (%s)\n", ic, col, f.Severity, colorReset, f.Title, loc)
		fmt.Fprintf(w, "   %s%s%s\n", colorGray, f.Message, colorReset)
		fmt.Fprintf(w, "   fix: %s\n", f.Fix)
		if f.Context != "" {
			fmt.Fprintf(w, "   %scontext: %s%s\n", colorYel, f.Context, colorReset)
		}
		fmt.Fprintln(w)
	}
}
