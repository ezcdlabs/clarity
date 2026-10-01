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

# The framework is ClarityCore, not Clarity, because the app target is Clarity
# and a module cannot share a name with the target that imports it. -prefix is
# the iOS counterpart of Android's -javapkg: without it the generated classes
# are CoreClient and CoreNew, which read like something else's.
echo "binding mobile/core -> $OUT"
exec go tool gomobile bind \
  -target=ios,iossimulator \
  -prefix=Clarity \
  ${TAGS:+-tags "$TAGS"} \
  -o "$OUT" \
  ./mobile/core
