# Releasing

Releases are cut automatically from `main`, but only off a green build.
Versioning follows [Conventional Commits](https://www.conventionalcommits.org).
The `release` job in `.github/workflows/ci.yml` `needs:` the test and
integration jobs, so a failing build never produces a tag. When a
release-worthy commit lands and CI passes, the job derives the next semantic
version, pushes the tag, and runs goreleaser to publish the GitHub release and
update the Homebrew/Scoop taps.

The bump is the highest implied by any commit since the last tag:

- `fix:` → patch
- `feat:` → minor
- a `!` marker (`feat!:`) or a `BREAKING CHANGE:` footer → major
- anything else (`docs:`, `refactor:`, `chore:`, `ci:`, `test:`, non-conventional) → no release

A push to `main` with no release-worthy commits produces no tag and no release.
Because the bump is computed from *every* commit since the last tag, a release
deferred by a red build (or a string of non-release commits) still accounts for
everything accumulated in between once a green, release-worthy commit lands.
The classification lives in `internal/release` (unit-tested) and is driven in
CI by the non-shipped `cmd/next-version` helper, which prints the next tag (or
nothing) from the git history.

The whole flow runs on the default `GITHUB_TOKEN` plus the existing
`TAP_GITHUB_TOKEN` secret — no personal access token is required. Tagging and
publishing happen in the **same** job rather than via a separate tag-triggered
workflow, because a tag pushed with `GITHUB_TOKEN` does not trigger another
workflow run. This is the only release path: pushing to `main` is how a release
is cut, with no manual tagging step.
