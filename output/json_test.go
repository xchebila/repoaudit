package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/xchebila/repoaudit/core"
)

func TestWriteJSONReport_Golden(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSONReport(&buf, goldenFindings, goldenScore); err != nil {
		t.Fatalf("WriteJSONReport: %v", err)
	}

	// Syntactic validity checked directly, not just assumed from the golden
	// diff below -- this is the property ADR 0009/docs/testing.md call out
	// as the first thing to verify about --format json.
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	goldenCompare(t, "testdata/report.json.golden", buf.Bytes(), nil)
}

func TestWriteJSONReport_EmptyFindingsIsEmptyArrayNotNull(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSONReport(&buf, nil, core.Score{Value: 100, Grade: "A"}); err != nil {
		t.Fatalf("WriteJSONReport: %v", err)
	}

	var decoded struct {
		Findings []any `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if decoded.Findings == nil {
		t.Error("findings is null, want an empty array -- a consumer shouldn't have to handle both null and []")
	}
	if len(decoded.Findings) != 0 {
		t.Errorf("got %d findings, want 0", len(decoded.Findings))
	}
}
