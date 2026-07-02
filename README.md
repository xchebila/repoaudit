# 🛡️ RepoAudit

RepoAudit is a 10-second security sanity check for Git repositories. It doesn't analyze code quality — it detects real-world security mistakes that leak data or break production: committed secrets, exposed keys, tokens hardcoded in source.

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

Exits with code 1 if the security score drops below 70, so it can gate a CI pipeline.

## Example output

```
❌ CRITICAL - AWS Access Key ID exposed (config.py:1)
   An AWS access key ID was found hardcoded in this file. Combined with its
   secret key, it grants immediate programmatic access to this AWS account.
   fix: Revoke this key in the AWS IAM console, then load credentials from
   environment variables or a secrets manager (AWS Secrets Manager, Vault).

❌ CRITICAL - .env file committed (.env)
   A .env file is tracked in this repository. These files typically hold
   database credentials, API keys, and other secrets meant to stay local.
   fix: Remove the file from version control (git rm --cached .env), add
   .env to .gitignore, and rotate any secrets it contained.

SECRETS SCORE: 0/100  (F)
```

## Status

Phase 1 (MVP) — secrets scanner: hardcoded credentials in the working tree (AWS, GitHub, Stripe, Slack, Discord, OpenAI keys, private key blocks, raw JWTs, committed `.env` files), with `.gitignore` support and a severity-weighted score. See [vision.md](docs/vision.md) for the full roadmap and [docs/decisions/](docs/decisions/) for design rationale.
