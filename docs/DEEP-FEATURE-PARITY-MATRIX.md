> **Updated feature-test verification (2026-10-28):** This file is a historical snapshot. For current 239-item statuses, mapped executable tests and CI gate use [`docs/FEATURE-TEST-COVERAGE-REPORT.md`](docs/FEATURE-TEST-COVERAGE-REPORT.md) and [`tests/feature-contracts.csv`](tests/feature-contracts.csv). Prior 127 `DOMAIN_PASS` is no longer valid; current direct domain evidence is 125. macOS/native release is NOT accepted.

# LAM 0.4.4 → LAM 2.0 逐功能验收矩阵

总行数 **239** = 111 个原始 IPC 命令 + 128 个用户可见流程/UI/平台验收点。

**判定口径**：DOMAIN_PASS=隔离纯 Go 业务测试通过（不代表端到端）；NATIVE_UNVERIFIED=源码已连接但真实依赖/macOS未验证；PARTIAL=行为差异；GAP=明确功能未对齐或发布阻断；EXCLUDED=原版已废弃。

**严格限制**：每条命令与 UI 入口并不是最终全覆盖的数学证明；功能场景的跨模块组合还须端到端与实机验收。

## 分状态统计

- DOMAIN_PASS: **127**
- EXCLUDED: **1**
- GAP: **6**
- NATIVE_UNVERIFIED: **82**
- PARTIAL: **23**

## 原始命令逐条追踪（111 项）

|编号|原始入口|新版服务|状态|差异/验证边界|
|---|---|---|---|---|
|CMD-001|health_check|internal/app/app.go|NATIVE_UNVERIFIED|Core.Doctor; full native composition is not compiled.|
|CMD-002|list_accounts|internal/account/account.go; internal/app/app.go|PARTIAL|只读自动发现、main 保护和注册 UI 已增加，旧缓存/发现后全功能操作仍有差异；原 UI 真机未测|
|CMD-003|list_cached_accounts|internal/account/account.go; internal/app/app.go|PARTIAL|只读自动发现、main 保护和注册 UI 已增加，旧缓存/发现后全功能操作仍有差异；原 UI 真机未测|
|CMD-004|list_sessions|internal/session/session.go|DOMAIN_PASS|Paged filtering, stable sort, tail metadata and indexed titles.|
|CMD-005|list_sessions_page|internal/session/session.go|DOMAIN_PASS|Paged filtering, stable sort, tail metadata and indexed titles.|
|CMD-006|query_sessions_page|internal/session/session.go|DOMAIN_PASS|Paged filtering, stable sort, tail metadata and indexed titles.|
|CMD-007|query_deletable_session_paths|internal/session/selection.go; internal/session/archive.go; internal/session/archive_index.go|DOMAIN_PASS|All filtered results; recent 20/7-day protection; transactional recoverable archival and title-index repair.|
|CMD-008|delete_sessions|internal/session/selection.go; internal/session/archive.go; internal/session/archive_index.go|DOMAIN_PASS|All filtered results; recent 20/7-day protection; transactional recoverable archival and title-index repair.|
|CMD-009|get_session_storage_summary|internal/session/selection.go; internal/session/archive.go; internal/session/archive_index.go|DOMAIN_PASS|All filtered results; recent 20/7-day protection; transactional recoverable archival and title-index repair.|
|CMD-010|plan_create_account|internal/account/account.go; internal/desktop/forms.go|DOMAIN_PASS|Create with CAS; optional config template and seven legacy home subdirectories. Bound templates require explicit rebind; no credential copy.|
|CMD-011|execute_create_account|internal/account/account.go; internal/desktop/forms.go|DOMAIN_PASS|Create with CAS; optional config template and seven legacy home subdirectories. Bound templates require explicit rebind; no credential copy.|
|CMD-012|plan_rename_account|internal/account/account.go; internal/desktop/forms.go|GAP|Display rename keeps stable ID/home/wrapper. Old rename physically changes home and wrapper names; not identical.|
|CMD-013|execute_rename_account|internal/account/account.go; internal/desktop/forms.go|GAP|Display rename keeps stable ID/home/wrapper. Old rename physically changes home and wrapper names; not identical.|
|CMD-014|delete_account|internal/account/delete.go; internal/account/account.go|DOMAIN_PASS|Managed deletion is recoverable; external data remains untouched. Note/date/proxy persisted.|
|CMD-015|update_account_note|internal/account/delete.go; internal/account/account.go|DOMAIN_PASS|Managed deletion is recoverable; external data remains untouched. Note/date/proxy persisted.|
|CMD-016|update_account_proxy|internal/account/delete.go; internal/account/account.go|DOMAIN_PASS|Managed deletion is recoverable; external data remains untouched. Note/date/proxy persisted.|
|CMD-017|plan_create_relay|internal/account/relay.go; internal/desktop/view.go|DOMAIN_PASS|Auxiliary relay profile with runtime/source provenance, preview/fingerprint, no authentication copy; not bulk Sync.|
|CMD-018|execute_create_relay|internal/account/relay.go; internal/desktop/view.go|DOMAIN_PASS|Auxiliary relay profile with runtime/source provenance, preview/fingerprint, no authentication copy; not bulk Sync.|
|CMD-019|build_resume_command|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-020|build_login_command|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-021|relay_resume_session|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-022|open_terminal_with_resume|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-023|open_terminal_with_command|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-024|open_terminal_for_login|internal/runtime/runtime.go; internal/runtime/terminals.go; internal/handoff/branches.go|NATIVE_UNVERIFIED|Typed launcher/argv replaces generic shell IPC. Source command construction tested; actual terminal apps unrun.|
|CMD-025|list_terminal_targets|internal/runtime/terminals.go; internal/settings/settings.go|NATIVE_UNVERIFIED|Terminal/Ghostty/cmux/Codex.app discovery; selection stored; actual macOS launch unrun.|
|CMD-026|get_selected_terminal_target|internal/runtime/terminals.go; internal/settings/settings.go|NATIVE_UNVERIFIED|Terminal/Ghostty/cmux/Codex.app discovery; selection stored; actual macOS launch unrun.|
|CMD-027|set_selected_terminal_target|internal/runtime/terminals.go; internal/settings/settings.go|NATIVE_UNVERIFIED|Terminal/Ghostty/cmux/Codex.app discovery; selection stored; actual macOS launch unrun.|
|CMD-028|get_profile_quota|internal/quota/quota.go; internal/app/app.go|DOMAIN_PASS|Shared snapshots; stale/unknown state; reset operation IDs and explicit confirmation. Live account access unrun.|
|CMD-029|refresh_all_quotas|internal/quota/quota.go; internal/app/app.go|DOMAIN_PASS|Shared snapshots; stale/unknown state; reset operation IDs and explicit confirmation. Live account access unrun.|
|CMD-030|reset_profile_quota|internal/quota/quota.go; internal/app/app.go|DOMAIN_PASS|Shared snapshots; stale/unknown state; reset operation IDs and explicit confirmation. Live account access unrun.|
|CMD-031|list_cached_quotas|internal/quota/quota.go; internal/app/app.go|DOMAIN_PASS|Shared snapshots; stale/unknown state; reset operation IDs and explicit confirmation. Live account access unrun.|
|CMD-032|refresh_usage_index|internal/usage/indexer.go; internal/app/app.go|DOMAIN_PASS|Incremental index/locks, archive inclusion, attribution; Core serializes refresh instead of exposing old try-refresh DTO.|
|CMD-033|try_refresh_usage_index|internal/usage/indexer.go; internal/app/app.go|DOMAIN_PASS|Incremental index/locks, archive inclusion, attribution; Core serializes refresh instead of exposing old try-refresh DTO.|
|CMD-034|refresh_account_usage_snapshot|internal/quota/quota.go; internal/usage/account.go; internal/desktop/account_usage.go|NATIVE_UNVERIFIED|Account data remains separate from local data; bounded-date comparison only; live RPC unrun.|
|CMD-035|get_usage_summary|internal/app/app.go; internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Service/view queries replace old DTOs. Exact original scope enumeration and presentation are not certified.|
|CMD-036|get_usage_dashboard|internal/app/app.go; internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Service/view queries replace old DTOs. Exact original scope enumeration and presentation are not certified.|
|CMD-037|get_usage_dashboard_response|internal/app/app.go; internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Service/view queries replace old DTOs. Exact original scope enumeration and presentation are not certified.|
|CMD-038|get_usage_scopes|internal/app/app.go; internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Service/view queries replace old DTOs. Exact original scope enumeration and presentation are not certified.|
|CMD-039|get_usage_overview|internal/app/app.go; internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Service/view queries replace old DTOs. Exact original scope enumeration and presentation are not certified.|
|CMD-040|get_usage_activity|internal/usage/activity.go; internal/usage/insights.go; internal/desktop/activity.go|DOMAIN_PASS|Daily/weekly activity, streaks, heatmap source and full-filter Fast/effort/skill metrics; real MyGo rendering unrun.|
|CMD-041|get_usage_insights|internal/usage/activity.go; internal/usage/insights.go; internal/desktop/activity.go|DOMAIN_PASS|Daily/weekly activity, streaks, heatmap source and full-filter Fast/effort/skill metrics; real MyGo rendering unrun.|
|CMD-042|get_usage_calls|internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Separate Calls/Threads pagination; real plugin execution unverified.|
|CMD-043|get_usage_threads|internal/integrations/mygosqlite/sqlite.go; internal/desktop/view.go|NATIVE_UNVERIFIED|Separate Calls/Threads pagination; real plugin execution unverified.|
|CMD-044|get_usage_diagnostics|internal/usage/diagnostics.go; internal/integrations/mygosqlite/sqlite.go|PARTIAL|已分辨常见解析诊断并持久化，但旧版全部细粒度事件分类仍需对账|
|CMD-045|get_usage_rate_card|internal/usage/legacy_prices.go; internal/settings/settings.go|DOMAIN_PASS|Versioned historical original rate card, overrides, cached-input and reasoning subset rules; not a current-price assertion.|
|CMD-046|reset_usage_index|internal/integrations/mygosqlite/sqlite.go; internal/app/app.go|NATIVE_UNVERIFIED|Reset preserves imported DB-only rows; compact uses actual plugin source.|
|CMD-047|compact_usage_db|internal/integrations/mygosqlite/sqlite.go; internal/app/app.go|NATIVE_UNVERIFIED|Reset preserves imported DB-only rows; compact uses actual plugin source.|
|CMD-048|get_call_raw_contents|internal/usage/activity.go; internal/desktop/view.go|DOMAIN_PASS|Bounded raw history preview, path containment and source records.|
|CMD-049|sync_tray_quota|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-050|show_main_window|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-051|show_usage_stats|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-052|take_pending_route|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-053|set_quota_popover_opacity|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-054|hide_quota_popover|internal/desktop/run.go; internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo windows/router/opacity/shared quota snapshots replace Tauri events and pending-route IPC.|
|CMD-055|list_providers|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-056|create_provider|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-057|update_provider|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-058|delete_provider|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-059|test_provider|internal/provider/health.go; internal/desktop/view.go|DOMAIN_PASS|Direct Responses /models health and persisted redacted observation; does not claim inference/stream compatibility.|
|CMD-060|plan_attach_provider_to_profile|internal/provider/binding.go; internal/provider/projection.go; internal/provider/reproject.go|DOMAIN_PASS|Plan/fingerprint/CAS/config backup/catalog/compaction settings; live Codex config RPC unrun.|
|CMD-061|attach_provider_to_profile|internal/provider/binding.go; internal/provider/projection.go; internal/provider/reproject.go|DOMAIN_PASS|Plan/fingerprint/CAS/config backup/catalog/compaction settings; live Codex config RPC unrun.|
|CMD-062|execute_attach_provider_to_profile|internal/provider/binding.go; internal/provider/projection.go; internal/provider/reproject.go|DOMAIN_PASS|Plan/fingerprint/CAS/config backup/catalog/compaction settings; live Codex config RPC unrun.|
|CMD-063|list_providers_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-064|discover_provider_models_v2|internal/provider/provider.go; internal/desktop/view.go|DOMAIN_PASS|Discovery previews catalog; explicit Save applies model changes and updates bindings.|
|CMD-065|test_provider_upstream_v2|internal/provider/health.go; internal/desktop/view.go|DOMAIN_PASS|Direct Responses /models health and persisted redacted observation; does not claim inference/stream compatibility.|
|CMD-066|refresh_provider_models_v2|internal/provider/provider.go; internal/desktop/view.go|DOMAIN_PASS|Discovery previews catalog; explicit Save applies model changes and updates bindings.|
|CMD-067|create_provider_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-068|approve_provider_auth_command_v2|internal/provider/provider.go; internal/desktop/settings.go|DOMAIN_PASS|Executable digest approval and state listing; arbitrary imported shell execution stays forbidden.|
|CMD-069|list_provider_auth_command_approvals_v2|internal/provider/provider.go; internal/desktop/settings.go|DOMAIN_PASS|Executable digest approval and state listing; arbitrary imported shell execution stays forbidden.|
|CMD-070|create_provider_with_keychain_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-071|create_provider_legacy_compat_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-072|update_provider_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-073|delete_provider_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-074|rotate_provider_credential_v2|internal/provider/provider.go; internal/migration/migration.go|DOMAIN_PASS|Unified V2 Go domain; credentials remain references, legacy formats imported explicitly.|
|CMD-075|list_profile_provider_bindings_v2|internal/model/model.go; internal/app/app.go; internal/desktop/view.go|DOMAIN_PASS|Current store bindings plus imported pending-reconnect records.|
|CMD-076|plan_attach_provider_v2|internal/provider/binding.go; internal/provider/projection.go; internal/provider/reproject.go|DOMAIN_PASS|Plan/fingerprint/CAS/config backup/catalog/compaction settings; live Codex config RPC unrun.|
|CMD-077|execute_attach_provider_v2|internal/provider/binding.go; internal/provider/projection.go; internal/provider/reproject.go|DOMAIN_PASS|Plan/fingerprint/CAS/config backup/catalog/compaction settings; live Codex config RPC unrun.|
|CMD-078|plan_detach_provider_v2|internal/provider/detach.go; internal/provider/binding.go; internal/desktop/forms.go|DOMAIN_PASS|Durable detach preview with revision/generation/config hash, confirmation and transactionally restored config.|
|CMD-079|execute_detach_provider_v2|internal/provider/detach.go; internal/provider/binding.go; internal/desktop/forms.go|DOMAIN_PASS|Durable detach preview with revision/generation/config hash, confirmation and transactionally restored config.|
|CMD-080|plan_api_account_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-081|execute_api_account_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-082|plan_api_account_model_switch_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-083|execute_api_account_model_switch_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-084|get_api_account_connection_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-085|update_api_account_connection_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-086|delete_api_account_v2|internal/provider/api_account.go; internal/desktop/forms.go; internal/desktop/view.go|DOMAIN_PASS|Stable creation plans, partial-create rollback, model switch, connection edit, delete with binding restoration on failure.|
|CMD-087|plan_gateway_port_migration_v2|internal/provider/port.go; internal/gateway/supervisor.go; internal/desktop/settings.go|DOMAIN_PASS|Port preview/commit, revision conflicts, authenticated own-process stop; no foreign-process kill.|
|CMD-088|execute_gateway_port_migration_v2|internal/provider/port.go; internal/gateway/supervisor.go; internal/desktop/settings.go|DOMAIN_PASS|Port preview/commit, revision conflicts, authenticated own-process stop; no foreign-process kill.|
|CMD-089|get_antigravity_quota|internal/quota/antigravity.go; internal/quota/antigravity_discovery.go|DOMAIN_PASS|Explicit configured port plus automatic current-user process CSRF; no arbitrary port scan; live Antigravity unrun.|
|CMD-090|upload_pat_credentials|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-091|get_pat_metadata|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-092|check_profile_token_expiration|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-093|add_pat_account|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-094|add_session_profile_account|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-095|switch_to_pat_account|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-096|export_cpa_credentials|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-097|update_pat_session_auth|internal/account/account.go; internal/account/secondary.go; internal/account/pat.go; internal/account/runtime_profile.go|DOMAIN_PASS|Preserve native auth fields, independent PAT homes, runtime vs quota auth, two-file switching/restore, CPA/session/PAT imports.|
|CMD-098|get_auth_mode|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-099|set_auth_mode|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-100|get_gateway_first_response_timeout_seconds|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-101|set_gateway_first_response_timeout_seconds|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-102|get_gateway_request_timeout_seconds|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-103|set_gateway_request_timeout_seconds|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-104|get_codex_launch_permission_preset|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-105|set_codex_launch_permission_preset|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-106|get_antigravity_port|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-107|set_antigravity_port|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-108|get_hide_dock_icon|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-109|set_hide_dock_icon|internal/settings/settings.go; internal/desktop/settings.go; internal/runtime/runtime.go|DOMAIN_PASS|Explicit settings/native numeric controls; restored first-response 10-600 and request 300-3600 second limits; native effects unrun.|
|CMD-110|restart_chatgpt|internal/runtime/chatgpt.go; internal/desktop/settings.go|NATIVE_UNVERIFIED|Scoped own-user ChatGPT process selection and window-bound parsing tested; real restart unrun.|
|CMD-111|quit_app|internal/desktop/run.go; internal/gateway/supervisor.go|NATIVE_UNVERIFIED|Desktop exit does not kill an independently owned active Go gateway.|

## 页面、UI 状态与系统流程（逐项）

|编号|区域|原版功能|原版源码|Go 对照|状态|说明|
|---|---|---|---|---|---|---|
|FLOW-001|导航|五页面导航与当前页|src/App.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|MyGo Router, native tester未运行|
|FLOW-002|导航|主窗口关闭隐藏与显式退出|src/App.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|macOS 生命周期未实测|
|FLOW-003|导航|主题深浅与系统主题|src/components/theme-toggle.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|系统主题及托盘同步需真机|
|FLOW-004|导航|设置页分组切换|src/routes/views.tsx|internal/desktop/settings.go|PARTIAL|原侧栏四类设置改为单页滚动|
|FLOW-005|导航|键盘导航/无障碍与 IME|src/components/shell.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|依赖MyGo;真实输入法未运行|
|FLOW-006|导航|多语言界面文本|src/App.tsx|internal/desktop/messages/messages.go|PARTIAL|部分新增控制仍为英文|
|FLOW-007|账号|启动时扫描主账号和子账号|src/routes/views.tsx|internal/account/account.go; internal/app/app.go|DOMAIN_PASS|已按 .codex* 信号文件只读自动展示|
|FLOW-008|账号|空 .codex 前缀目录不算账号|src/routes/views.tsx|internal/account/discovery_parity_test.go|DOMAIN_PASS|避免假账号|
|FLOW-009|账号|发现账号的注册按钮可点击|src/routes/views.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|修复此前按钮恒禁用;新增Tester待运行|
|FLOW-010|账号|main默认账号保护|src/routes/views.tsx|internal/account/account.go|DOMAIN_PASS|注册共享 ~/.codex 为main;禁止删除|
|FLOW-011|账号|旧版账号列表缓存行为|src/App.tsx|internal/account/account.go|PARTIAL|只读发现不等价原 cached_accounts 刷新契约|
|FLOW-012|账号|账号新建/绑定配置模板|src/routes/views.tsx|internal/account/account.go|DOMAIN_PASS|不复制凭据;隔离目录|
|FLOW-013|账号|重命名账号磁盘目录和 wrapper|src/routes/views.tsx|internal/account/account.go|GAP|目前仅编辑显示名;未执行旧目录迁移;高风险阻断|
|FLOW-014|账号|修改账号备注/续费日期|src/routes/views.tsx|internal/desktop/forms.go|NATIVE_UNVERIFIED|表单接入;真机未测|
|FLOW-015|账号|账户独立代理参数|src/components/account-proxy.tsx|internal/account/account.go; internal/desktop/forms.go|DOMAIN_PASS|后端验证;UI实机未测|
|FLOW-016|账号|账号删除和受管归档|src/routes/views.tsx|internal/account/delete.go|DOMAIN_PASS|保留用户外部目录|
|FLOW-017|账号|Profile 与 PAT 模式切换|src/routes/views.tsx|internal/account/pat.go|DOMAIN_PASS|状态/凭据独立;外部重启未测|
|FLOW-018|账号|PAT 共享认证双文件恢复|src/routes/views.tsx|internal/account/pat.go|DOMAIN_PASS|auth.json/auth-f.json 一致恢复|
|FLOW-019|账号|登录方式 ChatGPT/API key/Device code|src/routes/views.tsx|internal/desktop/forms.go|NATIVE_UNVERIFIED|需真实 Codex SDK 登录|
|FLOW-020|账号|粘贴 Session 与 PAT|src/routes/views.tsx|internal/desktop/forms.go|DOMAIN_PASS|auth归一化纯测试;GUI未执行|
|FLOW-021|账号|登录凭据私密导入导出|src/routes/views.tsx|internal/desktop/actions.go|NATIVE_UNVERIFIED|Keychain/文件对话框真机未测|
|FLOW-022|账号|创建 Relay 辅助账号|src/routes/views.tsx|internal/account/relay.go|DOMAIN_PASS|预览与受限创建实现|
|FLOW-023|账号|Token 到期倒计时|src/routes/views.tsx|internal/desktop/view.go|PARTIAL|有原始 expiry/详情;倒计时与徽章细节未等价|
|FLOW-024|账号|账号套餐徽标和卡片密度|src/components/plan-type-badge.tsx|internal/desktop/view.go|PARTIAL|已显示PlanType文字;可视化信息密度不同|
|FLOW-025|账号|账号卡片快捷操作菜单|src/routes/views.tsx|internal/desktop/view.go|PARTIAL|按钮集合集中顶部,原卡片内动作未全部对齐|
|FLOW-026|账号|账号当前配额状态|src/routes/views.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|后端缓存/状态测试通过,GUI未运行|
|FLOW-027|账号|复制/打开 Codex Home|src/routes/views.tsx|internal/desktop/actions.go|PARTIAL|只有应用数据目录打开/账号 Home 快捷不足|
|FLOW-028|账号|API Account 与普通账号区分|src/routes/views.tsx|internal/desktop/view.go|PARTIAL|Provider绑定可见,身份分组/徽标较简化|
|FLOW-029|会话|列表跨账号聚合|src/routes/views.tsx|internal/session/session.go|DOMAIN_PASS|活动/归档 JSONL 读取|
|FLOW-030|会话|搜索会话标题/工作目录|src/routes/views.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|索引和 UI 文本输入已写入|
|FLOW-031|会话|按修改时间/尺寸/标题排序|src/routes/views.tsx|internal/session/session.go|DOMAIN_PASS|纯逻辑测试|
|FLOW-032|会话|前后翻页/总计|src/routes/views.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|原生Table未运行|
|FLOW-033|会话|查看 Session 详情|src/routes/views.tsx|internal/session/session.go|DOMAIN_PASS|大小有界的详情读取|
|FLOW-034|会话|外部终端 Resume|src/routes/views.tsx|internal/runtime/terminals.go|NATIVE_UNVERIFIED|Terminal/Ghostty/cmux macOS待验|
|FLOW-035|会话|复制 Resume 命令|src/routes/views.tsx|internal/desktop/actions.go|NATIVE_UNVERIFIED|剪贴板系统需真机|
|FLOW-036|会话|单会话 Handoff 预览/冲突|src/routes/views.tsx|internal/handoff/handoff.go|DOMAIN_PASS|阻断与策略纯测试|
|FLOW-037|会话|最近会话一键接力|src/routes/views.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|托盘新增 Relay latest 入口,需手工验收|
|FLOW-038|会话|全过滤结果选择可删项|src/routes/views.tsx|internal/session/selection.go|DOMAIN_PASS|不是只选当前页|
|FLOW-039|会话|最近20条+7日保护|src/routes/views.tsx|internal/session/selection.go|DOMAIN_PASS|当前默认清理保护|
|FLOW-040|会话|归档后恢复与索引一致性|src/routes/views.tsx|internal/session/archive.go|DOMAIN_PASS|业务恢复流程测试|
|FLOW-041|会话|删除计划确认和错误反馈|src/routes/views.tsx|internal/desktop/actions.go|NATIVE_UNVERIFIED|MyGo Modal完整验证待运行|
|FLOW-042|会话|大 JSONL 尾部标题索引|src/routes/views.tsx|internal/session/session.go|DOMAIN_PASS|截断场景测试|
|FLOW-043|会话|Session表格选中项稳定|src/routes/views.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|使用MyGo ListState.Key而非手写选择器|
|FLOW-044|会话|跨账号会话分页重排稳定|src/routes/views.tsx|internal/session/session.go|DOMAIN_PASS|游标/offset稳定排序|
|FLOW-045|Provider|创建/编辑/删除 Provider|src/components/provider-center.tsx|internal/provider/provider.go|DOMAIN_PASS|事务/验证纯测试|
|FLOW-046|Provider|模型列表及默认模型编辑|src/components/provider-model-picker.tsx|internal/desktop/forms.go|NATIVE_UNVERIFIED|表单和高级JSON;真机未测|
|FLOW-047|Provider|模型发现/连接测试|src/components/provider-center.tsx|internal/provider/health.go|DOMAIN_PASS|HTTP fake;外部Provider未测|
|FLOW-048|Provider|按模型能力声明支持工具|src/components/provider-center.tsx|internal/model/model.go|DOMAIN_PASS|保留unknown/partial语义|
|FLOW-049|Provider|Provider 密钥来源审批|src/components/provider-center.tsx|internal/provider/provider.go|DOMAIN_PASS|明文不得持久化;Keychain运行未测|
|FLOW-050|Provider|Keychain 旋转密钥|src/components/provider-center.tsx|internal/provider/provider.go|NATIVE_UNVERIFIED|真实Keychain访问/签名未测|
|FLOW-051|Provider|原生 Responses 直连|src/components/provider-center.tsx|internal/provider/projection.go|NATIVE_UNVERIFIED|Codex CLI实际握手未测|
|FLOW-052|Provider|Chat Completions Gateway 绑定|src/components/provider-center.tsx|internal/provider/provider.go|NATIVE_UNVERIFIED|转换器端到端未测|
|FLOW-053|Provider|Attach/Detach 预览及回滚|src/components/provider-binding-dialog.tsx|internal/provider/detach.go|DOMAIN_PASS|事务纯测试通过;真实Config API未测|
|FLOW-054|Provider|保存绑定时同步 config.toml|src/components/provider-center.tsx|internal/provider/reproject.go|DOMAIN_PASS|备份/修订;SDK调用未实测|
|FLOW-055|Provider|API Account 独立CRUD/模型切换|src/components/api-account-flow.tsx|internal/provider/api_account.go|DOMAIN_PASS|服务测试;原生页面待跑|
|FLOW-056|Provider|Provider advanced字段直接编辑|src/components/provider-center.tsx|internal/desktop/forms.go|PARTIAL|已提供高级JSON编辑,原版表单细项有压缩|
|FLOW-057|Provider|网关状态和端口迁移|src/components/provider-center.tsx|internal/provider/port.go|DOMAIN_PASS|纯逻辑验证;端口冲突真机待验|
|FLOW-058|Provider|上游错误与重试提示|src/components/provider-center.tsx|internal/gateway/errors.go|DOMAIN_PASS|网关错误映射;GUI提示简化|
|FLOW-059|Provider|自定义模型目录与压缩阈值|src/components/provider-center.tsx|internal/provider/catalog.go|DOMAIN_PASS|业务契约测试;真实CLI接入未测|
|FLOW-060|Provider|模型推理努力档位|src/components/provider-center.tsx|internal/provider/reproject.go|DOMAIN_PASS|参数保存测试;模型选择UI简化|
|FLOW-061|Provider|绑定预览展示每项副作用|src/components/plan-view.tsx|internal/desktop/forms.go|PARTIAL|有确认计划;非原图文段落显示|
|FLOW-062|Usage|全部时间默认;仅活跃历史|src/App.tsx|internal/desktop/state.go|NATIVE_UNVERIFIED|已改all+active;源代码审查,Native未跑|
|FLOW-063|Usage|Today/Week/7days/Month/Custom预设|src/routes/usage.tsx|internal/usage/date_window.go|DOMAIN_PASS|UTC半开区间单测;UI未跑|
|FLOW-064|Usage|自定义起止日期选择|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|目前文本输入日期,非原生日历选择器|
|FLOW-065|Usage|日期筛选重置|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|新增Reset usage filters|
|FLOW-066|Usage|账户与Local数据来源分离|src/App.tsx|internal/usage/account.go|DOMAIN_PASS|明确不同统计口径|
|FLOW-067|Usage|多Workspace Scope导航|src/routes/usage.tsx|internal/desktop/view.go|PARTIAL|只有Profile和Workspace输入,无原分段Workspace tabs|
|FLOW-068|Usage|搜索Threads/CWD/Model|src/routes/usage.tsx|internal/usage/indexer.go|DOMAIN_PASS|新增参数化全文条件|
|FLOW-069|Usage|Effort和Pricing Confidence筛选|src/routes/usage.tsx|internal/usage/indexer.go|DOMAIN_PASS|条件参数化,部分列缺省仍待真实DB测试|
|FLOW-070|Usage|Sort主要列+稳定Tie-break|src/routes/usage.tsx|internal/usage/indexer.go|DOMAIN_PASS|安全列白名单;未包含duration/gap/attention三列|
|FLOW-071|Usage|按Duration/Gap/Attention排序|src/routes/usage.tsx|internal/usage/indexer.go|DOMAIN_PASS|增量checkpoint保留threadCallIndex/previousRecordId,SQL白名单与旧attention公式,真实SQLite待验证|
|FLOW-072|Usage|Load limit 5000/All|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|新增5000/10000/20000/All控件,detail 500条批量预算,UI未运行|
|FLOW-073|Usage|Calls分页/表格/详情|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|使用MyGo Table;未跑Tester|
|FLOW-074|Usage|Raw Call上下文查看|src/routes/usage.tsx|internal/usage/activity.go|DOMAIN_PASS|按源路径白名单读取|
|FLOW-075|Usage|Threads分页/点选关联 Calls|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|选中线程跳过滤器|
|FLOW-076|Usage|Usage摘要Tokens/Cache/Cost|src/routes/usage.tsx|internal/usage/counters.go|DOMAIN_PASS|缓存token不重复计数|
|FLOW-077|Usage|价格覆盖率和未知模型|src/lib/usage-pricing.ts|internal/usage/insights.go|DOMAIN_PASS|缺价明确unknown|
|FLOW-078|Usage|Fast mode统计|src/lib/usage-dashboard-analysis.ts|internal/usage/insights.go|DOMAIN_PASS|纯事件汇总|
|FLOW-079|Usage|Reasoning effort/技能统计|src/lib/usage-dashboard-analysis.ts|internal/usage/insights.go|DOMAIN_PASS|聚合测试|
|FLOW-080|Usage|Daily/Weekly/Cumulative活动视图|src/routes/usage.tsx|internal/usage/heatmap.go|DOMAIN_PASS|按原版每一日显示周总,测试先行|
|FLOW-081|Usage|Calls/Tokens活动指标切换|src/routes/usage.tsx|internal/desktop/activity.go|NATIVE_UNVERIFIED|Native按钮绑定待运行|
|FLOW-082|Usage|默认365天活动范围|src/routes/usage.tsx|internal/usage/heatmap.go|DOMAIN_PASS|原版一年日历,纯测试|
|FLOW-083|Usage|365天聚合不阻塞UI|src/routes/usage.tsx|internal/integrations/mygosqlite/sqlite.go|NATIVE_UNVERIFIED|单次UTC SQL group,真实SQLite待跑|
|FLOW-084|Usage|活动天数/最长连续天|src/routes/usage.tsx|internal/usage/activity.go|DOMAIN_PASS|统计测试|
|FLOW-085|Usage|按日期点击反查Calls|src/routes/usage.tsx|internal/desktop/activity.go|NATIVE_UNVERIFIED|Native事件未运行|
|FLOW-086|Usage|解析诊断详细原因|src/lib/usage-diagnostics.ts|internal/usage/parser_diagnostics.go|PARTIAL|部分原因已分离,尚缺原全部边界分类|
|FLOW-087|Usage|独立分区加载/刷新|src/routes/usage.tsx|internal/desktop/reads.go|GAP|目前页面整页异步读取,不是各section独立加载|
|FLOW-088|Usage|Loaded/Partial/Error状态独立|src/routes/usage.tsx|internal/desktop/state.go|PARTIAL|部分错误保留,整页读取仍可被单个失败阻断|
|FLOW-089|Usage|Status Chips/隐私与定价来源|src/routes/usage.tsx|internal/desktop/view.go|PARTIAL|标记较少;细节布局未等价|
|FLOW-090|Usage|本地Account用量Delta对比|src/routes/usage.tsx|internal/usage/account.go|DOMAIN_PASS|仅相同UTC边界可比|
|FLOW-091|Usage|Activity Tooltip/Month Labels|src/routes/usage.tsx|internal/desktop/activity.go|PARTIAL|Tooltip已做,月份标签密度未复刻|
|FLOW-092|Usage|日期峰值/工具使用洞察|src/routes/usage.tsx|internal/desktop/view.go|NATIVE_UNVERIFIED|有source/UI代码;可读性待实机|
|FLOW-093|Tray|Codex/Antigravity分组切换|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|已新增原生Tabs,未实机|
|FLOW-094|Tray|单账号配额刷新|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|已新增每Profile刷新;运行未测|
|FLOW-095|Tray|Relay Latest（目标账号）|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|先打开MyGo主窗预览，不直接不可逆写入|
|FLOW-096|Tray|PAT切换当前身份|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|强制确认;ChatGPT重启自动步骤需实机|
|FLOW-097|Tray|当前账号活跃状态标记|src/components/tray-quota-panel.tsx|internal/desktop/run.go|PARTIAL|缺原色点活跃状态标识|
|FLOW-098|Tray|5小时/周/月进度环|src/components/tray-quota-panel.tsx|internal/desktop/run.go|PARTIAL|原生Progress条可用;未复刻双环视觉|
|FLOW-099|Tray|重置次数及到期展示|src/components/tray-quota-panel.tsx|internal/desktop/view.go|PARTIAL|availableCount可见,到期点及overflow未对齐|
|FLOW-100|Tray|Antigravity模型组Quota|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|复用u.quotaView分组;数据源需验证|
|FLOW-101|Tray|动态浮窗高度与布局密度|src/components/tray-quota-panel.tsx|internal/desktop/run.go|PARTIAL|固定420x540,无原动态尺寸适配|
|FLOW-102|Tray|菜单栏Dock外观/失焦隐藏|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|MyGo原生Window生命周期待macOS|
|FLOW-103|Tray|托盘每两分钟刷新双Provider|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|Antigravity每120秒、Codex按设置周期自动刷新,真实GUI未测|
|FLOW-104|Tray|托盘明暗主题切换|src/components/tray-quota-panel.tsx|internal/desktop/settings.go|PARTIAL|全局主题设置可用;托盘内独立toggle未复刻|
|FLOW-105|Tray|快捷打开主窗口/关闭面板|src/components/tray-quota-panel.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|Window OnBlur/Show/Hide未验|
|FLOW-106|设置|账号展示模式(Profile/PAT/both)|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|设置字段/控件接入|
|FLOW-107|设置|外部终端选项+路径|src/routes/views.tsx|internal/runtime/terminals.go|NATIVE_UNVERIFIED|macOS检测未跑|
|FLOW-108|设置|Gateway首响应与全请求超时|src/routes/views.tsx|internal/settings/settings.go|DOMAIN_PASS|10..600/300..3600|
|FLOW-109|设置|网关端口迁移/健康检查|src/routes/views.tsx|internal/provider/port.go|DOMAIN_PASS|服务纯测;桌面OS未跑|
|FLOW-110|设置|Dock/Tray可见性|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|macOS行为未测|
|FLOW-111|设置|Tray透明度|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|字段写入与浮窗应用存在|
|FLOW-112|设置|Session Handoff冲突默认策略|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|策略选择UI和服务定义|
|FLOW-113|设置|重置Usage缓存和重新索引|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|长任务/取消需真实DB|
|FLOW-114|设置|价格卡编辑与启用|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|价格表Go领域测试通过|
|FLOW-115|设置|旧数据迁移和回滚|src/routes/views.tsx|internal/migration/metadata.go|DOMAIN_PASS|业务迁移有测试,SQLite/Keychain未实测|
|FLOW-116|设置|自动更新与签名验收|src/routes/views.tsx|internal/desktop/run.go|NATIVE_UNVERIFIED|MyGoNativeUpdater配置已编写,实际发布未验|
|FLOW-117|设置|高级授权/密钥命令审批|src/routes/views.tsx|internal/desktop/settings.go|NATIVE_UNVERIFIED|具体macOS执行身份需验证|
|FLOW-118|设置|Doctor与故障诊断|src/routes/views.tsx|internal/app/app.go|NATIVE_UNVERIFIED|源码存在,真实SDK未测|
|FLOW-119|设置|重启ChatGPT|src/routes/views.tsx|internal/runtime/chatgpt.go|NATIVE_UNVERIFIED|仅macOS原生可验|
|FLOW-120|平台|完全移除Rust/React/Tauri|src/App.tsx|tools/check-source.sh|DOMAIN_PASS|源文件禁止项扫描;不包含旧引用二进制|
|FLOW-121|平台|Go与MyGo真实全工程编译|src/App.tsx|go.mod|GAP|当前Go1.23与网络阻断;未验证工具链编译|
|FLOW-122|平台|MyGo Native UI Tester|src/App.handoff.test.tsx|internal/desktop/desktop_test.go|NATIVE_UNVERIFIED|已编写测试,未运行|
|FLOW-123|平台|MyGo SQLite/Secrets|src/App.tsx|internal/integrations/mygosqlite/sqlite.go|NATIVE_UNVERIFIED|Go依赖未解析|
|FLOW-124|平台|Codex真实Login/Quota SDK|src/App.tsx|internal/integrations/codexsdk/codexsdk.go|NATIVE_UNVERIFIED|固定SDK/CLI pair尚未真机|
|FLOW-125|平台|真实GatewayTranslator Goldens|src/App.tsx|internal/integrations/translator/translator.go|NATIVE_UNVERIFIED|纯Gateway测试使用fake converter|
|FLOW-126|平台|macOS ARM签名、公证、DMG|src/App.tsx|mygo.json|GAP|没有成功原生构建或真实签名安装|
|FLOW-127|平台|完整旧库/WAL迁移现场验证|src/App.tsx|internal/migration/usage.go|NATIVE_UNVERIFIED|隔离迁移代码已存在,真实WAL快照测试待运行|
|FLOW-128|平台|禁用的整账号批量Sync不复活|docs/FINAL-DESIGN.md|internal/handoff/handoff.go|EXCLUDED|原版2026-07-17已经废弃,不算遗漏|

## 未解决的硬性阻断

1. 原版账号重命名包括目录与 wrapper 的实际迁移；当前 Go 仅变更显示名。为避免不可恢复的真实账号数据损坏，尚未实现跨 SQLite、快照、Config、Wrapper 的原子重定位。
2. 原版 Usage 的排序与详细加载上限本轮已实现；多 Workspace Scope 分段、独立区块加载仍有差异。
3. 托盘仍存在视觉信息密度、独立主题切换、两来源定时刷新、配额环及 Reset Credit 细节差异。
4. 旧版诊断有比当前分类器更细的 skip/reset/error 原因，仍需和真实 SQLite 诊断数据逐项对账。
5. 全工程真实 MyGo/Codex SDK/Translator/SQLite 编译、macOS Keychain/Tray/输入法、DMG 签名和迁移回滚未验收。

## 实施优先级建议

1. **阻断发布**：完整依赖解析和原生构建 → 真实协议测试 → macOS 账号/凭据/数据库迁移（全部是已有源码必须通过的验收）。
2. **高优先级产品差异**：物理账号重命名的事务设计（绝不能直接裸 os.Rename）、Usage 缺失的筛选排序和独立分区加载。
3. **可见体验**：Tray 模式细节、Reset Credit 到期和主题、Usage Scope、设置项布局。

## 新版已直接修复（本轮）

- 开机可见真实 Codex 家目录，空目录不算账号；默认 `main` 保护规则恢复。
- “注册发现账号”的原生按钮取消错误禁用；发现后的写操作仍要求显式注册。
- Usage 新增搜索、Effort、Pricing Confidence、含 duration/gap/attention 的安全排序白名单、Reset Filters、原版时间预设、Detail Load Limit 与默认仅活跃历史。
- 365 天 Activity、Daily/Weekly/Cumulative 与 Calls/Tokens 的数据语义恢复；UTC 日汇总变成一次 SQLite GROUP BY（真实插件未测）。
- Usage 解析诊断改按原因计数并保留增量状态，仍不声称全原因等价。
- MyGo Table 使用稳定 `ListState.Key`，避免数据排序/更新后选择行漂移。
- Tray 新增 Codex/Antigravity 切换、双来源定时刷新、单账号刷新、Relay Latest 和 PAT Switch 路径；Reset Credit 到期及Token有效期提示，均需真实 MyGo 验证。
