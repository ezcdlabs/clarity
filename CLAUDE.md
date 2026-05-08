# CLAUDE.md

## Design document

README.md is the authoritative design document for this project, not just
a description. It documents decisions that have been made, not just intent.

When we make a design decision in a conversation — especially one that
diverges from, refines, or extends something in README.md — update README.md
before moving on. Treat it as a living spec.

Specifically, update README.md when:

- An open question (see "Open Questions" section) gets resolved
- An implementation reveals a detail the design didn't anticipate
- We decide to change an approach described in the doc
- A new constraint or behaviour is established

Do NOT update README.md for:

- Pure implementation details that belong in code comments
- Ephemeral task tracking (use TodoWrite for that)
- Anything already accurately described

## Test-driven development

Write tests before implementation. The red/green/refactor cycle is the default
approach for all new behaviour.

Specifically:

- New functions that classify or categorise (like `isFastForwardRejected`) must
  have a table-driven unit test covering all known cases before the function is
  written. When a new case is discovered at runtime, add the failing test first,
  then fix the code.
- New public API behaviour must have an acceptance test (or unit test) that fails
  before the implementation is added.
- Do not write implementation speculatively and backfill tests — always go
  red first.
