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
#   GITHUB_ACTION_REF   ref the action was invoked at, when the runner sets it
#   GITHUB_ACTION_PATH  where the runner unpacked the action; its last segment
#                       is the ref, and inside a composite action it is the
#                       only place that ref survives
#   GH_TOKEN            GitHub token; only needed for "latest" resolution to
#                       avoid the unauthenticated rate limit
#   CLARITY_RESOLVE_ONLY  print the resolved version and exit, without
#                       downloading anything (used by the resolution tests)

set -euo pipefail

REPO="ezcdlabs/clarity"

# --- 1. resolve version ------------------------------------------------------

# Precedence: an explicit `version:` input, then the ref the action was pinned
# at, then the latest release.
#
# Finding that ref is the awkward part, and getting it wrong is silent.
# GITHUB_ACTION_REF is the documented way, but the runner does not set it
# inside a composite action's steps — neither the environment variable nor the
# `github.action_ref` context is populated, both come back empty. So
# `uses: ezcdlabs/clarity@v0.3.1` fell through to "latest" and installed
# whatever had shipped most recently, ignoring the pin without saying so.
#
# The ref does survive in GITHUB_ACTION_PATH, which is where the runner
# unpacks the action:
#
#   /home/runner/work/_actions/ezcdlabs/clarity/v0.3.1
#
# so its last segment is the ref. GITHUB_ACTION_REF is still consulted first,
# so this keeps working if the runner ever starts populating it.
#
# A ref that is not a semver tag — a branch, a commit sha, or a local
# `uses: ./` checkout, whose path is the workspace rather than an unpack
# directory — falls through to the latest release, which is what those refs
# mean in practice.
SEMVER_RE='^v[0-9]+(\.[0-9]+)*$'

VERSION="${INPUT_VERSION:-}"
SOURCE="input"
if [ -z "$VERSION" ]; then
    ACTION_PATH_REF="$(basename -- "${GITHUB_ACTION_PATH:-/}")"
    if [[ "${GITHUB_ACTION_REF:-}" =~ $SEMVER_RE ]]; then
        VERSION="$GITHUB_ACTION_REF"
        SOURCE="action-ref"
    elif [[ "$ACTION_PATH_REF" =~ $SEMVER_RE ]]; then
        VERSION="$ACTION_PATH_REF"
        SOURCE="action-path"
    else
        VERSION="latest"
        SOURCE="default"
    fi
fi

# Resolution is separable from installation so the precedence above can be
# tested without a network round trip. See internal/actioninstall.
if [ -n "${CLARITY_RESOLVE_ONLY:-}" ]; then
    echo "version=$VERSION source=$SOURCE"
    exit 0
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
