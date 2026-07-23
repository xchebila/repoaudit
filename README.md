# 🛡️ RepoAudit

RepoAudit is a 10-second security sanity check for Git repositories. It doesn't analyze code quality — it detects real-world security mistakes that leak data or break production: committed secrets, exposed keys, tokens hardcoded in source, risky Dockerfile patterns, and CI/CD workflow misconfigurations.

Signal over noise: no 500 warnings, just what's actionable. Every finding explains *why* it's dangerous and *how* to fix it.

## Install / build

Requires Go 1.24+.

```bash
git clone git@github.com:xchebila/repoaudit.git
cd repoaudit
go build -o repoaudit .
```

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

Dependency vulnerability checking (`go.sum`, `requirements.txt` against OSV.dev) is also off by default — it's the only check that needs the network:

```bash
./repoaudit scan . --deps   # check pinned dependencies against known vulnerabilities (requires network)
```

Without `--deps`, a repo with checkable manifests gets a one-line pointer instead (`ℹ️  Found 12 dependencies — run with --deps to check them against known vulnerabilities`), so the feature stays discoverable without ever making a network call you didn't ask for.

Exits with code 1 if the security score drops below 70, so it can gate a CI pipeline.

`repoaudit diff` shows only what changed between two git refs — built for a pull request, where "what did this PR introduce or fix" matters more than a static score for the whole repo:

```bash
./repoaudit diff main feature-branch
```

Findings present on both refs (pre-existing issues the branch didn't touch) are never shown — only the delta. Exits with code 1 if anything NEW shows up, at any severity.

External plugins run as a separate process (never in-process Go code — see [docs/plugin-protocol.md](docs/plugin-protocol.md) for why), speaking a small JSON protocol over stdin/stdout:

```bash
./repoaudit scan . --plugin /path/to/your-plugin
```

A plugin that crashes, times out (5s per file), or sends a malformed response is dropped for the rest of the scan with a warning — it never fails the whole scan. A plugin only ever receives file bytes, never a path it could resolve itself; see [docs/examples/reference-plugin.py](docs/examples/reference-plugin.py) for a complete, runnable reference implementation in Python (the protocol has nothing Go-specific about it).

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

See [vision.md](docs/vision.md) for the full roadmap, [docs/decisions/](docs/decisions/) for design rationale, [docs/testing.md](docs/testing.md) for the test corpus and exit criteria, and [docs/benchmarks.md](docs/benchmarks.md) for the timing history behind them.
