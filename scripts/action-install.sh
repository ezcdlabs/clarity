#!/usr/bin/env bash
# Install the git-clarity binary into the runner's tool cache and append it
# to $GITHUB_PATH. Driven by environment variables so both action.yml and
# scripts/test-action-locally.sh can share this logic verbatim.
#
# Required env:
#   RUNNER_OS         "Linux" | "macOS" | "Windows"
#   RUNNER_ARCH       "X64" | "ARM64"
#   RUNNER_TOOL_CACHE writable dir to install into
#   GITHUB_PATH       file path; this script appends the install dir to it
#
# Optional:
#   INPUT_VERSION       explicit version override (e.g. v0.1.1)
#   GITHUB_ACTION_REF   ref the action was invoked at (used as default version
#                       when it looks like a semver tag)
#   GH_TOKEN            GitHub token; only needed for "latest" resolution to
#                       avoid the unauthenticated rate limit

set -euo pipefail

REPO="ezcdlabs/clarity"

# --- 1. resolve version ------------------------------------------------------

VERSION="${INPUT_VERSION:-}"
if [ -z "$VERSION" ]; then
    if [[ "${GITHUB_ACTION_REF:-}" =~ ^v[0-9]+(\.[0-9]+)*$ ]]; then
        VERSION="$GITHUB_ACTION_REF"
    else
        VERSION="latest"
    fi
fi

if [ "$VERSION" = "latest" ]; then
    # Resolve via the GitHub API. gh CLI is preinstalled on all GitHub runners
    # and authenticates with $GH_TOKEN automatically when set.
    if command -v gh >/dev/null 2>&1; then
        VERSION="$(gh release view --repo "$REPO" --json tagName -q .tagName)"
    else
        VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep -E '"tag_name":' | head -1 | cut -d'"' -f4)"
    fi
fi

VERSION_NO_V="${VERSION#v}"

# --- 2. detect OS / arch -----------------------------------------------------

case "${RUNNER_OS:-}" in
    Linux)   OS="linux" ;;
    macOS)   OS="darwin" ;;
    Windows) OS="windows" ;;
    *) echo "::error::unsupported RUNNER_OS: ${RUNNER_OS:-<unset>}" >&2; exit 1 ;;
esac

case "${RUNNER_ARCH:-}" in
    X64)   ARCH="amd64" ;;
    ARM64) ARCH="arm64" ;;
    *) echo "::error::unsupported RUNNER_ARCH: ${RUNNER_ARCH:-<unset>}" >&2; exit 1 ;;
esac

EXT="tar.gz"
[ "$OS" = "windows" ] && EXT="zip"

# --- 3. download + extract ---------------------------------------------------

ARCHIVE="git-clarity_${VERSION_NO_V}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

INSTALL_DIR="${RUNNER_TOOL_CACHE}/git-clarity/${VERSION}/${OS}-${ARCH}"
mkdir -p "$INSTALL_DIR"

# Idempotent: skip download if the binary is already cached at this version.
BINARY="git-clarity"
[ "$OS" = "windows" ] && BINARY="git-clarity.exe"
if [ ! -x "$INSTALL_DIR/$BINARY" ]; then
    echo "==> downloading $URL"
    TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR"' EXIT
    curl -fsSL --retry 3 -o "$TMPDIR/$ARCHIVE" "$URL"
    case "$EXT" in
        tar.gz) tar -xzf "$TMPDIR/$ARCHIVE" -C "$INSTALL_DIR" ;;
        zip)    unzip -qo "$TMPDIR/$ARCHIVE" -d "$INSTALL_DIR" ;;
    esac
fi

# --- 4. expose on PATH + verify ---------------------------------------------

if [ -n "${GITHUB_PATH:-}" ]; then
    echo "$INSTALL_DIR" >> "$GITHUB_PATH"
fi

echo "==> installed git-clarity $VERSION ($OS/$ARCH) at $INSTALL_DIR"
"$INSTALL_DIR/$BINARY" --version
