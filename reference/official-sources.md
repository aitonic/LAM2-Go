# LAM 2.0：官方来源和第三方包验证索引

> 核查日期：2026-10-27。以官方文档、仓库与 Go Package Index 作为实现前事实来源；版本需要在 P0 中锁定验证，本文不冒充编译后的兼容性报告。

## MyGo 官方

- [MyGo 文档首页](https://mygo.egoist.dev/docs) — Native UI 与 Web 界面的区分、系统 API、构建、平台要求。该文档写明 MyGo 开发需要 Go 1.27+。
- [MyGo Native UI](https://mygo.egoist.dev/docs/ui) — `ui.View`、纯 Go、无 WebView。
- [MyGo Getting Started](https://mygo.egoist.dev/docs/getting-started) — `native` 模板与 `go tool mygo`。
- [MyGo UI Views](https://mygo.egoist.dev/docs/ui/views) — `Window.Update`、frame 生命周期、vet。
- [MyGo Table](https://mygo.egoist.dev/docs/ui/table) — 原生 Table、虚拟化与选择。
- [MyGo SQLite](https://mygo.egoist.dev/docs/plugins/sqlite) — Go `sqlite.Open`、事务、不要求 CGO。
- [MyGo Native Updater](https://mygo.egoist.dev/docs/plugins/updater) — 使用 `plugins/updater/native` 避免更新窗口启动 WebView。
- [MyGo Terminal Plugin](https://mygo.egoist.dev/docs/plugins/terminal) — 原生内嵌终端；外部 cmux/Ghostty 仍应通过 OS 适配器处理。
- [MyGo Languages](https://mygo.egoist.dev/docs/i18n) — Typed Go 多语言 Catalog 和原生国际化。
- [MyGo Desktop APIs](https://mygo.egoist.dev/docs/native) — Dialog、通知、Secrets、文件监控等。
- [MyGo Updates](https://mygo.egoist.dev/docs/updates) — 更新签名、Delta 和发布。

## Codex 官方

- [Codex App Server](https://developers.openai.com/codex/app-server) — 账号、会话、用量、配置、通知与 JSON-RPC；以实际版本 schema 为准。
- [OpenAI Go SDK](https://github.com/openai/openai-go) — 上游 API Client，不是 Responses/Chat 协议转换器。

## 第三方 Go 包

- [codex-sdk-go](https://github.com/pmenglund/codex-sdk-go) 与 [Go Package Index](https://pkg.go.dev/github.com/pmenglund/codex-sdk-go) — 社区 App Server SDK，需锁 Codex CLI 协议版本及 `CODEX_HOME` 环境。
- [agent-parser](https://github.com/alexgorbatchev/agent-parser) — Codex JSONL 事件解析器；验证 Usage 数据口径。
- [CLIProxyAPI v8 translator](https://pkg.go.dev/github.com/router-for-me/CLIProxyAPI/v8/sdk/translator) — 公开 Go 协议转换 SDK；不是保证 LAM Custom/Namespace Tools 完全兼容。
- [go-toml/v2](https://github.com/pelletier/go-toml) — 仅用于官方 Config RPC 不覆盖的非破坏性编辑。
- [RelayKit](https://github.com/QuantumNous/new-api/tree/main/relaykit) — AGPL-3.0，不能无视许可条件直接变为默认依赖。

## 已知需要实证确认的点

1. MyGo 文档、首页与 GitHub Releases 可能存在版本缓存差异：P0 不按搜索摘要推定“最新版”；以真实可用 Go Tag 为准。
2. `codex-sdk-go` 仓库和 pkg.go.dev 抓取周期不同：以绑定的正式 Tag、SDK CompatibilityPolicy 和 Codex CLI 实际版本为准。
3. CLIProxy Translator 是否覆盖 `additional_tools`、custom/namespace tools、图像及 SSE Roundtrip：必须通过旧 LAM Fixtures，无损失败才采用。
4. MyGo Secrets 读取旧签名身份创建的 macOS Keychain Ref：必须在实际签名 `.app` 里验证。
5. 所有 Go 依赖应检查传递依赖及打包资源，确认没有被误带入旧 Rust 可执行程序或 WebView UI。
