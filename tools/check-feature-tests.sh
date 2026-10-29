#!/bin/sh
# A feature marked DOMAIN_PASS needs a named, actually passing Go test.
# Release requires every non-excluded feature to have full layer-specific proof.
set -eu
cd "$(dirname "$0")/.."
mode=${1:-domain}
mkdir -p reports
case "$mode" in
  domain)
    # Always regenerate events rather than trusting a checked-in green report.
    ./tools/test-offline.sh json > reports/feature-pass-events.jsonl
    GO111MODULE=off GOTOOLCHAIN=local go test -count=1 ./tools/featuregate
    GO111MODULE=off GOTOOLCHAIN=local go run ./tools/featuregate/main.go \
      -matrix tests/feature-contracts.csv -results reports/feature-pass-events.jsonl \
      -out reports/feature-test-results.csv -mode domain
    ;;
  release)
    if [ ! -s reports/feature-pass-events.jsonl ] || [ ! -s reports/native-tests.jsonl ]; then
       echo 'RELEASE BLOCKED: missing executed domain/native test events' >&2
       exit 1
    fi
    GO111MODULE=off GOTOOLCHAIN=local go test -count=1 ./tools/featuregate
    GO111MODULE=off GOTOOLCHAIN=local go run ./tools/featuregate/main.go \
      -matrix tests/feature-contracts.csv -results reports/feature-pass-events.jsonl \
      -integration-results reports/native-tests.jsonl \
      -system-evidence reports/system-acceptance.csv \
      -out reports/feature-release-gate.csv -mode release
    ;;
  *) echo 'usage: tools/check-feature-tests.sh domain|release' >&2; exit 2;;
esac
