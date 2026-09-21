// Package actioninstall holds the tests for scripts/action-install.sh, the
// composite action's installer.
//
// The script is shell because it runs on a runner before any Go binary
// exists, but its version resolution is the part users feel — pinning
// `uses: ezcdlabs/clarity@v1.2.3` has to install exactly v1.2.3 — and it is
// the part that silently regressed. Testing it from Go means it runs in the
// ordinary `go test ./...` on every push, rather than only in the workflow
// that touches the action.
package actioninstall
