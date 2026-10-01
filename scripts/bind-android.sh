#!/usr/bin/env bash
# Build the Go side of the Android app into an .aar the Gradle project consumes.
#
# The archive is a build product, not a source file: it is tens of megabytes of
# per-ABI native code, so it is gitignored and this script is how you get one.
# Run it before the first Gradle build and after any change under mobile/ or
# internal/.
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="${OUT:-mobile/apps/android/app/libs/clarity.aar}"
TAGS="${TAGS:-}"

# minSdk in app/build.gradle.kts. gomobile refuses a mismatch at runtime
# rather than at build time, so the two are kept deliberately equal.
API="${API:-24}"

if [ -z "${ANDROID_HOME:-}${ANDROID_NDK_HOME:-}" ]; then
  echo "error: ANDROID_HOME (or ANDROID_NDK_HOME) must point at an SDK with an NDK installed" >&2
  exit 1
fi

if ! command -v gomobile >/dev/null 2>&1; then
  echo "error: gomobile not on PATH — go install golang.org/x/mobile/cmd/gomobile@latest" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"

# -javapkg puts the generated classes under the app's own namespace. Without
# it they land in a top-level `core` package, which collides with anything else
# bound into the same app and reads like a stray dependency.
echo "binding mobile/core -> $OUT"
exec gomobile bind \
  -target=android \
  -androidapi="$API" \
  -javapkg=dev.ezcd.clarity \
  ${TAGS:+-tags "$TAGS"} \
  -o "$OUT" \
  ./mobile/core
