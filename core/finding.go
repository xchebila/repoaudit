package core

// Severity reflects real-world impact, not code-quality style. Critical means
// an active leak or immediate account access — see vision.md scoring principle.
type Severity string

const (
	Critical Severity = "CRITICAL"
	High     Severity = "HIGH"
	Medium   Severity = "MEDIUM"
	Low      Severity = "LOW"
)

// Finding is the only unit of output RepoScan produces. Message and Fix are
// mandatory: a Finding without them is noise, not signal (vision.md).
type Finding struct {
	ID         string
	Severity   Severity
	Title      string
	Message    string
	Fix        string
	File       string
	Line       int
	CommitHash string
	Category   string
	// Context is an optional, non-authoritative hint for triage (e.g. "path
	// looks like a test/fixture directory"). It never changes Severity —
	// see docs/decisions/0001-test-fixture-context.md for why RepoScan
	// refuses to auto-downgrade severity based on a path pattern.
	Context string
}
