// Package secrets implements RepoScan's Phase 1 detection rules: hardcoded
// credentials in the working tree (not git history — that's Phase 2).
package secrets

import (
	"bytes"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/xchebila/reposcan/core"
)

type rule struct {
	id       string
	title    string
	message  string
	fix      string
	severity core.Severity
	pattern  *regexp.Regexp
	// prefilter lists literal substrings, checked with bytes.Contains before
	// running pattern; the rule only proceeds to the regex if at least one
	// is present. Purely a speed optimization (skips the regex engine on
	// the vast majority of files that can't possibly match) — it never
	// changes which files match, since pattern always matches a
	// superstring containing one of these literals.
	prefilter [][]byte
	// exclude, if set, is checked against each raw match; a true result
	// drops that match without creating a Finding. Used for well-known,
	// publicly documented placeholder values that would otherwise match
	// the pattern exactly — not a general noise filter.
	exclude func(match []byte) bool
}

func anyContains(content []byte, substrs [][]byte) bool {
	for _, s := range substrs {
		if bytes.Contains(content, s) {
			return true
		}
	}
	return false
}

// Patterns are intentionally specific (prefix + length) rather than generic
// "looks like base64" heuristics — generic patterns are exactly the kind of
// noisy rule the vision.md "signal > bruit" principle rules out.
var rules = []rule{
	{
		id:       "secrets.aws_access_key",
		title:    "AWS Access Key ID exposed",
		severity: core.Critical,
		message:  "An AWS access key ID was found hardcoded in this file. Combined with its secret key, it grants immediate programmatic access to this AWS account.",
		fix:      "Revoke this key in the AWS IAM console, then load credentials from environment variables or a secrets manager (AWS Secrets Manager, Vault).",
		// False positive, confirmed in the wild: AWS's own SDKs and docs
		// use AKIAIOSFODNN7EXAMPLE as the canonical placeholder access key
		// — found in vendor/github.com/aws/aws-sdk-go's own source comments
		// during git-history testing on prometheus. AWS's documented
		// convention is that every example key they publish ends in
		// "EXAMPLE", so that's the exclusion, not a one-off literal.
		pattern:   regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		prefilter: [][]byte{[]byte("AKIA")},
		exclude: func(match []byte) bool {
			return bytes.HasSuffix(match, []byte("EXAMPLE"))
		},
	},
	{
		id:       "secrets.github_token",
		title:    "GitHub token exposed",
		severity: core.Critical,
		message:  "A GitHub personal access / app token was found hardcoded in this file. It can be used to access or modify repositories with this token's permissions.",
		fix:      "Revoke the token at github.com/settings/tokens, then use environment variables or GitHub Actions secrets instead.",
		// False positive: a redacted/truncated example token in docs that
		// happens to preserve the full 36+ char suffix length.
		pattern:   regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),
		prefilter: [][]byte{[]byte("ghp_"), []byte("gho_"), []byte("ghu_"), []byte("ghs_"), []byte("ghr_")},
	},
	{
		id:        "secrets.stripe_key",
		title:     "Stripe live secret key exposed",
		severity:  core.Critical,
		message:   "A Stripe live secret key was found hardcoded in this file. It grants full access to charge cards and read customer data on this Stripe account.",
		fix:       "Roll the key from the Stripe dashboard immediately, then load it from an environment variable.",
		pattern:   regexp.MustCompile(`\bsk_live_[0-9a-zA-Z]{16,}\b`),
		prefilter: [][]byte{[]byte("sk_live_")},
	},
	{
		id:        "secrets.slack_token",
		title:     "Slack token exposed",
		severity:  core.High,
		message:   "A Slack token was found hardcoded in this file. It can be used to read messages or post as this app/user depending on scope.",
		fix:       "Revoke the token in Slack app settings, then use environment variables.",
		pattern:   regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z-]{10,48}\b`),
		prefilter: [][]byte{[]byte("xoxb-"), []byte("xoxa-"), []byte("xoxp-"), []byte("xoxr-"), []byte("xoxs-")},
	},
	{
		id:       "secrets.discord_token",
		title:    "Discord bot token exposed",
		severity: core.High,
		message:  "A Discord bot token was found hardcoded in this file. It grants control of the bot account, including any server it has joined.",
		fix:      "Regenerate the token in the Discord Developer Portal, then load it from an environment variable.",
		pattern:  regexp.MustCompile(`\b[MN][A-Za-z\d]{23}\.[\w-]{6}\.[\w-]{27}\b`),
	},
	{
		id:       "secrets.openai_key",
		title:    "OpenAI API key exposed",
		severity: core.Critical,
		message:  "An OpenAI API key was found hardcoded in this file. It can be used to consume paid API credits on this account.",
		fix:      "Revoke the key at platform.openai.com/api-keys, then load it from an environment variable.",
		// False positive: OpenAI-compatible third-party APIs (e.g. some
		// local LLM proxies) reuse the "sk-" prefix convention for keys that
		// aren't OpenAI credentials at all.
		pattern:   regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`),
		prefilter: [][]byte{[]byte("sk-")},
	},
	{
		id:       "secrets.private_key_block",
		title:    "Private key exposed",
		severity: core.Critical,
		message:  "A PEM-encoded private key was found in this file. Anyone with repo access can now impersonate the holder of this key (TLS, SSH, or signing).",
		fix:      "Revoke/rotate the corresponding certificate or SSH key pair immediately, then remove the key from the repo and store it outside version control.",
		// Requires a full BEGIN/END block with a plausibly long body, not
		// just the header text — otherwise this matches documentation that
		// shows the PEM header followed by a truncated/placeholder body
		// (e.g. "MIIEowIBAAKCAQEA..." or "...xxxx") with no real key
		// material, which is common in tutorials and API reference docs.
		pattern:   regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |OPENSSH |DSA |)PRIVATE KEY-----.{60,}?-----END (?:RSA |EC |OPENSSH |DSA |)PRIVATE KEY-----`),
		prefilter: [][]byte{[]byte("-----BEGIN")},
	},
	{
		id:       "secrets.jwt",
		title:    "Raw JWT exposed",
		severity: core.Medium,
		message:  "A raw JSON Web Token was found hardcoded in this file. If it hasn't expired, it can be replayed to impersonate whatever it authenticates.",
		fix:      "Remove the token from source and treat it as compromised: rotate the signing key or reissue affected tokens.",
		// False positive: JWT format examples in API documentation or test
		// fixtures using non-expiring, non-sensitive dummy tokens.
		pattern:   regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		prefilter: [][]byte{[]byte("eyJ")},
	},
}

// sensitiveFilenames are files whose mere presence in the working tree is
// the finding, regardless of content (private key material, env files).
var sensitiveFilenames = regexp.MustCompile(`(^|/)(id_rsa|id_dsa|id_ecdsa|id_ed25519)$`)

type Analyzer struct{}

func New() *Analyzer { return &Analyzer{} }

func (a *Analyzer) Name() string { return "secrets" }

func (a *Analyzer) Run(file core.FileContext) []core.Finding {
	var findings []core.Finding

	findings = append(findings, checkFilename(file.Path)...)

	// Match each rule against the whole file in one pass rather than
	// splitting into lines and re-testing every rule per line: with 9
	// rules that's O(lines * rules) regex invocations, which is what
	// pushed large doc-heavy repos (e.g. fastapi's translated markdown)
	// past the 5s MVP scan budget. A literal prefilter skips the regex
	// engine entirely for files that can't possibly match.
	nl := newlineIndex(file.Content)
	for _, r := range rules {
		if r.prefilter != nil && !anyContains(file.Content, r.prefilter) {
			continue
		}
		for _, loc := range r.pattern.FindAllIndex(file.Content, -1) {
			if r.exclude != nil && r.exclude(file.Content[loc[0]:loc[1]]) {
				continue
			}
			findings = append(findings, core.Finding{
				ID:       r.id,
				Severity: r.severity,
				Title:    r.title,
				Message:  r.message,
				Fix:      r.fix,
				File:     file.Path,
				Line:     nl.lineAt(loc[0]),
				Category: "secrets",
			})
		}
	}

	// Advisory-only hint, never a severity change: a real secret hidden
	// under testdata/ is still a real secret (see
	// docs/decisions/0001-test-fixture-context.md).
	if core.LooksLikeTestPath(file.Path) {
		for i := range findings {
			findings[i].Context = "path looks like a test/fixture directory — verify this isn't a real secret before treating it as routine"
		}
	}

	return findings
}

// lineIndex maps a byte offset into a file's content to a 1-based line
// number, without re-scanning from the start for every match.
type lineIndex []int

func newlineIndex(content []byte) lineIndex {
	var idx lineIndex
	for i, b := range content {
		if b == '\n' {
			idx = append(idx, i)
		}
	}
	return idx
}

func (idx lineIndex) lineAt(offset int) int {
	return sort.SearchInts(idx, offset) + 1
}

// .pem/.key extensions are intentionally NOT flagged by filename alone: a
// .pem file is just as often a public certificate (safe to commit) as a
// private key. secrets.private_key_block catches the actual private key
// material by content instead, whatever the file's extension.
func checkFilename(path string) []core.Finding {
	base := filepath.Base(path)

	switch {
	case base == ".env":
		return []core.Finding{{
			ID:       "secrets.env_committed",
			Severity: core.Critical,
			Title:    ".env file committed",
			Message:  "A .env file is tracked in this repository. These files typically hold database credentials, API keys, and other secrets meant to stay local.",
			Fix:      "Remove the file from version control (git rm --cached .env), add .env to .gitignore, and rotate any secrets it contained.",
			File:     path,
			Category: "secrets",
		}}
	case sensitiveFilenames.MatchString(path):
		return []core.Finding{{
			ID:       "secrets.ssh_private_key",
			Severity: core.Critical,
			Title:    "SSH private key committed",
			Message:  "An SSH private key file is tracked in this repository, allowing anyone with repo access to authenticate as its owner.",
			Fix:      "Remove the file from version control immediately and rotate the SSH key pair on every host that trusts it.",
			File:     path,
			Category: "secrets",
		}}
	}
	return nil
}
