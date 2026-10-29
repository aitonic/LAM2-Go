# LAM 2.0 — 逐项功能测试覆盖审计（2026-10-28）

## 结论

**新硬性标准：每一个标为 `DOMAIN_PASS` 的功能，必须绑定具体的 `package:TestName`，且该测试在本次真实 `go test -json` 执行中通过。缺少测试、被跳过、测试失败、测试包最终失败、只存在源码/文件名，都不得算 PASS。**

- 冻结的基线是 239 项（`CMD-001..111` + `FLOW-001..128`），不可通过删除旧条目规避门禁。
- 先前审核的 127 项 `DOMAIN_PASS` 中，**125 项目前有直接指向相关行为并实际运行的 Go 测试，2 项 (`CMD-033` 非阻塞刷新、`CMD-071` 旧 Provider 兼容转换) 缺少等价测试，降级为 `TEST_GAP`**。这不代表 126 项都做过完整端到端验证。
- **82 项** Native/External、**23 项** PARTIAL、**6 项** GAP、**2 项** TEST_GAP 和 **1 项** EXCLUDED 均明确保留，不得凭测试存在改称已对齐。
- 测试结果分布：`TEST_PASS` **125**、`PARTIAL_TEST_PASS_FEATURE_INCOMPLETE` **13**、`DOMAIN_ONLY_NOT_NATIVE_VERIFIED` **14**、`TEST_NOT_EXECUTED` **35**、`TEST_MISSING` **51**、`EXCLUDED` **1**。
- 当前项目内可检索到 **206 个 Go 测试函数**（包含依赖/原生测试）；当前离线 JSON 测试流中有 **186 个顶层测试 PASS**。不能把 206 算成全部执行，也不能把 186 除以 239 得出“功能覆盖率”。

**发布验收仍不通过**；还剩 **113 条非排除项目缺少完整层级证据**，其中 51 项当前没有关联到具体可执行测试，35 项关联的测试未在本环境执行。状态与逐项原因见同目录 `FEATURE-TEST-TRACEABILITY.csv`。

## 可重复的自动化门禁

```sh
./tools/check-feature-tests.sh domain
./tools/check-feature-tests.sh release
```

`domain` 会重新执行隔离的 Go 业务测试，生成 `reports/feature-pass-events.jsonl`，随后校验冻结 239 项 ID、每个 DOMAIN_PASS 的具名测试存在、实际测试运行 PASS 且整个 package PASS。输入与结果在 `tests/feature-contracts.csv` 和 `reports/feature-test-results.csv`；不会使用上一次的“全绿”日志。

`release` 依赖 Go 1.27.3+ 和真实 MyGo 依赖的 `reports/native-tests.jsonl`，以及 macOS 系统手动验收产物 `reports/system-acceptance.csv`。缺少任何证据即返回非零。`reports/system-acceptance.csv` 当前只有表头，**没有伪造任何手动验收 PASS**。`DOMAIN_PASS` 不自动等于 Native PASS，`PARTIAL/GAP/TEST_GAP` 永远不能由“有关联测试”直接升级成 COMPLETE。

门禁自身具有 4 个 Go 单元测试，覆盖：不存在的测试目标、未执行测试、跳过或失败的子测试、失败或未完成的包、重复 ID 以及 `PARTIAL` / 原生层级不可自动提升的约束。

## 本轮新增行为测试

- `account:TestEditPersistsNoteRenewalAndProxyWithRevision`：真正编辑并回读备注、续期、代理；拒绝旧 revision 和非法日期。
- `account:TestProfileTokenExpirationStatusTransitions`：未知、已过期、7 日内、30 日内、有效以及账号不存在。
- `quota:TestCachedQuotaReadAndStaleTransition`：未获取不能显示零、缓存不调用上游、过期仍保留数据且标 stale。
- `provider:TestProviderListAndDeleteRequireConfirmation`：列表排序、删除二次确认、删除一个 Provider 不影响另一个。
- `provider:TestApproveAuthCommandRequiresExecutableAndExplicitConsent`：非绝对路径、未批准与持久化批准记录。
- `commands:TestNoRustReactTauriApplicationSource`：扫描工程禁止旧 Rust、React、Tauri 源码和构建配置。

进一步补齐严格行为覆盖：

- `settings:TestAccountModeReadWriteAndInvalidValue`：Profile/PAT 模式真正保存回读与非法值拒绝。
- `settings:TestGatewayTimeoutSettingsReadWriteAndBounds`：首响应/总请求超时合法区间与数据库 CAS 回读。
- `settings:TestLaunchPermissionPresetSettingsReadWriteAndRejection`：权限审批、沙箱级别持久化与非法策略拒绝。
- `settings:TestAntigravityPortSettingsReadWriteAndBounds`：端口范围、关闭与越界回滚。
- `settings:TestDockVisibilitySettingsReadWriteAndCAS`：Dock 显示切换与过期 revision 拒绝。
- `session:TestMultiProfileQueryStablePaginationSortAndDetails`：两个 Profile 的列表聚合、三类排序、分页不重叠、详情与读取隔离。
- `session:TestStorageSummaryCountsBytesAndProtectedRanges`：StorageSummary 计数、文件体积、删除保护，未知账号报错。
- `provider:TestPerModelCapabilityPreflightUsesExplicitSupportAndUnknown`：Model 层覆盖与未知/不支持能力显式处理。
- `usage:TestRawCallSelectedContextAndMissingSource`：所选原始 Call 上下文、缺失源文件、路径隔离。

此前 Settings 的 12 项全部误指向只检查主题和刷新周期的宽泛测试，现已替换为与各字段直接相关的测试。Session 的多账号聚合/详情/排序同样更换为直接行为测试。`CMD-071` 经复核缺失旧格式专用等价测试，因此从 127 项历史通过中再降级一项，**不应为了提高绿色数量而保留不真实的通过状态**。

在 **真实 MyGo SQLite 插件依赖**的集成测试文件中还增加了：

- `mygosqlite:TestScopedDashboardQueriesAndPaginationAgainstMyGoSQLite`（尤其 `CMD-035/036/037/039/042/043`，针对相同 Profile 的 Summary/Calls/Threads/Daily 查询与分页一致性）。
- `mygosqlite:TestNativeUsageCompactAndResetPreserveRebuildableReadModel`（尤其 `CMD-047`）。

**以上两个 MyGo SQLite 测试未在当前 Go 1.23.2 环境执行，不得称 PASS。** `CMD-038 get_usage_scopes` 仍没有直指其完整业务契约的测试，已移除之前错误的 checkpoint 关联。

## 原版功能状态与测试状态（两套独立维度）

| 旧/当前状态 | 项数 | 说明 |
|---|---:|---|
| DOMAIN_PASS | 125 | 业务测试具名且本次执行通过，但非端到端 |
| NATIVE_UNVERIFIED | 82 | 功能实现有源码，原生或外部边界尚未验证 |
| PARTIAL | 23 | 有子功能，但原版观察行为不全 |
| GAP | 6 | 未实现或无功能等价性 |
| TEST_GAP | 2 | 原先误标为对齐，缺少等价行为测试 |
| EXCLUDED | 1 | 旧版已废弃的批量 Session Sync |

## 按领域计数

| 领域 | 项数 | 具名域测试 PASS | 测试缺失 | 仍未完全验收 |
|---|---:|---:|---:|---:|
| Legacy IPC | 111 | 76 | 7 | 35 |
| Provider | 17 | 11 | 3 | 6 |
| Tray | 13 | 0 | 3 | 13 |
| Usage | 31 | 15 | 12 | 16 |
| 会话 | 16 | 9 | 6 | 7 |
| 导航 | 6 | 0 | 3 | 6 |
| 平台 | 9 | 1 | 2 | 7 |
| 设置 | 14 | 3 | 9 | 11 |
| 账号 | 22 | 10 | 6 | 12 |

## 必须补齐的项目

**P0：测试缺失/对等行为缺口**

- `CMD-071`：原版 Provider legacy compatibility 创建与字段映射需要专门 Go 测试。普通 CRUD+Keychain 测试不构成等价行为覆盖。
- `CMD-033`：原版 `try_refresh_usage_index` 的 try-lock/占用时立即返回语义尚无 Go 对等操作测试；新版目前只是串行化刷新，不得判为已对齐。
- `FLOW-087`：Usage 分区独立的请求/取消/错误状态仍是 GAP，必须先写分区重入与 stale-result 不覆盖新 Scope 的测试再改服务。
- `CMD-012/013` 与 `FLOW-013`：账号目录 + wrapper **物理重命名**的事务及崩溃回滚仍是 GAP；既有修改显示名测试不能算覆盖。
- 所有未分配 TestID 的 UI/Tray/API 行，需针对用户点击、实际 Service 调用、错误/恢复写测试，不能把 `TestFiveNativePages` 泛用成所有操作的测试。

**P0：真实环境校验**

- 按官方工具链在 macOS arm64、Go 1.27.3+ 执行 `./tools/bootstrap.sh`、`./tools/verify.sh` 和完整 MyGo Tester / SQLite / SDK / translator Contract Tests；未安装官方依赖前没有全工程编译证据。
- macOS Keychain、真实 Codex 登录、Antigravity、Tray、Ghostty/cmux、DMG 签名、公证、Updater、数据迁移及回滚，需要各自的机器验收结果与日志。
- 部分依赖可在 Linux 跑集成测试，但 **真实 macOS 操作**不能用假窗口或单元测试替代。

## 运行记录

- `reports/feature-pass-events.jsonl`：本次隔离 Go `-json` 运行事件，含测试与包最终结果。
- `reports/domain-coverage.out`：**业务隔离范围**的语句级覆盖率数据，不是整体项目覆盖率。
- `reports/feature-test-results.csv`：239 项真实测试证据判定。
- `reports/feature-negative-gate.txt`：故意把具名测试替换成不存在的函数，门禁返回失败并指出受影响 ID。
- `reports/feature-source.txt`、`reports/feature-vet.txt`、`reports/feature-race-confirmed.txt`：静态/Race 验证。

**说明：** Go 测试事件证明对应 Test 函数实际执行，不自动证明所有业务条件和异常路径完整；具名测试的断言仍需代码审查。所有系统功能的验收结果要有机器/环境证据，不能仅凭当前 Go 文件和映射判定通过。
