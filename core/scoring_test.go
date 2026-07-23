package core

import "testing"

// TestComputeCategoryBreakdown_PartitionsWithoutDuplicationOrLoss is the
// explicit test requested before merging the HTML dashboard's category
// breakdown: every finding must land in exactly one category's Score,
// never two, never zero. The oracle is ComputeCategoryScore itself,
// already trusted — each category's breakdown value must equal what you
// get by manually filtering findings to that category and scoring them
// directly. If partitioning ever double-counted or dropped a finding,
// these two computations would diverge.
func TestComputeCategoryBreakdown_PartitionsWithoutDuplicationOrLoss(t *testing.T) {
	findings := []Finding{
		{ID: "a", Severity: Critical, Category: "secrets"},
		{ID: "b", Severity: Low, Category: "secrets"},
		{ID: "c", Severity: High, Category: "docker"},
		{ID: "d", Severity: Medium, Category: "cicd"},
		{ID: "e", Severity: Medium, Category: "cicd"},
	}
	categories := []string{"secrets", "docker", "cicd"}

	breakdown := ComputeCategoryBreakdown(findings)
	if len(breakdown) != len(categories) {
		t.Fatalf("expected %d categories, got %d: %+v", len(categories), len(breakdown), breakdown)
	}

	got := map[string]Score{}
	for _, s := range breakdown {
		if _, dup := got[s.Category]; dup {
			t.Fatalf("category %q appeared twice in the breakdown", s.Category)
		}
		got[s.Category] = s
	}

	totalFindingsAccountedFor := 0
	for _, cat := range categories {
		var filtered []Finding
		for _, f := range findings {
			if f.Category == cat {
				filtered = append(filtered, f)
			}
		}
		totalFindingsAccountedFor += len(filtered)

		want := ComputeCategoryScore(filtered)
		want.Category = cat

		g, ok := got[cat]
		if !ok {
			t.Errorf("category %q missing from breakdown", cat)
			continue
		}
		if g.Value != want.Value || g.Grade != want.Grade {
			t.Errorf("category %q: breakdown gave %+v, direct filter+score gave %+v", cat, g, want)
		}
	}

	if totalFindingsAccountedFor != len(findings) {
		t.Fatalf("test fixture itself is inconsistent: %d findings across known categories, %d total findings", totalFindingsAccountedFor, len(findings))
	}
}

// TestComputeCategoryBreakdown_TotalIsNotAnAggregateOfCategoryScores
// guards the scoring principle the repoaudit-finding skill and multiple
// ADRs (0003, 0005) call non-negotiable: a critical must dominate the
// total score, never get diluted by averaging against clean categories
// elsewhere. This is the failure mode a naive dashboard implementation
// could introduce by computing the total as sum/len(categoryScores)
// instead of calling ComputeCategoryScore on the full findings list.
func TestComputeCategoryBreakdown_TotalIsNotAnAggregateOfCategoryScores(t *testing.T) {
	findings := []Finding{
		{ID: "leak", Severity: Critical, Category: "secrets"},
		{ID: "tag", Severity: Low, Category: "docker"},
		{ID: "dep", Severity: Low, Category: "cicd"},
	}

	total := ComputeCategoryScore(findings)
	breakdown := ComputeCategoryBreakdown(findings)

	sum := 0
	for _, s := range breakdown {
		sum += s.Value
	}
	naiveAverage := sum / len(breakdown)

	if total.Value == naiveAverage {
		t.Fatalf("total score (%d) equals the naive average of category scores (%d) — this fixture no longer distinguishes independent scoring from averaging; adjust severities so the two diverge", total.Value, naiveAverage)
	}
	if total.Value >= 50 {
		t.Errorf("a single CRITICAL finding should dominate the total (expected well under 50), got %d — naive average of categories was %d", total.Value, naiveAverage)
	}
}
