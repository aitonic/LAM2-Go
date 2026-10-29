#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
./tools/check-environment.sh
if [ ! -s go.sum ]; then
 echo 'ERROR: run tools/bootstrap.sh in an online environment first; go.sum is not fabricated.' >&2
 exit 1
fi
mkdir -p reports build
export GOFLAGS=-mod=readonly
# Do not mask a failed test with a successful tee command.
go mod verify > reports/native-module-verify.txt 2>&1
go tool mygo generate > reports/native-generate.txt 2>&1
go tool mygo i18n check > reports/native-i18n.txt 2>&1
go test -json -count=1 -timeout=5m -coverprofile=reports/full-coverage.out ./... > reports/native-tests.jsonl 2> reports/native-tests.stderr
go test -race -count=1 -timeout=5m ./... > reports/native-race.txt 2>&1
go vet ./... > reports/native-vet.txt 2>&1
go tool mygo vet > reports/native-mygo-vet.txt 2>&1
go build -trimpath -o build/lam2 . > reports/native-build.txt 2>&1
go list -m -json all > reports/resolved-modules.json
go version -m build/lam2 > reports/build-modules.txt
./tools/check-source.sh > reports/source-audit.txt
./tools/check-feature-tests.sh domain > reports/domain-feature-gate.txt 2>&1
printf 'Full Go build and automated tests passed. macOS manual acceptance is still separate.\n'
