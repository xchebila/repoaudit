package core

import "sort"

// severityImpact are fixed points-per-finding within the ranges defined in
// vision.md's scoring principle. Repeated findings of the same severity are
// weighted with diminishing returns (each extra one counts half as much as
// the last) so that, say, five LOW findings can never outweigh one
// CRITICAL — additive-to-parity scoring is explicitly rejected by design.
var severityImpact = map[Severity]float64{
	Critical: 60,
	High:     25,
	Medium:   10,
	Low:      3,
}

// Score is the 0-100 result for one category (e.g. "secrets"). Grade is a
// letter derived from Score for the CLI summary line.
type Score struct {
	Category string
	Value    int
	Grade    string
}

// ComputeCategoryScore applies severity impacts with diminishing weight per
// repeat, starting from a perfect 100 and clamping at 0. A single Critical
// finding alone already drops the score to 40 — inside the "incident, not
// a deduction" zone described in vision.md.
func ComputeCategoryScore(findings []Finding) Score {
	counts := map[Severity]int{}
	for _, f := range findings {
		counts[f.Severity]++
	}

	total := 100.0
	for sev, impact := range severityImpact {
		count := counts[sev]
		weight := 1.0
		for i := 0; i < count; i++ {
			total -= impact * weight
			weight /= 2
		}
	}
	if total < 0 {
		total = 0
	}

	value := int(total)
	return Score{Value: value, Grade: grade(value)}
}

// ComputeCategoryBreakdown returns one Score per distinct Category present
// in findings — the per-category view vision.md's Phase 5 reporting spec
// shows (Secrets/Docker/CI-CD/... each scored, then a separate total).
// Each finding belongs to exactly one bucket, matching its own Category
// field exactly: partitioned first, then ComputeCategoryScore is applied
// to each bucket independently, so a finding can never be counted toward
// two categories or dropped from all of them. Sorted by category name for
// stable, deterministic output across runs.
//
// The total score (ComputeCategoryScore called separately on the full,
// unpartitioned findings list) is NOT derived from these per-category
// values — it is not their sum or average. Doing that would let a
// CRITICAL in one category get diluted by clean categories elsewhere,
// exactly what vision.md's scoring principle forbids: a critical must
// dominate the total regardless of which category it came from.
func ComputeCategoryBreakdown(findings []Finding) []Score {
	byCategory := map[string][]Finding{}
	for _, f := range findings {
		byCategory[f.Category] = append(byCategory[f.Category], f)
	}

	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	scores := make([]Score, 0, len(categories))
	for _, cat := range categories {
		s := ComputeCategoryScore(byCategory[cat])
		s.Category = cat
		scores = append(scores, s)
	}
	return scores
}

func grade(value int) string {
	switch {
	case value >= 90:
		return "A"
	case value >= 80:
		return "B"
	case value >= 70:
		return "C"
	case value >= 60:
		return "D"
	default:
		return "F"
	}
}
