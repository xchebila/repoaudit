# Security Policy

RepoScan scans other people's repositories for security problems, so a vulnerability in RepoScan itself deserves at least the same seriousness it asks of everyone else's code.

## Supported versions

RepoScan is pre-1.x-stable and mono-maintained. Only the latest tagged release and `main` are supported — there is no long-term-support branch yet.

| Version | Supported |
|---|---|
| latest tag | ✅ |
| `main` | ✅ |
| older tags | ❌ |

## Reporting a vulnerability

**Please use [GitHub's private vulnerability reporting](https://github.com/xchebila/reposcan/security/advisories/new)** for this repository, rather than a public issue. This keeps the report private until a fix is available.

If that isn't available to you for some reason, open a regular issue asking to be contacted privately — don't post exploit details or proof-of-concept code in a public issue.

Please include:
- What you found and where (file/line if applicable).
- A minimal way to reproduce it.
- What you think the impact is (what an attacker could actually do with it).

## What counts as a security issue here

In scope:
- Anything that lets a scanned repository's content (a crafted `Dockerfile`, `.github/workflows/*.yml`, `go.sum`/`requirements.txt`, or any file RepoScan reads) cause RepoScan to execute code, read/write outside the scanned path, or otherwise do something the user running it didn't ask for.
- Anything in the [plugin protocol](docs/plugin-protocol.md) that breaks the byte-only boundary between the host and a plugin process (e.g. a plugin gaining access to something beyond the file bytes it's handed).
- Credential/secret handling bugs in RepoScan's own code (not in the repos it scans) — e.g. a secret RepoScan detects ending up logged or written somewhere it shouldn't.
- Supply-chain issues in RepoScan's own dependencies or release/distribution path (`action.yml`, the [Homebrew tap](https://github.com/xchebila/homebrew-reposcan), `go install`).

Out of scope (please file as a regular bug instead):
- False negatives or false positives in detection rules (a secret pattern RepoScan misses, or flags incorrectly) — these are accuracy bugs, not vulnerabilities in RepoScan itself, unless the false negative is *caused by* an exploitable flaw in the detection logic rather than a rule simply not covering that case yet.
- Findings from `--deps` about RepoScan's own third-party dependencies (`go.mod`/`go.sum`) — please open a regular issue or PR; these get the same triage as any other dependency update.

## Response

This is a solo-maintained project. There's no SLA, but reports will be acknowledged and triaged as soon as reasonably possible, and credited in the fix's commit/release notes unless you ask not to be.
