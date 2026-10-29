> **Updated feature-test verification (2026-10-28):** This file is a historical snapshot. For current 239-item statuses, mapped executable tests and CI gate use [`docs/FEATURE-TEST-COVERAGE-REPORT.md`](docs/FEATURE-TEST-COVERAGE-REPORT.md) and [`tests/feature-contracts.csv`](tests/feature-contracts.csv). Prior 127 `DOMAIN_PASS` is no longer valid; current direct domain evidence is 125. macOS/native release is NOT accepted.

> **Latest evidence, 2026-10-28**: superseded by `docs/DEEP-AUDIT-TEST-REPORT.md`. Offline domain 171 top-level / 188 test-and-subtest pass records; targeted account+usage Race PASS, offline vet/source PASS, native full build BLOCKED. The following historical numbers describe the previous handoff only.

# LAM 2 review-fix test report

Date: 2026-10-27. **DOMAIN REGRESSION PASS / COMPLETE NATIVE ACCEPTANCE NOT MET**.

## Reproducible environment and source

- Linux amd64; installed Go 1.23.2.
- Application directly pins MyGo v0.7.1 and Go 1.27.3. No dependency checksum or alternate stub framework was fabricated.
- 126 Go files, 35 Go test files, 21,239 Go lines.
- 111 original command entries inventoried; 42 historical fixture files retained. Neither number is a count of passing end-to-end scenarios.
- New application root remains isolated at ~/.lam-next. All executed tests use temporary homes and synthetic credentials/local HTTP peers.

## Final runs

| Check | Actual result | Raw evidence |
|---|---|---|
| Business test run | PASS: 162 top-level tests, 179 test/subtest pass events, 0 fail, 0 skip | `reports/review-116-config-copy-green.jsonl` |
| Business race detector | PASS, all 15 tested production packages | `reports/review-117-final-race.txt` |
| Business go vet | PASS, exit 0; empty output is expected | `reports/review-118-final-vet.txt` |
| Source formatting/forbidden-language scan | PASS | `reports/review-119-final-source-audit.txt` |
| Full go test ./... | BLOCKED before compilation: Go 1.27.3 proxy DNS failure | `reports/review-120-full-build-attempt.txt` |
| Literal SQL cross-check | PASS with Python SQLite 3.46.1; not the Go plug-in | `reports/review-121-sql-literals.json` |

Statement coverage: **66.90%**, 4539/6785 instrumented statements. Scope is the unmodified business packages copied by `tools/test-offline.sh`, including test support instrumentation. It excludes desktop, app/CLI composition and all third-party integration packages; it is **not whole-app coverage**.

`reports/review-94-final-race.txt` records an interrupted tool invocation, not a successful race result. The complete subsequent final run is `reports/review-117-final-race.txt`. Earlier `*-red*` logs intentionally retain failing-before-implementation evidence. They do not override the final run. An empty vet log is a successful quiet command, not a missing report.

## What executed tests actually establish

- Namespaced/custom tool shim identities and custom input restoration, history handling, bounded reasoning state, local HTTP/SSE lifecycle and status/error redaction.
- Account/PAT home separation, secondary quota credentials, native-auth extension preservation, dual-file switch/recovery, config-template behavior and independent relay-profile creation.
- Handoff prefix continuation and original divergence strategies, compatibility rejection, fingerprint invalidation and no authentication-file transfer.
- Full-filter selection, archive/title-index transaction and crash recovery, path and recent-session protection.
- Provider catalog/compaction projection, bound connection changes, API-account create/switch/delete rollback, health observation, detachable preview and port migration.
- Antigravity configured-port/process-token parsing, grouped model normalization and controlled local HTTP/TLS test servers, not a live installed Antigravity process.
- Authoritative last_token_usage, cached/reasoning subset arithmetic, historical pricing, archived session and PAT-time attribution, daily/account comparisons, full-result Insights and diagnostics.
- Logical migration staging, source change detection, metadata mapping, imported row preservation and fake-repository rollback contracts.

## External boundaries are not certified

Gateway transport tests use test converters; direct compatibility-shim tests do not execute the pinned CLIProxyAPI registry. Index/migration unit tests use fake readers/repositories. The separately executed SQL check only extracts and executes literal production SQL for schema, publication, alias deduplication, reset retention and rollback. It does not call the MyGo Go API, load its native library or exercise the WAL reader.

Written but **not run**: MyGo UI Tester, actual SQLite/WAL and full-filter Insights integration, actual agent-parser, actual CLIProxyAPI adapter, real HTTP-client adapter, application composition, full CLI build. No full CLI+SDK version-pair login/usage/reset test, macOS tray/IME/Keychain/cmux/ChatGPT process test, signing, notarization, installation, updater or production rollback was performed.

## Residual behavioral differences

See `docs/PARITY-REVIEW.md`: physical rename/wrapper naming still differs; automatic discovered-account registration/listing is not identical; parser diagnostic categories are not all surfaced separately. These are not relabeled as passing parity tests. Any remaining type/API/runtime incompatibility may first appear during a real Go 1.27.3 build.

## Acceptance

**Do not treat this ZIP as a signed production replacement.** It is corrected source with reproducible core regression evidence. The original P0-P9/GW/CDX/PRV/SES/QTA/USE/UI/MIG specification remains the acceptance target; unrun test IDs are not marked passed. No real account, production directory, Keychain or remote repository was modified by this review.
