# LAM 2.0 二次对齐复查：实际测试记录（2026-10-28）

## 执行环境

- Linux amd64，系统已安装 **Go 1.23.2**。
- 真实 `go.mod` 指定 **Go 1.27.3**、MyGo **v0.7.1**、Codex SDK、agent-parser、CLIProxyAPI Converter；`go.sum` 仍需联网生成。
- 网络 DNS 不可访问 `proxy.golang.org`，全工程编译在工具链下载阶段被阻断；无 macOS 签名 App、Codex 凭据或原生 Keychain 测试环境。
- 离线测试只复制 **原样**纯 Go 领域包到临时 Go 1.23 模块，**没有**用伪造 MyGo/SDK 或替身生产代码欺骗全工程测试。

## 本轮最终命令及实际结果

| 验证 | 结果 | 源日志 |
|---|---|---|
| `./tools/test-offline.sh json` | **PASS** — 171 顶层测试，连同子测试 188 条通过记录；0 失败 | `reports/deep-final-domain.jsonl` |
| `./tools/test-offline.sh vet` | **PASS** — 离线领域包 | `reports/deep-final-vet.txt` |
| `./tools/check-source.sh` | **PASS** — 无 Rust/React/Tauri 源、无自制 GUI/SQLite 内核及格式问题 | `reports/deep-final-source.txt` |
| 针对更新模块的 `go test -race -count=1` | **PASS** — `internal/account`、`internal/usage` | `reports/deep-targeted-race.txt` |
| 全量 `./tools/test-offline.sh race` | **未验证** — 一次完整范围运行被环境执行超时中断，不能记为 PASS | `reports/106-review-race.txt`（早期尝试） |
| 真实 `go test ./...` | **BLOCKED** — 工具链下载 DNS 失败，没进入源文件编译 | `reports/deep-full-build.txt` |
| MyGo UI/SQLite/Secrets/官方 Codex SDK/真实 Translator | **NOT RUN** | 需要 Go 1.27.3+ 与依赖下载 |
| macOS arm64 UI、DMG、签名公证、真实认证/迁移 | **NOT RUN** | 当前没有对应环境 |

离线测试覆盖 **15 个生产业务包**；测试工具辅助包计入输出包数，但不能当成独立生产包。离线范围覆盖 **4,687 / 6,945 语句 = 67.49%**，并不覆盖 Desktop、应用 Composition、真实外部 Adapter 和发布链路。

## 本轮先失败后修复的测试证据

- `reports/111-main-red.jsonl` → `112-main-green.jsonl`：默认 `.codex` 注册必须映射受保护 main。
- `reports/deep-sort-red.jsonl` → `deep-sort-green.jsonl`：Duration/Gap/Attention 排序与增量链路信息。
- `reports/deep-limit-red.jsonl` → `deep-limit-green.jsonl`：5k/10k/20k/All 预算分页。
- `reports/113-activity-red.jsonl` → `114-activity-green.jsonl`：原版周/累计活动统计。
- `reports/deep-tray-domain.jsonl`：加入托盘 UI 源码后的业务层回归；**不涉及 GUI 编译**。

## 不可推断的结论

- 离线测试通过 **不能证明** MyGo Native UI v0.7.1 的 API/类型兼容。
- 假上游或 Fake Converter 的 Go Gateway 测试 **不能证明**真实 Codex 请求、custom tools、SSE 与 CLIProxy Translator 的语义等价。
- 239 个功能追踪点是基于源码入口和页面流程的**审查清单**；不等于 239 项验收已经执行。
- 当前版本不能直接安装替换旧版；必须先完成全工程编译、旧版数据迁移与 macOS 真机测试。
