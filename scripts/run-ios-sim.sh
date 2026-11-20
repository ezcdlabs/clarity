#!/usr/bin/env bash
# Build the iOS app and run it in a simulator, terminal only — no Xcode GUI.
#
# Checks everything it needs first and says how to get whatever is missing,
# then: Swift protobuf → Go core bind (skipped when nothing changed, because
# it is the slow step) → xcodegen → xcodebuild → boot a simulator → install →
# launch. Re-run it after any change; each step is cheap except the bind, and
# the bind only reruns when a Go source is newer than the last framework.
set -euo pipefail

cd "$(dirname "$0")/.."

# --- checks ------------------------------------------------------------------

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: the iOS simulator needs macOS" >&2
  exit 1
fi

missing=0
need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing: $1 — install with: $2" >&2
    missing=1
  }
}
need go "brew install go"
need xcodegen "brew install xcodegen"
need protoc "brew install protobuf"
need protoc-gen-swift "brew install swift-protobuf"

# Full Xcode, not just the Command Line Tools: the simulator, the iOS SDK and
# xcodebuild's -destination all live in the app.
if ! xcodebuild -version >/dev/null 2>&1; then
  echo "missing: full Xcode — install it from the App Store, then:" >&2
  echo "  sudo xcode-select -s /Applications/Xcode.app" >&2
  missing=1
fi

if [ "$missing" -ne 0 ]; then
  exit 1
fi

# --- pick a simulator --------------------------------------------------------

# A booted one wins — that is the one you are looking at. Otherwise the last
# available iPhone, which within simctl's listing is the newest runtime's.
udid="$(xcrun simctl list devices booted | grep -Eo '[0-9A-F]{8}-[0-9A-F-]{27}' | head -1 || true)"
if [ -z "$udid" ]; then
  line="$(xcrun simctl list devices available | grep -E '^[[:space:]]+iPhone' | tail -1 || true)"
  if [ -z "$line" ]; then
    echo "error: no iPhone simulator is available — in Xcode: Settings → Platforms, install an iOS runtime" >&2
    exit 1
  fi
  udid="$(echo "$line" | grep -Eo '[0-9A-F]{8}-[0-9A-F-]{27}')"
  echo "simulator: $(echo "$line" | sed -E 's/^[[:space:]]+//; s/ \([0-9A-F-]{36}\).*//')"
else
  echo "simulator: already booted"
fi

# --- build -------------------------------------------------------------------

./scripts/gen-ios-proto.sh

# The xcframework is a build product (gitignored), and binding is the slow
# step — skip it when no Go source is newer than the last bind.
fw="mobile/apps/ios/ClarityCore.xcframework"
if [ ! -f "$fw/Info.plist" ] ||
  [ -n "$(find mobile/core mobile/internal internal clarityrefs proto go.mod \
    -name '*.go' -newer "$fw/Info.plist" -print -quit 2>/dev/null)" ]; then
  ./scripts/bind-ios.sh
else
  echo "core unchanged — keeping $fw"
fi

cd mobile/apps/ios
xcodegen generate

# A pinned derivedDataPath so the built .app is at a path this script knows,
# rather than somewhere under ~/Library keyed by a project hash.
xcodebuild build \
  -project Clarity.xcodeproj \
  -scheme Clarity \
  -configuration Debug \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath build/sim \
  -quiet \
  CODE_SIGNING_ALLOWED=NO

app="build/sim/Build/Products/Debug-iphonesimulator/Clarity.app"
if [ ! -d "$app" ]; then
  echo "error: build finished but $app is not there" >&2
  exit 1
fi

# --- run ---------------------------------------------------------------------

# bootstatus -b boots it if needed and waits until it is actually ready,
# which is what makes the install below reliable on a cold start.
xcrun simctl bootstatus "$udid" -b >/dev/null
open -a Simulator

xcrun simctl terminate "$udid" dev.ezcd.clarity >/dev/null 2>&1 || true
xcrun simctl install "$udid" "$app"
xcrun simctl launch "$udid" dev.ezcd.clarity >/dev/null

echo "running — logs: xcrun simctl spawn $udid log stream --predicate 'subsystem contains \"dev.ezcd\"'"
