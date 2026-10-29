# LAM 2.0 — Engineering Agents Contract

## Mission
Implement a complete clean-room **Go + MyGo Native UI** replacement for LAM 0.4.4 using `docs/01-architecture-and-functional-spec.md` through `04-test-spec-and-release.md` as normative design input. Do not port legacy source verbatim.

## Non-negotiables

1. All shipped application code must be Go. Do not add Rust, Cargo, Tauri, React, TypeScript, Vite, WebView-dependent frontend or legacy Rust helper/gateway binaries.
2. MyGo-first: search MyGo Core and official plugins before introducing any UI/desktop/SQLite/terminal/update/file-watcher abstraction. Native UI only.
3. Do not optimize for the fewest dependencies. Prefer stable, licensed, publicly importable Go packages for generic Codex SDK, JSONL parsing and protocol conversion, and pin/tag/test them.
4. Maintain Profile vs PAT semantics, source credentials isolation, single-session Handoff rather than removed bulk Sync, provider transaction recovery, complete Usage accounting and Gateway protocol fidelity.
5. Never silently drop Responses fields, custom/namespace tools, SSE terminal events or account attribution. Unsupported behavior must fail explicitly.
6. Preserve data: original `~/.lam`, `~/.codex` and other Codex homes are read-only during development; new stores use a separate LAM_HOME. No implicit migrations, auth export or production Keychain mutation.
7. Write Go tests and failure/golden cases **before** production code. Every PR must cite tests from `docs/04-test-spec-and-release.md` plus a failure/edge scenario.
8. The Go Gateway may run as a process started from the same Go executable, so CLI sessions survive desktop window closing. Do not implement gateway lifecycle as GUI-only state.
9. UI state update across goroutines uses MyGo `Window.Update`; inspect MyGo `ui.View` frame-lifetime guidance. Use `ui.NewTester` and MyGo vet.
10. Every new dependency must have a purpose, license, fixed version, compatibility test and explicit no-downgrade policy for secrets/protocols.

## Source order

- Normative: `docs/01-architecture-and-functional-spec.md`, `docs/02-dependencies-and-gateway.md`, `docs/03-implementation-plan.md`, `docs/04-test-spec-and-release.md`.
- Historical evidence: `reference/source-inventory.md`, `tests/legacy-fixtures/`, source archive `LAM.zip` (read-only, separately provided).
- External: MyGo official docs, Codex App Server official docs, pinned Go package docs (`reference/official-sources.md`).

## First work item (P0)

- Check `go version`, macOS/architecture availability, MyGo official `native` template and compatible module tags.
- Establish no-legacy-dependencies check and test ID mapping; seed Golden tests from `tests/legacy-fixtures/`.
- Build/execute the project only on a Go 1.27+ compatible environment; do not claim success on Go 1.23.
- Before adding a generic abstraction, explain which MyGo/Go library capability was insufficient.


## Feature-level tests are mandatory, not optional

- Maintain `tests/feature-contracts.csv` as the canonical frozen ID -> Go test contract for the 239 legacy behaviors. Never remove legacy IDs to improve the percentage.
- Every feature marked `DOMAIN_PASS` must have explicit `test_targets` resolving to a *behaviorally relevant* `package:TestName`, and `test_assertion` must say what positive/error or boundary behavior is asserted. A generic file name or an unrelated high-level test is not evidence.
- Run `./tools/check-feature-tests.sh domain` and commit the corresponding regression test when fixing a feature. The gate parses fresh `go test -json` events and fails if the named test was skipped, absent, failed, or its package did not finish PASS.
- UI (`native`), MyGo SQLite/SDK (`integration`) and macOS (`system`) require executed proof at their respective layers. Offline `DOMAIN_PASS` never substitutes native evidence. Explicitly mark `TEST_GAP`, `PARTIAL`, `GAP` or `NATIVE_UNVERIFIED` instead of inflating pass counts.
- Release must pass `./tools/check-feature-tests.sh release` with complete MyGo/macOS evidence. Never fabricate system acceptance rows or mark tests as passed because they were added to source.
- If a test assertion no longer covers the entire legacy operation, add a targeted test or downgrade the claimed feature; do not keep false parity statuses.

## Definition of done

Complete all documented core features, tests, migration snapshots, rollback drills, Mac arm64 signed/native packages, licensing/SBOM review and zero legacy runtime dependency check. Explicitly report unverified behavior; never mark unrun tests green.
