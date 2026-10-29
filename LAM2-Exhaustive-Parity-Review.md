# LAM 2.0 — 原版样式、布局、功能逐项 Review（全量追踪）

> 2026-10-29 · 基线：上传的 LAM.zip（v0.4.4）· 目标：100% Go + MyGo Native UI · **当前并未完成全部对齐/原生验收**。

## 一、检查范围与真实状态

| 维度 | 全量数量 | 当前证据状态 |
|---|---:|---|
| CSS 参数声明（包含 `@theme`） | 3,504 | 160 个源码值有映射；7 个结构映射；其余 3,337 个尚无静态验证或属于未验收特效/断点；0 个原生渲染验收 PASS |
| CSS 选择器规则组 | 819 | 每组均列出 CSS 行、属性条数、候选组件；全部待原生验收 |
| 原版功能/操作 | 239 | 125 项业务测试 PASS；113 项尚未全部验收；1 项历史废弃 |
| 原版截图×视觉状态 | 60 | 全部 NOT_RUN（原版只有 6 张静态截图，部分状态需另行捕获） |
| 已知不等价 CSS | 4 | 需要真实 MyGo 实现、验收；不能假报 PASS |
| 响应式待核对 | 30 | 覆盖≤1240px、≤640px等规则 |
| 效果/动画待核对 | 118 | Hover、Focus、Transition、Glass、滤镜等 |

**重要区别**：枚举 3,504 项不是完成 3,504 项的手工/真实计算样式核对。源码参数值相同不能证明原生绘制相同。`NATIVE_VERIFIED` 唯一正确依据是带状态的实机截图、对应元素实际 Layout/Style 记录及已执行的验收测试。

### 全量逐条 CSV 与 Excel

- `reports/visual-trace/all-3504-css-review.csv`：逐 CSS 声明的唯一 ID、优先级、页/组件、原始行、选择器、属性、精确值、条件、组件引用候选、Go 候选位置、证据、处理结论与验收要求。
- `reports/visual-trace/all-css-rules-review.csv`：819 个 CSS 规则组，避免一条多属性 CSS 规则中的属性被漏检。
- `reports/visual-trace/feature-239-review.csv`：239 条已独立通过/缺失/部分通过/未执行的功能追踪记录。
- `reports/visual-trace/visual-state-60-review.csv`：六个界面在默认、暗色、悬停、错误、弹窗、断点、长内容、键盘中文输入、Retina、Inspector 等场景的验收证据状态。
- `reference/visual/legacy-style-declarations.json` 与 `legacy-style-contracts.json`：严格源值契约，原始样式哈希锁定。
- 对外 Excel：`LAM2-Complete-Parity-Checklist.xlsx`，包含全部清单及高优先级差异。

## 二、本轮已经确认并修正的具体差异

| 对应原版 | 原始准确值/语义 | Go 源代码修正 | 证据/状态 |
|---|---|---|---|
| `.usageDashboardPage` | 内边距 18px、间距 14px | `internal/desktop/view.go` | 静态源码 + Go 视觉常量测试通过；原生未执行 |
| `.usageDashboardHeader h2` | 22px | `view.go` | 静态证据；原生未执行 |
| `.usageDashboardLogo` | 34×34px、8px 圆角，原始 SVG | `view.go` | 静态证据；原生未执行 |
| `.usageHeatmapCell` | 14×14px、3px 间隔，超出窗口日不可点击 | `internal/desktop/activity.go` | 静态证据；原生未执行 |
| `--heatmap-bg-0..4` | 深浅主题各五级原始颜色 | `activity.go` | 源码颜色检查；原生未执行 |
| `.titlebar @media <=1240px` | 自动换行与 12px/16px 内边距 | `view.go` | 静态证据；原生未执行 |
| `.metricCard` | 仅由内容高度决定，CSS 不设置 `height` | `internal/desktop/design.go` 移除旧版 Go 强制 85 DIP | 结构检查通过；原生未执行 |
| `.tableWrap` | 最大滚动高度 560px，圆角 12px | `internal/desktop/view.go` | 源码结构检查通过；原生未执行 |
| `formatStorageBytes()` | 字节、KB/MB/GB/TB 进位与 10 以下小数规则 | `internal/visualspec/format.go` | 纯 Go 格式测试通过（含 Race）；原生未执行 |
| `.sessionsTable` | 最小宽度 1280px | `visualspec/geometry.go`, `view.go` | 纯 Go 尺寸用例通过；原生未执行 |
| `.sessionCol*` | Session/Size/Last active/cwd/Provider/Actions，22/7/13/16/12/30% | `view.go` 真实六列和行内 Copy/Terminal/Relay/Details | 源码语义检查通过；真实表格排版未验 |
| `.accountCard` | 渐变末端计算为 `#f2f7ff` | `internal/desktop/overview.go` | 静态源码值通过；原生未执行 |

本轮新增/确认 168 条“原版源值→Go 代码”契约（上轮 129）；全部仅是 `SOURCE_MATCH_RENDER_UNTESTED` 或 `STRUCTURE_MAPPED_RENDER_UNTESTED`。**实际 native PASS = 0**。

## 三、逐组差异的处理顺序与明确缺口

### P0 — 阻断发布

1. **Global / Chrome**：`.content` 20px 24px 120px 由真实滚动内容的盒模型负责；当前 Go 外层仍以 14px 底部 padding 为主，未达到同等滚动末端留白。`titlebar`/Dock 某些响应/动效仍无实机证据。
2. **Overview**：统计卡固定高度已修，账号卡及弹出层仍需 Hover/Focus、多列窄断点、深浅主题对照。
3. **Sessions**：六列顺序与尺寸已改；仍需实机验证最小宽度、横向滚动、列排序、点击行展示详情以及表格内容字号/省略行为。
4. **Usage**：热力图已改，KPI 渐变文字背景裁剪、复杂 Tooltip、维度密度以及长列表状态尚未证明原生等价。
5. **Provider / API**：`AddAPI` 弹窗的分段表单、行距/阈值、模型选择、错误反馈与旧版截图仍需比对。
6. **Dialogs**：原版 `.modalHead / .modalBody / .modalFoot` 采用不同 padding（头部、正文、底部分别控制），Go 版多个 Dialog 仍使用一层统一 padding，不等价。
7. **Settings**：已恢复左侧分类，但右侧行式布局、控件 minWidth、长文本折行、窄屏状态需要 Inspector 验证。
8. **Tray**：需要 328×520 原生窗口、两套配色、较长分组配额列表、缩放/失焦、动态圆环测试。
9. **动态 Glass/滤镜**：CSS `backdrop-filter: blur(28px) saturate(1.25)` 与 MyGo 模糊 API 不是同一实现，不能直接判“完全相同”。
10. **工具链/真实渲染**：仓库要求 Go 1.27.3 + MyGo v0.7.1，当前环境 Go1.23.2，无 macOS 原生渲染/快照/Keychain/托盘实测。

### P1 — 每项需要保留原版交互证据

- 所有 CSS `@media`/状态选择器的相邻宽度、暗色/浅色、Focus、Hover、Disabled、Active、Loading、进度动画。
- `SOURCE_CLASS_NOT_LOCATED` 涉及的 350 条属性，必须先审查动态拼接/条件类名及历史规则；仅词法搜索不到，不能宣告“无效、可删除”。
- 历史 239 项功能测试状态不能用截图代替；每项保持功能单元/集成/系统测试证据。

## 四、自动化验收规则

`make visual-audit` 执行：源 CSS 哈希与 3,504 声明、全部源码映射合同、原始 SVG、主要结构参数、生成全量规则组追踪表、核对 239 功能与 60 视觉状态条目。此命令通过只说明“台账完整、已声明映射源值匹配”，**不代表视觉通过**。

`make visual-native` / `make release-gate` 必须拒绝：存在 `NOT_RUN`、缺少 MyGo 实机截图/Inspector、未覆盖业务状态、使用虚假 Native PASS。所有项目都要有正常、失败、边界场景，不能复用不相关的测试来标绿。

## 五、已实际运行的测试与风险结论

- 离线 Go Domain：187 个顶层测试通过、208 个含子测试通过记录，0 失败、0 跳过；并不是 MyGo UI 完整测试。
- 纯 Go `visualspec`：8 条尺寸/布局及旧版文件大小格式测试通过（包含 Sessions 六列与比例）。
- `make visual-audit`：已通过，3,504 / 819 / 239 / 60 的 ID 完整性与样式源码映射检查通过。
- `tools/check-native-structure.py`：13 条结构源检查通过；属于源码检查。
- `tools/check-source.sh` 与离线 `go vet`：通过。
- `tools/check-visual-trace.py --release`：**按预期 FAIL**，阻断全部未渲染的 CSS、60 视觉场景和未完成的功能验收。
- `go test ./...`（真实 MyGo/SDK/Go1.27.3）、macOS 原生视觉/签名/安装：未执行/不可在当前容器完成，绝不记 PASS。

### 判断

**全量清单已建立，部分明确差异修复并有静态/Go 测试证据，但“逐个最终验收、所有点全部对齐”尚未完成。** 继续以 Excel 的每一行作为独立审查工单，不得按区域或单个截图批量标 PASS。
