# LAM 2 parity review and repair report

**Date: 2026-10-27. Result: reviewed-source repair; domain regression PASS; full native release acceptance NOT MET.**

This review changes code, not only documentation. It compares the original LAM 0.4.4 archive, the approved four implementation specifications, and the previous development-source handoff. The original project is used as a read-only behavioral reference; none of its Rust/React/Tauri source or binaries are shipped.

## Evidence scope

- Original 111 registered command entries: `reference/legacy-command-inventory.json`; each maps to Go source in `docs/LEGACY-COMMAND-MATRIX.md`.
- Five original user-facing sections plus tray; original account/PAT, Handoff, provider/gateway, quotas, usage, migration and terminal sources inspected.
- 42 original data fixtures retained. Redacted captures are not complete live recordings. Where a test reconstructs a tool definition, it is a targeted synthetic contract test, not unchanged production replay.
- Core tests use real unmodified business packages. Real MyGo/SDK/parser/converter compilation and macOS behavior remain unverified.

## Repairs

Old paths beginning `services/` refer to `apps/desktop/src-tauri/src/`; frontend paths refer to `apps/desktop/`.

| # | Difference found | Original evidence | Go changes | Repair / scope | Evidence |
|---|---|---|---|---|---|
| 1 | Gateway namespace/custom/additional tools | services/adapters/; codex-gateway-contract fixtures | internal/gateway/compatibility.go; internal/integrations/translator/translator.go | Targeted namespace flattening, identity restoration, custom-input framing, history and additional_tools; no silent loss. | internal/gateway/parity_test.go |
| 2 | Reasoning and native Responses routing | services/gateway/; services/adapters/deepseek.rs | internal/gateway/reasoning.go; internal/gateway/reasoning_collector.go; internal/gateway/passthrough.go | Bounded binding/generation/session reasoning; configured native Responses gateway route retained. | internal/gateway/parity_test.go |
| 3 | Upstream error and SSE transport | services/gateway/routes.rs; services/adapters/sse.rs | internal/gateway/server.go; internal/gateway/errors.go | HTTP errors/headers redacted, stream state/cancellation/limits retained; real translator integration still unrun. | internal/gateway/gateway_test.go |
| 4 | Model catalog and context/compaction defaults | services/provider_api_v2.rs; services/provider_config_editor.rs | internal/model/catalog.go; internal/provider/catalog.go; internal/provider/projection.go | Original model metadata defaults/data, generated catalog, context windows and 90-percent auto-compaction threshold. | internal/provider/provider_test.go |
| 5 | Bound-provider edits | services/provider_api_v2.rs | internal/provider/reproject.go | Update model/reasoning/native-gateway route transactionally; keep original restore snapshot; invalidate correct home. | internal/provider/provider_test.go |
| 6 | API-account lifecycle | services/provider_api_v2.rs | internal/provider/api_account.go | Create, connection read/edit, model switch and delete; failed delete restores prior binding when safe. | internal/provider/api_account_test.go |
| 7 | Provider health action | services/provider_api_v2.rs: probe_provider_upstream_service_v2 | internal/provider/health.go | Explicit direct Responses /models probe; persisted redacted observation; rejects arbitrary success JSON. | internal/provider/health_test.go |
| 8 | Detach preview contract | services/provider_api_v2.rs: plan_detach_service_v2 | internal/provider/detach.go | Preview, fingerprint, revision/generation/config conflict validation before restoring original config. | internal/provider/detach_test.go |
| 9 | Gateway port migration | services/gateway/supervisor.rs; services/provider_api_v2.rs | internal/provider/port.go; internal/gateway/supervisor.go | Preview and commit with readiness/identity, no foreign PID termination; settings cannot silently change occupied runtime port. | internal/provider/port_test.go |
| 10 | Codex-profile credential source | services/provider_credentials.rs | internal/account/secondary.go; internal/provider/provider.go | Read only selected profile API key; never substitute OAuth/PAT/secondary quota token. | internal/account/account_test.go |
| 11 | PAT separate homes and launch mode | services/account.rs; services/types.rs | internal/account/runtime_profile.go; internal/account/pat.go | All identities retain private homes; shared authentication is a deliberate global launch mode, not the storage model. | internal/account/account_test.go |
| 12 | Native auth and CPA/session/PAT | services/account.rs | internal/account/account.go; internal/account/secondary.go | Preserve auth extensions/last_refresh, runtime/secondary credentials, expiry and confirmed CPA export. | internal/account/account_test.go |
| 13 | Two-file PAT switch/recovery | services/account.rs | internal/account/pat.go | auth.json plus auth-f.json, encrypted baseline and conflict checks; preserve event-time usage timeline. | internal/account/account_test.go |
| 14 | Profile configuration template | services/account.rs: execute_create_account_with_route | internal/account/account.go; internal/desktop/forms.go | Optional byte-preserving config copy, seven original home subdirectories; no auth transfer. Bound source must be rebound explicitly. | internal/account/account_test.go |
| 15 | Auxiliary relay profile | services/account.rs: create_relay_plan / execute_create_relay | internal/account/relay.go | Separate previewed profile with source/runtime/policy provenance and no copied identity; CLI and native entry added. | internal/account/relay_test.go |
| 16 | Session source and title index | services/session.rs | internal/session/session.go | Active/archived discovery, latest title index, large-file tail metadata, bounded raw detail. | internal/session/parity_test.go |
| 17 | Whole-filter selection and storage summary | services/session.rs | internal/session/selection.go | Selection is not confined to visible page; retained recent20/recent7days protection and capacity summary. | internal/session/parity_test.go |
| 18 | Archive, restore and crash recovery | services/session.rs | internal/session/archive.go; internal/session/archive_index.go | File/index coordination, preserve newly appended title records, recover interrupted operation. | internal/session/parity_test.go |
| 19 | Handoff compatibility | services/relay.rs | internal/handoff/compatibility.go | Original provider-sensitive encrypted/hosted/unknown/tool-history blockers; confirmation cannot waive unrepresentable protocol. | internal/handoff/parity_test.go |
| 20 | All original divergence strategies | services/relay.rs | internal/handoff/branches.go | Prefix continuation; prefer_source/target; timeline auxiliary fork; target-account summary; original target retained where required. | internal/handoff/parity_test.go |
| 21 | Antigravity discovery and groups | services/antigravity.rs | internal/quota/antigravity.go; internal/quota/antigravity_discovery.go | Configured port, own-process CSRF, local TLS/HTTP behavior, grouped models and cache/unknown metadata. | internal/quota/antigravity_parity_test.go |
| 22 | Usage token counters and historical prices | services/usage.rs; src/lib/usage-pricing.ts | internal/usage/counters.go; internal/usage/legacy_prices.go | Prefer last_token_usage, cumulative fallback, no cached/reasoning double count, versioned historical pricing and unknown costs. | internal/usage/usage_test.go |
| 23 | PAT attribution and archive index | services/usage.rs; PAT usage timeline | internal/usage/indexer.go; internal/app/app.go | Account attribution at event time; distinguish isolated PAT from shared timeline; archived data remains visible. | internal/usage/usage_test.go |
| 24 | SQLite-only history migration | services/usage.rs: init_usage_db | internal/migration/usage.go; internal/integrations/mygosqlite/legacy.go | Preserve every raw table/column; paged staged publication; alias dedup after JSONL replay; reset retains DB-only occurrences. | internal/migration/usage_test.go; native sqlite_test.go UNRUN |
| 25 | Legacy metadata and pending bindings | services/lam_paths.rs; account/proxy/provider stores | internal/migration/metadata.go; internal/migration/bindings.go | Settings, notes/renewal/proxy/expiry, PAT timeline, relay provenance and bindings retained; no old executable-token reuse. | internal/migration/migration_test.go |
| 26 | Account Usage, activity and pagination | services/usage.rs; src/components/Usage* | internal/usage/account.go; internal/desktop/account_usage.go; internal/desktop/activity.go | Account/local separate; only same complete date ranges compared; heatmap, daily/weekly activity and Calls/Threads source. | internal/usage/account_test.go; actual UI UNRUN |
| 27 | Full-filter Insights and diagnostics | src/lib/usage-dashboard-analysis.ts; services/usage.rs | internal/usage/insights.go; internal/usage/diagnostics.go; internal/integrations/mygosqlite/sqlite.go | Fast usage, effort distribution, skill counts, cache/context diagnostics across full query, not visible page. | internal/usage/insights_test.go; real SQLite UNRUN |
| 28 | Legacy global account snapshots | services/usage.rs account_usage_snapshot/day tables | internal/integrations/mygosqlite/legacy.go; internal/usage/diagnostics.go | Preserve/display old global snapshot without inventing a missing account identity. | internal/usage/insights_test.go; real SQLite UNRUN |
| 29 | cmux, executable discovery and ChatGPT restart | services/terminal.rs; services/account.rs | internal/runtime/terminals.go; internal/runtime/executable.go; internal/runtime/chatgpt.go | Original workspace-create arguments and restricted fallback, Finder PATH fallback, scoped restart/restore bounds. | internal/runtime/runtime_test.go; macOS UNRUN |
| 30 | Settings and timeout boundaries | services/types.rs | internal/settings/settings.go; internal/desktop/settings.go | First-response10..600/request300..3600; real opacity, Dock and native numeric forms; no fictitious updater feed. | internal/settings/settings_test.go; macOS UNRUN |

## Important original behaviors that should NOT be "fixed" incorrectly

1. Ordinary Codex compaction is a request to `/responses`; the legacy gateway did not provide a separate compatible `/v1/responses/compact`. The latter still explicitly returns 501. This is not counted as a newly missing old feature.
2. Antigravity used a user-configured port plus discovered local-process authentication. The repair does not add arbitrary port scanning or kill a process found by lsof.
3. Whole-account session Sync was explicitly removed in the original 2026-07-17 product revision. It is not reintroduced. Auxiliary relay-profile creation and single-session Handoff remain separate operations.
4. The old rate card is historical data, not asserted to be the latest provider price list. Local dollar amounts remain API-equivalent estimates, not a subscription invoice.
5. The old account-usage database may lack an account identity. Those snapshots stay explicitly unattributed instead of being assigned to the current user.

## Actual differences and incomplete acceptance -- not hidden by passing tests

| Area | Actual boundary | Consequence |
|---|---|---|
| Physical account rename | New Edit changes display name and keeps stable profile ID/home/wrapper. Old code physically renames the home and human-readable wrapper. | This is not byte-for-byte or command-name compatibility. No automatic relocation of a user-owned old home is performed. |
| Automatic discovered-account listing | New code offers read-only discovery and explicit registration/import; it does not yet make every newly found old home a fully managed row automatically. | Exact startup/discovery behavior differs from original list_accounts. |
| Detailed diagnostic taxonomy | All imported raw tables/fields are preserved, but some new parser skip categories are aggregated. | Not every old diagnostic category is reproduced as a separate UI metric. |
| Real converter/SDK/parser integration | Real pinned adapters and tests are present but not built or run. | Full namespace/custom/reasoning/image/structured-output protocol equivalence cannot be certified from fake-boundary tests. |
| Native MyGo SQLite | Real migration/queries and native tests are written; only separate SQL literal checks executed. | WAL snapshot consistency and plug-in API compatibility still need native integration execution. |
| Native desktop | Source includes new controls, pages and wired actions; MyGo Tester/build not executed. | No assertion of exact visual parity, input-method, focus, tray, chart, accessibility or terminal UX acceptance. |
| Live account authorization / signing | No real accounts, Apple credentials, remote workflows or production data were used. | No signed app, notarized DMG, live quota/SDK-pair or updater/rollback acceptance. |

Additional deliberate safety differences: old data is imported into the approved isolated new root rather than rewritten in place; old bindings require explicit new-Go reconnection; external homes are not recursively deleted; configuration copying from a bound source requires an explicit new binding rather than copying another identity's executable/token reference.

## Test result

- 162 top-level business tests; 179 test/subtest passes; 0 failed and 0 skipped in final business run.
- 15 production packages with tests; testutil is support-only and not an extra tested business package.
- Race and go vet pass for the same business scope. Statement coverage 66.90% (4539/6785), not whole-application coverage.
- Full `go test ./...` fails before compilation while downloading Go 1.27.3 because container DNS cannot reach the module proxy.
- See `TEST-REPORT.md` for final raw log names and all excluded integration boundaries.

## Primary external references checked

- MyGo tagged module: https://raw.githubusercontent.com/egoist/mygo/v0.7.1/go.mod (declares Go 1.27.3).
- Native UI: https://mygo.egoist.dev/docs/ui (real Go-native UI, no WebView frontend required).
- SQLite: https://mygo.egoist.dev/docs/plugins/sqlite (native Go uses sqlite.Open directly; no duplicate database framework).

## Acceptance verdict

**This is a materially repaired source handoff, not a certified no-omission replacement release.** Native compilation and live OS integration are unknown, and the behavioral differences above remain explicit. The original normative specifications are unchanged; no failed or unexecuted requirement has been deleted to make acceptance appear green.
