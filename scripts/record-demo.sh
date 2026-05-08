#!/usr/bin/env bash
# Records the named scenario as an animated GIF.
#
# Prerequisites:
#   pip install asciinema        (or: sudo apt install asciinema)
#   go install github.com/asciinema/agg@latest
#
# Usage:
#   ./scripts/record-demo.sh [scenario]
#
# Output: assets/<scenario>.gif

set -euo pipefail

SCENARIO="${1:-happy-path}"
CAST="/tmp/${SCENARIO}.cast"
GIF="assets/${SCENARIO}.gif"

mkdir -p assets

go build -o /tmp/clarity-demo ./cmd/demo

# Clarity's TUI has a header, body, and footer, so it needs a couple more
# rows than pushq's inline display. 110×24 fits a 5-commit history with
# room for variants without horizontal truncation.
asciinema rec --overwrite --cols 110 --rows 24 \
    --command "/tmp/clarity-demo --play ${SCENARIO}" \
    "$CAST"

agg "$CAST" "$GIF"
rm "$CAST"

echo "Written: $GIF"
