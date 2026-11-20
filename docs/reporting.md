# Reporting from CI

## Reporting from CI: `git clarity report <stage> <status>`

Inside your pipelines:

```yaml
- run: git clarity report ci started
- run: ./build.sh
- run: git clarity report ci passed
- run: ./deploy.sh && git clarity report deploy passed || git clarity report deploy failed
```

In GitHub Actions the existing `GITHUB_TOKEN` is sufficient — no additional secrets needed. Stages are exactly two: `ci` and `deploy`. Statuses are `started`, `passed`, or `failed`.

A monorepo that deploys several things adds an optional target to its deploy
reports — `git clarity report deploy passed ios` — and the TUI gains a selectable
flow per target. CI never takes one. See [Deploy targets](design/deploy-targets.md#deploy-targets).

## Reporting from GitHub Actions: `ezcdlabs/clarity`

For GitHub Actions specifically there is a setup action that handles installation and (optionally) the report call in one step. Pin the action ref to a release and you get the matching binary automatically — no separate `version:` input needed in the common case.

The job needs `contents: write` on the `GITHUB_TOKEN` because clarity reports by pushing an event to `refs/clarity/events`. The default token permissions vary by org and repo; declaring it explicitly makes the workflow portable.

Install only, then report manually:

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v4
  - uses: ezcdlabs/clarity@v1.1.0
  - run: ./build.sh && git clarity report ci passed
```

Install and report in one step:

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v4
  - uses: ezcdlabs/clarity@v1.1.0
    with:
      stage: ci
      status: started

  - run: ./build.sh

  - if: always()
    uses: ezcdlabs/clarity@v1.1.0
    with:
      stage: ci
      status: ${{ job.status == 'success' && 'passed' || 'failed' }}
```

Inputs:

- `version` — explicit version override (e.g. `v1.1.0`, `latest`). Defaults to the action's own ref when that's a semver tag, otherwise to the latest GitHub release.
- `stage` — pipeline stage to report (`ci` or `deploy`). When combined with `status`, the action runs `git clarity report <stage> <status>` after install.
- `status` — status to report (`started` / `passed` / `failed`).
- `target` — which deployable a `deploy` event is about, for a repo shipping several from one trunk (e.g. `ios`). Rejected on `ci`. Omit it for the untargeted deploy.

`actions/checkout` must run first so the action has a repository to push events from.
