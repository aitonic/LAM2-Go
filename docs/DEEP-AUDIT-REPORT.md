> **Updated feature-test verification (2026-10-28):** This file is a historical snapshot. For current 239-item statuses, mapped executable tests and CI gate use [`docs/FEATURE-TEST-COVERAGE-REPORT.md`](docs/FEATURE-TEST-COVERAGE-REPORT.md) and [`tests/feature-contracts.csv`](tests/feature-contracts.csv). Prior 127 `DOMAIN_PASS` is no longer valid; current direct domain evidence is 125. macOS/native release is NOT accepted.

# LAM 0.4.4 → LAM 2.0 二次深度功能对齐审计

> 2026-10-28；基准：用户提供的 `LAM.zip` / LAM 0.4.4、`docs/01`—`04` 完整重写规格、上一版 `LAM2-Go-MyGo-Review-Fixed.zip`。本文件报告**实际源代码状态**，不是正式发布证明。

## TL;DR

本轮按**原始注册 IPC → 原版 UI 操作 → Go 实现入口 → 用户可观察行为 → 测试证据**五层重新核对，生成 `DEEP-FEATURE-PARITY-MATRIX.md` 和独立 CSV。共 **239 个追踪点**：**111 个旧 IPC、128 个页面/托盘/数据/系统流程**。已有源码、隔离 Go 单测、真实 MyGo/Codex/SQLite 集成和 macOS 交付四种证据**严格区分**。

当前状态：`DOMAIN_PASS` **127**，`NATIVE_UNVERIFIED` **82**，`PARTIAL` **23**，`GAP` **6**，`EXCLUDED` **1**。`DOMAIN_PASS` 是**对应领域有已执行回归及源码映射**，不是每行都有独立用例，更不等于全功能端到端验收；不能以 127/239 计算正式完成率。GAP 6 行对应 4 类根本问题。**未达到“与原版无差异”或可正式替换的验收标准。**

### 分类统计

| 来源/领域 | 总数 | DOMAIN_PASS | NATIVE_UNVERIFIED | PARTIAL | GAP | EXCLUDED |
|---|---:|---:|---:|---:|---:|---:|
| 原始 IPC | 111 | 78 | 28 | 3 | 2 | 0 |
| 导航/多语言 | 6 | 0 | 4 | 2 | 0 | 0 |
| 账号 | 22 | 10 | 5 | 6 | 1 | 0 |
| Sessions | 16 | 9 | 7 | 0 | 0 | 0 |
| Provider | 17 | 11 | 4 | 2 | 0 | 0 |
| Usage | 31 | 15 | 10 | 5 | 1 | 0 |
| Tray | 13 | 0 | 8 | 5 | 0 | 0 |
| 设置 | 14 | 3 | 11 | 0 | 0 | 0 |
| 构建/平台 | 9 | 1 | 5 | 0 | 2 | 1 |
| **总计** | **239** | **127** | **82** | **23** | **6** | **1** |

## 本轮已修复及验证的功能差异

| 原版操作/合同 | 旧版证据 | 当前 Go 修复位置 | 测试/状态 |
|---|---|---|---|
| 启动可见用户已有 `.codex`、`.codex-*` 账号，不把空目录当账号 | `services/account.rs`, `list_accounts` | `account.Discover`, `ListVisible`, `app.BasicSnapshot` | `account/discovery_parity_test.go` 通过；Native UI 未运行 |
| 默认 `~/.codex` 必须是受保护 `main`，而不是普通可删 External | 同上，`MAIN_ACCOUNT_DELETE_BLOCKED` | `account.Register`、`account.Delete` | `TestDiscoveredSharedCodexIsTheProtectedMainIdentity` 通过 |
| 发现账号应能进入注册操作而不是按钮永远禁用 | `routes/views.tsx` | `desktop/view.go` 的按钮条件 | `desktop/desktop_test.go` 已写；MyGo Tester 未运行 |
| Usage 默认全部时间/仅活跃，具备日期预设及 Reset | `routes/usage.tsx`, `src/App.tsx` | `desktop/state.go`, `view.go`, `usage/date_window.go` | 日期计算域测试通过；界面未运行 |
| Usage 可按 CWD/Model/Thread 字样、Effort、Pricing Confidence 查询 | `routes/usage.tsx` | `usage.Filter.SQL`, `desktop/view.go` | 参数化/恶意字符串回归通过；SQLite 未运行 |
| Usage 按 Duration/Gap/Attention 排序，不回退到 Time | `usage-dashboard-analysis.ts` | `usage.Filter.CallOrderSQL`、`UsageEvent.ThreadCallIndex/PreviousRecordID`、增量 Checkpoint | 先红后绿；`call_sort_parity_test.go` 通过；真实 SQLite 未运行 |
| Details 上限 5k/10k/20k/All（与 Call/Thread 分页独立） | `routes/usage.tsx` | `Filter.DetailBatch`, `mygosqlite.Insights/Diagnostics`, `desktop/view.go` | 先红后绿；`detail_limit_parity_test.go` 通过；UI/SQLite 未运行 |
| Usage 活动 Daily / Weekly / Cumulative 和 Calls/Tokens、完整一年 | `routes/usage.tsx` | `usage/heatmap.go`, `desktop/activity.go`, `app.ReadUsage`, `mygosqlite.Daily` | 周聚合与单测通过；UTC 一次 GROUP BY，插件未运行 |
| 解析失败原因不只给一个 skipped 总量 | 原版 Usage diagnostics, `services/usage.rs` | `usage/parser_diagnostics.go`, Checkpoint, `mygosqlite.Diagnostics` | 已覆盖常见诊断；完整旧原因仍 PARTIAL |
| 更新/排序后会话/账号/Calls 选中项绑定稳定 ID | 原版 React keyed rows | MyGo `ui.ListState.Key` in `desktop/view.go` | 使用 MyGo 官方接口，原生测试未运行 |
| Tray 切换 Codex / Antigravity、单账号刷新、Relay Latest、PAT Switch | `tray-quota-panel.tsx` | `desktop/run.go` | 源码接入，需 MyGo/macOS 实测；接力先走预览确认 |
| Tray 双来源定时更新、Reset Credit 到期及 Token 有效期 | 同上 | `desktop/run.go`, `desktop/view.go` | 两分钟 Antigravity、Codex 按设置刷新；运行未验证 |

**关于 Usage 排序**：原版 `duration` 的实际排序键是 `threadCallIndex`，不是毫秒时长；`gap` 的排序键是 `previousRecordId` 是否存在；`attention` 是 Token 与低 Cache 比率的加权分数。新 SQL 按实际旧算法映射，避免按词面误实现“时长”。在新索引中新增链路数据并提升 Checkpoint 元数据版本，旧缓存会安全重建。

**关于 Usage 加载上限**：原版 5000/10000/20000/All 控制 Insights/Diagnostics 细节查询预算，Calls/Threads 仍独立分页；新设计采用单页最多 500 行批量累加，不一次性将 20k records 加载到 UI。完整 SQLite 行为仍须按实际驱动验证。

## 尚未解决：6 条 GAP = 4 类根问题

| 追踪编号 | 未达成的原版语义/验收 | 为什么本轮未粗暴改写 | 所需完成条件 |
|---|---|---|---|
| `CMD-012` / `CMD-013` / `FLOW-013` | 原版账号重命名实际移动 `CODEX_HOME` 目录、改写 wrapper、维护账号/代理/缓存；Go 当前仅更新稳定 ID 的显示名 | 裸 `os.Rename` 会让 SQLite 源路径、原 JSONL、认证运行中的 CLI、wrapper、Binding 引用与事务恢复不一致，可能直接破坏账号 | 先做重定位 Preview/Collision/Process Check/Journal/CAS/全源引用迁移及崩溃回滚测试，再启用物理改名 |
| `FLOW-087` | Usage 各分区独立读取、刷新和加载状态 | 当前虽可报告 section-specific error，但仍是单个 `ReadUsage` 异步任务，每次切换可能重拉全部 section | 采用独立 section request + generation/cancellation + scoped cache + 单分区加载反馈；不能使用过期结果覆盖当前 Scope |
| `FLOW-121` | 真实 `Go 1.27.3` + MyGo v0.7.1 编译 | 当前 Linux 主机 Go 1.23.2，访问 proxy.golang.org 的 DNS 拒绝；尚未获得真实 package type-check | 可联网 Go 1.27.3+ 环境执行 `go test ./...`、`go vet ./...`、`go tool mygo vet`，解决所有 API/类型错误 |
| `FLOW-126` | macOS arm64 打包、签名、公证、DMG 和回滚 | 当前无 macOS/签名与真实账号环境，不能伪称完成 | macOS 实机安装/升级、Keychain 权限、菜单栏、输入法、Codex CLI、Gateway 进程、签名公证、回滚全部有证据 |

## 部分对齐：23 条 PARTIAL 的分组处理

- **账号**：旧 `list_cached_accounts` 缓存/自动注册语义，Profile 显示与实际重命名分离、Token 到期视觉、套餐徽章、卡片快捷菜单、Home 打开/复制和 API Account 差异化展示。
- **Provider**：高级模型/能力字段的表单展示仍偏通用 JSON；绑定计划目前是结构化 JSON 预览，非旧版逐副作用可读清单。
- **Usage**：多 Workspace Scope 分段导航、旧版完整解析诊断分类、Loaded/Partial/Error 芯片、价格来源及原活动月标记/Tooltip 细节；其中独立 section Loading 属于 GAP。
- **Tray**：活跃账号标识、5 小时/周/月进度环、动态面板尺寸、浮窗独立主题控件、原信息密度与展开方式。
- **导航与多语言**：设置分组布局简化，部分新 UI 控件只提供英文字符串。

完整逐行位置与具体说明见 `docs/DEEP-FEATURE-PARITY-MATRIX.md` 和根目录外附的 `LAM2-Deep-Audit-Feature-Parity.csv`，没有将 PARTIAL 误算为 PASS。

## 82 条 Native/External 未验收不等于缺代码

这些条目能追溯到 Go 源码调用，但还没有真实 MyGo v0.7.1、SQLite Plugin、Secrets、Codex SDK、agent-parser、CLIProxyAPI Translator 和 macOS 行为的集成证据。比如：

- 账号真实 ChatGPT 登录、OAuth/PAT 令牌有效性与独立 `CODEX_HOME` 进程；
- 原生 MyGo 列表、图表、选择器、Toast/Modal、IME、Accessibility、托盘失焦和窗口生命周期；
- Gateway 真实 Responses/Chat/Tool/SSE Goldens（纯网关测试使用明确 Fake Converter）；
- SQLite WAL 快照、Keychain 提权/迁移、真实 CLI 兼容、Usage 新旧库逐行对账；
- GitHub CI 构建、DMG 签名公证、Native Updater、错误与中断恢复。

无法在当前环境验证时明确保留在阻断清单；不得改用模拟 MyGo 包造成“看起来通过”。

## 核心测试证据与判定

详情 `docs/DEEP-AUDIT-TEST-REPORT.md`：本轮离线业务检查 **171 个顶层测试，含子测试 188 个通过记录，0 失败**；有编译覆盖的核心包执行 `go vet` 和 source audit 通过。修改影响最大的 `internal/account`/`internal/usage` 使用 `go test -race` 有单独通过记录。**这些均不等于 MyGo 全应用测试。**

## 下一轮真正应做的工作

1. **最高风险数据事务**：把目录/Wrapper 重命名从显示名编辑中拆出，先写一致性恢复测试，再实现实际迁移，并与 Usage 原始路径和 PAT 身份隔离联动。
2. **Usage 独立刷新**：按 `scopes/overview/activity/insights/calls/threads/diagnostics` 定义独立控制与状态，保留当前过滤条件和取消旧结果的安全性。
3. **真实 Go/MyGo 构建**：修复任何 Compiler/API 变更；真实 SDK、Translator 和 SQLite Fixtures 验证；完整旧版 42 Fixture 逐项执行并记录通过/失败数。
4. **原生 UI 逐页验收**：对照原图和现行控件补齐 23 个 PARTIAL，保留 MyGo Core/UI/Plugins，禁止再造框架。
5. **发布验收**：macOS ARM 真机完成帐号、Handoff、网关、用量、托盘、签名/升级/回滚的端到端流程。

**终止条件**：所有 239 个追踪点达到恰当验收状态、零未解释 GAP/PARTIAL；原本废弃的整账号批量 Sync 不恢复；跨模块、平台与许可证/依赖检查全部通过。当前**未达到该条件**。
