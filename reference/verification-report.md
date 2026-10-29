# 静态交接包验证报告

**检查日期：2026-10-27**

| 检查项目 | 结果 | 说明 |
|---|---|---|
| 旧版源码基线 | 已静态核对 | LAM.zip / 0.4.4，路径详 `source-inventory.md` |
| Rust 实现文件 | 64 | 仅作为旧版参考，未进入新工程源文件 |
| Rust 集成测试文件 | 56 | 只保留行为契约和测试映射 |
| 旧 TS/TSX 文件 | 69 | 不进入新项目的依赖链 |
| 历史协议/配置样本 | 42 | 37 JSON、4 TOML、1 SSE |
| JSON 样本语法 | 36 个可解析，1 个故意损坏 | `legacy-provider-contract/provider-store/malformed.json` 应保留，用于异常输入测试 |
| 文档 | 4 份详尽规格 + README + AGENTS + 源码对照 + 来源索引 | 均为 UTF-8 Markdown |
| MyGo 官方 `native` 模板 | 文档已确认 | 纯 Go、无 Bun/WebView，需 Go 1.27+ |
| 当前本地 Go 环境 | Go 1.23.2, Linux amd64 | **不能在此运行 MyGo 1.27+ 构建** |
| 下载新 Go 工具链 | 失败 | 运行环境无法解析/访问 `proxy.golang.org`；不代表 Go 1.27.3 不存在 |
| 官方依赖能力 | 通过网络检索文档确认 | 不等同于固定模块版本的编译、测试 |
| 新 Go 业务代码 | 未执行 | 本交接包是实现基线与测试输入，不是已经发布的新版应用 |
| macOS 真实签名/Keychain/Tray | 未执行 | 必须由 P0-P9 后续实机验证 |

## 风险与验证优先级

- **最高风险**：旧 Codex Responses Gateway → Go 转换的 Tool Calls / SSE / reasoning 无损兼容，需完整 Golden tests。
- **数据风险**：Config Attach 跨文件/Keychain/CLI 的事务、Profile/PAT 隔离、SQLite/WAL 迁移。
- **框架风险**：MyGo/Go 版本、系统托盘生命周期、Native Updater、长时间运行 Gateway 子进程。
- 不以静态“存在 API”推定“运行符合要求”，禁止将未执行测试标为 PASS。
