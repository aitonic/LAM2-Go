# LAM 0.4.4 Source Inventory and Go Rewrite Crosswalk

Generated from the uploaded LAM.zip source tree on 2026-10-27.
Paths are relative to `apps/desktop`. New destinations are planning targets, not existing files.

- Rust implementation files: **64**
- Rust integration tests: **56**
- TypeScript/TSX source files: **69**
- Legacy golden fixtures: **42**

## 1. Rust implementation files

| Original file | Proposed ownership |
|---|---|
| `src-tauri/src/adapters/codex/mod.rs` | `review per exported contract` |
| `src-tauri/src/bin/lam-auth-helper.rs` | `internal/account + internal/codex` |
| `src-tauri/src/bin/lam-core.rs` | `review per exported contract` |
| `src-tauri/src/bin/lam-provider-gateway.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/bin/lam.rs` | `review per exported contract` |
| `src-tauri/src/commands/mod.rs` | `review per exported contract` |
| `src-tauri/src/errors.rs` | `review per exported contract` |
| `src-tauri/src/lib.rs` | `review per exported contract` |
| `src-tauri/src/main.rs` | `review per exported contract` |
| `src-tauri/src/models.rs` | `review per exported contract` |
| `src-tauri/src/services/account.rs` | `internal/account + internal/codex` |
| `src-tauri/src/services/account_proxy.rs` | `internal/account + internal/codex` |
| `src-tauri/src/services/adapters/deepseek.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/mod.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/nonstream.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/protocol.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/registry.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/request.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/sse.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/adapters/terminal.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/antigravity.rs` | `internal/quota` |
| `src-tauri/src/services/codex_api_key_auth.rs` | `internal/account + internal/codex` |
| `src-tauri/src/services/codex_command.rs` | `internal/runtime` |
| `src-tauri/src/services/credential_store.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/error.rs` | `review per exported contract` |
| `src-tauri/src/services/gateway/binding.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/catalog.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/identity.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/launch_planner.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/launcher.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/listener_handoff.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/mod.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/observability.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/recovery.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/routes.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/server.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/sidecar.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/supervisor.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/gateway/upstream.rs` | `internal/gateway + tests/contract` |
| `src-tauri/src/services/lam_paths.rs` | `internal/storage` |
| `src-tauri/src/services/mod.rs` | `review per exported contract` |
| `src-tauri/src/services/provider.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_api_v2.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_attach_transaction.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_attach_update.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_auth_command.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_binding.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_capability.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_config_editor.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_credentials.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_keychain.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_model_discovery.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_planner.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_relay_compatibility.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_runtime.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/provider_v2.rs` | `internal/provider + internal/storage` |
| `src-tauri/src/services/quota.rs` | `internal/quota` |
| `src-tauri/src/services/relay.rs` | `internal/session + internal/handoff` |
| `src-tauri/src/services/runtime.rs` | `internal/runtime` |
| `src-tauri/src/services/session.rs` | `internal/session + internal/handoff` |
| `src-tauri/src/services/storage.rs` | `internal/storage` |
| `src-tauri/src/services/types.rs` | `review per exported contract` |
| `src-tauri/src/services/usage.rs` | `internal/usage` |
| `src-tauri/src/tray.rs` | `internal/ui/tray` |

## 2. Rust integration test files

| Original test | Proposed target |
|---|---|
| `src-tauri/tests/account_proxy.rs` | `internal/account + internal/codex` |
| `src-tauri/tests/account_proxy_routing.rs` | `internal/account + internal/codex` |
| `src-tauri/tests/codex_terminal_entry.rs` | `internal/runtime` |
| `src-tauri/tests/integration_pat_accounts.rs` | `internal/account + internal/codex` |
| `src-tauri/tests/integration_pat_auth.rs` | `internal/account + internal/codex` |
| `src-tauri/tests/phase1_core.rs` | `review per exported contract` |
| `src-tauri/tests/probe_connection.rs` | `review per exported contract` |
| `src-tauri/tests/provider_adapter_deepseek.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_nonstream.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_registry.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_request.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_schema.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_sse.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_terminal.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_adapter_tools.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_api_v2.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_attach_transaction.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_auth_command.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_auth_runtime.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_binding.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_codex_api_key_auth.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_codex_catalog.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_codex_contract.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_codex_launch_planner.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_config_editor.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_credentials.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_gateway_adapter_integration.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_binding.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_identity.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_launcher.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_listener_handoff.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_observability.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_recovery.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_routes.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_server.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_sidecar.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_supervisor.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_timeout_config.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_gateway_upstream.rs` | `internal/gateway + tests/contract` |
| `src-tauri/tests/provider_keychain.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_legacy_contract.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_model_discovery.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase1_g2.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase1_production.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase2_g3.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase3_g4.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase3_process_g4.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase4_api_account.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase4_capability.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase4_relay_analyzer.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_phase4_relay_integration.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_planner.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_sidecar_production.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_v2.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/provider_versioned_store.rs` | `internal/provider + internal/storage` |
| `src-tauri/tests/removed_bulk_sync.rs` | `review per exported contract` |

## 3. TypeScript and TSX source files

| Original file | Proposed ownership |
|---|---|
| `src/App.handoff.test.tsx` | `internal/session + internal/handoff` |
| `src/App.tsx` | `internal/ui + internal/app` |
| `src/components/account-proxy.test.tsx` | `internal/account + internal/codex` |
| `src/components/account-proxy.tsx` | `internal/account + internal/codex` |
| `src/components/api-account-flow.test.tsx` | `internal/account + internal/codex` |
| `src/components/api-account-flow.tsx` | `internal/account + internal/codex` |
| `src/components/icons.tsx` | `internal/ui + internal/app` |
| `src/components/plan-type-badge.tsx` | `internal/ui + internal/app` |
| `src/components/plan-view.tsx` | `internal/ui + internal/app` |
| `src/components/provider-binding-dialog.test.tsx` | `internal/provider + internal/storage` |
| `src/components/provider-binding-dialog.tsx` | `internal/provider + internal/storage` |
| `src/components/provider-center-flow.test.tsx` | `internal/provider + internal/storage` |
| `src/components/provider-center.test.tsx` | `internal/provider + internal/storage` |
| `src/components/provider-center.tsx` | `internal/provider + internal/storage` |
| `src/components/provider-model-picker.tsx` | `internal/provider + internal/storage` |
| `src/components/quota-window.tsx` | `internal/quota` |
| `src/components/shell.tsx` | `internal/ui + internal/app` |
| `src/components/theme-toggle.tsx` | `internal/ui + internal/app` |
| `src/components/tray-quota-panel.test.tsx` | `internal/quota` |
| `src/components/tray-quota-panel.tsx` | `internal/quota` |
| `src/components/ui-button.tsx` | `internal/ui + internal/app` |
| `src/lib/antigravity.test.ts` | `internal/quota` |
| `src/lib/antigravity.ts` | `internal/quota` |
| `src/lib/api-runtime.test.ts` | `internal/runtime` |
| `src/lib/api.test.ts` | `review per exported contract` |
| `src/lib/api.ts` | `review per exported contract` |
| `src/lib/auth.test.ts` | `internal/account + internal/codex` |
| `src/lib/auth.ts` | `internal/account + internal/codex` |
| `src/lib/format.test.ts` | `review per exported contract` |
| `src/lib/format.ts` | `review per exported contract` |
| `src/lib/provider-api-v2.test.ts` | `internal/provider + internal/storage` |
| `src/lib/provider-contract.test.ts` | `internal/provider + internal/storage` |
| `src/lib/provider-models.test.ts` | `internal/provider + internal/storage` |
| `src/lib/provider-models.ts` | `internal/provider + internal/storage` |
| `src/lib/quota.test.ts` | `internal/quota` |
| `src/lib/quota.ts` | `internal/quota` |
| `src/lib/reset.test.ts` | `internal/quota` |
| `src/lib/reset.ts` | `internal/quota` |
| `src/lib/theme.ts` | `review per exported contract` |
| `src/lib/tray-popover-prefs.ts` | `internal/ui/tray` |
| `src/lib/tray-popover-size.ts` | `internal/ui/tray` |
| `src/lib/types.ts` | `review per exported contract` |
| `src/lib/usage-dashboard-analysis.ts` | `internal/usage` |
| `src/lib/usage-dashboard-data.ts` | `internal/usage` |
| `src/lib/usage-dashboard-format.ts` | `internal/usage` |
| `src/lib/usage-dashboard.test.ts` | `internal/usage` |
| `src/lib/usage-diagnostics.ts` | `internal/usage` |
| `src/lib/usage-pricing.ts` | `internal/usage` |
| `src/main.tsx` | `review per exported contract` |
| `src/routes/account-proxy.test.tsx` | `internal/account + internal/codex` |
| `src/routes/handoff.test.tsx` | `internal/session + internal/handoff` |
| `src/routes/types.ts` | `internal/ui + internal/app` |
| `src/routes/usage.test.tsx` | `internal/usage` |
| `src/routes/usage.tsx` | `internal/usage` |
| `src/routes/views.tsx` | `internal/ui + internal/app` |
| `src/stores/accounts.test.ts` | `internal/account + internal/codex` |
| `src/stores/accounts.ts` | `internal/account + internal/codex` |
| `src/stores/app.test.ts` | `internal/ui + internal/app` |
| `src/stores/app.ts` | `internal/ui + internal/app` |
| `src/stores/providers-v2.test.ts` | `internal/provider + internal/storage` |
| `src/stores/providers.ts` | `internal/provider + internal/storage` |
| `src/stores/quota.test.ts` | `internal/quota` |
| `src/stores/quota.ts` | `internal/quota` |
| `src/stores/sessions.test.ts` | `internal/session + internal/handoff` |
| `src/stores/sessions.ts` | `internal/session + internal/handoff` |
| `src/stores/usage.test.ts` | `internal/usage` |
| `src/stores/usage.ts` | `internal/usage` |
| `src/tray-quota-app.tsx` | `internal/quota` |
| `src/vite-env.d.ts` | `review per exported contract` |

## 4. Legacy test fixtures

| Fixture |
|---|
| `src-tauri/tests/fixtures/codex-gateway-contract/manifest.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/function-tool-events.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/function-tool-followup-request.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/function-tool-initial-request.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/resume-events.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/resume-initial-request.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/resume-request.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/text-stream-events.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/normal/text-stream-request.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/auth-helper.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/determinism.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/disconnect.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/failure-determinism.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/model-catalog.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/normal.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/retries.json` |
| `src-tauri/tests/fixtures/codex-gateway-contract/observations/route-inventory.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/account/accounts-cache.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/account/managed-account-metadata.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/config/auth-conflict-placeholder.toml` |
| `src-tauri/tests/fixtures/legacy-provider-contract/config/crlf-unicode.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/config/legacy-flat.toml` |
| `src-tauri/tests/fixtures/legacy-provider-contract/config/official-provider.toml` |
| `src-tauri/tests/fixtures/legacy-provider-contract/config/top-level-only.toml` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/attach-provider-request.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/attach-provider-result.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/create-provider-request-backend.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/create-provider-request.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/operation-plan.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/dto/provider-profile-view.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/manifest.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/provider-store/empty.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/provider-store/future-version.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/provider-store/legacy-wire-openai.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/provider-store/legacy-wire-responses.json` |
| `src-tauri/tests/fixtures/legacy-provider-contract/provider-store/malformed.json` |
| `src-tauri/tests/fixtures/provider-adapter/deepseek-thinking-nonstream.json` |
| `src-tauri/tests/fixtures/provider-adapter/deepseek-thinking-stream.sse` |
| `src-tauri/tests/fixtures/provider-adapter/manifest.json` |
| `src-tauri/tests/fixtures/provider-gateway-g4/manifest.json` |
| `src-tauri/tests/fixtures/provider-gateway-g5/manifest.json` |
| `src-tauri/tests/fixtures/remote-provider-gateway-phase0-approval.json` |

## 5. Explicitly NOT in scope

- Historical full-account Sessions bulk Sync from docs/FINAL-DESIGN.md earlier sections (removed 2026-07-17).
- Reuse of any Rust, Cargo, Tauri, React, Vite or TS/TSX artifact in the new runtime.

## 6. Authoritative source precedence

Runtime code and tests > recent correction docs > historical design drafts. When a test and the implementation disagree, capture current behavior in a fixture before redesigning.
