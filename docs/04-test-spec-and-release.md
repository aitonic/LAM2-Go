# LAM 2.0：测试规格、兼容性验证、迁移和发布

> 规则：**先落 Contract Tests，再落实现代码**。旧版通过并不等于新版自动通过；每一条“通过”都需要 Go 测试报告或 macOS 实机证据。

## 1. 测试层级

| 层级 | 使用工具 | 目标 | 执行节奏 |
|---|---|---|---|
| Domain Unit | `go test ./...`、Table-driven tests | 领域不变量、错误路径、类型边界 | 每提交 |
| SDK/Parser Compatibility | Fake App Server / 小型 JSONL fixtures | 固定 Codex SDK、Parser 行为 | 每依赖变更 |
| Gateway Golden | `httptest.Server`, JSON/SSE fixtures | 协议、工具、错误、取消与认证 | 每提交 |
| SQLite Integration | MyGo SQLite + temp directory | WAL、schema、索引、事务、旧库查询 | 每存储改动 |
| Native UI | `ui.NewTester` + MyGo `vet` | 路由、列表、对话框、状态/异步异常 | 每 UI 改动 |
| Desktop System | macOS arm64 签名 `.app`、DMG | Tray、Dock、Keychain、终端、辅助进程 | 发布前 |
| Performance/Soak | Fake upstream、并发请求、可复现样本 | 负载、背压、内存和进程恢复 | Gateway 版本/发布前 |
| Upgrade/Rollback | 旧 LAM 快照 + 新 `~/.lam-next` | 不丢数据、不污染旧安装 | 发布前 |

旧测试与 Fixture 根：`apps/desktop/src-tauri/tests/`；`apps/desktop/src-tauri/tests/fixtures/`。应原样保留原始样本副本的 Hash，并仅在已脱敏的 Go fixtures 中运行测试。部分旧 fixture 本来就是合成样本，不以它替代现场真实数据测试。

## 2. Codex/App Server 契约

| ID | 输入与动作 | 预期结果 |
|---|---|---|
| CDX-001 | 两个不同 `CODEX_HOME` 同时启动 SDK | 用户身份、认证文件、会话互不混用 |
| CDX-002 | 两 Profile 同时 `thread/list` | 每个列表只包含自己的 thread |
| CDX-003 | App Server 上游通知 account rate limits | 对应 Profile 状态刷新，其他 Profile 不变 |
| CDX-004 | 读取 Config、BatchWrite 单一 Profile | 仅相应配置被修改 |
| CDX-005 | 进程正常关闭/取消 | SDK 子进程退出，无僵尸/泄露 |
| CDX-006 | 高频并行查询 | 保持有界连接/进程，UI 不阻塞 |
| CDX-007 | CLI 缺失 | `CODEX_NOT_FOUND`，无盲目创建无效认证 |
| CDX-008 | CLI 版本不匹配 | 明确版本冲突，不静默忽略 SDK 兼容检查 |
| CDX-009 | PAT 与 Profile 两种身份路径 | 显示与隔离语义一致，不互相覆盖 |
| CDX-010 | 过期认证或未登录 | 状态 unavailable，错误脱敏且有可执行操作提示 |

## 3. Provider / Auth / Config 契约

| ID | 输入与动作 | 预期结果 |
|---|---|---|
| PRV-001 | 导入旧 Provider V2 JSON | ID、Model、Capability、Credential Ref、Revision 完整 |
| PRV-002 | 导入旧 legacy provider fixtures | 兼容转换不丢未知值，必要时拒绝 future schema |
| PRV-003 | Provider CRUD | 不影响其他 Provider、Binding |
| PRV-004 | Discover models、测试上游 | 不隐式修改 Provider 或凭据 |
| PRV-005 | Attach `responses` Provider | Codex 直连，配置模型与 secret 引用准确 |
| PRV-006 | Attach `chat_completions` Provider | Gateway ready、Binding Token、URL 一致 |
| PRV-007 | Gateway 未就绪 | 无假成功，原 config 仍然可用 |
| PRV-008 | 两个线程同一 Revision 提交 | 只有一个写入成功，另一返回 Revision Conflict |
| PRV-009 | 修改 Config 中断 | Journal 恢复原值或确认完整新值，无半提交 |
| PRV-010 | Rotate Keychain Secret | 旧绑定和新绑定版本关系正确，日志无 Secret |
| PRV-011 | Unicode、CRLF、注释、用户自定义 TOML | 不丢失不相关字段和用户格式（官方 API 不保证处须验证） |
| PRV-012 | Auth Command 非批准路径/修改文件 | 运行被拒绝，敏感 stdout/stderr 不泄漏 |

映射旧测试：`provider_v2.rs`, `provider_api_v2.rs`, `provider_attach_transaction.rs`, `provider_config_editor.rs`, `provider_keychain.rs`, `provider_credentials.rs`, `provider_gateway_binding.rs`。

## 4. Gateway 完整行为契约

| ID | 场景 | 预期 |
|---|---|---|
| GW-001 | 纯文本非流式 Responses -> Chat -> Responses | 内容、Model、Usage、完成状态一致 |
| GW-002 | 纯文本流式多 chunk | Responses SSE 事件顺序正确，终止一次 |
| GW-003 | 输入被切为任意 HTTP/SSE 分片 | 生成内容不变，不丢 UTF-8 字节 |
| GW-004 | Function Tool 首轮调用 | `call_id`、name、args 不丢 |
| GW-005 | Tool Result 第二轮发送 | 与上轮 call 对应且可以完整回放 |
| GW-006 | 多 Tool Calls / Interleaving | 分别累积 args，顺序与 ID 不串用 |
| GW-007 | `additional_tools`、namespace tool | 语义保留；若无法保留，则明确 `PROTOCOL_LOSS` |
| GW-008 | custom tool / tool_choice | 已支持能力正确映射，否则可诊断地拒绝 |
| GW-009 | DeepSeek Thinking + reasoning history | 推理片段和多轮关联按旧约定处理 |
| GW-010 | 图片或多模态输入 | 能力足够才转发，不能静默变文本 |
| GW-011 | JSON Schema / structured output | 能力和限制明确，无 silent downgrade |
| GW-012 | 未知合法新字段 | 保留或返回可诊断的兼容性错误 |
| GW-013 | 400/401/403/429/5xx 上游响应 | 状态和 Retry-After 不被成功响应覆盖 |
| GW-014 | HTML 错误页、非 JSON body | 稳定错误 DTO，不出现 SSE success |
| GW-015 | SSE 中途截断/缺少 terminal | 错误终止，释放资源，不伪造完成 |
| GW-016 | 上游完成后重复 terminal 片段 | 下游只收到一次合法终止 |
| GW-017 | 空 delta / heartbeat / 注释 | 不错误计入 TTFT/Usage，状态保持合法 |
| GW-018 | 极长 Tool Arguments / 超大 SSE Frame | 在明确上限拒绝，内存保持有界 |
| GW-019 | 下游关闭连接 | 上游 `context` 取消，释放队列与 semaphore |
| GW-020 | 流式 vs 非流式相同语义 | 内容和工具序列一致，允许不同 Event 形态 |
| GW-021 | 不带 Binding Token | 401，无任何上游访问 |
| GW-022 | 使用 Profile A Token 访问 B Binding | 拒绝跨 Binding 泄露 |
| GW-023 | Key 轮换过程中已有请求 | 使用请求开始时冻结的正确 Credential 版本 |
| GW-024 | 同时 32 请求 / per Binding 4 | 有界执行，其余按旧配置排队或拒绝 |
| GW-025 | 超过全局/局部队列上限 | 稳定拒绝，记录结构化原因 |
| GW-026 | Port 已被陌生进程占用 | 不 kill、不劫持、不宣称 ready |
| GW-027 | Gateway 旧进程遗留、二次启动 | 验证可执行文件身份后复用/恢复，避免孤儿 |
| GW-028 | 启动失败/崩溃恢复 | 保留绑定状态诊断，按可重试性安全恢复 |
| GW-029 | GUI 被隐藏、窗口关闭 | 活跃 CLI 请求不受影响 |
| GW-030 | GUI 真正 Quit 后外部 CLI 仍活跃 | 按策略维护 Gateway，直至显式停止/安全退出 |
| GW-031 | 上游 TTFT / 全请求超时 | 分别正确分类，取消和错误处理无泄露 |
| GW-032 | Health challenge-response | 不将陌生监听器误判为本项目 Gateway |

映射旧测试：`provider_adapter_request.rs`, `provider_adapter_nonstream.rs`, `provider_adapter_sse.rs`, `provider_adapter_tools.rs`, `provider_adapter_deepseek.rs`, `provider_gateway_routes.rs`, `provider_gateway_server.rs`, `provider_gateway_upstream.rs`, `provider_gateway_identity.rs`, `provider_gateway_supervisor.rs`, `provider_phase3_g4.rs`，以及 `fixtures/codex-gateway-contract/`、`fixtures/provider-adapter/`。

## 5. Sessions / Handoff / Safe Delete

| ID | 场景 | 预期 |
|---|---|---|
| SES-001 | 旧 Codex JSONL 发现和筛选 | Session ID、Profile、更新时间、名称正确 |
| SES-002 | Cursor 分页/Filter 改变 | 无跨页重复、排序稳定 |
| SES-003 | 文件非法 UTF-8 / 异常行 | 跳过可容错行，保留诊断，不整页崩溃 |
| SES-004 | Resume 指定 Session | 命令指向正确 CODEX_HOME 与 session ID |
| SES-005 | Relay Latest | 精确接力当前源最近一次会话，不打包整账号 |
| SES-006 | 源/目标为同一 Profile | 拒绝，不产生写入 |
| SES-007 | 目标 session 路径已经存在，内容相同 | 幂等，不发生无意义覆盖 |
| SES-008 | 目标已 diverged | 需要明确策略；无策略时零写入 |
| SES-009 | Provider/Model 不兼容 | Block 或显式确认兼容性损失 |
| SES-010 | Compatibility Fingerprint 在确认前变化 | 取消执行并要求重新确认 |
| SES-011 | 临时文件写入过程中进程中断 | 原目标不损坏，可恢复/清理临时文件 |
| SES-012 | session path symlink/path traversal | 拒绝越过 sessions 根目录 |
| SES-013 | Handoff 过程中检查目标认证目录 | `auth.json` 未修改、未复制 |
| SES-014 | 删除最近 20 或最近 7 天 Session | 常规清理排除/保护且解释原因 |
| SES-015 | 已删除 Session 索引残留 | 下次刷新一致，不能显示不存在的文件可 Resume |
| SES-016 | 文件变更 Watch overflow | 重扫并收敛到实际文件状态 |

## 6. Quota / Usage / Pricing

| ID | 场景 | 预期 |
|---|---|---|
| QTA-001 | 有 primary/secondary 两窗口 | 百分比、重置时间、window duration 正确 |
| QTA-002 | 只有一窗口或 unknown | 不能虚构另一窗口 |
| QTA-003 | 获取失败/过期缓存 | UI 明确 stale/unavailable，非 0% |
| QTA-004 | Reset Credit 只有 availableCount | 不编造 expiry 或详细列表 |
| QTA-005 | Reset Credit 幂等重试 | 不发生两次实际消费，重新拉限额 |
| QTA-006 | Antigravity 本地服务不存在 | 明确 unavailable，不影响 Codex Quota |
| QTA-007 | Antigravity 分组变化、模型缺失 | 只读解析并保留来源标记 |
| QTA-008 | Overview 与 Tray 同时刷新 | 同一数据快照，不发生 UI 值冲突 |
| USE-001 | JSONL 同一事件重复出现 | `recordID` 去重，Token 不重复加总 |
| USE-002 | SourceFile append、字节 Offset | 只解析新增数据，账目与全量一致 |
| USE-003 | 文件被截断/替换/归档 | 安全重建或状态迁移，不丢归属 |
| USE-004 | `input=100, cached=40` | `uncached=60`，total 不双倍加 cached |
| USE-005 | reasoning tokens | 是 output 的子集时不重复计入总量 |
| USE-006 | 各 Profile 归属 | 同名 Session ID 在不同 Profile 不混账 |
| USE-007 | 工作区归属不确定 | `unknown` 保留，不乱分配给当前账号 |
| USE-008 | 旧 Usage SQLite 导入 | 表/索引/数据聚合口径核对通过 |
| USE-009 | 单价未知 | Cost unknown，不默认 0 美元宣称免费 |
| USE-010 | 自定义 Model/Price | 价格版本、币种和日期可查 |
| USE-011 | Local vs Account Usage | 同作用域才算 Delta，差异解释可观察 |
| USE-012 | 多天日期与时区边界 | 日/周汇总不跨日错位 |
| USE-013 | 巨量 Session/Calls 分页 | UI/数据库内存有界，不卡主线程 |
| USE-014 | 数据库占用/损坏 | 返回错误并保留重建路径，不损坏旧库 |
| USE-015 | Reset/Compact 索引 | 可取消、有进度、可恢复，不删除原始会话 |
| USE-016 | User filters/insights | 显示结果与同条件 SQL/fixtures 一致 |

## 7. MyGo Native UI 测试

- UI-001：五个 Router 路由、前进/返回、Sidebar 当前项、Keyboard shortcuts。
- UI-002：Overview 两个 Account Cards，Quota 正常/stale/failed 三态。
- UI-003：Sessions 虚拟化 Table 10,000 行，排序/筛选改变后选中项保持正确。
- UI-004：Provider Dialog 添加/编辑/失败/提交，确认弹窗不能在未提交时显示成功。
- UI-005：Usage Charts 轴、Tooltip/键盘、数据为空、未知价格，深浅主题可读。
- UI-006：Tray 失焦、显示/隐藏、Open Main、短暂网络失败时仍可操作。
- UI-007：中文 IME、复制/粘贴、可访问性 Label、键盘焦点；UI 线程不跑长数据库任务。
- UI-008：MyGo `ui.NewTester` 不留 `ui.Element` 于 struct/goroutine，`go tool mygo vet` 通过。
- UI-009：MyGo Native Updater 的窗口不启动 WebView；更新失败/取消行为可恢复。
- UI-010：内嵌终端仅用 MyGo Terminal；外部 Ghostty/cmux 不误认为内嵌终端实例。

## 8. 数据迁移与回滚测试

### 8.1 迁移前准备

1. 保持旧 LAM 原版安装可启动；退出旧程序并确认旧 Gateway / SQLite 写入已停止（否则首先使用 SQLite 一致性 Backup API，而不是直接复制单个 `.sqlite3`）。
2. 创建旧 `~/.lam`、可能的 `~/.config/agent-workspace`、App Support provider-hub、相关 `CODEX_HOME` 的只读清单（Hash + File Metadata + schema）；记录当前 Keychain service/account ref，不导出 Key 明文。
3. 新版只写 `~/.lam-next`，维护 `migration_manifest`（源版本、目标版本、checksum、已完成阶段、Rollback Marker）。
4. Provider JSON 和 Binding 验证后方可导入；旧 Usage 数据优先由安全 SQLite Snapshot 导入，并允许通过原 JSONL 重建。
5. 只有新数据、SDK Profile、Keychain、Handoff 和 Gateway 复验通过后，用户显式启用新 LAM 作为主要入口。

### 8.2 失效注入

- MIG-001：源目录不存在 → 新版干净安装仍可启动。
- MIG-002：Provider JSON future version → 明确拒绝，不降级重写。
- MIG-003：旧 SQLite WAL 未 checkpoint → 不允许不一致的文件级复制。
- MIG-004：Keychain 被签名权限拒绝 → 保留引用、UI 提示授权，不创建假的空 Key。
- MIG-005：导入到一半进程退出 → 可安全重试，不重复导入。
- MIG-006：两个应用误同时写一个 Codex Config → 检测冲突，不静默覆盖。
- MIG-007：新 Gateway 占用旧端口 → 不 kill 老进程，提供切换策略。
- MIG-008：新版本回退 → 原旧版和旧 `~/.lam` 数据可以重新使用。
- MIG-009：旧数据库索引部分丢失 → 可从 JSONL 重建，本地 Usage 不永久丢失。

### 8.3 回滚

1. 停止新版 GUI 的写事务、对新 Gateway 发优雅停止并等待活跃请求完成/显式确认终止。
2. 对由新版修改过的 Codex `config.toml` 恢复带校验的备份；不覆盖近期来自别的工具的并发更改。
3. 恢复旧 LAM 执行入口；不把 `~/.lam-next` 的数据库强写回旧 `.lam`。
4. 如有新建的 Profile，保留它们独立目录，用户决定是否导出或手动接回。
5. 生成回滚报告，指明哪些状态未恢复以及原因，避免“回滚成功”但实际不一致。

## 9. CI、发布和依赖验证

| 作业 | 平台 | 内容 |
|---|---|---|
| `unit` | Linux | `go test ./...`、静态检查、Race（能运行的模块） |
| `contract` | Linux | fake Codex App Server、SSE Goldens、旧 JSON fixtures |
| `native-ui` | Linux / macOS | `ui.NewTester`、`go tool mygo vet` |
| `macos-smoke` | macOS arm64 | Signed app、Tray、Keychain、Codex CLI 与 Terminal 验收 |
| `release` | macOS | `go tool mygo build`、签名与公证、DMG、更新签名、校验和 |
| `supply-chain` | Linux | Go Module 固定版本、许可证、SBOM、Rust/JS/Tauri 禁止项扫描 |
| `migration` | macOS arm64 | 离线只读旧数据快照迁移、再执行回滚测试 |

**最终发布接受条件**：全部 GW/CDX/PRV/SES/QTA/USE/UI/MIG 测试通过；Go 和 MyGo 版本记录，实际应用包审计没有旧 Rust/Tauri/React；macOS 真机可用；签名/公证/更新与回退均完成。

**未能验证的内容必须如实标记**：在本次分析环境（Go 1.23.2 / Linux）无法编译要求 Go 1.27+ 的 MyGo Native App，也没有用户 macOS 真机与真实已授权 Codex Profile；因此此文档属于待执行的完整测试规格，不是现有测试结果。
