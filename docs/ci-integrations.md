# CI integrations beyond GitHub Actions

The [GitHub Action](../README.md#github-action) (`action.yml`) is the only integration RepoAudit publishes as a versioned, installable unit. For GitLab CI and Jenkins, this page is a documented copy-paste snippet instead — not a published GitLab CI/CD Component or a Jenkins Shared Library.

**Why a snippet, not a published artifact, for these two**: a GitLab CI/CD Component only appears in the GitLab.com catalog if it's released from a project hosted *on* gitlab.com with its own semver tags and release pipeline — RepoAudit's canonical repo is on GitHub, so publishing one would mean standing up and maintaining a second, mirrored hosting location indefinitely. A Jenkins Shared Library is real, ongoing Groovy code to maintain in a project that's otherwise 100% Go. Both are a bigger commitment than "pure packaging" — a snippet gets the same `diff`/`scan --format json` capability with zero new infrastructure. Revisit either if real usage demands it.

**Validation gap, stated plainly**: unlike `action.yml` (proven end-to-end by `.github/workflows/repoaudit-self-check.yml` against real GitHub Actions runs — see ADR 0011), neither snippet below has run on an actual GitLab or Jenkins instance. There's no such instance in this project's CI. Treat them as reviewed-but-unrun; if something's off, it'll surface as a real user report, not as a bug this repo's own CI would have caught.

## GitLab CI

```yaml
repoaudit:
  image: golang:1.25
  variables:
    GOPROXY: direct
    GOSUMDB: 'off' # installing repoaudit itself from its own repo, not a third-party dependency
  before_script:
    - go install github.com/xchebila/repoaudit@main # pin to a release tag once one exists
  script:
    - |
      if [ "$CI_PIPELINE_SOURCE" = "merge_request_event" ]; then
        # CI_MERGE_REQUEST_SOURCE_BRANCH_SHA is set on "merged results"
        # pipelines (CI_COMMIT_SHA there is a synthetic merge commit) but
        # empty on plain merge request pipelines (where CI_COMMIT_SHA IS
        # the real source-branch commit) -- prefer it, fall back otherwise.
        head_sha="${CI_MERGE_REQUEST_SOURCE_BRANCH_SHA:-$CI_COMMIT_SHA}"
        repoaudit diff "$CI_MERGE_REQUEST_DIFF_BASE_SHA" "$head_sha"
      else
        repoaudit scan . --format json | tee repoaudit-report.json
      fi
  artifacts:
    when: always
    paths:
      - repoaudit-report.json
    expire_in: 1 week
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

Notes:

- Fetch depth: GitLab's default shallow clone (`GIT_DEPTH`) can leave `CI_MERGE_REQUEST_DIFF_BASE_SHA` unresolvable in `.git` locally. Set `GIT_DEPTH: "0"` in this job's `variables:` (or project-wide in CI/CD settings) if `repoaudit diff` reports it can't find the base commit — same underlying issue `fetch-depth: 0` solves for the GitHub Action (ADR 0011).
- `artifacts: when: always` mirrors the GitHub Action's `if: !cancelled()` on its upload step: the report should still be available after a failing job, not only on success.
- No `--deps`/`--plugin` equivalent on the merge-request branch of this snippet, for the same reason `diff` doesn't support them on the CLI itself (see README) — only the push/default-branch branch passes `--format json` and could add `--deps` there.

## Jenkins (declarative Jenkinsfile)

Requires a Multibranch Pipeline job with the appropriate SCM source plugin configured (GitHub Branch Source, GitLab Branch Source, Bitbucket Branch Source, ...) — that plugin is what populates `CHANGE_ID`/`CHANGE_TARGET`. Jenkins has no built-in concept of a pull/merge request outside of that plugin.

```groovy
stage('RepoAudit') {
    environment {
        GOPROXY = 'direct'
        GOSUMDB = 'off' // installing repoaudit itself from its own repo, not a third-party dependency
    }
    steps {
        sh 'go install github.com/xchebila/repoaudit@main' // pin to a release tag once one exists
        script {
            if (env.CHANGE_ID) {
                // CHANGE_TARGET is a branch name, not a SHA (same footgun
                // `github.base_ref` would have been for the GitHub Action --
                // see ADR 0011) -- resolve the actual merge-base commit.
                sh "git fetch origin ${env.CHANGE_TARGET}"
                def baseSha = sh(script: "git merge-base HEAD origin/${env.CHANGE_TARGET}", returnStdout: true).trim()
                sh "repoaudit diff ${baseSha} ${env.GIT_COMMIT}"
            } else {
                sh 'repoaudit scan . --format json | tee repoaudit-report.json'
            }
        }
    }
    post {
        always {
            archiveArtifacts artifacts: 'repoaudit-report.json', allowEmptyArchive: true
        }
    }
}
```

Notes:

- This Multibranch job needs a checkout depth deep enough for `git merge-base` to find a common ancestor — Jenkins' own shallow-clone default varies by SCM plugin; if `git merge-base` fails, increase it (or disable shallow clone) the same way `fetch-depth: 0` does for the GitHub Action.
- Unlike the GitHub Action's install step (ADR 0011), there's no "unresolvable synthetic commit" risk here: `repoaudit diff` reads whatever's already in the local `.git` object database, and both PR discovery strategies leave a real, locally-present commit for `env.GIT_COMMIT` to point to. What differs between strategies is which commit that is: the Branch Source plugin can be configured to check out either the raw PR head or a Jenkins-built merge of the PR into its target ("Merge Commit" strategy) — know which one your Multibranch job uses, since `repoaudit diff` will report against whichever `env.GIT_COMMIT` actually is.
