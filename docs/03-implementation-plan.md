# LAM 2.0：整体重写实施工单与交付顺序

> 用途：分给多个编码 Agent 或工程师直接执行，不再重新讨论技术栈。  
> 原则：**代码一次性采用 Go + MyGo Native UI；开发可分批，正式版本必须功能完整。**  
> 基线：LAM 0.4.4 的现行行为、fixtures、用户数据兼容，例外为已经明确废弃的批量 Sync。

## 0. 执行规则

- 项目新建为独立 `lam2` Go Module，不在旧 `apps/desktop` 直接覆盖源代码。
- 每个工单按 **Contract Test -> Red -> Implementation -> Green -> Regression -> Review** 提交；测试可以继承旧版 fixture 的预期，但不能把旧 Rust 代码带入编译产物。
- 依赖先核实 Module 公共 API、许可证与必要的 Go 版本；一旦集成固定 Tag，不使用浮动 `latest` 参与 CI。
- **MyGo-first review**：每次提交包含 UI/OS/SQLite/Updater/Terminal 自研代码，必须先说明 MyGo 为什么不能满足。
- 允许并行开发、统一通过稳定的接口定义协调，不引入宏大的仓库级架构和自研框架。
- 不进行“只先做 Dashboard”的假完成。首个正式发布需完成账号、Session/Handoff、Provider/Gateway、Quota、Usage、设置与托盘。
- 每个工单必须有至少 1 个正常、1 个失败/边界用例；不将日后人工补测作为替代。

## P0：基线锁定与代码仓库创建

**输入**：`LAM.zip`；旧项目 `apps/desktop/src-tauri/tests/fixtures/`；现有 `docs/FINAL-DESIGN.md` 的 2026-07-17 修订；本实施文档。

**任务**：
1. 初始化 `mygo init -template native`，生成 `mygo.json`、资源和 Go Module；正式版本使用 Go 1.27+。
2. 保留 MyGo 官方 `.agents/skills/mygo-maintenance`，配置 CI 跑 `go test ./...`, `go vet ./...`, `go tool mygo vet`；不得修改框架自身代码。
3. 输出 `source-contract-matrix.md`：旧源文件/旧测试/新领域服务/新 Go 测试一一对应；复制需要的测试 JSON/SSE 样本到 `tests/fixtures` 并清理敏感值。
4. 创建稳定的基础领域类型及 `AppError{Code, Message, Recoverable}`；错误信息禁止包含密钥和 Prompt。
5. 仅固定确定要使用的 SDK 与 MyGo 版本，记录 `go.mod/go.sum`、官方 CLI 和测试环境。

**测试先于实现**：CLI 模式分发、AppState 线程边界、错误码序列化、无 Rust/Tauri/React 代码依赖检测。

**交付**：可编译 MyGo Native 空壳 + Go 单元测试基线 + 同源 Test Fixtures。**不宣称功能完成**。

## P1：MyGo Native UI、应用生命周期与服务容器

**任务**：
1. 以 MyGo `ui.Router` 构建五页 Shell：Overview、Usage、Sessions、Providers、Settings；点击导航、返回/前进、URL-like 状态保留。
2. `ui.View` 仅读取窗口 State；所有异步结果 `Window.Update()` 回主线程；不保存 `ui.Element`。
3. MyGo Tray 和浮窗：点击打开配额视图、失焦关闭、快捷打开主窗口、关闭主窗口隐藏、退出应用时正确运行清理逻辑。
4. 原生 Dialog、Toast、Popover、Keyboard Shortcuts、系统主题；中文/英文文本资源通过 MyGo 本地化能力，按系统语言切换。
5. 集成 MyGo Native Terminal Demo（仅用于后续内嵌终端功能），同时保留外部终端接口。
6. 定义 `ApplicationServices`：Account、Session、Handoff、Provider、Quota、Usage、Settings、Gateway Supervisor；没有服务真值时页面展示明确空状态，不填 mock 额度。

**测试**：`ui.NewTester` 对五页导航、空状态、表格筛选、弹窗关闭、异步错误与 Keybinding；系统级托盘/窗口用 macOS 人工脚本和真机检查。

**交付**：完整 Native UI Shell 和稳定服务入口。

## P2：存储、路径与配置迁移

**任务**：
1. 路径解析：`LAM_HOME`、默认 `~/.lam-next`，只读发现旧 `~/.lam` 和 legacy 来源；保留 `~/.codex` 和 `~/.codex-*` 原位。
2. Provider/Binding/Config JSON 采用稳定的 Go DTO 与版本标记；对旧版 `camelCase/snake_case` 做明确转换，不因未知枚举而回退默认。
3. 使用 MyGo SQLite 实现 `usage_events`、`source_files`、`thread_summaries`、诊断、账户 Usage 快照对应的读取和新库初始化。
4. 新版文件写入使用版本号/CAS、路径安全与临时文件原子替换；多资源操作有 journal，可恢复不完整提交。
5. 旧 SQLite 只读一致性快照 + 源 JSONL 重新建立索引的路径分别测试；迁移失败不改变旧版运行状态。
6. MyGo Secrets 读取旧 Keychain 版本化引用，验证签名变化下的访问授权；较长 Token 数据只使用受保护的文件/Keychain 引用，禁止明文回退。

**测试**：future schema 拒绝、丢字段、文件锁、重复迁移、系统掉电的事务恢复模拟、路径穿越、SQLite WAL、旧 Provider 样本互通。

**交付**：隔离的新数据根 + 原始旧数据仍完整。

## P3：Codex Account、认证和 App Server

**任务**：
1. 集成 `codex-sdk-go` 并对 `codex app-server` 版本做强校验；封装 `CodexClientFactory`，每个 `CODEX_HOME` 独立 SDK 生命周期。
2. 账号发现、创建、重命名、删除、备注、代理、Wrapper 生成/修复、身份隔离；保留 Profile 与 PAT 模式区别。
3. 通过官方 `account/login/start` 等接口支持 ChatGPT/API Key 登录；验证 PAT/Session 导入与 CPA 导出兼容旧版字段。
4. SDK 高阶 API 未暴露的官方方法使用 SDK `rpc` 薄封装，禁止另造完整 JSON-RPC。
5. Launcher 校验 CLI 路径、终端可用性、权限预设、代理和环境；支持 `lam launch --profile` 和 Resume。
6. 认证错误、令牌到期、无 Codex CLI 时给用户可执行的恢复建议。

**测试**：两个独立 Home 并行、PAT 切换和恢复、错误凭据、Codex CLI 缺失/版本不匹配、代理隔离、Wrapper 可修复性。

**交付**：Overview 上账号可真实创建、登录、启动、切换。

## P4：Session、Handoff 与安全删除

**任务**：
1. 封装 App Server `thread/list/read/turns/list/items/list`，跨 Profile 分页聚合。
2. 使用 `agent-parser` 解析历史 Codex JSONL 元数据与 Token；处理文件截断、非法 UTF-8 和异常字段。
3. 实现 Session 搜索/排序/过滤/详情、最近 Session 快捷 Relay、启动 Resume。
4. Handoff 严格复现现有 `services/relay.rs` 兼容性与 diverged 策略：只复制选定 Session，提供 preview/fingerprint/confirmation。
5. 安全删除先提供可删除集合与保护原因，再执行 Trash/Archive/Backup；保留最近 20、近 7 天保护语义。
6. 文件变更通知用 `mygo.WatchFiles`；过载/overflow 触发全量重扫，不要求每个文件事件都能增量映射。

**测试**：Session ID 冲突、目标 Provider 不兼容、拒绝复制 auth、目标文件不可写、原文件被并发改写、分叉策略、归档与恢复。

**交付**：Sessions 页面和 Handoff 页面可实际使用。

## P5：Provider、Binding、API Account 与 Config 事务

**任务**：
1. 接入现有 Provider V2 模型全部字段和旧版文件格式，CRUD、模型发现、连接测试、密钥轮换和权限审批。
2. 通过 Codex App Server 的 `config/read/batchWrite` 完成可支持的编辑；兜底 TOML Writer 必须保持注释和用户配置。
3. Attach/Detach 使用跨文件/Keychain/Config 的事务 Journal，校验 Revision、Model、Credential、Gateway readiness。
4. 完成 API Account 创建、模型切换、连接设置和删除；Provider 从未配置到可运行需明确 Readiness 状态。
5. 绑定与 Session Handoff 的 Provider 兼容性检查共享唯一的能力模型，但不要让 Provider 逻辑依赖 UI 路由。

**测试**：旧 JSON schema、future schema、CAS 冲突、进程中断、密钥轮换、Keychain 缺失、错误 MODEL/URL、Config TOML 保真。

**交付**：Providers 页面可进行完整配置与绑定，但 Chat Gateway 路由在 P6 验证通过后才对外标 READY。

## P6：全部 Go Gateway（不保留 Rust Sidecar）

**任务**：
1. 实现同一个 Go binary 的独立 Gateway Mode、健康握手、进程身份、Port claim、PID 管理及恢复。
2. 采用 `net/http` 实现 `GET /health`, `GET /v1/models`, `POST /v1/responses` 等已证实路由。
3. 复用 CLIProxyAPI Translator 的公开 API，将旧源码的 Responses->Chat Requests、Chat->Responses Responses/Stream 架构拆成插件和小型兼容层。
4. 完整通过 SSE、Tool Roundtrip、DeepSeek Thinking、multimodal/unknown fields、超时/取消/背压、HTTP Error Contract；无法无损转换的结构明确拒绝。
5. 认证 Binding Token 与上游密钥分离，Keychain 只在本进程按需读取，不把 token 放在命令行参数中。
6. 限流、缓存/历史分隔、错误码、TTFT、请求诊断和优雅关闭；支持桌面退出后保持活跃 CLI 连接。
7. 保留原始网关测试中观察到的容量边界作为基线，建立测量后再改变容量参数。

**测试**：真实旧 fixture + fake upstream golden，包含 4xx/5xx、任意 SSE chunk 切分、重复 terminal、下游中断、未知路由、端口占用、并发和资源释放。

**交付**：Gateway 100% Go，与旧 Rust 网关的关键协议行为等价；新发行包不含旧 Helper/Gateway。

## P7：Quota、Antigravity、Reset Credits

**任务**：
1. 官方 App Server `account/rateLimits/read` + `account/usage/read`；配额事件推送/轮询与有界缓存。
2. Rate-limit Reset Credits 读取 availableCount/details，消费使用固定幂等键，成功/重复消费均重新获取额度。
3. Antigravity 本地只读 Adapter，以健康状态、访问权限和显式 source/staleness 归一化；失败不是 0% 或 100%。
4. Overview 和 Tray 显示同一数据源与刷新时间，避免 UI 各自重复抓取导致状态冲突。

**测试**：无认证、部分窗口、Reset Credit 过期未知、并发刷新单飞、API 不可用、Antigravity 端口无服务、stale 展示。

**交付**：配额数据真实、来源明确，Reset Credit 与 Tray 对齐。

## P8：完整 Usage、费用与诊断

**任务**：
1. `agent-parser` 对旧会话事件做规范化，增量 Indexer 记录 SourceFile/Offset/mtime/Parser State，不可用时全量重建。
2. MyGo SQLite 读模型复现旧 Usage `usage_events/source_files/thread_summaries/diagnostic_snapshots/account_usage_snapshot` 等的关键字段、过滤与排序。
3. Account Usage 和 Local Usage 分开查询，Delta 只比较相同作用域；缓存 Tokens、reasoning tokens、credits 与 Cost 使用相同规则。
4. Pricing 有版本号、时间和置信度，避免把 USD API 估价伪装订阅账单；未知模型显示 unknown。
5. UI 原生 Charts、Calendar-like Heatmap、Tooltips、Calls/Threads 细节和原始诊断，不能为了省代码省略 Insights。
6. `Refresh / Compact / Reset index` 是后台任务，取消、错误、进度状态可被 UI 正确观察。

**测试**：异常 JSONL、文件改名/截断、重复 Token 事件、缓存 Token 不重复加总、Account/Local Delta、旧 SQLite 对账、巨大历史数据分页。

**交付**：Usage 界面的数据口径与旧版一致或差异可解释。

## P9：设置、完善 Native UI、打包与替换

**任务**：
1. 完成 Settings 全部开关、表单、成本表、Gateway 配额刷新间隔与终端权限预设。
2. 增强 MyGo Native UI：键盘导航、无障碍、深浅主题、中文输入、缩放、列表虚拟化、无窗口 Tester。
3. 使用 MyGo Native Updater，并配置 MyGo 官方构建签名、macOS arm64 DMG 和 GitHub Actions 发布流水线。
4. 将旧版真实使用数据先备份，再迁入 `~/.lam-next`；运行完成清单后才可切换新程序为默认入口；保留旧版可回退。
5. 产出 `MIGRATION-REPORT.md`、`TEST-REPORT.md`、`LICENSES.md`、发布二进制 SHA-256；检查动态库/Resource 及无旧 Rust/Tauri/React。

**测试**：`go test`、race tests、MyGo Tester、签名校验、DMG 安装、首次启动/升级/回滚、外部 CLI 不中断和 UI 视觉验收。

**交付**：首个可正式替代旧 LAM 的全 Go 发布版本。

## 并行开发建议（边界清晰）

| 工作流 | 负责人职责 | 允许并行的阶段 | 前置约定 |
|---|---|---|---|
| A — MyGo UI | Shell、Overview、Usage、Sessions、Providers、Settings、Tray | P1 + P4/P5/P7/P8 页面 | 仅依赖 Service 接口/DTO |
| B — Codex/Accounts | Profile、App Server SDK、Session、Handoff | P3/P4 | P2 Store/Paths、CodexClient 接口 |
| C — Gateway | Translator、HTTP、SSE、Tool、Supervisor | P6（可与 P3 并行完成测试） | 固定 Provider Binding DTO |
| D — Data/Quota | MyGo SQLite、JSONL、Usage、Quota、迁移 | P2/P7/P8 | Source fixtures 和 Schema 基线 |
| E — QA/Release | Goldens、CI、License、DMG、Migration | 每个阶段同步进行 | 先有失败/边界测试再实现 |

当跨模块语义冲突时，以本文件、`01` 和 `02` 的领域契约为准，不由单个开发者在 UI 或 Gateway 临时发明字段。

## 首次提交检查清单

- [ ] 创建完全独立的 Native MyGo 工程；不拷贝旧 React/Tauri/Rust。
- [ ] 固定 Go/MyGo/Codex SDK/JSONL Parser/Translator 的兼容版本和许可证。
- [ ] 导入 42 个旧 fixture（检查/脱敏）与主要 Rust 行为测试映射。
- [ ] 先写 `ui.NewTester` 和 Gateway 协议 Contract tests，并确认其初始失败是预期的。
- [ ] 先打通独立 CODEX_HOME 的 `codex app-server` SDK 接入，再实现真实数据列表。
- [ ] 所有开发使用隔离 `LAM_HOME`，不得直接改本机用户旧 `.lam` 数据。

**实施时不再要求重新批准“是否采用 Go + MyGo”**；需要额外决策的仅限新增的外部账户授权、公开发布许可方案变化、不可逆用户数据操作。
