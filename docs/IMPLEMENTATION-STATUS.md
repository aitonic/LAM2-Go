> **2026-10-28 updated audit**: See `DEEP-FEATURE-PARITY-MATRIX.md`, `DEEP-AUDIT-REPORT.md` and `DEEP-AUDIT-TEST-REPORT.md` for the current 239-item traceability matrix, code fixes, 171 passing domain tests and remaining native build blockers. The older review below is retained as historical context and its status counts are superseded.

# Implementation status after parity review

The original architecture/specification files remain normative. This file records evidence, not a changed definition of done.

| Phase | Changes in this review | Actual verification boundary |
|---|---|---|
| P0 | Original command inventory and source trace; pinned real dependencies and forbidden-language audit | Core suite/source scan pass; full dependency/toolchain resolution blocked |
| P1 | Wired native forms/actions, account usage, activity heatmap, thread pages, relay/template and connection actions | Native source/UITester written; real build/render unrun |
| P2 | Raw legacy table/column preservation, staged SQLite import, metadata/notes/proxy/expiry/relay/pending bindings | Core fake-reader tests pass; literal SQL checks pass; real MyGo/WAL unrun |
| P3 | Separate PAT homes, mode-aware launch, runtime/secondary quota credentials, native auth preservation, configuration template and auxiliary relay | Account/runtime tests pass; actual Keychain/SDK/live login unrun |
| P4 | Large JSONL tail, title index, full-filter selection, archive recovery and all old Handoff divergence strategies | Core tests pass; actual terminal/native UI unrun |
| P5 | Catalog/context/compaction, reproject bound settings, API accounts, health observations, detach preview, port transaction | Core fake-RPC/secret and local HTTP tests pass; actual config RPC/Secrets unrun |
| P6 | Tool/namespace/custom/history shim, reasoning state, native Responses passthrough, error/status and identity behavior | Core/shim tests pass; real converter/Codex integration unrun |
| P7 | Original Antigravity configured port and process authentication/groups; shared Codex snapshots/reset | Local-peer and normalization tests pass; live account/service unrun |
| P8 | Authoritative token counters, historical rates, archived/PAT attribution, heatmap/thread/Insights/diagnostics and global snapshot source | Core tests pass; actual SQLite/native views unrun |
| P9 | Updated reports, full patch, source hashes and package checks | No native build, signing, notarized DMG, live updater, or system acceptance |

## Not exact old behavior

- Display rename is supported, but the old home-directory and human-readable wrapper relocation is not reproduced; profile IDs stay stable.
- Discovery is read-only with explicit registration/import, rather than automatically placing every discovered old home into the new account list.
- Raw imported diagnostics are preserved, while some new parser categories are aggregated rather than separate metrics.
- Approved new-root isolation, explicit binding reconnection and recoverable deletion are retained. A bound source configuration cannot be blindly copied with another profile's binding identity.

## Not missing by mistake

- Removed bulk session Sync is not revived.
- The legacy gateway's compaction worked through ordinary /responses; a separate /responses/compact remains unsupported rather than being falsely simulated.
- Antigravity does not scan arbitrary ports: it uses the configured port with discovered process authentication.
- Global legacy account-usage snapshots without an account identity stay explicitly unattributed.

See `PARITY-REVIEW.md`, `LEGACY-COMMAND-MATRIX.md` and `../TEST-REPORT.md`. Source present, tests written, core tests passed, and native integration accepted are four different states.
