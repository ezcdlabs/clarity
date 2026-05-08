#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$(which git-clarity 2>/dev/null || echo "/usr/local/bin/git-clarity")"
BIN="$REPO_ROOT/git-clarity"

DEV_VERSION="dev-$(date +%s)"
echo "Building git-clarity ($DEV_VERSION)..."
go build -ldflags "-X main.version=$DEV_VERSION" -o "$BIN" "$REPO_ROOT/cmd/git-clarity"

echo "Installing to $DEST..."
if [ -w "$(dirname "$DEST")" ]; then
    mv "$BIN" "$DEST"
else
    sudo mv "$BIN" "$DEST"
fi

echo "Installed: $(git-clarity --version)"
