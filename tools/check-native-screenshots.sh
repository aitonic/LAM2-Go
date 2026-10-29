#!/bin/sh
# Six original reference screens, no screenshot = no visual parity.
# A deterministic fixture must produce the candidate images with matching
# viewport/DPI. Pixel-exact baseline: zero tolerance and zero changed pixels.
# Run on macOS after capturing real MyGo native windows.
set -eu
cd "$(dirname "$0")/.."
for screen in OverView Usage Settings Bar AddAPI AddAcount; do
    base="reference/visual/$screen.png"
    candidate="reports/native-screenshots/$screen.png"
    if [ ! -f "$candidate" ]; then
        echo "VISUAL PARITY BLOCKED: no native screenshot: $candidate" >&2
        exit 1
    fi
    GOTOOLCHAIN=local GO111MODULE=off go run ./tools/visualdiff \
        -reference "$base" -candidate "$candidate" \
        -channel-tolerance 0 -max-changed 0
 done
