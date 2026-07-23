package core

import (
	"os"
	"testing"
	"time"
)

// slowAnalyzer simulates a future analyzer (or a bug in one) that hangs on
// a given file -- exactly the risk this file exists to guard against,
// flagged as the single most serious silent architectural gap in an
// external review: nothing structurally prevented a hang before this.
type slowAnalyzer struct {
	name  string
	delay time.Duration
}

func (s slowAnalyzer) Name() string { return s.name }

func (s slowAnalyzer) Run(file FileContext) []Finding {
	time.Sleep(s.delay)
	return []Finding{{ID: s.name + ".finding", Category: "test"}}
}

func TestRunAnalyzer_TimesOutWithoutHanging(t *testing.T) {
	orig := AnalyzerTimeout
	AnalyzerTimeout = 50 * time.Millisecond
	defer func() { AnalyzerTimeout = orig }()

	findings := RunAnalyzer(slowAnalyzer{name: "slow", delay: 500 * time.Millisecond}, FileContext{Path: "big.txt"})
	if findings != nil {
		t.Errorf("got %v, want nil -- an analyzer that exceeds the budget must not contribute findings from that call", findings)
	}
}

func TestRunAnalyzer_ReturnsFindingsWhenWithinBudget(t *testing.T) {
	orig := AnalyzerTimeout
	AnalyzerTimeout = 200 * time.Millisecond
	defer func() { AnalyzerTimeout = orig }()

	findings := RunAnalyzer(slowAnalyzer{name: "fast", delay: 0}, FileContext{Path: "small.txt"})
	if len(findings) != 1 || findings[0].ID != "fast.finding" {
		t.Errorf("got %+v, want [{ID: fast.finding}] -- an analyzer well within budget must not be affected", findings)
	}
}

// TestRunAnalyzer_ScanDoesNotWaitForAStuckAnalyzer confirms the guard at
// the level it actually matters: a Scanner with one slow analyzer and one
// normal one still returns promptly with the normal analyzer's findings,
// instead of the whole scan hanging on the slow one.
func TestRunAnalyzer_ScanDoesNotWaitForAStuckAnalyzer(t *testing.T) {
	orig := AnalyzerTimeout
	AnalyzerTimeout = 50 * time.Millisecond
	defer func() { AnalyzerTimeout = orig }()

	dir := t.TempDir()
	if err := os.WriteFile(dir+"/file.txt", []byte("irrelevant content"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(dir, slowAnalyzer{name: "stuck", delay: 10 * time.Second}, slowAnalyzer{name: "normal", delay: 0})

	done := make(chan []Finding, 1)
	go func() {
		findings, err := scanner.Scan()
		if err != nil {
			t.Error(err)
		}
		done <- findings
	}()

	select {
	case findings := <-done:
		var gotNormal bool
		for _, f := range findings {
			if f.ID == "normal.finding" {
				gotNormal = true
			}
			if f.ID == "stuck.finding" {
				t.Errorf("got a finding from the stuck analyzer -- it should have been abandoned after the timeout, not waited out")
			}
		}
		if !gotNormal {
			t.Errorf("got %+v, want the normal analyzer's finding present despite the stuck one", findings)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Scan() did not return within 2s -- it waited for the stuck analyzer instead of abandoning it per AnalyzerTimeout")
	}
}
