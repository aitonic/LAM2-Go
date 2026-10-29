#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
./tools/check-environment.sh
# Pin direct requirements in go.mod. The first online run resolves authentic
# transitive hashes; no synthetic go.sum is included in this source handoff.
go mod download
go mod tidy
go mod verify
go tool mygo install-skills
go tool mygo generate
go tool mygo i18n check
printf '\nResolved dependencies. Review and commit go.mod, go.sum and generated catalogs.\n'
