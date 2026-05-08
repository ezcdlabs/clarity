#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# smoke-test.sh — exercise the binary end-to-end in a scratch repo.
#
# Builds git-clarity, creates a throw-away bare remote + working clone under
# /tmp/clarity-smoke, and runs `git clarity report` a couple of times so you
# can see the success output and the resulting events on the ref.
#
# After the script exits, the scratch repo is left in place so you can also
# launch the TUI manually:
#
#   (cd /tmp/clarity-smoke/client && /tmp/clarity-smoke/git-clarity)
#
# Press q to quit the TUI. Re-run this script to reset the scratch state.
# ---------------------------------------------------------------------------

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SMOKE_DIR="/tmp/clarity-smoke"
BIN="$SMOKE_DIR/git-clarity"

rm -rf "$SMOKE_DIR"
mkdir -p "$SMOKE_DIR"

echo "==> building git-clarity..."
go build -o "$BIN" "$REPO_ROOT/cmd/git-clarity"

echo "==> setting up scratch bare remote and client clone..."
git init --bare --initial-branch=main -q "$SMOKE_DIR/bare"
git -C "$SMOKE_DIR/bare" config user.email t@t
git -C "$SMOKE_DIR/bare" config user.name T

git clone -q "$SMOKE_DIR/bare" "$SMOKE_DIR/seed"
git -C "$SMOKE_DIR/seed" config user.email t@t
git -C "$SMOKE_DIR/seed" config user.name T
touch "$SMOKE_DIR/seed/.gitkeep"
git -C "$SMOKE_DIR/seed" add .
git -C "$SMOKE_DIR/seed" commit -qm "initial"
git -C "$SMOKE_DIR/seed" push -q origin main

git clone -q "$SMOKE_DIR/bare" "$SMOKE_DIR/client"
git -C "$SMOKE_DIR/client" config user.email t@t
git -C "$SMOKE_DIR/client" config user.name T

echo
echo "==> git clarity report build started"
(cd "$SMOKE_DIR/client" && "$BIN" report build started)

echo "==> git clarity report build passed"
(cd "$SMOKE_DIR/client" && "$BIN" report build passed)

echo "==> git clarity report deploy passed"
(cd "$SMOKE_DIR/client" && "$BIN" report deploy passed)

echo
echo "==> events that landed on refs/clarity/events:"
git -C "$SMOKE_DIR/bare" ls-tree -r refs/clarity/events

echo
echo "==> bad usage check (expect non-zero exit + usage line):"
(cd "$SMOKE_DIR/client" && "$BIN" report 2>&1 || true)

echo
echo "Done. To launch the TUI against this scratch repo:"
echo
echo "  (cd $SMOKE_DIR/client && $BIN)"
echo
echo "Press q in the TUI to quit. Re-run $0 to reset the scratch state."
