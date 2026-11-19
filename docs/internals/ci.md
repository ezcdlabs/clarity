# What CI Covers, and Where

Six stages in one workflow, ordered so each answers a question the one before it cannot.

- **Test / Integration (SSH)** — `go vet`, `go test`, and the SSH-backed suite. Linux only.
- **Action** — the composite action on Linux, macOS and Windows, on every push and pull request.
- **Android** — `gomobile bind`, then the app's unit tests and a debug build.
- **iOS** — generate the Swift protobuf and the Xcode project, bind, and run the unit tests in a simulator. macOS only.
- **Release** — tag and publish, gated on all of the above.
- **Smoke** — install the version just published, on all three platforms.

The mobile jobs gate the release even though nothing they build is released. A
tag is a statement about a commit, and a commit where the bind no longer
compiles is not one to make that statement about. The iOS job gates it for a
second reason: it is the only thing in the project that compiles the Swift at
all, so an ungated version of it would be a check nobody has to look at — which
is exactly how `aux.go` survived four months. It is also the only thing that
proves clarity's dependency graph still crosses an FFI boundary: the bound
archive is not committed, so a dependency that reaches for `os/exec`, cgo, or a
syscall Android does not have fails here and nowhere else.

## Why the action is tested before the release and smoked after

They are different questions, and one workflow cannot answer both.

**Before**: does the repository check out, and does the installer work? Both exist in the commit under test, so this belongs pre-merge — and it is gated on nothing, because the job starts by checking out the whole repository and *any* file in it can break that. It installs an old release deliberately: the binary is incidental, the installer is the subject, and the version this commit will become does not exist yet.

**After**: is the thing we just published actually installable? Structurally unanswerable earlier — before the release job runs there is no such artefact, so a pre-merge job can only ever verify some older binary. Nothing checked that a freshly cut release was installable at all until this existed. It is skipped when no release was cut, since a docs- or test-only push has nothing new to smoke.

## Why the action job carries no path filter

It used to live in its own workflow, gated on `action.yml`, `scripts/action-install.sh` and the workflow file — on the assumption that only a change to the action can break the action's test.

That assumption is false, and it cost four months. `internal/core/aux.go` matched none of those paths, so the workflow did not run; AUX is a reserved device name on Windows, and the repository was uncheckoutable there from 2026-05-17 until it was found on 2026-09-28. The last green run was 2026-05-11, six days before the file landed. The filter also hid the fix: the rename touched only `internal/core/`, so the workflow did not run on that commit either and had to be dispatched by hand.

The general rule it broke: **a filter may only be narrower than what the job actually depends on.** This job depends on every file in the repository, because it checks all of them out.
