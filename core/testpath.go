package core

import "regexp"

var testPathPattern = regexp.MustCompile(`(^|/)(test|tests|testdata|fixture|fixtures)(/|$)`)

// LooksLikeTestPath reports whether path contains a conventional test/fixture
// directory segment. It is advisory only — see Finding.Context — and must
// never be used to change a Finding's Severity: a real secret hidden under
// testdata/ is still a real secret (docs/decisions/0001-test-fixture-context.md).
func LooksLikeTestPath(path string) bool {
	return testPathPattern.MatchString(path)
}
