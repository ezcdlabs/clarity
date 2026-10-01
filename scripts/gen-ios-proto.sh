#!/usr/bin/env bash
# Generate the Swift protobuf the iOS app uses.
#
# Separate from the Go and Java generation, which run anywhere protoc does,
# because protoc-gen-swift is itself a Swift binary: it needs a Swift toolchain,
# which the Linux box most of clarity is written on does not have. So this runs
# on a Mac — or on the macOS CI runner, which runs it before every iOS build so
# the committed output cannot silently drift from the schema.
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="mobile/apps/ios/Clarity/Proto"

for tool in protoc protoc-gen-swift; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "error: $tool not on PATH — brew install protobuf swift-protobuf" >&2
    exit 1
  fi
done

mkdir -p "$OUT"
protoc -I=proto --swift_out="$OUT" --swift_opt=FileNaming=DropPath proto/clarity/v1/view.proto
echo "wrote $OUT"
