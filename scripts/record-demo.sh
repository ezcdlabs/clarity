#!/usr/bin/env bash
# Records the named scenario as an animated GIF (always) and an MP4 (when
# ffmpeg is installed). The MP4 is what to link in places like Discord and
# Slack — they embed/play MP4 inline but typically only show the first
# frame of a URL-linked GIF. The GIF is what to embed in the README.
#
# Prerequisites:
#   pip install asciinema        (or: sudo apt install asciinema)
#   go install github.com/asciinema/agg@latest
#   ffmpeg                       (optional — MP4 output is skipped without it)
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
# Per-frame PNGs: set FRAMES=1 to also dump every frame to
# assets/<scenario>.frames/frame-NNN.png. Useful for blog posts or pulling
# a representative still without recording manually.
#
#   FRAMES=1 ./scripts/record-demo.sh happy-path
#
# Output: assets/<scenario>.gif, optionally assets/<scenario>.mp4 and
# assets/<scenario>.frames/

set -euo pipefail

SCENARIO="${1:-happy-path}"
THEME="${THEME:-github-dark}"
CAST="/tmp/${SCENARIO}.cast"
GIF="assets/${SCENARIO}.gif"
MP4="assets/${SCENARIO}.mp4"
FRAMES_DIR="assets/${SCENARIO}.frames"

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

# MP4 from the GIF — Discord/Slack-friendly. Even dimensions required by
# the h264 encoder, hence the trunc-to-multiple-of-2 scale filter. faststart
# moves metadata to the front so the file streams without a full download.
if command -v ffmpeg >/dev/null 2>&1; then
    ffmpeg -y -loglevel error \
        -i "$GIF" \
        -movflags faststart \
        -pix_fmt yuv420p \
        -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" \
        "$MP4"
    echo "Written: $MP4"
else
    echo "Skipped: $MP4  (ffmpeg not installed)"
fi

# Optional per-frame PNG dump.
if [ "${FRAMES:-}" = "1" ]; then
    if command -v ffmpeg >/dev/null 2>&1; then
        rm -rf "$FRAMES_DIR"
        mkdir -p "$FRAMES_DIR"
        ffmpeg -y -loglevel error -i "$GIF" "$FRAMES_DIR/frame-%03d.png"
        echo "Written: $FRAMES_DIR/frame-*.png"
    else
        echo "Skipped: $FRAMES_DIR  (ffmpeg not installed)"
    fi
fi
