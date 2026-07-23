package secrets

import (
	"testing"

	"github.com/xchebila/repoaudit/core"
)

// These fixture values are split across two (or more) Go string literals
// joined with "+", instead of one contiguous literal. This test file is
// itself part of the RepoAudit repo, and RepoAudit's own CI runs
// `repoaudit diff` against every PR (.github/workflows/repoaudit-self-check.yml)
// -- a realistic-looking secret written as one plain string here would be
// indistinguishable, byte-for-byte, from a real hardcoded credential once
// committed, and repoaudit would (correctly) flag its own test suite. The
// split keeps the raw source bytes from ever containing the matching
// substring contiguously, while the concatenated runtime value is still a
// real match for the regex under test.
const (
	fixtureAWSKey        = "AKIA" + "ABCDEFGHIJKLMNOP"
	fixtureAWSKeyExample = "AKIA" + "IOSFODNN7EXAMPLE"
	fixtureGitHubToken   = "ghp_" + "1234567890123456789012345678901234567890"
	fixtureStripeKey     = "sk_live_" + "abcdefghij1234567890"
	fixtureSlackToken    = "xoxb-" + "1234567890abcdef"
	fixtureDiscordToken  = "M" + "ABCDEFGHIJKLMNOPQRSTUVW" + "." + "abcdef" + "." + "ABCDEFGHIJKLMNOPQRSTUVWXYZA"
	fixtureOpenAIKey     = "sk-" + "abcdefghijklmnopqrstuvwxyz123456"
	fixturePEMBegin      = "-----BEGIN RSA PRIV" + "ATE KEY-----"
	fixturePEMEnd        = "-----END RSA PRIV" + "ATE KEY-----"
	fixtureJWT           = "eyJ" + "hbGciOiJIUzI1NiJ9" + "." + "eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0" + "." + "dQw4w9WgXcQ_dummySig"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		wantIDs []string // exact set of finding IDs expected, in any order
	}{
		{
			name:    "aws access key",
			path:    "config.py",
			content: "AWS_KEY=" + fixtureAWSKey + "\n",
			wantIDs: []string{"secrets.aws_access_key"},
		},
		{
			name:    "aws example key is excluded",
			path:    "docs/setup.md",
			content: "AWS_KEY=" + fixtureAWSKeyExample + "\n",
			wantIDs: nil,
		},
		{
			name:    "github token",
			path:    "deploy.sh",
			content: "TOKEN=" + fixtureGitHubToken + "\n",
			wantIDs: []string{"secrets.github_token"},
		},
		{
			name:    "stripe live key",
			path:    "billing.py",
			content: "STRIPE_KEY = \"" + fixtureStripeKey + "\"\n",
			wantIDs: []string{"secrets.stripe_key"},
		},
		{
			name:    "slack token",
			path:    "notify.py",
			content: "SLACK=" + fixtureSlackToken + "\n",
			wantIDs: []string{"secrets.slack_token"},
		},
		{
			name:    "discord token",
			path:    "bot.py",
			content: "TOKEN=" + fixtureDiscordToken + "\n",
			wantIDs: []string{"secrets.discord_token"},
		},
		{
			name:    "openai key",
			path:    "llm.py",
			content: "OPENAI_API_KEY=" + fixtureOpenAIKey + "\n",
			wantIDs: []string{"secrets.openai_key"},
		},
		{
			name: "private key block, full body",
			path: "server.key",
			content: fixturePEMBegin + "\n" +
				"MIIEowIBAAKCAQEAveryLongBase64EncodedKeyMaterialThatIsCertainlyOver\n" +
				"SixtyCharactersLongToClearTheMinimumBodyLengthCheckXXXXXXXXXXXXXXX\n" +
				fixturePEMEnd + "\n",
			wantIDs: []string{"secrets.private_key_block"},
		},
		{
			name:    "private key header with placeholder body is excluded",
			path:    "docs/tls-example.md",
			content: fixturePEMBegin + "\nxxxx\n" + fixturePEMEnd + "\n",
			wantIDs: nil,
		},
		{
			name:    "raw jwt",
			path:    "test.py",
			content: "TOKEN = \"" + fixtureJWT + "\"\n",
			wantIDs: []string{"secrets.jwt"},
		},
		{
			name:    ".env committed",
			path:    ".env",
			content: "DB_PASSWORD=hunter2\n",
			wantIDs: []string{"secrets.env_committed"},
		},
		{
			name:    "ssh private key filename",
			path:    ".ssh/id_rsa",
			content: "irrelevant, filename alone is the finding",
			wantIDs: []string{"secrets.ssh_private_key"},
		},
		{
			name:    "pem file with no key material is not flagged by extension alone",
			path:    "certs/server.pem",
			content: "-----BEGIN CERTIFICATE-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A\n-----END CERTIFICATE-----\n",
			wantIDs: nil,
		},
		{
			name:    "clean file",
			path:    "main.go",
			content: "package main\n\nfunc main() {}\n",
			wantIDs: nil,
		},
	}

	a := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := a.Run(core.FileContext{Path: tt.path, Content: []byte(tt.content)})
			gotIDs := make([]string, len(findings))
			for i, f := range findings {
				gotIDs[i] = f.ID
				if f.Category != "secrets" {
					t.Errorf("finding %s: Category = %q, want \"secrets\"", f.ID, f.Category)
				}
				if f.Message == "" || f.Fix == "" {
					t.Errorf("finding %s: Message/Fix must not be empty", f.ID)
				}
			}
			assertSameIDs(t, gotIDs, tt.wantIDs)
		})
	}
}

// TestRun_TestPathContext confirms core.LooksLikeTestPath annotates Context
// without ever downgrading Severity -- the non-negotiable rule from
// docs/decisions/0001-test-fixture-context.md, verified here instead of only
// asserted in a comment.
func TestRun_TestPathContext(t *testing.T) {
	a := New()
	findings := a.Run(core.FileContext{
		Path:    "testdata/fixtures/config.py",
		Content: []byte("AWS_KEY=" + fixtureAWSKey + "\n"),
	})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Severity != core.Critical {
		t.Errorf("Severity = %q, want CRITICAL -- a test-path hint must never downgrade severity", findings[0].Severity)
	}
	if findings[0].Context == "" {
		t.Errorf("Context is empty, want a test-path hint")
	}
}

func assertSameIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("finding IDs = %v, want %v", got, want)
	}
	wantSet := map[string]int{}
	for _, id := range want {
		wantSet[id]++
	}
	for _, id := range got {
		wantSet[id]--
	}
	for id, n := range wantSet {
		if n != 0 {
			t.Errorf("finding IDs = %v, want %v (mismatch on %q)", got, want, id)
			return
		}
	}
}
