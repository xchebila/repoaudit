package output

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/xchebila/repoaudit/core"
)

// timestampPattern matches WriteHTMLReport's "GeneratedAt" stamp
// (time.Now().Format("2006-01-02 15:04")) so it can be normalized before
// comparing against a golden file -- the real value changes every run by
// definition, so a literal byte-diff would never be stable otherwise.
var timestampPattern = regexp.MustCompile(`generated \d{4}-\d{2}-\d{2} \d{2}:\d{2}`)

func normalizeHTMLTimestamp(b []byte) []byte {
	return timestampPattern.ReplaceAll(b, []byte("generated 2000-01-01 00:00"))
}

func TestWriteHTMLReport_Golden(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTMLReport(&buf, goldenFindings, goldenScore, "/repo"); err != nil {
		t.Fatalf("WriteHTMLReport: %v", err)
	}

	assertBalancedHTML(t, buf.Bytes())

	goldenCompare(t, "testdata/report.html.golden", buf.Bytes(), normalizeHTMLTimestamp)
}

func TestWriteHTMLReport_EmptyFindings(t *testing.T) {
	var buf bytes.Buffer
	err := WriteHTMLReport(&buf, nil, core.Score{Value: 100, Grade: "A"}, "/repo")
	if err != nil {
		t.Fatalf("WriteHTMLReport: %v", err)
	}
	assertBalancedHTML(t, buf.Bytes())
	if !bytes.Contains(buf.Bytes(), []byte("No findings.")) {
		t.Error("empty-findings report does not show the empty-state message")
	}
}

// assertBalancedHTML is a minimal structural check (every opening tag has a
// matching close, in order) -- not a full HTML validator, just enough to
// catch a broken template (an unclosed {{if}}/{{range}} producing malformed
// markup), same spirit as the html.parser-based check described in
// docs/testing.md, done here with the stdlib instead of shelling out to
// Python.
func assertBalancedHTML(t *testing.T, doc []byte) {
	t.Helper()
	tagPattern := regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)[^>]*?(/?)>`)
	voidElements := map[string]bool{"meta": true, "link": true, "br": true, "img": true, "input": true, "hr": true}

	var stack []string
	for _, m := range tagPattern.FindAllSubmatch(doc, -1) {
		closing, name, selfClosed := string(m[1]) == "/", string(m[2]), string(m[3]) == "/"
		if selfClosed || voidElements[name] {
			continue
		}
		if closing {
			if len(stack) == 0 || stack[len(stack)-1] != name {
				t.Fatalf("unbalanced HTML: found </%s> with stack %v", name, stack)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		stack = append(stack, name)
	}
	if len(stack) != 0 {
		t.Fatalf("unbalanced HTML: unclosed tags remain: %v", stack)
	}
}
