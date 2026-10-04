#!/usr/bin/env bash
# Build the Go side of the iOS app into an .xcframework the Xcode project links.
#
# macOS only — gomobile shells out to clang from the iOS SDK. The output is a
# build product like the Android .aar, gitignored for the same reason.
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="${OUT:-mobile/apps/ios/ClarityCore.xcframework}"
TAGS="${TAGS:-}"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: binding for iOS needs macOS and the iOS SDK" >&2
  exit 1
fi

rm -rf "$OUT"
mkdir -p "$(dirname "$OUT")"

# gomobile shells out to gobind, which has to be on PATH. Build the pinned one
# from go.mod rather than trusting whatever a developer happens to have
# installed — a stray gobind in ~/go/bin is exactly why this passed on a laptop
# and could never have passed anywhere clean.
GOBIND="$(mktemp -d)"
trap 'rm -rf "$GOBIND"' EXIT
go build -o "$GOBIND/gobind" golang.org/x/mobile/cmd/gobind
export PATH="$GOBIND:$PATH"

# The framework is ClarityCore, not Clarity, because the app target is Clarity
# and a module cannot share a name with the target that imports it. -prefix is
# the iOS counterpart of Android's -javapkg: without it the generated classes
# are CoreClient and CoreNew, which read like something else's.
echo "binding mobile/core -> $OUT"
go tool gomobile bind \
  -target=ios,iossimulator \
  -prefix=Clarity \
  ${TAGS:+-tags "$TAGS"} \
  -o "$OUT" \
  ./mobile/core
