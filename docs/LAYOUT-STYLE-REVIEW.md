# LAM 2.0 / MyGo：原版布局与样式逐屏复核

**结论：这一轮已经实际修复多处明显偏差；源码/设计令牌层面取得进展，但不能认证视觉零差异。**

本轮审查来源：用户上传的 `LAM.zip`（LAM 0.4.4），其 `apps/desktop/src/styles.css`、`apps/desktop/src/components/icons.tsx`、`apps/desktop/src-tauri/tauri.conf.json`、`apps/desktop/src-tauri/icons/tray-menu-template.png`、应用 Icon，以及 `pics/{OverView,Usage,Settings,Bar,AddAPI,AddAcount}.png`。新版审查对象：上一轮交付的 `LAM2-Go-MyGo-Visual-Style-Fixed.zip`，本轮修改后源码在交付 ZIP 内。

## 1. 本轮确认并修复的偏差

| 项目 | 原版事实 | 上一版偏差 | 本轮改动 |
|---|---|---|---|
| 主窗口 | 默认 1280×820 DIP，最小 1100×700 | 1280×880，最小 1050×720 | 恢复原值，标题 `LAM` |
| 托盘窗口 | 默认 328×520 DIP、可调整大小 | 440×720，禁止调整 | 恢复 328×520，不再强制 DisableResize |
| 主背景 | CSS 从中心白色到 `#eef0f4` 径向渐变；暗色中心 `#161a24` 到 `#0b0d12` | 线性渐变 | 用原色阶原生 MyGo SVG radial gradient，不用 WebView |
| 底部导航 | `position: fixed` 浮在内容之上；图标 32×32、文字 10px；背景 blur(28px) | 曾是占高度的正常行，字符替代真实 SVG，且无 backdrop blur | 修复浮层；使用原版 SVG 路径；用 MyGo `plugins/glass` 的 `glass.Blur{Radius:28}` 和半透明叠色 |
| 顶栏 | 56px，高度固定，Profile Mode 在中心布局区，主题切换在右侧 | 尺寸与按钮位置变形 | 修复高度、按钮位置；主题按钮换原版 Device/Sun/Moon SVG |
| 状态栏 | 26px 独立于 Dock | 曾被 Dock 影响高度 | 修复独立位置 |
| Overview 指标 | 四张 85 DIP 小卡、图标 42×42、无顶部彩条 | 曾加入彩条，字符假图标 | 恢复尺寸和原版 Users/Activity/Providers/Clock SVG |
| 账号卡 | `auto-fill minmax(280px,1fr)`，Relay/Switch Model + Handoff | 人工断点列数、底部 Manage | 恢复自动填充、原动作和右上角溢出菜单 |
| Usage | 三张大 KPI 卡，53 周粉色活动热力图，四个 Insights tabs | 通用指标卡、图表结构不一致、tabs 索引错位 | 恢复三 KPI、横向 53 周、四个 tabs 顺序及索引 |
| Settings | 无外层卡片，220 DIP 左侧分类，行式 Preferences | 大卡片堆叠式设置 | 恢复左栏、分组及行式设置；Mode Availability 显示原版文本 |
| Tray 头部 | 30px Logo，LAM 15px，统计为三等列、数值 16px | 36px Logo / 22px 字体、过高头部 | 恢复原比例，并补回始终可见的 Active Source 行 |
| 资源与品牌 | 原始 Orbit Logo + macOS Tray Template + 原始 App Icon | 软件包曾使用深色方块白色 L；托盘使用人工生成的 L | 三者全部恢复原始 PNG 字节资源 |
| 弹窗 | 原版 Add Account / External API 具有不同的标题、分段、分组和底部操作 | 通用表单 | 按原版结构重新分组、保留原生表单能力 |

**图标是精确资源复用而非近似绘制：** 从旧项目 `components/icons.tsx` 提取 12 组原始 SVG 图元，MyGo 通过 `ui.MustParseSVG`/`ui.Icon` 直接绘制。旧的安装包图标和系统托盘模板 PNG 也按 SHA-256 确认字节一致。

## 2. 六张原始截图的验收状态

| 画面 | 原始截图像素 | 源码/布局修正 | 真正 native 截图比较 |
|---|---:|---|---|
| Overview | 3350×1942 | 顶部、4 指标、卡片网格、Dock、原始 SVG 已调整 | **未运行** |
| Usage | 3360×1932 | KPI、热力图、Tabs、过滤器位置已调整 | **未运行** |
| Settings | 3354×1940 | 220 DIP 侧边栏、分类与 Preferences 已调整 | **未运行** |
| Bar (Tray) | 654×1410 | 默认尺寸、Logo、Active 行、统计与账户卡已调整 | **未运行** |
| AddAPI | 1556×1216 | 顶栏、Advanced、双栏、Footer 已调整 | **未运行** |
| AddAcount | 1020×1506 | 标题、Profile/导入分段、敏感信息区与 Footer 已调整 | **未运行** |

原始图片是 macOS Retina 物理像素、包含动态账号资料，且部分包含操作系统窗口标题栏；**不能把图片的像素长宽直接当作窗口的 DIP 尺寸**。实际 MyGo 截图必须统一窗口内容区域、Retina 倍率、字体环境、显示语言、主题、数据快照和所处的 UI 状态，才有可解释的 pixel diff。

## 3. 仍不能宣称“完全不变”的地方

1. **没有实际 MyGo/macOS 渲染截图。** 所有页面源码静态匹配，仍无法证明文字度量、阴影、弹窗尺寸、输入框高度和布局在真实渲染器下与 Tauri/WebKit 一致。
2. **浏览器 backdrop-filter 饱和度不保证等价。** 已改为 MyGo 原生 28 DIP 背景模糊，但原 CSS 还要求 `saturate(1.25)`；未证实渲染完全同色。
3. **部分数据密度和文案不同。** Usage 第二、三张指标卡的内容字段与原版截图仍有差别；实际业务数据与截图源数据也不同，不应利用填充假数据造出视觉通过。
4. **两个创建弹窗仍有交互差异。** `Overwrite wrapper` 当前明确禁用；Profile 创建后再导入认证不是单事务；External API 保存的两阶段流程也需与原版确认。
5. **原项目没有提供 Sessions / Providers 主页面的截图。** 只能依据旧 CSS 与组件代码审查，不具备截图级的完整原始视觉目标。
6. **系统字体与原生控件。** 即使尺寸/颜色一致，React/CSS 的字体渲染与 MyGo GPU 原生绘制可能存在差异。不能跳过实际截图核验。

## 4. 已执行的测试（本轮）

| 检查 | 结果 |
|---|---|
| `make visual-audit` | **通过**，21 个原生结构/令牌守卫检查；12 个旧 SVG 图元、原始应用图标、原始托盘模板与径向渐变源数据校验通过 |
| `go test ./internal/visualspec ./tools/visualdiff`（离线、标准库） | **通过**，原尺寸、账号自适应列数、截图差异函数测试 |
| `tools/check-source.sh` | **通过**，源码格式/禁止旧技术栈检查 |
| 离线 Go 业务测试 | **187 个顶层测试通过，208 条通过记录，无失败事件**；不是 MyGo UI 测试 |
| `tools/test-offline.sh vet` | **通过** |
| 功能测试证据门禁 | **通过业务级**：239 项中 125 项有实际具名业务测试证据，其余保持非通过状态 |
| 整批 `go test -race` | **超时/未完成**；日志为部分包通过，不能标整批 PASS |
| 完整 `go test ./...` | **未进入编译**：当前 Go 1.23.2 不认识项目所需的 `tool` 指令，项目要求 Go 1.27.3；缺少完整真实依赖环境 |
| MyGo 真实窗口、SQLite、macOS / 图形渲染 | **未运行** |
| `make visual-native` | **按预期失败**：缺少六张实际运行的 native 截图，禁止冒充视觉验收 |

## 5. 自动化和禁止假通过

- 代码：`internal/visualspec/geometry.go` 和 `_test.go`；`tools/check-original-icons.py`；`tools/check-visual-style.py`。
- 截图：`reference/visual/baseline-{sha256,dimensions}.json` + 原版六图；`tools/visualdiff` 负责同尺寸截图差异，`tools/check-native-screenshots.sh` **必须有真实候选截图才运行**。
- UI：`internal/desktop/original_layout_test.go`、`design_test.go` 等 MyGo `ui.NewTester`，需要真实框架工具链运行。
- E2E：官方 `github.com/egoist/mygo/plugins/automation`，通过只在 `e2e` build tag 存在的 `automation_e2e.go` 启用（不会进入发布产物），官方 `capture` 可输出真实 MyGo native window PNG。来源：https://mygo.egoist.dev/docs/plugins/automation 。
- CI：已增加资源一致性门禁；发布标签上运行真正的截图对比，缺截图会拒绝发布。

### 实施前提与置信度

- **Assumptions**：用户要求的“不变”意味着以原始 LAM 0.4.4 视觉及当前有效业务为唯一基线，同时继续保持纯 Go + MyGo Native UI，禁止回退到 React/Tauri/WebView。
- **Confidence 高**：原始资源、旧尺寸和 CSS 源代码提取，相关源码修复及离线检查。
- **Confidence 中**：MyGo 原生控件可以表达的结构与视觉效果（有官方文档支撑）。
- **Confidence 未知**：实际原生像素一致、交互布局在 macOS 上完全等价、最终包的编译与发布。

**最终判定：本轮是实际代码修复 + 可验证的静态视觉对照，不是像素级最终验收。需要有真实 MyGo/macOS 截图后才能把“布局与样式不变”标记为 PASS。**
