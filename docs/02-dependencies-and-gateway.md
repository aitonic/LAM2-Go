# LAM 2.0：依赖选型、Codex 接入与 Go Gateway 详细设计

> 本文是可执行技术规格，不是“最少依赖清单”。允许优质依赖，但不允许把完整第三方应用作为黑箱子服务嵌入。

## 1. 第三方依赖决策表

| 能力 | 推荐依赖 / API | 角色 | 选择结论 | 验证点 |
|---|---|---|---|---|
| 桌面 UI | `github.com/egoist/mygo`、`github.com/egoist/mygo/ui` | 必须 | 直接使用 | 构建、输入法、托盘与状态更新 |
| 图表 | `github.com/egoist/mygo/ui/chart` | 必须 | 直接使用 | Line/Bar/Heatmap/Tooltip 和键盘操作 |
| SQLite | `github.com/egoist/mygo/plugins/sqlite` | 必须 | 直接使用 | WAL / 大结果分页 / 事务 / 旧库 Schema |
| 内嵌终端 | `github.com/egoist/mygo/plugins/terminal` | 按功能 | 官方组件 | 打包 `libghostty-vt`，不自建 PTY |
| 更新器 | `github.com/egoist/mygo/plugins/updater/native` | 发布阶段 | 官方 Native UI 实现 | 不引 WebView Updater |
| 文件监控、Keychain、窗口 | MyGo Core API | 必须 | 直接使用 | 旧 Keychain 访问与权限 |
| Codex App Server | `github.com/pmenglund/codex-sdk-go` | 优先 | 社区 SDK，Apache-2.0 | 精确 CLI 版本兼容、初始化/关闭、并行 Profile |
| JSONL Parser | `github.com/alexgorbatchev/agent-parser` | 优先 | MIT，公开 Go API | 旧 LAM Token、事件和标题语义是否一致 |
| Responses/Chat 翻译 | `github.com/router-for-me/CLIProxyAPI/v8/sdk/translator` | 优先 | MIT，公开 Go Registry API | 初始化内置 Translator、Tool/Custom/Namespace、SSE |
| TOML 兜底 | `github.com/pelletier/go-toml/v2` | 按需 | 只用于官方 Config RPC 覆盖不到的场景 | 文本不破坏及兼容性 |
| 上游 HTTP 与 Server | Go 标准库 `net/http`、`context`、`slog` | 必须 | 标准库 | 连接复用/超时/取消/流式 |
| OpenAI API Client | `github.com/openai/openai-go` | 按需 | 官方 Client，但不当协议转换器 | 使用场景必须胜过直接 http.Client |
| Provider 费用数据 | 有明确来源与日期的外部 Price Feed | 按需 | 数据输入不是逻辑依赖 | 版本、币种、缺省和缓存价 |

**不用作默认依赖**：`new-api/relaykit` 虽然是可独立引入的 Go 协议转换模块，却采用 AGPL-3.0；现有 LAM 使用 MIT，发布模式未确认前不引入。不要把 `CLIProxyAPI` 整个网关当独立服务打包；只考虑它的公开转换 SDK，且必须检查实际编译后引入的全部依赖。`codexm` 与 `codex-usage` 等完整 Go CLI 作为参考，不能直接导入其 `internal/` 业务包。

所有依赖均在 Phase 0 用固定 Tag + `go.sum` 锁定。避免 `@latest` 出现在生产 go.mod；移除未使用依赖；生成 SPDX/许可证归档。MyGo 具体 Tag 以实际 Go Module 和 GitHub 正式 Tag 交叉验证，避免抓取缓存导致误判。Go 工具链使用 **1.27+**。

依赖来源：
- MyGo: https://mygo.egoist.dev/docs/getting-started 、https://mygo.egoist.dev/docs/plugins/sqlite
- Codex SDK: https://github.com/pmenglund/codex-sdk-go 、https://pkg.go.dev/github.com/pmenglund/codex-sdk-go
- Agent Parser: https://pkg.go.dev/github.com/alexgorbatchev/agent-parser
- CLIProxy Translator: https://pkg.go.dev/github.com/router-for-me/CLIProxyAPI/v8/sdk/translator
- RelayKit: https://github.com/QuantumNous/new-api/blob/main/relaykit/README.md
- App Server: https://developers.openai.com/codex/app-server

## 2. Codex App Server Adapter 设计

### 2.1 对应关系（不是自主实现完整 JSON-RPC）

| 业务操作 | 官方 App Server 能力 | LAM 额外业务 |
|---|---|---|
| 登录 | `account/login/start`、`account/updated` | Profile 身份、PAT 切换、UI 可视化 |
| 额度/Reset Credits | `account/rateLimits/read`、`account/rateLimitResetCredit/consume` | 缓存、过期与多 Profile 汇总 |
| 上游账户用量 | `account/usage/read` | 与 LAM Local Usage 并列、Delta |
| Session 列表 | `thread/list`、`thread/read`、`thread/turns/list`、`thread/items/list` | 跨 Home 聚合、旧 JSONL 回溯、归属 |
| Session Fork | `thread/fork` | 不等价跨 Home Handoff，不能简单替换 |
| 配置 | `config/read`、`config/value/write`、`config/batchWrite` | Provider/Binding 多资源事务与版本协调 |
| 模型 | App Server Model API | Provider 自定义 Model Catalog、兼容状态 |

### 2.2 每个 Profile 独立进程

一个 `CODEX_HOME` 对应一个 `CodexClientSession`，以显式环境变量启动 `codex app-server`。资源池键是规范化的 Profile ID/Home，而不是全局 Current Account。UI 同时刷新多个账号时每个独立客户端保持自己的认证环境。

- SDK `codex.New` 在创建时进行 Codex CLI 版本兼容性检查；不能默默忽略。版本不支持则 `CODEX_VERSION_UNSUPPORTED`，不采用全局 `Ignore` 模式。
- 短任务可在请求结束后回收；常用 Profile 允许有界复用。UI 关闭时子进程完成取消与 Close，不能留僵尸进程。
- `CODEX_HOME` 指向账号专属目录，不复制 `auth.json`；不会把一个 Profile 的 Token 传入其他 Profile 的 App Server。
- SDK 暴露高阶 API 不足时，通过其公开 `Client()/rpc` 对官方方法做薄封装，**不另写通用 JSON-RPC Client**。
- 官方 App Server 如遇 Protocol 变更，以实际 CLI 版本重新生成/校验 Contract Fixtures。

## 3. Go Gateway 设计

### 3.1 职责边界

Gateway 不是 Provider Manager，不负责 UI、数据库、模型调度策略和账单；它承载 LAM 已绑定的特定 Provider 的协议访问：

`Codex CLI [Responses] -> loopback Go Gateway -> [Chat Completions] Upstream`

而 `Responses` 原生 Provider 的推荐路径是 `Codex CLI -> Provider` 直连，不应平白经过一层 Gateway。

Gateway 需要：身份绑定、健康与模型列表、协议转换、流式、工具调用映射、上游 HTTP、错误映射、容量、日志、运行状态。禁止为了扩展性而实现全新通用 AI Gateway 框架或多租户账户系统。

### 3.2 HTTP 接口

| 路由 | 使用者 | 行为 |
|---|---|---|
| `GET /health` | Desktop/Supervisor | challenge-response 身份确认，防止只凭端口认为是自己的进程 |
| `GET /v1/models` | Codex/健康面板 | 返回当前 Binding 可用模型，不泄漏其他 Profile Models |
| `POST /v1/responses` | Codex CLI | 同一 Binding 的认证、合法性检查、Responses 转 Chat、响应反向组装 |
| `POST /v1/responses/compact` | 仅当 CLI 真正需要且兼容时 | 不明确支持则显式拒绝，绝不隐式转发到错误端点 |

其他 CLI 实际使用路由必须通过 Codex Contract Capture 确认后加入；不能为了显得完整随意开放 `/v1/*`。

### 3.3 Middleware 顺序

1. Loopback Listener（仅 IPv4 `127.0.0.1`，必要时双栈单独校验；不监听公共网卡）。
2. Request ID + 结构化日志（不记录请求体/密钥）。
3. Content Type / 方法和路径过滤 / Body Size Limit。
4. Gateway Binding Token 验证（固定时间比较、绑定 Profile 与 Generation、拒绝跨 Binding 串用）。
5. 全局与 per-Binding 并发/排队限制。
6. 解析、schema 验证、能力预检。
7. Translator Request 转换 + 上游请求 + Stream 或 Nonstream 转换。
8. Usage Metrics / TTFT / Completion / Cancellation / Error Code。

启动预算以旧版实测上限作为初始基线：全局并发 32、per Binding 并发 4、全局排队 64、per Binding 排队 8、HTTP Body 4 MiB、SSE Frame 256 KiB、Tool Args 1 MiB、128 Tools；从旧版源码提取到 `gateway_limits_test.go`，调大需测得吞吐与内存改善。不要凭空声称 Go 的吞吐更高。

### 3.4 转换器集成

首选 CLIProxyAPI v8 `sdk/translator`，但**必须**验证两件事：

1. 导入其 `sdk/translator/builtin` 或相应注册入口后，实际存在所需方向的 Request、Nonstream 和 Stream Transformer。不能因为 Registry 类型存在就假定方向已经注册。
2. 旧 Gateway 支持的 Tool Call、`additional_tools`、custom tool、namespace tool、reasoning、image input 和多轮历史不能被新包静默丢弃；插件转换结果需要按旧 Contract Fixtures 逐一对比。

如果有明确的损失：在 LAM 自身 Adapter 层增补必要方向的转换、保留源字段与 context；暂时无法语义等价的请求在 Attach 预检或请求阶段返回 `PROTOCOL_LOSS`。**既不全量自研 Translator，也不允许无感字段丢失。**

### 3.5 SSE 状态机（核心要求）

- 每个 HTTP request 的状态独立：`created -> in_progress -> output_item -> text/tool deltas -> terminal`，每个需要的事件必须与实际 Codex fixtures 的格式/顺序对应。
- 对 Tool Args 片段累积并作长度上限、JSON 合法性及 call ID 匹配；工具调用输出和后续输入必须可 Roundtrip。
- SSE `data:` 可跨 HTTP chunk 任意拆分；连续 event、无空行、UTF-8 边界、注释、未知 event 均应按协议处理。
- Terminal 恰好一次；客户端取消、上游 EOF、心跳、网络重试、并发条件下不产生重复 `response.completed`。
- 从上游非流式响应转换到目标非流式 DTO，不将错误 HTML 或无效 JSON 混入成功输出。
- 应按源协议合法语义映射 Usage、Reasoning、Finish Reason；不可通过“注入空 content”掩盖不支持的工具。
- 对响应传输实施背压和有界缓冲；客户端断连必须传播 `context` 取消到上游 HTTP Request，释放 Semaphore。
- TTFT 按首个**真实模型数据**与请求开始计算，流式终止/取消时能正确结束 Request 统计。

### 3.6 安全认证

- Binding Token 与上游 API Key 分离。Binding Token 只用于 Loopback 认证，Keychain 存上游 Secret 的版本化引用。
- Gateway Secret 从批准的引用取得；HTTP Header 和 URL Query 参数做非敏感字段校验，禁止把 Credential 放入 query string 或 Config 明文。
- `auth --binding` 不经过 Shell 字符串拼接；只输出单 Token，stdout 不含附加日志/换行内嵌。
- Auth Command 为可执行路径和 `argv`，经白名单/显式批准；不接受来自任意远端 Provider JSON 的隐式 shell 执行。
- 旧 Keychain `service = lam.remote-provider` 的现存引用优先兼容，修改签名后做真实访问测试。
- 调试与审计只能记录引用 ID、请求 ID、错误码、耗时，不能记录 Bearer 值、Prompt、完整 Tool Args。

### 3.7 进程与故障恢复

`GatewaySupervisor` 不是桌面 UI 子组件；同一 `lam` binary 在 `gateway` 模式重新 exec。Supervisor 管理 PID + executable identity + nonce + listen address + generation：

- Desktop 确认当前 gateway 所属程序和 Binding；同端口陌生进程不得 kill。
- 启动必须先 reserve/claim，再完成健康握手后标记 READY；若端口冲突，返回具体错误并建议显式迁移端口。
- Gateway 遇到上游不可用时保留服务并返回上游错误；不能将失败自动伪装成模型响应。
- 崩溃恢复读取本地事务记录和旧状态，但不会无条件恢复不再存在的 Keychain 引用或未验证 Binding。
- 活跃连接期间主窗口/Tray 关闭不破坏 CLI；显式 Quit 是否同时停止 Gateway 由 UI 明示，缺省优先保护活跃 CLI 会话。
- 单个 Gateway 可支持多个 Binding 的前提是鉴权/容量/会话历史的完全隔离；初版可按旧版运行模型分组，但不允许设计成每次请求无限启动新进程。

## 4. 数据与可重复性

- `gateway_binding_snapshot`（RAM）在请求开始时冻结，避免密钥轮换中半途变更上游。
- `config.transaction` 持久化阶段：`planned/prepared/applied/verified/rolled_back`；启动恢复时不依赖前端是否在线。
- 模型与能力声明存储版本戳；未知能力不当作支持，明确 `unknown`。
- 所有 Provider/API Account 写操作须 expected revision；失败保证没有半提交配置。必要的临时文件位于原目录同一卷。
- Gateway 请求日志按请求 ID 与 Binding ID 可筛选，默认不记录原始内容。对性能上限保存基准脚本，不以调参替代协议验证。

## 5. 外部升级、许可证与替代路线

- **Codex CLI**：官方协议是演进中的；SDK 当前分支随 Codex 版本更新。必须固定可用的 CLI+SDK pair 并记录最后一次 Compatibility Capture 日期。严禁将固定版本检查全局设为 Ignore。
- **CLIProxyAPI Translator**：公开 API 与模块版本可能升级；升级必须过协议 Goldens。若 Tool Namespaces 无法正确转换，实施一个定向 Go Patch，不引入完整 CLIProxyAPI Server。
- **RelayKit**：仅当项目发布策略可以满足 AGPL-3.0 时再考虑作为替代；不要同时引入两套转换器。
- **Agent Parser**：它规范化原始事件，不负责持久化本项目的多账号归属、增量 Offset 和价格置信度；这些由 LAM 自己实现。

## 6. 验收矩阵索引

| 目标 | 测试编号（详见 `04-test-spec-and-release.md`） |
|---|---|
| 两个 `CODEX_HOME` 严格隔离 | CDX-001..006 |
| 认证失效/SDK 与 CLI 版本冲突 | CDX-007..010 |
| Provider Attach / Revision / Keychain | PRV-001..012 |
| Responses 到 Chat / Custom Tools / SSE | GW-001..020 |
| Gateway 并发/端口/生命周期 | GW-021..032 |
| Antigravity 配额与 stale | QTA-001..008 |
| Token 归属和价格 | USE-001..016 |
| 跨 Profile Handoff / 安全删除 | SES-001..016 |

结论：以包/SDK 消除通用代码，但 LAM 自身的领域事务、归属与设备运行策略必须完整实现；不能把“现成包丰富”误解为“完全无需自己写服务”。
