# LAM 2.0：原版 3,504 条样式参数逐项审查报告

- 日期：2026-10-29
- 原项目：`LAM.zip` → `apps/desktop/src/styles.css`，115,188 字节
- 样式原件 SHA-256：`95a2bb7c1147c87922575cad73853bc4a0ebaffa60158efb6b58cdebb962cb55`
- 技术边界：Go + MyGo Native UI；不得重新引入 React、Tauri、Rust、CSS 运行时或者 WebView 主界面
- 核验基线：819 个 CSS 声明规则块、3,504 项 CSS 属性（包含 `@theme` token、Media、Container 和 Keyframes）

## 结论

**未达到“全部参数完全相同”**。本次完整解析并登记 3,504 条原始 CSS 声明，但静态源码映射只覆盖 129 条，其中 125 条找到精确源值证据、3 条属于结构/断点映射、1 条已建立源码映射但存在已知渲染不等价；另有 3 条 Filter 属性确认不等价。剩余 3,375 条尚未建立有效映射。本环境没有拿到 MyGo 原生渲染的逐元素 computed style 或六张实机截图（零像素容差），因此通过原生视觉验收的属性数量为零。

该结果与此前“布局/样式基本恢复”的印象结论不同。**不将 Go 代码里出现同名常量视为相应控件一定采用该值，也不将源码对齐当作像素对齐。**

## 逐项统计

| 状态 | 数量 | 严格解释 |
|---|---:|---|
| `SOURCE_MATCH_RENDER_UNTESTED` | 125 | 原 CSS 数值及关联 Go 代码值已核对；没有 native 实测 |
| `STRUCTURE_MAPPED_RENDER_UNTESTED` | 3 | 网格/容器位置映射存在，真实宽度与布局计算尚未实测 |
| `KNOWN_NON_EQUIVALENCE` | 4 | MyGo 原生 Blur 没有原 CSS 的 `saturate(1.25)` 等价参数 |
| `NOT_VERIFIED` | 3,220 | 无逐属性有效 Go 实现证据，不能算保持一致 |
| `RESPONSIVE_NOT_VERIFIED` | 34 | Media/Container 条件尚未完成实际断点截图校验 |
| `EFFECT_NOT_VERIFIED` | 118 | 动画、变换、hover/渐变文字/Filter 等尚未验证 |
| **总计** | **3,504** | 无一条完成 native 视觉验收 |

**已审查与未映射的含义不同：** 所有 3,504 条已完成逐行提取和分区分类，但只有 129 条建立了明确 Go 源码映射，不可把剩余 3,375 条称为“完成”。

完整清单：`LAM2-All-CSS-Parameters-Audit.xlsx` 或 `.csv`。每条均包含 `CSS-####` ID、源码行号、CSS 选择器、属性、旧值、媒体条件、核验状态、Go 位置、缺口原因。Excel 提供筛选并单独标记 528 条高优先待核验行。

## 本次已修正的具体参数

| 位置 | 原版精确要求 | 本轮修改 |
|---|---|---|
| 根字体 | 14px，行距 1.45 | `visualspec.RootFontSize=14`、`RootLineHeight=1.45`，应用于 MyGo 主题和根 View |
| 深色主题 | `--accent=#3b82f6`、`--accent-soft=rgba(59,130,246,.15)` | 改回原主题继承值和半透明颜色，校正 Green/Amber/Purple Soft |
| Overview 数值 | 26px / weight 700 / letter-spacing -0.02em | 恢复 26px、字间距与 2px 标题间隔 |
| Overview 账号卡 | `linear-gradient(152deg, ...)`、三层 box-shadow | 原生 152° 三色渐变、分层阴影、hover 色混合 |
| Usage KPI | 20px padding、16px gap、16px radius、30px 主值 / weight 800 | 取消错误的固定 255px 高度，恢复所有数值与三组颜色渐变 |
| Usage 强调条 | top=0、height=3px | 使用原生浮动覆盖元素，恢复高度及位置 |
| Settings 内容 | 32px 40px 140px padding | 恢复原始四边 padding |
| Settings 行 | 16px 20px、gap 24px；标题 13.5px | 恢复 padding、间隔和文字大小 |
| 响应式断点 | `@media(max-width:1240px)`：统计卡及账号卡单列 | 修正 Go 的条件分支；窄 Dock 图标 28px、圆角 8px、gap 6px |
| Tray 头部 | gap 8px、padding 10px 12px 9px、title 15px/800 | 删除错误的强制 62px 高度、恢复确切 padding 和字重 |
| Tray 统计栏 | 字重 750/900、gap 1px、padding 7px 6px 6px | 改回实际数值，光标样式/动态效果仍未实测 |
| Tray 活跃行 | 外层 8px 12px，内层 5px 10px、gap 8px、radius 6px | 恢复内外两层原生 UI 结构 |
| Tray 独立配色 | `--head-*`、`--stats-*`、`--active-*`、`--foot-*` | 新增 `tray_tokens.go`，精准映射 28 条浅/深色主题声明 |
| Tray 底部 | padding 6px 10px 8px | 恢复原始 padding，移除错误固定 50px 高度 |

以上只保证所列原始常量/代码声明存在一致关系，不保证 MyGo 最终渲染时完全像 CSS。

## 主要尚存差异与未完成事项

1. **原版布局未逐屏完成**：Overview、Usage、Settings、Tray、Add Account 和 Add API 的响应式尺寸、文字换行、边框与阴影需真实 MyGo Inspector 逐组件核验。Sessions、Providers 的截图基线不完整。
2. **渐变颜色空间与文字裁切**：原版 Usage 主数值使用 `background-clip:text` 和透明文字，当前 Go 仍显示单一文字颜色；需要 MyGo Native 的文字着色方案和真实截图验收。
3. **Backdrop Filter**：Dock 是 `blur(28px) saturate(1.25)`、Tray 是 `blur(20px) saturate(1.1)`，官方 `glass.Blur` 并无 CSS `saturate()` 参数，必须通过 MyGo 原生扩展或渲染器方案处理，不能认为单纯模糊等价。
4. **悬停、焦点、动画**：121 条效果声明没有动态 native 证据，包括 0.12～0.3 秒过渡、transform、动画重放等。
5. **表单/模态层**：原版 `.modalHead`、`.modalBody`、选择框视觉、输入 focus glow、Add Account 和 External API 的区域布局，当前没有逐属性验证。
6. **缺失参数映射**：`Provider/API` CSS 425 项、`Sessions/Handoff` 84 项、`Dialogs` 55 项等必须按原组件继续建立实际 Go 组件映射，不能靠全局 Token 替代。
7. **字号与字重**：MyGo Core Text 与旧 Chromium 浏览器的字体 fallback、抗锯齿、字距和基线不保证完全一致。数值 14px 相同仍可能像素不同。

## 测试和发布门禁

| 检查 | 结果 |
|---|---|
| 旧 CSS SHA 与 3,504 条清单完整性 | **通过** |
| 129 条原值→Go 源码契约 | **通过 129/129** |
| 旧图标和六张原版截图基线 | **原件检查通过** |
| 前一轮布局源码检查 | **通过 21/21**（非真实渲染） |
| Go 离线业务回归 | **通过 187 个顶层测试，含子测试 208 条通过记录，0 失败** |
| `go vet`（离线业务范围） | **通过** |
| Race 检测 | **15 个生产包分别有通过日志；整批执行曾超时**，不宣称整批无中断 |
| `go test ./...`（真实 MyGo 项目） | **未运行通过**：Go1.23.2 Linux 环境与 MyGo 要求版本不兼容，外部依赖不可取 |
| 原生六屏渲染 / DPI / Inspector | **未验收** |
| `check-style-contracts.py --require-all` | **预期失败**：3,504 项原生视觉证据均不完整，阻止标记完全一致 |

源码门禁：`tools/check-style-contracts.py` 验证原始 CSS SHA/声明数量及全部 129 条 Go 源值映射；严格门禁 `--require-all` 对任何缺原生验证的参数 fail closed，已接入 `visual-native` 和 `release-gate`。

## 官方来源（2026-10-29 查阅）

- MyGo Native UI：https://mygo.egoist.dev/docs/ui
- MyGo Styling：https://mygo.egoist.dev/docs/ui/styling
- MyGo Inspector：https://mygo.egoist.dev/docs/ui/inspector
- MyGo Glass Blur：https://mygo.egoist.dev/docs/plugins/glass

**下一验收步骤**：在 macOS arm64 + 适配 Go/MyGo 版本下运行完整构建，使用原版窗口尺寸、相同显示缩放产生六张截图；逐像素比较采用 tolerance=0、max-changed=0；再用 MyGo Inspector 检查布局计算值并补齐 3,375 条未映射属性。成功前不以“所有样式参数保持一样”作为交付状态。
