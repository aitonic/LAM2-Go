#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
bad=$(find . -type f \( -name '*.rs' -o -name 'Cargo.toml' -o -name 'Cargo.lock' -o -name 'tauri.conf.json' -o -name '*.tsx' -o -name '*.ts' -o -name '*.jsx' -o -name '*.js' -o -name 'package.json' -o -name '*.ttf' -o -name '*.otf' -o -name '*.woff*' \) -not -path './.git/*' -not -path './build/*')
if [ -n "$bad" ]; then printf 'Forbidden application source/assets:\n%s\n' "$bad"; exit 1; fi
if grep -E 'tauri|react|wails|fyne|modernc.org/sqlite|mattn/go-sqlite' go.mod; then
 echo 'Unexpected framework or duplicate SQLite dependency' >&2; exit 1
fi
unformatted=$(find internal -name '*.go' -exec gofmt -l {} +; gofmt -l main.go)
if [ -n "$unformatted" ]; then printf 'Unformatted Go files:\n%s\n' "$unformatted"; exit 1; fi
printf 'PASS: no legacy-language application files, font files, duplicate GUI/SQLite modules, or unformatted Go files.\n'
printf 'Scope: source tree only. Resolved transitive dependencies and packaged resources require an online build audit.\n'
