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
# Theme: agg's default is "asciinema", which renders ANSI 12 (our blue) as a
# purple-blue. Override with $THEME for a different palette. Built-in agg
# themes: asciinema, dracula, github-dark, github-light, kanagawa,
# kanagawa-dragon, kanagawa-light, monokai, nord, solarized-dark,
# solarized-light, gruvbox-dark.
#
#   THEME=nord ./scripts/record-demo.sh happy-path
#
# Output: assets/<scenario>.gif

set -euo pipefail

SCENARIO="${1:-happy-path}"
THEME="${THEME:-github-dark}"
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

agg --theme "$THEME" "$CAST" "$GIF"
rm "$CAST"

echo "Written: $GIF  (theme=$THEME)"
