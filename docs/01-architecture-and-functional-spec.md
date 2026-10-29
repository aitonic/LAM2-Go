# LAM 2.0：全 Go + MyGo Native UI 整体重写架构与功能规格

> 版本：1.0 · 日期：2026-10-27 · 状态：实施基线（已确定技术方向；尚未宣称代码完成）  
> 目标平台：首发 macOS arm64。Windows/Linux 预留平台适配，但不能以未验证平台作为首发承诺。  
> 基础来源：用户提供的 `LAM.zip`（LAM 0.4.4）、实检源码、MyGo 官方文档、Codex App Server 官方文档。

## 1. 决策及不可变约束

**完整重写 LAM 2.0，保留用户可观察行为、数据兼容性，替换整个技术栈。** 不是逐行翻译，不是迁移一部分组件，也不是只做 UI。

1. 应用源码全部 Go；界面采用 MyGo Native UI。禁止保留 Rust、Cargo、Tauri 2、React、TypeScript、Zustand、Vite、前端 WebView 和相应 JS 构建链。
2. **MyGo-first**：先查 MyGo Core，再查其官方插件，其次 Go 标准库，再选成熟 Go 包。仅业务差异化逻辑自行实现。不得另建 UI 框架、Router、通用事件总线、SQLite 引擎、终端模拟器、文件监控、软件更新器。
3. 对第三方 Go 包不以“最少依赖”为目标；以适配能力、许可证、维护状况、公开 API、可测试性为准。每个依赖固定版本，建立兼容性测试。
4. 同一个 Go 可执行文件支持 `desktop`、`gateway`、`auth`、`launch` 等运行模式；允许同一程序启动独立 Go 子进程。不得打包旧 Rust Sidecar/Helper。
5. 旧版 LAM 数据与 Codex Home 不做破坏性迁移；新程序默认读写隔离目录，正式迁移前先快照并校验。跨账户 Handoff 不拷贝认证材料。
6. **测试先行**：每个能力的契约与失败用例先于实现落地；尽量复用旧版 fixtures、行为断言及 MyGo 官方 `ui.NewTester`。
7. 不因为现成 SDK 缺少一两个特性，就把整个 SDK 重写。以边界 Adapter 补齐差异；但遇到协议字段静默丢失，必须明确失败或兼容标记。
8. 不承诺旧版全部文档草案为当前功能。**2026-07-17 已废弃整账号 Sessions 批量 Sync**；仅保留单 Session 的 Relay Latest / Handoff。

## 2. 经源码确认的基线

| 范围 | 证据（相对 `LAM.zip` 解压根目录） | 事实 |
|---|---|---|
| 版本 | `apps/desktop/package.json`, `apps/desktop/src-tauri/Cargo.toml` | `0.4.4` |
| 前端 | `apps/desktop/src/App.tsx`, `src/routes/types.ts`, `src/stores/` | React 18、TypeScript、Zustand、五个页面 |
| 桌面生命周期 | `apps/desktop/src-tauri/src/main.rs` | 主窗口隐藏、Tray、小浮窗、启动迁移与恢复、Command 注册 |
| 业务核心 | `apps/desktop/src-tauri/src/services/` | Account、Session、Relay、Provider V2、Quota、Antigravity、Usage、Storage |
| Gateway | `services/gateway/`, `services/adapters/` | 监听/鉴权/路由、SSE、DeepSeek、工具调用、进程监督、超时容量控制 |
| 数据根 | `services/lam_paths.rs` | `~/.lam/config`, `provider-hub`, `usage/usage.sqlite3`, Reset Credit |
| 使用量数据库 | `services/usage.rs::init_usage_db` | `usage_events`, `source_files`, `thread_summaries`, `account_usage_snapshot`, `account_usage_daily_buckets`, `refresh_meta`, diagnostics 表 |
| 存量测试 | `src-tauri/tests/`、`src-tauri/tests/fixtures/` | 56 个 Rust 集成测试文件、42 个测试 fixture 文件 |
| 旧设计纠正 | `docs/FINAL-DESIGN.md` 开头 | 整账号 Sync 已废弃，旧章节不能当现行规格 |

附录 `reference/source-inventory.md` 包含实际源码列表与迁移归属；不是仅凭旧 README 归纳。

### 2.1 现行信息架构

- Overview：账号卡、Codex 与 Antigravity 配额、快捷登录、切换、Relay Latest。
- Usage：本地 Usage vs Codex Account Usage、Token/Cost、Activity、Insights、Calls、Threads、诊断与价格卡。
- Sessions：筛选/分页/排序、详情、受保护删除、Resume/Handoff。
- Providers：Provider CRUD、模型发现、连通性、认证源、Binding、API Account、Gateway。
- Settings：运行/终端/权限、代理、刷新、Dock/Tray、Gateway 超时、用量索引、重置额度、偏好。
- 独立托盘配额窗口：总览、刷新、快速回到主窗口，生命周期独立于页面 Router。

## 3. 目标逻辑架构

```text
                         +------------------------------+
                         |      LAM 2.0 - one Go exe    |
                         +------------------------------+
                                        |
                +-----------------------+-----------------------+
                |                       |                       |
         Desktop Mode             Gateway Mode             CLI/Auth Mode
      MyGo App + Native UI        net/http server          login/launch/token
         + Tray + Update          Protocol Adapter         Profile-aware env
                |                       |                       |
                +-----------------------+-----------------------+
                                        |
                           Application Services (Go)
          Account | Session | Handoff | Provider | Quota | Usage | Settings
                                        |
                             Typed Domain Contracts
                                        |
            +---------------------------+---------------------------+
            |                           |                           |
     Codex App Server             Local Adapters                 Storage
     per CODEX_HOME             HTTP / Terminal / OS          JSON + SQLite
     official JSON-RPC           Secrets / File Watch         Keychain refs
            |                           |                           |
         Codex CLI                  External APIs                ~/.lam-next
                                                                    |
                                                         legacy ~/.lam snapshot
```

### 3.1 运行模式与生命周期

- `lam` 或 `lam desktop`：MyGo Native UI、Tray、App State，初始化只读发现和必要的后台工作。主窗关闭可隐藏，不代表杀死活跃 Gateway。
- `lam gateway --home <LAM_HOME> --binding-group <id>`：由 Launcher 创建/复用的 Go 子进程；独立 Loopback HTTP 监听，严格鉴权；只有明确停止或空闲条件达成才退出。
- `lam auth --binding <id>`：Codex `auth.command` 的 Go 入口，标准输出**只能**包含令牌值；错误只输出脱敏错误码到 stderr；不启动 GUI。
- `lam launch --profile <id> [--resume <session>]`：校验 Profile、Provider Binding、路由及终端后启动对应的 Codex CLI，传入明确的 `CODEX_HOME` 和允许的环境变量。
- Desktop 只管发起/显示；Gateway 子进程不创建 MyGo 窗口；App Server 池按 Profile 建立隔离实例。
- 单实例限制只适用于 Desktop Mode；Gateway/`auth`/`launch` 模式不受 Desktop 单实例锁误拦截。

**关键反例**：强制把网关绑进 GUI 进程会导致外部 Codex CLI 在 GUI 真正退出时断线。因此“单 Go 程序”必须区别于“只有一个 OS 进程”。

### 3.2 代码结构（目标，不复制旧源码）

```text
lam2/
  main.go                   # dispatch desktop/gateway/auth/launch; Go only
  go.mod                    # pinned MyGo and Go libraries; tool directive
  mygo.json                 # official native app metadata (不是 mygo.config.ts)
  resources/                # icon, localization, optional templates
  internal/
    app/                    # composition, startup, shutdown, desktop state
    ui/                     # MyGo Views: shell, overview, sessions, usage,
                            # providers, settings, tray, dialogs
    account/                # Profile, auth mode, login, PAT and wrapper
    codex/                  # SDK Adapter and App Server pool per CODEX_HOME
    session/                # list/index/details, archival, safe deletion
    handoff/                # plan, conflict, fingerprint, copy, resume
    provider/               # CRUD, capability, binding, config transaction
    gateway/                # HTTP, auth, stream, adapters, metrics, lifecycle
    quota/                  # Codex/Antigravity adapters, caches, freshness
    usage/                  # parser, incremental index, attribution, pricing
    storage/                # MyGo SQLite, versioned JSON, snapshots
    runtime/                # terminal launch, env, process identity
    settings/               # preferences, migration coordination
  tests/
    contract/               # protocol / DTO golden
    integration/            # isolated home / fake CLI / localhost upstream
    fixtures/               # copied/sanitized legacy fixtures
  docs/                      # specs, release notes, migration reports
```

原则：控制文件数量但不把所有代码塞进 `main.go`；不模仿旧版每条 Tauri Command。UI 调 Service 接口，Service 不反向引用 MyGo 控件，便于不打开 GUI 测试业务。

## 4. MyGo-first 映射（按官方 API）

| 需求 | 优先能力 | 实施规则 |
|---|---|---|
| 原生窗口 / 子窗口 / Dock / Tray | MyGo `App`、`Window`、`NewTray`、原生菜单 | 不另写窗口系统和托盘框架 |
| 页面导航 | `ui.Router`, `ui.Sidebar`, `ui.Tabs` | 每个窗口维护自己的 Router State |
| 大型列表 | `ui.List` / `ui.Table` 虚拟化 | Session 不全量渲染 |
| Usage 图表和热力图 | `github.com/egoist/mygo/ui/chart`、Grid | 不采用 JS Chart 库 |
| Toast、Dialog、Popover | Native UI Widgets | 保持可访问性、键盘操作 |
| 后台更新 UI | `Window.Update(func(){...})` | 背景 goroutine 不直接改 view state、不得持久缓存 `ui.Element` |
| 内置终端 | `plugins/terminal` 的 `terminal.New` / `terminal.View` | 需要内嵌终端时才启用；不替代外部 Ghostty/cmux |
| SQLite | `plugins/sqlite` 直接 `sqlite.Open(ctx, path, ...)` | Native Go 不需 `mygo.Use(sqlite.Plugin)` |
| 变更监控 | `mygo.WatchFiles` | watch 事件只是重新扫描提示，必须可全量重扫 |
| 凭据 | MyGo `Secrets` + 当前 macOS Keychain | 旧 service/account 键名兼容须实测 |
| 自动更新 | `plugins/updater/native` | **禁止**使用可能额外启动 WebView 的默认 updater 页面版 |
| UI 自动化测试 | `ui.NewTester`, `go test`, `go tool mygo vet` | 必须检查 UI Build Pass 生命周期 |
| 打包与更新签名 | `go tool mygo build`, MyGo signing/update | 使用官方 Native 模板，`mygo.json` |

官方文档：https://mygo.egoist.dev/docs/ui 、https://mygo.egoist.dev/docs/ui/views 、https://mygo.egoist.dev/docs/plugins/sqlite 、https://mygo.egoist.dev/docs/plugins/terminal 、https://mygo.egoist.dev/docs/plugins/updater 。

**例外与边界**：Go `net/http` 是 Gateway 标准库服务，MyGo 无 Codex 专用协议服务器；`os/exec` 用于 Ghostty/cmux/Codex CLI；业务上的额度与统计仍由 LAM 计算。MyGo Terminal 通过原生 `libghostty-vt` 运行时库，属于其官方组件，不是旧 Rust Sidecar；不误写成“任何二进制依赖都不存在”。

## 5. 功能规格与操作事务

### 5.1 Overview / Accounts

- 探测 `~/.codex`、`~/.codex-*` 及受管 Profile；显示来源、管理状态、认证状态、最后活动、会话数、模型/Provider 和配额。
- 支持新建/重命名/删除受管账号；未受管账号采取保守操作，不重写不属于 LAM 的身份文件。
- 支持 ChatGPT Login、API Key、PAT/Session 导入、CPA 导出（输出密钥操作需用户确认）；Profile 与 PAT 两种模式**不可混成同一种隔离承诺**。
- Profile 模式：每个 `CODEX_HOME` 互不共享认证；PAT 模式：涉及共享认证状态时显式串行化并提示影响范围。
- 每个账号的代理设置独立；登录/Resume 启动时确保代理环境归属正确。
- UI 状态：`loading/ready/stale/error/partial`；只有认证源实际报告的状态才能显示可用，不能根据 session token 估算 quota。

### 5.2 Sessions / Relay / Handoff

- 支持按 Profile、时间、关键词、尺寸、Workspace 过滤和分页；Session 源位置以文件+元数据映射，不以 SQLite 结果反向定义真相。
- 支持查看详情、Resume 命令、打开终端、单会话 Relay Latest / Handoff；不恢复整账号批量 Sync。
- Handoff：`Preview -> Compatibility -> Fingerprint -> Confirm -> Atomic Copy -> Verify -> Resume`；阻断型 mismatch 必须零写入；允许损失的情况需要显式确认。
- 源文件不可越过 `CODEX_HOME/sessions`；目标已存在时区分完全一致、目标更近、分叉和损坏，默认不覆盖；备份后才允许有意的替换。
- **绝不复制 `auth.json`、API Key、Token、secret cache**。默认只接力选中的 session JSONL；目标目录写入用临时文件+同步+rename。
- 保留现有默认受保护删除行为：最近 20 个与近 7 天修改的会话不纳入常规批量删除范围，除非用户另行明确的高级操作契约。

### 5.3 Providers / API Accounts / Bindings

- Provider：`id/name/protocol/baseURL/models/defaultModel/capabilities/credentialRef/compatibilityProfile/codexOptions/revision`。
- 协议：`responses` 直连优先；`chat_completions` 经 Go Gateway 做 Codex Responses 到上游 Chat 转换；不能在无转换能力时宣称可 Attach。
- 认证来源：环境变量、MyGo Secrets Keychain 引用、已批准 auth command、Codex 自身身份；不将 Secret 字符串持久化在 JSON/TOML/UI State。
- 支持创建、编辑、删除、模型发现/刷新、上游测试、密钥轮换、模型切换和 Profile 绑定。
- Attach/Detach 统一 `Plan -> Validate -> Commit -> Verify`，含 expectedRevision、旧配置快照与事务记录；配置冲突时保留原状态并显示错误，而不是静默覆盖。
- Codex 配置优先走官方 `config/read`、`config/value/write`、`config/batchWrite`；只能用直接 TOML 编辑器实现官方接口覆盖不到的文件语义，且保留无关字段、注释及换行。
- 外部 Provider 连接失败、Gateway 未就绪、凭据缺失、模型能力不明时必须显示独立原因；保存 Provider 不等于已经能够运行 Codex。

### 5.4 Quota / Reset Credit / Antigravity

- Codex：优先 App Server `account/rateLimits/read`，有变更通知时刷新；Reset Credit 列表及到期只有官方字段可靠时才显示日期；可用总数以 `availableCount` 为准。
- `account/rateLimitResetCredit/consume` 必须提供 idempotencyKey，遇到重试沿用同一个 key；消费结果后重新读取额度，不能由返回值推断新余量。
- `account/usage/read` 用于 Codex Account Usage，不可与本地 JSONL Token Usage 混算。
- Antigravity：作为只读本地适配器；探测进程/端口仅面向本机用户实际运行的服务，认证/状态失败则返回 unavailable 或 stale，不用估值填充。
- Quota Cache 包含 `source/fetchedAt/expiresAt/staleness/planType/windows/credits`，UI 明确来源和刷新失败情况。

### 5.5 Usage / Insights

- 原始输入为 Codex JSONL（可由现成 Go Parser 解析）；本地 SQLite 为可重建 Read Model。
- 支持增量扫描 `source_file + size + mtime + offset + parser_cursor`，截断/文件替换时执行安全重建；事件去重依据记录稳定标识。
- 按 Profile、Workspace、Thread、Model、日期/小时聚合；表格包含 Calls、Threads、Token 分类、Cache Ratio、Reasoning Output、成本估计、定价置信度和异常诊断。
- `input_tokens` 已包含 cached input 时，`uncached_input = max(input - cached, 0)`；禁止再次加上 cached input。金额使用指定版本价格表，明确标注 API-equivalent estimate 而非账户账单。
- 旧版来源归属、`thread_key`、`pricing_confidence` 和历史已归档标记须保持语义；Codex Account Usage 与 LAM Local Usage 分别展示，并显示可解释的 Delta。

### 5.6 Settings / Tray / Desktop

- 偏好：外部终端选择（Terminal.app/Ghostty/cmux）、Dock/Tray、主题、刷新时间、Gateway first-response/request timeout、启动权限、代理和 Usage 维护。
- Dock 主窗口关闭可隐藏；托盘小窗失焦隐藏；Quit 会区分 GUI 退出和活跃 Gateway，会话运行中默认不静默杀连接。
- MyGo Native Updater、通知、原生 Dialog、密码框、文件选择、系统主题与多语言控制都使用框架官方能力。

## 6. 实体、枚举、错误码

| 实体 | 主键与关键字段 | 不变量 |
|---|---|---|
| Agent | `codex / antigravity` | 首发 Codex 完整；Antigravity 以 Quota 为主 |
| Profile | `profileID`, `agentID`, `home`, `authMode`, `managed`, `proxy` | Profile 所属 Home 唯一，PATH 规范化 |
| CredentialRef | `kind/service/account/version` | 不携带 Secret 明文 |
| Provider | `providerID`, `protocol`, `baseURL`, `models`, `capabilities`, `revision` | 协议和模型能力显式，未知值不回退默认 |
| Binding | `profileID`, `providerID`, `modelID`, `generation`, `routeKind` | 在 Config 和 Store 中有一致的已提交版本 |
| Session | `profileID`, `sessionID`, `path`, `cwd`, `modifiedAt`, `sizeBytes` | 单 session 唯一，不能跨 Home 解引用 |
| QuotaSnapshot | `profileID`, `source`, `windows`, `credits`, `fetchedAt`, `stale` | 不以本地 Tokens 伪装上游配额 |
| UsageEvent | `recordID`, `workspaceID`, `attributedAccountID`, `tokens...` | record idempotent，Token 计算不重记 |
| OperationPlan | `planID`, `kind`, `expectedRevisions`, `effects`, `warnings`, `fingerprint` | Plan 无 Secret，可复验后再执行 |

错误统一为 `{code, message, details?, recoverable, action?}`，不得把 Secret、完整 `auth.json`、Handoff 原始 Prompt 打进错误日志。稳定码至少覆盖：`PROFILE_NOT_FOUND`、`AUTH_EXPIRED`、`PROVIDER_NOT_READY`、`BINDING_CONFLICT`、`GATEWAY_UNAVAILABLE`、`GATEWAY_UNAUTHORIZED`、`PROTOCOL_LOSS`、`SESSION_NOT_FOUND`、`HANDOFF_CONFLICT`、`HANDOFF_COMPATIBILITY_BLOCKED`、`DATABASE_LOCKED`、`USAGE_INDEX_STALE`。

## 7. 公共业务接口（Go 形状，不是 UI/HTTP API）

```go
type AccountService interface {
    List(ctx context.Context) ([]Profile, error)
    PlanCreate(ctx context.Context, req CreateProfileRequest) (OperationPlan, error)
    Create(ctx context.Context, planID string, expectedRevision uint64) (Profile, error)
    SetProxy(ctx context.Context, id string, proxy ProxySettings) error
}
type SessionService interface {
    Query(ctx context.Context, q SessionQuery) (Page[Session], error)
    Read(ctx context.Context, profileID, sessionID string) (SessionDetail, error)
    PlanDelete(ctx context.Context, ids []SessionRef) (OperationPlan, error)
    Delete(ctx context.Context, planID string) (DeletionResult, error)
}
type HandoffService interface {
    Plan(ctx context.Context, src, dst SessionRef, opts HandoffOptions) (OperationPlan, error)
    Execute(ctx context.Context, planID, confirmation string) (HandoffResult, error)
}
type ProviderService interface {
    List(ctx context.Context) ([]Provider, error)
    PlanBind(ctx context.Context, req BindingRequest) (OperationPlan, error)
    CommitBind(ctx context.Context, planID string) (Binding, error)
    TestConnection(ctx context.Context, providerID string) (ConnectionCheck, error)
}
type QuotaService interface {
    Read(ctx context.Context, profileID string) (QuotaSnapshot, error)
    Refresh(ctx context.Context, profileIDs []string) ([]QuotaSnapshot, error)
}
type UsageService interface {
    Refresh(ctx context.Context, scope UsageScope) (IndexResult, error)
    Summary(ctx context.Context, query UsageQuery) (UsageSummary, error)
    Calls(ctx context.Context, query UsageQuery) (Page[UsageCall], error)
}
```

接口由实现者保持简单，Page/DTO 类型只在需要的包中共享；禁止因接口示例而建立自定义 IOC 容器、EventBus 框架、ORM 或复杂 RPC Server。

## 8. 约束与风险

- **Go 版本**：MyGo 官方文档要求 Go 1.27+。本次分析环境是 Go 1.23.2，不能在当前环境宣称 MyGo 应用已编译或实机验收。
- **MyGo 版本**：公开抓取的 Releases 列表与之前回答中的版本编号存在不同步。实际实施以 `go list -m -versions`、明确的正式 Tag 和 `go.mod/go.sum` 锁定为准，不接受浮动 `latest`。
- **SDK**：社区 `codex-sdk-go` 不是 OpenAI 官方 SDK；当 SDK 缺少官方 App Server 新方法时优先给 SDK 增加薄适配，不重建全套 JSON-RPC Client。
- **凭据**：MyGo Secrets 单条大小受限，旧格式中的长认证 JSON 不应简单整体塞入；新签名身份读取旧 Keychain Item 可能触发授权。
- **Gateway**：Responses 与 Chat 不完全同构，必须对自定义 Tool/namespace、流式取消、图片输入、reasoning 和历史进行契约验证。
- **许可证**：旧项目自身 MIT；`RelayKit`（AGPL-3.0）不能在未确认兼容发布方式前默认引入，优先 MIT 协议翻译库。
- **未来扩展**：不在首发增加完整 Claude Code / OpenCode 多 Agent 管理、远程多租户服务、账户共享或批量 Sessions Sync；领域对象可以预留扩展字段，但不开展无需求的实现。

## 9. 完成定义（DoD）

- 新仓库业务/GUI 全 Go，零 Rust `.rs`、零 Cargo、零 Tauri、零 React/TSX、零 JS frontend build，零 Rust helper 和 gateway binary。
- 旧版现行五主页面 + Tray 所有 P0 功能有可执行 Go 测试和人工验收；不复活废弃 Sync。
- `go test ./...`、`go vet ./...`、`go tool mygo vet`、原生 UI Tester、Gateway contract tests 全通过；macOS arm64 DMG 实机安装通过。
- Codex CLI 正常与异常、取消/掉线、Token Usage、Keychain、SQLite WAL、数据导入与回滚均经过真实样本测试。
- 发布构建的进程清单、App Resources、依赖与许可证清单可审计；用户数据迁移有恢复快照。

下一步开发严格按 `docs/03-implementation-plan.md` 和 `docs/04-test-spec-and-release.md` 执行。
