#!/usr/bin/env bash
# Smoke-test the action's install logic locally without pushing to GitHub.
# Sets the env vars the composite action would and runs scripts/action-install.sh,
# then asserts git-clarity --version works.
#
# Usage:
#   ./scripts/test-action-locally.sh              # installs latest
#   ./scripts/test-action-locally.sh v0.1.1       # installs v0.1.1
#
# Requires: bash, curl, tar; gh CLI optional (used only for "latest" resolution).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-latest}"

# Translate the local platform to the values GitHub Actions uses for
# RUNNER_OS / RUNNER_ARCH so action-install.sh sees consistent inputs.
case "$(uname -s)" in
    Linux*)  RUNNER_OS="Linux" ;;
    Darwin*) RUNNER_OS="macOS" ;;
    *)       echo "unsupported local OS: $(uname -s)"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64)         RUNNER_ARCH="X64" ;;
    aarch64|arm64)        RUNNER_ARCH="ARM64" ;;
    *) echo "unsupported local arch: $(uname -m)"; exit 1 ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

export INPUT_VERSION="$VERSION"
export GITHUB_ACTION_REF=""
export RUNNER_OS RUNNER_ARCH
export RUNNER_TOOL_CACHE="$WORK/cache"
export GITHUB_PATH="$WORK/path"
mkdir -p "$RUNNER_TOOL_CACHE"
: > "$GITHUB_PATH"

echo "==> testing action install: version=$VERSION os=$RUNNER_OS arch=$RUNNER_ARCH"
bash "$REPO_ROOT/scripts/action-install.sh"

# action-install.sh appends the install dir to $GITHUB_PATH; replicate
# Actions' own behaviour of prepending those entries to PATH for verification.
INSTALL_DIR="$(cat "$GITHUB_PATH")"
PATH="$INSTALL_DIR:$PATH"

echo "==> verifying via git-clarity --version"
git-clarity --version

echo "==> ok"
