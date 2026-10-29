# LAM 2 - Go / MyGo parity-review source

> 2026-10-28. Development deep-feature-audit build. **Core regression passes; complete native replacement acceptance is not met.**

This package contains actual repaired source, not only a plan. It compares the previous handoff with LAM 0.4.4 and the approved full rewrite specification. Read [the review](docs/PARITY-REVIEW.md), [111-command matrix](docs/LEGACY-COMMAND-MATRIX.md), [status](docs/IMPLEMENTATION-STATUS.md) and [test report](TEST-REPORT.md).

## 2026-10-28 deep feature-by-feature audit

A further original-code and UI review tracks **239 behavior points**: 111 original IPC commands and 128 UX/system flows. See [the deep matrix](docs/DEEP-FEATURE-PARITY-MATRIX.md), [fixes and remaining gaps](docs/DEEP-AUDIT-REPORT.md), and [current tests](docs/DEEP-AUDIT-TEST-REPORT.md). New code includes native list discovery, safe main-profile registration, expanded Usage filters/sort/load limits/activity, diagnostic categories, and tray actions/dual-source refresh. Source-level repairs do **not** imply native acceptance: 6 GAP, 23 PARTIAL and 82 NATIVE_UNVERIFIED traces remain. Do not replace production LAM yet.

## 2026-10-28 mandatory feature-to-test coverage gate

Every legacy feature marked `DOMAIN_PASS` must point to a **specific `package:TestName` and an explicit expected behavior**, and the named test **and whole Go package must pass in current `go test -json` output**. The 239 IDs (111 commands + 128 user flows) are frozen in `tests/feature-contracts.csv`. Missing, skipped, failed, broad-but-unrelated, or unexecuted tests may not be labeled verified. MyGo UI, SQLite, SDK and macOS system functionality require their own native/integration acceptance, not only offline domain tests.

Run `make feature-test` to execute and validate all domain-pass contracts; run `make release-gate` after collecting native and system evidence. **Release gate is intentionally red**, because current full MyGo/macOS validation is blocked. See `docs/FEATURE-TEST-COVERAGE-REPORT.md` and `docs/FEATURE-TEST-TRACEABILITY.csv` for per-feature truth and precise pending test paths.

At the time of this update, **125/239** are `DOMAIN_PASS` with executed direct Go test evidence; **2 previously reported passes have been downgraded** due to missing semantic tests, and **113 non-excluded** features remain without complete verification. This is an evidence count, not full application feature coverage.

## Included

- Go-only source with per-feature Go tests and a 239-row traceability matrix.
- MyGo-native five-page desktop, tray, charts/heatmap, dialogs, terminal, watcher and updater integration source; no alternative UI framework.
- Fully Go account/session/Handoff/provider/gateway/quota/usage services; no old Rust helper or gateway.
- Repairs for custom/namespace tool shims, provider catalog and config transactions, PAT home/credential semantics, SQLite-only history, Antigravity process discovery, all old Handoff divergence policies and native action wiring.
- 42 historical fixtures and original design documents. Fixtures included does not mean every fixture passed a live integration test.
- Real pinned MyGo, Codex SDK, agent-parser and CLIProxyAPI references. No fake framework or fake go.sum.
- Raw red/green/final logs and reproducible offline/online checks. No font files or Rust/React/Tauri/TypeScript/frontend JS source.

## Verification

| Scope | Result |
|---|---|
| Unmodified business packages | 171 top-level tests; 188 pass records including subtests; 15 tested production packages |
| Business race / go vet | PASS |
| Source formatting and forbidden-language audit | PASS |
| Measured statement coverage | 67.49%, offline business scope only |
| Full Go build/test | BLOCKED before compilation: Go 1.27.3 download DNS failure |
| Actual MyGo/SQLite/SDK/parser/translator integration | NOT RUN |
| macOS/signing/DMG/live accounts | NOT RUN |

Known non-equivalences remain explicit: physical account-directory/wrapper rename, independent Usage section loading, account cache/discovery edge cases, UX details and full diagnostic-category presentation. Automatic *read-only* discovery is now implemented. Native API/type compatibility is unknown until a real build succeeds. See the review rather than assuming a high source-file count establishes parity.

## Development and verification

The tagged MyGo v0.7.1 module requires Go **1.27.3**. `mise.toml` pins it. In a suitable online macOS environment:

```sh
mise install
mise exec -- ./tools/bootstrap.sh
mise exec -- ./tools/verify.sh
mise exec -- go tool mygo dev
```

Bootstrap resolves actual dependencies/checksums and generates MyGo localization output. `go.sum` could not be generated in this container; no hashes were invented. Review and commit the resolved graph after real download. `verify.sh` fails on errors and does not hide failures behind a successful pipe. CI files are supplied but no remote workflow was dispatched.

The offline command is intentionally narrower:

```sh
./tools/test-offline.sh json
./tools/test-offline.sh race
./tools/test-offline.sh vet
./tools/check-source.sh
```

It copies unmodified business packages to a temporary Go 1.23 module and excludes native/integration packages. Test fakes on external boundaries are not replacements for production dependencies.

## Data and compatibility

Default application root: `~/.lam-next`. Imported old Codex homes remain in place. Profile and PAT identities have independent homes; global PAT activation deliberately changes shared authentication with a recoverable encrypted baseline. Never use production credentials for development acceptance.

The new logical legacy SQLite importer preserves raw tables/columns and normalizes usage rows, including history that has no surviving JSONL. Publication is staged; imported aliases avoid replay duplicates. The old writer must be stopped; hashes and data_version detect changes. **Real MyGo/WAL integration has not been run**, so keep the original database and backups intact.

Old bindings are preserved as pending reconnect operations, not silently pointed at a removed Rust binary. Credentials stay references/encrypted snapshots. Source templates do not copy auth, and bound source configs require an explicit new binding.

## Commands and package map

See [CLI reference](docs/CLI.md). One binary supports desktop/gateway/auth/launch and administrative modes. `auth` prints only a token; credentials are read from explicit files/stdin/references, not literal CLI flags.

- `internal/account`, `session`, `handoff`, `provider`, `quota`, `usage`: business behavior.
- `internal/gateway`: authenticated loopback, targeted compatibility shim, bounded SSE and Go process lifetime.
- `internal/desktop`: real MyGo controls/state/actions.
- `internal/integrations`: actual third-party APIs, not yet native-tested here.
- `internal/storage`, `runtime`, `settings`, `migration`: local persistence/OS/config/migration.
- `reports`: exact final logs and historical red/green evidence.

## Distribution

`tools/build-macos.sh` requires macOS and complete online verification. The current app signing identity is ad-hoc, not a public distribution identity. No notarized app, live update endpoint or signing key is supplied. Dependency graph/SBOM and native resources require the real build audit. MIT notice is preserved; no font assets are redistributed.
