package output

import (
	"html/template"
	"io"
	"time"

	"github.com/xchebila/reposcan/core"
)

// WriteHTMLReport renders a self-contained HTML dashboard: no external
// CSS/font/JS, works offline, matches the rest of this project's "no
// network unless explicitly opted in" discipline. The total score is
// computed independently over every finding (core.ComputeCategoryScore on
// the full list) — never derived from the per-category breakdown, which
// would let a CRITICAL in one category get diluted by clean categories
// elsewhere. See core/scoring_test.go and
// docs/decisions/0010-html-dashboard.md.
func WriteHTMLReport(w io.Writer, findings []core.Finding, score core.Score, path string) error {
	data := htmlData{
		Path:        path,
		GeneratedAt: time.Now().Format("2006-01-02 15:04"),
		Score:       score,
		Breakdown:   core.ComputeCategoryBreakdown(findings),
		Findings:    findings,
	}
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"statusOf":       statusOf,
		"severityStatus": severityStatus,
		"severityIcon":   severityIcon,
		"shortHash":      shortHash,
	}).Parse(htmlTemplate))
	return tmpl.Execute(w, data)
}

type htmlData struct {
	Path        string
	GeneratedAt string
	Score       core.Score
	Breakdown   []core.Score
	Findings    []core.Finding
}

// shortHash mirrors the CLI report's own truncation (output/cli.go),
// guarded against a hash shorter than 7 characters — every real caller
// sets a full commit SHA, but a template panic on a malformed value would
// be a rough failure mode for what's otherwise a purely display concern.
func shortHash(h string) string {
	if len(h) <= 7 {
		return h
	}
	return h[:7]
}

// statusOf maps a letter grade to one of the dataviz skill's fixed status
// roles (good/warning/serious/critical) — never a generated or
// interpolated color, always one of the four reserved steps.
func statusOf(s core.Score) string {
	switch s.Grade {
	case "A", "B":
		return "good"
	case "C":
		return "warning"
	case "D":
		return "serious"
	default:
		return "critical"
	}
}

func severityStatus(sev core.Severity) string {
	switch sev {
	case core.Critical:
		return "critical"
	case core.High:
		return "serious"
	case core.Medium:
		return "warning"
	default:
		return "good"
	}
}

// severityIcon mirrors the CLI report's own icon() (output/cli.go) —
// same two icons for the same severities, so a reader moving between
// --format cli and --format html sees a consistent visual language. Also
// satisfies the dataviz skill's "status color never carries meaning
// alone" rule: the badge always pairs the color with this icon and the
// severity's own name as text, never color by itself.
func severityIcon(sev core.Severity) string {
	switch sev {
	case core.Critical, core.High:
		return "❌"
	default:
		return "⚠️"
	}
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>RepoScan report — {{.Path}}</title>
<style>
  :root {
    color-scheme: light;
    --surface-1: #fcfcfb;
    --page: #f9f9f7;
    --text-primary: #0b0b0b;
    --text-secondary: #52514e;
    --text-muted: #898781;
    --gridline: #e1e0d9;
    --border: rgba(11,11,11,0.10);
    --status-good: #0ca30c;
    --status-warning: #fab219;
    --status-serious: #ec835a;
    --status-critical: #d03b3b;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      color-scheme: dark;
      --surface-1: #1a1a19;
      --page: #0d0d0d;
      --text-primary: #ffffff;
      --text-secondary: #c3c2b7;
      --text-muted: #898781;
      --gridline: #2c2c2a;
      --border: rgba(255,255,255,0.10);
      --status-good: #0ca30c;
      --status-warning: #fab219;
      --status-serious: #ec835a;
      --status-critical: #d03b3b;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    background: var(--page);
    color: var(--text-primary);
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    line-height: 1.5;
  }
  .wrap { max-width: 860px; margin: 0 auto; padding: 32px 20px 64px; }
  header { margin-bottom: 32px; }
  h1 { font-size: 20px; margin: 0 0 4px; }
  .meta { color: var(--text-secondary); font-size: 13px; }
  .card {
    background: var(--surface-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 24px;
    margin-bottom: 24px;
  }
  .hero { display: flex; align-items: baseline; gap: 16px; flex-wrap: wrap; }
  .hero-value { font-size: 56px; font-weight: 600; line-height: 1; }
  .hero-grade {
    display: inline-flex; align-items: center; gap: 6px;
    font-size: 15px; font-weight: 600; color: var(--text-primary);
    padding: 4px 10px; border-radius: 6px; background: var(--gridline);
  }
  .dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
  .dot--good { background: var(--status-good); }
  .dot--warning { background: var(--status-warning); }
  .dot--serious { background: var(--status-serious); }
  .dot--critical { background: var(--status-critical); }
  h2 { font-size: 14px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--text-secondary); margin: 0 0 16px; }
  .meter-row { display: grid; grid-template-columns: 120px 1fr 60px; align-items: center; gap: 12px; margin-bottom: 10px; }
  .meter-label { font-size: 13px; color: var(--text-primary); text-transform: capitalize; }
  .meter-track { height: 10px; background: var(--gridline); border-radius: 5px; overflow: hidden; }
  .meter-fill { height: 100%; border-radius: 5px; }
  .meter-fill--good { background: var(--status-good); }
  .meter-fill--warning { background: var(--status-warning); }
  .meter-fill--serious { background: var(--status-serious); }
  .meter-fill--critical { background: var(--status-critical); }
  .meter-value { font-size: 13px; color: var(--text-secondary); text-align: right; font-variant-numeric: tabular-nums; }
  .finding { padding: 16px 0; border-top: 1px solid var(--gridline); }
  .finding:first-child { border-top: none; padding-top: 0; }
  .finding-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
  .badge { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; }
  .badge--good { color: var(--status-good); }
  .badge--warning { color: #a97300; }
  .badge--serious { color: #b3441c; }
  .badge--critical { color: var(--status-critical); }
  .finding-title { font-size: 15px; font-weight: 600; }
  .finding-loc { font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .finding-message { font-size: 14px; color: var(--text-secondary); margin: 4px 0; }
  .finding-fix { font-size: 13px; color: var(--text-secondary); }
  .finding-fix strong { color: var(--text-primary); }
  .finding-context { font-size: 12px; color: var(--text-muted); margin-top: 4px; font-style: italic; }
  .empty { color: var(--text-secondary); font-size: 14px; }
</style>
</head>
<body>
<div class="wrap">
  <header>
    <h1>RepoScan report</h1>
    <div class="meta">{{.Path}} — generated {{.GeneratedAt}}</div>
  </header>

  <div class="card hero">
    <div class="hero-value">{{.Score.Value}}<span style="font-size:24px;color:var(--text-secondary)">/100</span></div>
    <div class="hero-grade"><span class="dot dot--{{statusOf .Score}}"></span>Grade {{.Score.Grade}}</div>
  </div>

  {{if .Breakdown}}
  <div class="card">
    <h2>By category</h2>
    {{range .Breakdown}}
    <div class="meter-row">
      <div class="meter-label">{{.Category}}</div>
      <div class="meter-track"><div class="meter-fill meter-fill--{{statusOf .}}" style="width:{{.Value}}%"></div></div>
      <div class="meter-value">{{.Value}}/100</div>
    </div>
    {{end}}
  </div>
  {{end}}

  <div class="card">
    <h2>Findings ({{len .Findings}})</h2>
    {{if .Findings}}
      {{range .Findings}}
      <div class="finding">
        <div class="finding-head">
          <span class="badge badge--{{severityStatus .Severity}}">{{severityIcon .Severity}} {{.Severity}}</span>
          <span class="finding-title">{{.Title}}</span>
          <span class="finding-loc">{{.File}}{{if .Line}}:{{.Line}}{{end}}{{if .CommitHash}} · commit {{shortHash .CommitHash}}{{end}}</span>
        </div>
        <div class="finding-message">{{.Message}}</div>
        <div class="finding-fix"><strong>Fix:</strong> {{.Fix}}</div>
        {{if .Context}}<div class="finding-context">{{.Context}}</div>{{end}}
      </div>
      {{end}}
    {{else}}
      <div class="empty">No findings.</div>
    {{end}}
  </div>
</div>
</body>
</html>
`
