# Contributing

Issues and pull requests are welcome. For anything non-trivial, open an issue first so the design conversation happens before the code — the [design docs](docs/design/README.md) are a living spec, and behaviour changes land together with the matching design-doc update.

## Build and test

```sh
go build ./...
go vet ./...
go test ./...
```

The SSH-backed integration suite is behind the `ssh` build tag and needs a container runtime; CI runs it on every push. See [what CI covers](docs/internals/ci.md).

## Test-driven development

This repo works red/green/refactor: write the failing test first, then the implementation. Classifier-style functions get table-driven tests covering all known cases; new public behaviour gets an acceptance test that fails before the implementation exists.

## Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org), and the type decides the release: merging to `main` with `fix:` cuts a patch, `feat:` a minor, a `!` marker or `BREAKING CHANGE:` footer a major. `docs:`, `chore:`, `test:`, `refactor:` and `ci:` cut no release. See [Releasing](docs/internals/releasing.md).

## Coding agents

If you're working on this repo with a coding agent, point it at [AGENTS.md](AGENTS.md).

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md).
