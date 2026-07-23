# 🛡️ RepoAudit

**v1.0.0** — the full [vision.md](docs/vision.md) roadmap (Phases 1–5) is implemented: secrets, git history, Docker, CI/CD, dependency vulnerabilities, a diff mode for PRs, an external plugin system, and CLI/JSON/HTML reporting.

RepoAudit is a 10-second security sanity check for Git repositories. It doesn't analyze code quality — it detects real-world security mistakes that leak data or break production: committed secrets, exposed keys, tokens hardcoded in source, risky Dockerfile patterns, CI/CD workflow misconfigurations, and known-vulnerable dependencies.

Signal over noise: no 500 warnings, just what's actionable. Every finding explains *why* it's dangerous and *how* to fix it.

## Install / build

**Homebrew** (macOS or Linux):

```bash
brew tap xchebila/repoaudit
brew install repoaudit
```

Builds from source (`depends_on "go" => :build`) — no precompiled binaries, no separate release pipeline to maintain, same reasoning as the GitHub Action below. See [xchebila/homebrew-repoaudit](https://github.com/xchebila/homebrew-repoaudit).

**`go install`**, if you already have Go 1.24+:

```bash
go install github.com/xchebila/repoaudit@v1.0.0   # or @latest, or a commit SHA
```

**From source**:

```bash
git clone git@github.com:xchebila/repoaudit.git
cd repoaudit
go build -o repoaudit .
```

Or, with `make`: `make build` (same command, plus an embedded version via `--version`), `make check` (`go build`, `go vet`, `gofmt -l`, `go test` — the same checklist every PR in this project runs before merge), `make test`, `make clean`.

## Usage

```bash
./repoaudit scan .
./repoaudit scan /path/to/other/repo
```

By default, `scan` checks both the working tree and recent git history (secrets committed and later removed — the one thing a working-tree-only scan can never catch), bounded by a short time budget so it stays fast even on repos with a lot of history:

```bash
./repoaudit scan . --full-history   # no time budget: walk all reachable history, plus dangling commits from deleted branches
./repoaudit scan . --no-history     # skip history entirely, working tree only
```

(`--full-history` and `--no-history` are mutually exclusive — passing both is a usage error, not a silent override.)

Dependency vulnerability checking (`go.sum`, `requirements.txt` against OSV.dev) is also off by default — it's the only check that needs the network:

```bash
./repoaudit scan . --deps   # check pinned dependencies against known vulnerabilities (requires network)
```

Without `--deps`, a repo with checkable manifests gets a one-line pointer instead (`ℹ️  Found 12 dependencies — run with --deps to check them against known vulnerabilities`), so the feature stays discoverable without ever making a network call you didn't ask for.

Exits with code 1 if the security score drops below 70, so it can gate a CI pipeline.

`--format json` gives the same findings and score as a machine-readable document instead of colored terminal output (diagnostics like `.gitignore` warnings still go to stderr, never mixed into stdout):

```bash
./repoaudit scan . --format json
```

See [docs/decisions/0009-json-output-schema.md](docs/decisions/0009-json-output-schema.md) for the schema and why it's versioned separately from RepoAudit's internal Go types.

`--format html` renders a self-contained dashboard instead — no external CSS/fonts/JS, works offline, with a score breakdown per category alongside the total:

```bash
./repoaudit scan . --format html > report.html
```

See [docs/decisions/0010-html-dashboard.md](docs/decisions/0010-html-dashboard.md) for why the total score is never derived from the category breakdown (a single CRITICAL must still dominate, regardless of which category it's in).

`repoaudit diff` shows only what changed between two git refs — built for a pull request, where "what did this PR introduce or fix" matters more than a static score for the whole repo:

```bash
./repoaudit diff main feature-branch
```

Findings present on both refs (pre-existing issues the branch didn't touch) are never shown — only the delta. Exits with code 1 if anything NEW shows up, at any severity.

`diff` compares two point-in-time snapshots — it doesn't walk the commits between them, so a secret introduced and removed again entirely between `ref-a` and `ref-b` isn't caught here (that's `--full-history`'s job on `scan`, a different kind of check). `diff` also has no `--deps`, `--plugin`, or `--format` equivalent yet — it always runs the secrets/Docker/CI/CD rules and always prints colored terminal output.

External plugins run as a separate process (never in-process Go code — see [docs/plugin-protocol.md](docs/plugin-protocol.md) for why), speaking a small JSON protocol over stdin/stdout:

```bash
./repoaudit scan . --plugin /path/to/your-plugin
./repoaudit scan . --plugin /path/to/plugin-a --plugin /path/to/plugin-b   # --plugin is repeatable
```

A plugin that crashes, times out (5s per file), or sends a malformed response is dropped for the rest of the scan with a warning — it never fails the whole scan, and one misbehaving plugin has no effect on any other. A plugin only ever receives file bytes, never a path it could resolve itself; see [docs/examples/reference-plugin.py](docs/examples/reference-plugin.py) for a complete, runnable reference implementation in Python (the protocol has nothing Go-specific about it).

### Flags reference

`repoaudit scan [path]` — defaults to `.` if `path` is omitted:

| Flag | Default | What it does |
|---|---|---|
| `--full-history` | off | No time budget on git history scanning; also sweeps dangling commits from deleted branches. Mutually exclusive with `--no-history`. |
| `--no-history` | off | Skip git history scanning entirely, working tree only. Mutually exclusive with `--full-history`. |
| `--deps` | off | Check `go.sum`/`requirements.txt` against OSV.dev — the only flag here that touches the network. |
| `--plugin <path>` | none | Run an external plugin executable alongside the built-in rules. Repeatable. |
| `--format <cli\|json\|html>` | `cli` | Output format. `json` and `html` both still respect the exit-code-70 threshold below. |

`repoaudit diff <ref-a> <ref-b>` takes no flags — see the note above on what it doesn't cover yet.

Every mode that produces a score (`scan` in any `--format`) exits with code 1 if the score is below 70; `diff` exits with code 1 if anything is `NEW`, regardless of score or severity.

## GitHub Action

A composite action (`action.yml` at the repo root) wraps the CLI — no new checks, pure packaging:

```yaml
- uses: xchebila/repoaudit@main
  with:
    fail-on-new: true   # pin to a release tag instead of @main once one exists
    deps: true          # optional: pass --deps to scan runs (ignored on pull_request)
```

On a `pull_request` event it runs `repoaudit diff <base-sha> <head-sha>` (the actual commit SHAs from the event payload, not branch names); on any other event (e.g. a push to `main`) it runs `repoaudit scan . --format json` and uploads the result as a build artifact. `fail-on-new: false` reports without failing the build. The action does its own `actions/checkout` with `fetch-depth: 0` and installs `repoaudit` itself via `go install` — nothing needs to be pre-installed on the runner. See [docs/decisions/0011-github-action.md](docs/decisions/0011-github-action.md) for why (and for the module rename to `github.com/xchebila/repoaudit` that `go install` required). `.github/workflows/repoaudit-self-check.yml` runs this action against the repo's own PRs and pushes, so it's proven against real CI, not just YAML that parses.

Not on GitHub Actions? See [docs/ci-integrations.md](docs/ci-integrations.md) for a GitLab CI and a Jenkins snippet doing the same `diff`/`scan --format json` split — documented copy-paste examples rather than a published artifact (see [docs/decisions/0012-multi-ci-integrations.md](docs/decisions/0012-multi-ci-integrations.md) for why), and not validated against a real GitLab/Jenkins run the way `action.yml` is.

## Example output

```
❌ CRITICAL - AWS Access Key ID exposed (config.py:1)
   An AWS access key ID was found hardcoded in this file. Combined with its
   secret key, it grants immediate programmatic access to this AWS account.
   fix: Revoke this key in the AWS IAM console, then load credentials from
   environment variables or a secrets manager (AWS Secrets Manager, Vault).

❌ CRITICAL - GitHub token exposed (deploy.sh:12, commit a83f1c2)
   A GitHub personal access / app token was found hardcoded in this file. It
   can be used to access or modify repositories with this token's permissions.
   fix: Revoke the token at github.com/settings/tokens, then use environment
   variables or GitHub Actions secrets instead.

SECURITY SCORE: 0/100  (F)
```

A finding with a commit hash means the secret isn't in your working tree today — it's still reachable from git history and needs to be revoked, not just deleted.

A Dockerfile with an unpinned base image and no non-root user contributes its own findings to the same score:

```
❌ CRITICAL - AWS Access Key ID exposed (config.py:1)
   An AWS access key ID was found hardcoded in this file. Combined with its
   secret key, it grants immediate programmatic access to this AWS account.
   fix: Revoke this key in the AWS IAM console, then load credentials from
   environment variables or a secrets manager (AWS Secrets Manager, Vault).

⚠️ MEDIUM  - No non-root USER set (Dockerfile:1)
   This Dockerfile never switches away from the default root user. A
   container escape or a compromised process in this image runs as root on
   the host's container runtime.
   fix: Add a USER instruction for a non-root user before the final
   CMD/ENTRYPOINT.

⚠️ LOW     - Image pulled without a pinned tag (Dockerfile:1)
   This FROM instruction has no tag (defaults to "latest") or is pinned to
   "latest" explicitly. The same Dockerfile can produce a different image
   tomorrow with no record of what changed.
   fix: Pin an explicit version tag or, better, a digest.

SECURITY SCORE: 27/100  (F)
```

Note the CRITICAL secret still drives nearly all of the drop (100 → 40) — the MEDIUM and LOW Docker findings only nudge the score further (40 → 27), never compete with it on equal footing. The same two Docker findings alone, with no secret, score 87/100 (B).

A GitHub Actions workflow with `permissions: write-all` and an action pinned to `@main` instead of a version tag:

```
❌ HIGH    - Workflow grants write-all permissions (.github/workflows/deploy.yml:3)
   This workflow's GITHUB_TOKEN has write access to every scope (contents,
   packages, deployments, etc.). A compromised action or a malicious PR
   that reaches this workflow can use that token to push code, publish
   packages, or modify releases.
   fix: Replace write-all with the specific scopes this workflow actually
   needs (e.g. contents: read, pull-requests: write).

⚠️ MEDIUM  - Action pinned to a mutable branch ref (.github/workflows/deploy.yml:8)
   This action is referenced by branch name (@main/@master) instead of a
   version tag or commit SHA. Whoever controls that branch can change what
   code runs in this workflow, with access to its secrets, at any time.
   fix: Pin to a release tag (@v4) or, for maximum safety, a full commit SHA.

⚠️ LOW     - No Dependabot configuration (.github/dependabot.yml)
   This repo uses GitHub Actions but has no .github/dependabot.yml.
   fix: Add a .github/dependabot.yml enabling updates for your package
   ecosystem(s) and for github-actions.

SECURITY SCORE: 62/100  (D)
```

A `go.sum` pinning an old version with known CVEs, checked with `--deps` (this dependency has 4 distinct known vulnerabilities after deduplication — OSV tracks the same issue under multiple ID schemes, e.g. both a GHSA and a PYSEC id, and RepoAudit collapses those aliases so the same real vulnerability isn't scored twice):

```
⚠️ MEDIUM  - GHSA-5rcv-m4m3-hfh7: known vulnerability in golang.org/x/text@v0.3.0 (go.sum)
   golang.org/x/text Infinite loop
   fix: Upgrade golang.org/x/text past the vulnerable range — see
   https://osv.dev/vulnerability/GHSA-5rcv-m4m3-hfh7 for the fixed version.

❌ HIGH    - GHSA-69ch-w2m2-3vjp: known vulnerability in golang.org/x/text@v0.3.0 (go.sum)
   golang.org/x/text/language Denial of service via crafted Accept-Language header
   fix: Upgrade golang.org/x/text past the vulnerable range — see
   https://osv.dev/vulnerability/GHSA-69ch-w2m2-3vjp for the fixed version.

   (2 more, same dependency)

SECURITY SCORE: 47/100  (F)
```

A finding without a `context:` line has an official severity rating from OSV.dev's source database (usually a GitHub Security Advisory); one *with* a `context:` line is either a rough estimate from a raw CVSS vector, or a plain Medium default because OSV had no severity data at all for that record — both are disclosed rather than presented as equally certain.

`repoaudit diff main feature-branch` on a branch that fixes an old Dockerfile issue but introduces a secret:

```
✔️  FIXED  - No non-root USER set (Dockerfile:1)

❌ NEW CRITICAL - AWS Access Key ID exposed (config.py:2)
   An AWS access key ID was found hardcoded in this file. Combined with its
   secret key, it grants immediate programmatic access to this AWS account.
   fix: Revoke this key in the AWS IAM console, then load credentials from
   environment variables or a secrets manager (AWS Secrets Manager, Vault).
```

## Status

Phase 1 — secrets scanner: hardcoded credentials in the working tree (AWS, GitHub, Stripe, Slack, Discord, OpenAI keys, private key blocks, raw JWTs, committed `.env` files), with `.gitignore` support and a severity-weighted score.

Phase 2 — git history analyzer (the same secret rules applied to every commit's changed files, so a secret that was committed and later deleted still gets caught), Docker analyzer (unpinned/`latest` base images, `ADD` used where `COPY` would do, containers with no non-root `USER`), and CI/CD analyzer (`permissions: write-all`, actions pinned to `@main`/`@master`, secrets echoed into build logs, missing Dependabot config). Secrets hardcoded in a Dockerfile's or workflow's `ENV`/`ARG` are already caught by the secrets rules above — they're just text files like any other.

Phase 3 — dependency vulnerability scanning for `go.sum` and `requirements.txt` against OSV.dev, opt-in via `--deps` (the only network-dependent check RepoAudit has; the default scan stays 100% local and deterministic — see `docs/decisions/0004-dependency-scanner-network.md`); and Security Diff Mode (`repoaudit diff <ref-a> <ref-b>`), which reuses the same secrets/Docker/CI/CD rules against two git refs read directly from git (no checkout) and reports only what changed.

Phase 4 — plugin system (`--plugin`): external detection rules run as a separate process speaking a small JSON protocol, never as in-process Go code — see [docs/plugin-protocol.md](docs/plugin-protocol.md) for the contract and `docs/decisions/0008-plugin-system-scope.md` for why.

Phase 5 — reporting: `--format json` for machine-readable output, and `--format html` for a self-contained dashboard with a per-category score breakdown alongside the total. This closes vision.md's roadmap to v1.0.

Post-v1.0 — a GitHub Action (see above) is the first item off [docs/roadmap-long-term.md](docs/roadmap-long-term.md): pure packaging around `diff`/`scan --format json`, no new CLI feature.

See [vision.md](docs/vision.md) for the full v1.0 roadmap, [docs/roadmap-long-term.md](docs/roadmap-long-term.md) for what's planned after it, [docs/decisions/](docs/decisions/) for design rationale, [docs/testing.md](docs/testing.md) for the test corpus and exit criteria, and [docs/benchmarks.md](docs/benchmarks.md) for the timing history behind them.
