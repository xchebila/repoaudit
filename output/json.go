package output

import (
	"encoding/json"
	"io"

	"github.com/xchebila/reposcan/core"
)

// schemaVersion versions the JSON output independently of core.Finding's
// Go shape — the same reasoning as the plugin protocol's protocol_version
// (docs/decisions/0008-plugin-system-scope.md): a machine consumer parsing
// --format json shouldn't break because an internal Go field got renamed
// for readability. Bump this, not silently change field names, if the
// shape needs to change.
const schemaVersion = "1.0"

type jsonFinding struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	Fix        string `json:"fix"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	CommitHash string `json:"commit_hash"`
	Category   string `json:"category"`
	Context    string `json:"context"`
}

type jsonScore struct {
	Value int    `json:"value"`
	Grade string `json:"grade"`
}

type jsonReport struct {
	SchemaVersion string        `json:"schema_version"`
	Score         jsonScore     `json:"score"`
	Findings      []jsonFinding `json:"findings"`
}

// WriteJSONReport serializes findings and score for a machine consumer
// (a CI script parsing results, another tool). Every Finding field is
// always present, even when empty (e.g. Context, CommitHash) — omitting
// unset fields (encoding/json's omitempty) would make a consumer unable
// to tell "not applicable" apart from "the field was dropped this time".
func WriteJSONReport(w io.Writer, findings []core.Finding, score core.Score) error {
	report := jsonReport{
		SchemaVersion: schemaVersion,
		Score:         jsonScore{Value: score.Value, Grade: score.Grade},
		Findings:      make([]jsonFinding, len(findings)),
	}
	for i, f := range findings {
		report.Findings[i] = jsonFinding{
			ID:         f.ID,
			Severity:   string(f.Severity),
			Title:      f.Title,
			Message:    f.Message,
			Fix:        f.Fix,
			File:       f.File,
			Line:       f.Line,
			CommitHash: f.CommitHash,
			Category:   f.Category,
			Context:    f.Context,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
