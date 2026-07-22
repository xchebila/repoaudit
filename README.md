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

Exits with code 1 if the security score drops below 70, so it can gate a CI pipeline.

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

## Status

Phase 1 — secrets scanner: hardcoded credentials in the working tree (AWS, GitHub, Stripe, Slack, Discord, OpenAI keys, private key blocks, raw JWTs, committed `.env` files), with `.gitignore` support and a severity-weighted score.

Phase 2 — git history analyzer (the same secret rules applied to every commit's changed files, so a secret that was committed and later deleted still gets caught), Docker analyzer (unpinned/`latest` base images, `ADD` used where `COPY` would do, containers with no non-root `USER`), and CI/CD analyzer (`permissions: write-all`, actions pinned to `@main`/`@master`, secrets echoed into build logs, missing Dependabot config). Secrets hardcoded in a Dockerfile's or workflow's `ENV`/`ARG` are already caught by the secrets rules above — they're just text files like any other.

Phase 3 (in progress) — dependency vulnerability scanning (OSV.dev) is next, opt-in via `--deps` rather than on by default: unlike everything above, it needs the network, and the default scan stays 100% local and deterministic (see `docs/decisions/0004-dependency-scanner-network.md`).

See [vision.md](docs/vision.md) for the full roadmap, [docs/decisions/](docs/decisions/) for design rationale, [docs/testing.md](docs/testing.md) for the test corpus and exit criteria, and [docs/benchmarks.md](docs/benchmarks.md) for the timing history behind them.
