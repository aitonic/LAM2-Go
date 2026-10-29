#!/bin/sh
# Run UNMODIFIED standard-library domain packages, NOT native integrations.
# This does not replace or stub MyGo, the SDK, the parser or the translator.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/internal" "$root/reports"
for name in fault model storage account codex session handoff provider gateway quota usage runtime settings migration commands testutil; do
  if [ -d "$root/internal/$name" ]; then cp -R "$root/internal/$name" "$stage/internal/"; fi
done
cp -R "$root/tests" "$stage/"
printf 'module lam.local/lam2\n\ngo 1.23.0\n' > "$stage/go.mod"
cd "$stage"
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
if [ "${1:-}" = "vet" ]; then
  go vet ./internal/...
elif [ "${1:-}" = "race" ]; then
  go test -race -p 1 -count=1 -timeout=60s ./internal/...
elif [ "${1:-}" = "json" ]; then
  go test -json -count=1 -timeout=120s -coverprofile="$root/reports/domain-coverage.out" ./internal/...
else
  go test -count=1 -timeout=120s -cover ./internal/...
fi
