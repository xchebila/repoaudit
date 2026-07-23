package dependencies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xchebila/repoaudit/core"
)

const (
	osvBatchURL = "https://api.osv.dev/v1/querybatch"
	osvVulnURL  = "https://api.osv.dev/v1/vulns/"

	// Short, per ADR 0004: this is opt-in, but a hung request still
	// shouldn't hang the whole scan indefinitely. The batch query gets
	// more time since one request can cover hundreds of dependencies;
	// each detail fetch is a single small GET.
	batchTimeout  = 10 * time.Second
	detailTimeout = 5 * time.Second

	// Bounds concurrent detail fetches (one per distinct vulnerability ID
	// found) so a repo with many vulnerable dependencies doesn't open
	// hundreds of simultaneous connections to the same API.
	detailConcurrency = 8

	// OSV.dev's querybatch endpoint rejects more than this many queries in
	// one request with {"code":3,"message":"too many queries"} — this
	// isn't documented anywhere; found by bisecting against the live API
	// after a 1075-dependency repo (prometheus, 5 go.sum files) silently
	// got zero results from a request that turned out to 400. Chunking
	// below is the fix.
	maxBatchSize = 1000
)

// Result carries findings plus a non-fatal Warning: a network failure
// degrades to "checked nothing, said why" rather than failing the scan —
// see docs/decisions/0004-dependency-scanner-network.md.
type Result struct {
	Findings []core.Finding
	Warning  string
}

// CheckVulnerabilities queries OSV.dev for every dependency and returns a
// Finding for each known vulnerability found. Network calls only happen
// here, never in Discover.
func CheckVulnerabilities(deps []Dependency) Result {
	if len(deps) == 0 {
		return Result{}
	}

	batchClient := &http.Client{Timeout: batchTimeout}
	vulnIDsPerDep, err := queryBatch(batchClient, deps)
	if err != nil {
		return Result{Warning: fmt.Sprintf("dependency check skipped: %v", err)}
	}

	uniqueIDs := map[string]bool{}
	for _, ids := range vulnIDsPerDep {
		for _, id := range ids {
			uniqueIDs[id] = true
		}
	}
	if len(uniqueIDs) == 0 {
		return Result{}
	}

	detailClient := &http.Client{Timeout: detailTimeout}
	details := fetchDetailsConcurrently(detailClient, uniqueIDs)

	var findings []core.Finding
	for i, dep := range deps {
		for _, id := range dedupeAliases(vulnIDsPerDep[i], details) {
			findings = append(findings, buildFinding(dep, id, details[id]))
		}
	}
	return Result{Findings: findings}
}

// dedupeAliases collapses vulnerability IDs that are aliases of each other
// down to one representative ID. OSV mirrors the same real vulnerability
// under multiple ID schemes (GHSA, PYSEC, CVE, the ecosystem-native GO-*
// IDs) and a single package+version can match more than one of them at
// once — confirmed against the real API: GHSA-2xpw-w6gg-jr37 and
// PYSEC-2026-1994 are the same urllib3 vulnerability, word-for-word same
// summary, linked via GHSA-2xpw-w6gg-jr37's own "aliases" field. Without
// this, the same real issue would be reported (and scored) twice.
func dedupeAliases(ids []string, details map[string]*osvVulnDetail) []string {
	if len(ids) < 2 {
		return ids
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	canonical := make(map[string]string, len(ids))
	for _, id := range ids {
		canonical[id] = id
	}
	resolve := func(id string) string {
		for canonical[id] != id {
			id = canonical[id]
		}
		return id
	}

	for _, id := range ids {
		d := details[id]
		if d == nil {
			continue
		}
		for _, alias := range d.Aliases {
			if !idSet[alias] {
				continue
			}
			a, b := resolve(id), resolve(alias)
			if a == b {
				continue
			}
			canonical[a] = preferredID(a, b)
			canonical[b] = canonical[a]
		}
	}

	seen := make(map[string]bool, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		c := resolve(id)
		if !seen[c] {
			seen[c] = true
			result = append(result, c)
		}
	}
	return result
}

// preferredID picks which of two alias-linked IDs to keep: GHSA records
// tend to have more complete database_specific.severity data (see ADR
// 0006), so a GHSA- ID wins when one side has it and the other doesn't.
// Otherwise the choice is arbitrary but deterministic (lexicographic), so
// repeated scans of the same repo report the same ID.
func preferredID(a, b string) string {
	aGHSA := strings.HasPrefix(a, "GHSA-")
	bGHSA := strings.HasPrefix(b, "GHSA-")
	switch {
	case aGHSA && !bGHSA:
		return a
	case bGHSA && !aGHSA:
		return b
	case a < b:
		return a
	default:
		return b
	}
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvQuery struct {
	Package osvPackage `json:"package"`
	Version string     `json:"version"`
}

type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

type osvVulnRef struct {
	ID string `json:"id"`
}

type osvBatchResult struct {
	Vulns []osvVulnRef `json:"vulns"`
}

type osvBatchResponse struct {
	Results []osvBatchResult `json:"results"`
}

// queryBatch returns, for each dependency (by index, matching deps), the
// IDs of vulnerabilities OSV knows about for that exact package+version.
// This endpoint deliberately returns only ID + modified timestamp per
// vuln — full details need a separate GET per ID, done in
// fetchDetailsConcurrently. Requests over maxBatchSize queries are split
// into sequential chunks; a single 400 from any chunk fails the whole
// check (partial dependency coverage would be more misleading than an
// explicit "skipped").
func queryBatch(client *http.Client, deps []Dependency) ([][]string, error) {
	result := make([][]string, len(deps))
	for start := 0; start < len(deps); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(deps) {
			end = len(deps)
		}
		chunk, err := queryBatchChunk(client, deps[start:end])
		if err != nil {
			return nil, err
		}
		copy(result[start:end], chunk)
	}
	return result, nil
}

func queryBatchChunk(client *http.Client, deps []Dependency) ([][]string, error) {
	queries := make([]osvQuery, len(deps))
	for i, d := range deps {
		queries[i] = osvQuery{Package: osvPackage{Name: d.Name, Ecosystem: d.Ecosystem}, Version: d.Version}
	}

	body, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		return nil, err
	}

	resp, err := client.Post(osvBatchURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("could not reach OSV.dev (%w) — check network access or a proxy blocking outbound HTTPS", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSV.dev returned %s", resp.Status)
	}

	var parsed osvBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("unexpected OSV.dev response: %w", err)
	}

	result := make([][]string, len(deps))
	for i := range parsed.Results {
		if i >= len(result) {
			break
		}
		for _, v := range parsed.Results[i].Vulns {
			result[i] = append(result[i], v.ID)
		}
	}
	return result, nil
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type osvVulnDetail struct {
	ID               string        `json:"id"`
	Summary          string        `json:"summary"`
	Details          string        `json:"details"`
	Aliases          []string      `json:"aliases"`
	Severity         []osvSeverity `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

func fetchDetailsConcurrently(client *http.Client, ids map[string]bool) map[string]*osvVulnDetail {
	results := make(map[string]*osvVulnDetail, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, detailConcurrency)

	for id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			detail, err := fetchVulnDetail(client, id)
			if err != nil {
				return // this one ID's detail fetch failed; buildFinding degrades gracefully for it
			}
			mu.Lock()
			results[id] = detail
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return results
}

func fetchVulnDetail(client *http.Client, id string) (*osvVulnDetail, error) {
	resp, err := client.Get(osvVulnURL + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var detail osvVulnDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func buildFinding(dep Dependency, id string, detail *osvVulnDetail) core.Finding {
	if detail == nil {
		// The batch query confirmed this vulnerability applies to this
		// exact dependency, but the follow-up detail fetch failed (a
		// separate network call from the batch query itself). Report it
		// anyway, degraded, rather than silently dropping a confirmed
		// finding because of a secondary failure.
		return core.Finding{
			ID:       "dependencies.osv_" + strings.ToLower(dep.Ecosystem),
			Severity: core.Medium,
			Title:    fmt.Sprintf("%s: known vulnerability in %s@%s", id, dep.Name, dep.Version),
			Message:  fmt.Sprintf("OSV.dev confirms %s is affected by %s, but fetching the advisory's details failed (network error).", dep.Name, id),
			Fix:      fmt.Sprintf("See https://osv.dev/vulnerability/%s for the affected version range and fixed release, then upgrade %s.", id, dep.Name),
			File:     dep.Manifest,
			Category: "dependencies",
			Context:  "advisory details could not be fetched — severity defaulted to Medium, not derived from any OSV data",
		}
	}

	severity, context := mapSeverity(detail)
	message := detail.Summary
	if message == "" {
		message = detail.Details
	}
	if message == "" {
		message = fmt.Sprintf("%s has no summary in OSV.dev; see the advisory for details.", id)
	}

	return core.Finding{
		ID:       "dependencies.osv_" + strings.ToLower(dep.Ecosystem),
		Severity: severity,
		Title:    fmt.Sprintf("%s: known vulnerability in %s@%s", id, dep.Name, dep.Version),
		Message:  message,
		Fix:      fmt.Sprintf("Upgrade %s past the vulnerable range — see https://osv.dev/vulnerability/%s for the fixed version.", dep.Name, id),
		File:     dep.Manifest,
		Category: "dependencies",
		Context:  context,
	}
}

// mapSeverity buckets an OSV record into RepoAudit's four-level Severity.
// database_specific.severity (a simple CRITICAL/HIGH/MODERATE/LOW string,
// common on GHSA-sourced records) is authoritative when present — no
// Context needed. Otherwise, a CVSS vector is used to *estimate* severity
// via a coarse heuristic, not a real CVSS score computation (out of scope:
// implementing the full FIRST formula for three CVSS versions is more
// machinery than an MVP dependency check needs). Confirmed empirically
// that both cases occur in practice: a real GHSA record had both fields,
// a real native Go vulnerability database entry (GO-2023-1571) had
// neither, hence the final Medium fallback.
func mapSeverity(detail *osvVulnDetail) (core.Severity, string) {
	if s := strings.ToUpper(detail.DatabaseSpecific.Severity); s != "" {
		switch s {
		case "CRITICAL":
			return core.Critical, ""
		case "HIGH":
			return core.High, ""
		case "MODERATE":
			return core.Medium, ""
		case "LOW":
			return core.Low, ""
		}
	}

	for _, sev := range detail.Severity {
		if !strings.HasPrefix(sev.Type, "CVSS") {
			continue
		}
		if severity, ok := estimateSeverityFromCVSS(sev.Score); ok {
			return severity, "severity estimated from a CVSS vector, not an official severity rating from the source database"
		}
	}

	return core.Medium, "no severity information available from OSV.dev for this vulnerability — defaulted to Medium"
}

// estimateSeverityFromCVSS is a deliberately coarse heuristic over a CVSS
// v3/v3.1 vector string's Attack Vector and Impact metrics, not a real
// CVSS base score calculation. Roughly mirrors the shape of the official
// severity bands (network-exploitable + multiple high impacts ~=
// Critical; network-exploitable + one high impact ~= High) without
// implementing FIRST's actual weighted formula.
func estimateSeverityFromCVSS(vector string) (core.Severity, bool) {
	metrics := map[string]string{}
	for _, part := range strings.Split(vector, "/") {
		if k, v, ok := strings.Cut(part, ":"); ok {
			metrics[k] = v
		}
	}

	av, ac, ui := metrics["AV"], metrics["AC"], metrics["UI"]
	c, i, a := metrics["C"], metrics["I"], metrics["A"]
	if av == "" && c == "" && i == "" && a == "" {
		return "", false // not a CVSS v3-shaped vector this heuristic understands
	}

	highImpacts := 0
	for _, m := range []string{c, i, a} {
		if m == "H" {
			highImpacts++
		}
	}

	switch {
	case highImpacts >= 2 && av == "N" && ac == "L" && ui == "N":
		return core.Critical, true
	case highImpacts >= 1 && av == "N":
		return core.High, true
	case highImpacts >= 1:
		return core.Medium, true
	default:
		return core.Low, true
	}
}
