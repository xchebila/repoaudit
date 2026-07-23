package dependencies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withFakeOSV points osvBatchURL/osvVulnURL at srv for the duration of one
// test, restoring the real URLs on cleanup -- these two package-level vars
// exist for exactly this seam (see osv.go).
func withFakeOSV(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origBatch, origVuln := osvBatchURL, osvVulnURL
	osvBatchURL = srv.URL + "/querybatch"
	osvVulnURL = srv.URL + "/vulns/"
	t.Cleanup(func() {
		osvBatchURL, osvVulnURL = origBatch, origVuln
		srv.Close()
	})
}

func TestDedupeAliases(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		details map[string]*osvVulnDetail
		want    []string
	}{
		{
			name: "single id, nothing to dedupe",
			ids:  []string{"GHSA-1111"},
			want: []string{"GHSA-1111"},
		},
		{
			name: "two unrelated ids stay separate",
			ids:  []string{"GHSA-1111", "GHSA-2222"},
			details: map[string]*osvVulnDetail{
				"GHSA-1111": {ID: "GHSA-1111"},
				"GHSA-2222": {ID: "GHSA-2222"},
			},
			want: []string{"GHSA-1111", "GHSA-2222"},
		},
		{
			name: "aliased pair collapses to the GHSA id",
			ids:  []string{"PYSEC-2026-1994", "GHSA-2xpw-w6gg-jr37"},
			details: map[string]*osvVulnDetail{
				"PYSEC-2026-1994":     {ID: "PYSEC-2026-1994"},
				"GHSA-2xpw-w6gg-jr37": {ID: "GHSA-2xpw-w6gg-jr37", Aliases: []string{"PYSEC-2026-1994"}},
			},
			want: []string{"GHSA-2xpw-w6gg-jr37"},
		},
		{
			name: "chain of aliases (A->B->C) all collapse to one",
			ids:  []string{"GO-2026-1", "GHSA-aaaa", "CVE-2026-1"},
			details: map[string]*osvVulnDetail{
				"GO-2026-1":  {ID: "GO-2026-1", Aliases: []string{"GHSA-aaaa"}},
				"GHSA-aaaa":  {ID: "GHSA-aaaa", Aliases: []string{"GO-2026-1", "CVE-2026-1"}},
				"CVE-2026-1": {ID: "CVE-2026-1", Aliases: []string{"GHSA-aaaa"}},
			},
			want: []string{"GHSA-aaaa"},
		},
		{
			name: "missing detail for one id doesn't panic, no dedup happens for it",
			ids:  []string{"GHSA-1111", "GHSA-2222"},
			details: map[string]*osvVulnDetail{
				"GHSA-1111": {ID: "GHSA-1111"},
				// GHSA-2222 has no detail entry at all.
			},
			want: []string{"GHSA-1111", "GHSA-2222"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeAliases(tt.ids, tt.details)
			assertSameStrings(t, got, tt.want)
		})
	}
}

func TestPreferredID(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"PYSEC-2026-1994", "GHSA-2xpw-w6gg-jr37", "GHSA-2xpw-w6gg-jr37"},
		{"GHSA-2xpw-w6gg-jr37", "PYSEC-2026-1994", "GHSA-2xpw-w6gg-jr37"},
		{"CVE-2026-1", "GO-2026-1", "CVE-2026-1"}, // neither is GHSA: lexicographic
		{"GO-2026-2", "GO-2026-1", "GO-2026-1"},
	}
	for _, tt := range tests {
		if got := preferredID(tt.a, tt.b); got != tt.want {
			t.Errorf("preferredID(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMapSeverity(t *testing.T) {
	tests := []struct {
		name      string
		detail    *osvVulnDetail
		wantSev   string
		wantNoCtx bool // true: Context must be empty (authoritative source)
	}{
		{
			name: "database_specific.severity is authoritative, no context",
			detail: &osvVulnDetail{DatabaseSpecific: struct {
				Severity string `json:"severity"`
			}{Severity: "CRITICAL"}},
			wantSev:   "CRITICAL",
			wantNoCtx: true,
		},
		{
			name: "MODERATE maps to this project's Medium",
			detail: &osvVulnDetail{DatabaseSpecific: struct {
				Severity string `json:"severity"`
			}{Severity: "MODERATE"}},
			wantSev:   "MEDIUM",
			wantNoCtx: true,
		},
		{
			name: "CVSS vector estimated, context discloses it's an estimate",
			detail: &osvVulnDetail{
				Severity: []osvSeverity{{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}},
			},
			wantSev:   "CRITICAL",
			wantNoCtx: false,
		},
		{
			name:      "no severity data at all: Medium fallback, context discloses it",
			detail:    &osvVulnDetail{},
			wantSev:   "MEDIUM",
			wantNoCtx: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sev, ctx := mapSeverity(tt.detail)
			if string(sev) != tt.wantSev {
				t.Errorf("severity = %q, want %q", sev, tt.wantSev)
			}
			if tt.wantNoCtx && ctx != "" {
				t.Errorf("context = %q, want empty (authoritative source)", ctx)
			}
			if !tt.wantNoCtx && ctx == "" {
				t.Errorf("context is empty, want a disclosure note")
			}
		})
	}
}

func TestEstimateSeverityFromCVSS(t *testing.T) {
	tests := []struct {
		name    string
		vector  string
		wantOK  bool
		wantSev string
	}{
		{
			name:   "not a CVSS-shaped vector at all",
			vector: "not a vector",
			wantOK: false,
		},
		{
			name:    "network, low complexity, no interaction, all high impact -> Critical",
			vector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
			wantOK:  true,
			wantSev: "CRITICAL",
		},
		{
			name:    "network-exploitable, one high impact -> High",
			vector:  "CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:H/I:N/A:N",
			wantOK:  true,
			wantSev: "HIGH",
		},
		{
			name:    "local access, one high impact -> Medium",
			vector:  "CVSS:3.1/AV:L/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
			wantOK:  true,
			wantSev: "MEDIUM",
		},
		{
			name:    "no high impacts at all -> Low",
			vector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N",
			wantOK:  true,
			wantSev: "LOW",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sev, ok := estimateSeverityFromCVSS(tt.vector)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && string(sev) != tt.wantSev {
				t.Errorf("severity = %q, want %q", sev, tt.wantSev)
			}
		})
	}
}

// TestQueryBatch_Chunking is the regression test for the real bug this
// project found against the live API (prometheus, 1075 deps, silent
// failure past 1000 queries per request): confirms deps are split into
// multiple requests once they exceed maxBatchSize, against a fake server
// that fails any single request over that size.
func TestQueryBatch_Chunking(t *testing.T) {
	var requestSizes []int

	mux := http.NewServeMux()
	mux.HandleFunc("/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req osvBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		requestSizes = append(requestSizes, len(req.Queries))
		if len(req.Queries) > maxBatchSize {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":3,"message":"too many queries"}`)
			return
		}
		// Echo back an empty result set per query -- chunking behavior is
		// what's under test here, not vulnerability content.
		resp := osvBatchResponse{Results: make([]osvBatchResult, len(req.Queries))}
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	withFakeOSV(t, srv)

	deps := make([]Dependency, maxBatchSize+1)
	for i := range deps {
		deps[i] = Dependency{Name: fmt.Sprintf("pkg%d", i), Version: "1.0.0", Ecosystem: "Go"}
	}

	client := &http.Client{}
	result, err := queryBatch(client, deps)
	if err != nil {
		t.Fatalf("queryBatch: %v", err)
	}
	if len(result) != len(deps) {
		t.Fatalf("got %d results, want %d", len(result), len(deps))
	}
	if len(requestSizes) < 2 {
		t.Fatalf("expected at least 2 chunked requests for %d deps, got %d requests: %v", len(deps), len(requestSizes), requestSizes)
	}
	for _, n := range requestSizes {
		if n > maxBatchSize {
			t.Errorf("a request carried %d queries, want <= %d (maxBatchSize)", n, maxBatchSize)
		}
	}
}

func assertSameStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	wantSet := map[string]int{}
	for _, s := range want {
		wantSet[s]++
	}
	for _, s := range got {
		wantSet[s]--
	}
	for s, n := range wantSet {
		if n != 0 {
			t.Errorf("got %v, want %v (mismatch on %q)", got, want, s)
			return
		}
	}
}
