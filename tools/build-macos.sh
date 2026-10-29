#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ "$(uname -s)" != Darwin ]; then echo 'Run this on macOS to create a DMG.' >&2; exit 1; fi
./tools/verify.sh
# Signing identity, update feed/public key and notarization are configured by
# the owner in mygo.json. No guessed credentials or publishing destination.
go tool mygo build -platform darwin/arm64
find build -type f \( -name '*.dmg' -o -name '*.zip' \) -exec shasum -a 256 {} \; > reports/artifact-sha256.txt
