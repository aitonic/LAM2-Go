# Dependency and license inventory

This records the selected direct dependencies, not a completed transitive/SBOM audit. Versions are pinned in go.mod; Go modules could not be downloaded in this environment. Preserve upstream notices when distributing resolved libraries/runtime assets.

| Dependency | Pinned version | Declared upstream license | Role |
|---|---|---|---|
| github.com/egoist/mygo | v0.7.1 | MIT | Native UI, SQLite, Secrets, terminal, updater, watcher |
| github.com/pmenglund/codex-sdk-go | v0.163.0 | Apache-2.0 | Codex App Server client |
| github.com/alexgorbatchev/agent-parser | v1.0.1 | MIT | Codex JSONL normalization |
| github.com/router-for-me/CLIProxyAPI/v8 | v8.0.44 | MIT | Public translator SDK, not a bundled proxy server |
| golang.org/x/net | v0.61.0 | BSD-style | Per-profile HTTP proxy handling |

No RelayKit/AGPL converter was added. This is an inventory, not legal advice or a statement that every transitive license has been reviewed. `tools/verify.sh` records resolved modules after a successful online build. Native runtime resources brought in by MyGo must also be included in the final distribution audit; no such built resource bundle is shipped here.

The legacy LAM MIT copyright notice is retained in LICENSE, including for the historical test data. No fonts or old compiled helpers are included.

Sources:
- https://github.com/egoist/mygo/tree/v0.7.1
- https://github.com/pmenglund/codex-sdk-go/tree/v0.163.0
- https://github.com/alexgorbatchev/agent-parser/tree/v1.0.1
- https://github.com/router-for-me/CLIProxyAPI/tree/v8.0.44
- https://pkg.go.dev/golang.org/x/net
