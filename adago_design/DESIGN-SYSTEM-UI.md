# 画布 UI 设计系统 · 优化改造文档

> 版本：v1（2026-11-08）
> 范围：`mcp_projects/adago_design` 的**浏览器画布界面**（工具条、左侧双浮窗、检查器、项目管理弹层、
> 空态/提示、检查标注层、截图输出）。八套设计体系预览文档（`ds-*`）不在本轮范围内。
> 定位：对齐仓库 `docs/19-ued.md`（Black Swan 纸墨、令牌单源、单色品牌、克制）的**工具型界面**落地版。

---

## 0. 一句话结论

现有画布 UI 的**信息架构是对的**（工具条 + 左双浮窗 + 右检查器 + 无限画布），但**视觉层没有设计系统**：
39 个十六进制色、450 个 px 字面值、13 种圆角、6 档字号（含 8px）、5 处原生 `prompt/confirm`、
0 处 `prefers-reduced-motion`、主操作在被禁用色上做白字（4.35:1 不合格）。
本轮把它收敛成**一套令牌驱动的纸墨界面**：纸面底 + 墨黑 chrome + 单一品牌色（墨）+ 语义状态色，
颜色/圆角/字号/间距全部走令牌，交互组件（弹层/菜单/toast/分段）统一形态，并加机器门禁防止回退。

---

## 1. 现状审计（带证据）

### 1.1 度量

| 指标 | 现状 | 证据位置 |
|---|---|---|
| `canvas.css` 十六进制色值 | **39** | `static/canvas.css` |
| `canvas.css` `rgba()` 字面值 | **16** | 同上 |
| `canvas.css` px 字面值 | **450** | 同上 |
| `border-radius` 不同取值 | **13** 种（4/5/6/7/8/9/10/12/14/18/50%/999/0-0-12-12） | 同上 |
| `font-size` 不同取值 | **6** 档（8 / 10 / 11 / 12 / 13 / 14px） | 同上 |
| `canvas.js` 十六进制色值 | **11** 处（连线箭头、截图底色、便签、标注） | `static/canvas.js` |
| `inspect.js` 十六进制色值 | **9** 处（A/B 标注色、关系文字色） | `static/inspect.js` |
| JS 模板内联 `style=` | **7** 处（体系详情面板） | `canvas.js:1324-1339` |
| 原生 `prompt()` / `confirm()` | **5** 处（重命名、删项目、画布尺寸、清空、便签） | `canvas.js:1215,1230,1556,1699,1713` |
| `prefers-reduced-motion` | **0** 处 | `canvas.css` |
| `:focus-visible` 规则 | **3** 条（只覆盖工具条按钮与两个输入框） | `canvas.css:154,381` |
| `z-index` 魔法数 | 10 个（5/20/32/34/40/50/80/81/9999/2147483000/2147482000） | `canvas.css` |
| 主题持久化 | **无**（`btn-theme` 只改属性，刷新丢失） | `canvas.js:1693` |
| `-theme` 启动参数 | **已解析但从未生效**（`Config.Theme` 无任何读取方） | `internal/config/config.go:67` |
| 禁用品牌色 | `--accent2:#25f4ee` 定义后**无人使用** | `canvas.css:4` |

### 1.2 对比度实测（WCAG 2.1 相对亮度公式）

| 前景 / 背景 | 比值 | 判定 |
|---|---|---|
| `#ffffff` on `--accent #7c5cff`（主按钮、`tb-active`、`.comp-item` hover） | **4.35** | ✗ 正文/12px 需 ≥ 4.5 |
| `--accent #7c5cff` on light `--bg #f2f3f7`（链接/当前项文字） | **3.92** | ✗ |
| `#ffffff` on `#f59e0b`（`group-label` 项目标签） | **2.15** | ✗✗ 严重 |
| `#ffffff` on `#d97706`（`group-label:hover`） | **3.19** | ✗ |
| `--bad #fe2c55` on light `#ffffff`（错误 toast 文字） | **3.68** | ✗ |
| `--fg2 #6b7080` on light `#f7f8fa`（次要文字） | 4.64 | ✓（勉强） |
| `--fg #e8e9ee` on dark `#0f1014` | 15.68 | ✓ |

> 结论：凡「白字压在彩色底」的地方全部不合格——根因是**没有 `on-brand` 语义令牌**，
> 于是每个彩色底都写死 `color:#fff`，一旦底色换成亮色系就必然失败。

### 1.3 结构性问题（比配色更影响观感）

1. **色彩噪音**。界面同时出现紫（品牌）、青（`accent2`，未使用）、橙（项目标签）、绿（连接状态）、
   红（危险），再加八套体系卡片各自的品牌色 chip。工具 chrome 也在「炫技」，与预览内容抢注意力。
2. **几何没有网格**。圆角 13 种、字号 6 档、间距随手写（`padding:6px 8px` / `5px 9px` / `1px 7px` …），
   同一层级的控件高度不一（28/30/24/22/20/16）。
3. **层级没有体系**。`z-index` 全是魔法数，新增一层要翻整份文件确认「会不会压住 tooltip」。
4. **原生对话框打断设计语言**。`prompt()/confirm()` 由浏览器绘制，字体、圆角、语气全部脱离体系；
   且 `prompt` 在部分环境被静默禁用 → 删除项目、便签直接不可用。
5. **可访问性缺口**。全局只有工具条按钮有焦点环；左侧树行是 `div` 无 `role`/无 Tab 可达；
   树形展开按钮无 `aria-expanded`；toast 无 `aria-live`；弹层无 Tab 陷阱与焦点归还。
6. **JS 复制 CSS 常量**。`leftDockWidth()` 返回 292、`visibleRect()` 写 288/64/12、`fitView()` 写
   296/320/148 —— 这些全是 CSS 里的布局常量副本，改一处样式就要改三处 JS，`fitView` 居中会漂移。
7. **主题链断掉**。`data-theme` 硬编码在 `index.html`、`-theme` 参数无读取方、切换不持久化、
   截图导出把 `#7c5cff/#16171d` 写死 —— 浅色主题下导出的连线仍是深色主题色。

---

## 2. 设计决策

### 2.1 品牌：Black Swan 纸墨（单色品牌）

| 决策 | 取值 | 理由 |
|---|---|---|
| 界面底（画布工作区） | 浅 `#EDE8DC` / 深 `#14151A` | 与「稿纸」同源；节点卡片是白色预览，必须与背景拉开明度差 |
| 浮窗/工具条面 | 浅 `#FBF8F1` / 深 `#1C1E24` | 纸面层，比画布亮一档 |
| 主文字 | `#16171A` / `#F2EFE7` | 墨黑 × 宣纸，16:1 以上 |
| 品牌/交互强调 | **墨黑本身**（浅色用墨、深色用纸） | 单色品牌：选中、主按钮、激活态一律「墨底纸字 / 纸底墨字」 |
| 状态色 | success/warning/danger **成对 soft + ink** | 只表达状态，不做装饰；成对使用保证 4.5:1 |
| 检查标注 A/B | 浅 `#0B5FFF`/`#C81E48`，深 `#6E9BFF`/`#FF7D96` | A/B 是**数据编码色**（同语法高亮的定位），不算品牌色；必须与体系肤色无关 |
| 项目分组标签 | 由橙色块改为**纸墨中性胶囊** | 橙色 2.15:1 不可读，且是界面里唯一的第三种彩度 |

**为什么单色**：画布上同时存在 8 套体系 × 明暗 × 组件预览，它们自带品牌色。工具 chrome 一旦有彩色，
同屏出现两套「谁是主角」的竞争。chrome 退成纸墨后，预览卡成为唯一彩色源，对比更锋利；
这也与仓库 `19-ued.md` §19.1「克制：单色品牌、无渐变、无发光」一致。

### 2.2 补齐 UED 缺失的语义令牌

`19-ued.md` 的令牌清单（`--ph-*`）是**宿主页面**的；本模块是自包含 Go module（`GOWORK=off`），
不能 import 宿主的 `tokens.css`。因此本轮建立**同构的本地令牌层** `--ui-*`，
语义命名与 `--ph-*` 一一对齐（canvas/surface/elevated/subtle/line/ink/muted/faint/brand/on-brand/
success-\*/warn-\*/danger-\*/focus/scrollbar/motion），便于未来并库时改名即可。

命名约束：

- 语义令牌 `--ui-*`：只允许出现这组名字 + 状态后缀，组件 CSS **不得**出现原始色值。
- 标尺令牌 `--ui-space-*` / `--ui-radius-*` / `--ui-text-*` / `--ui-z-*` / `--ui-dur-*`：尺寸的唯一定义处。
- 数据色 `--ui-ins-a` / `--ui-ins-b`：仅检查标注使用，允许与主题联动。

---

## 3. 令牌规格（实现即此表）

### 3.1 尺寸标尺

| 组 | 档位 | 用途 |
|---|---|---|
| 间距 `--ui-space-*` | 2 / 4 / 6 / 8 / 10 / 12 / 16 / 20 / 24 / 32 | 偶数网格（2 的倍数），奇数档不收 |
| 圆角 `--ui-radius-*` | `xs`2 / `sm`4 / `md`6 / `lg`8 / `xl`10 / `2xl`12 / `full`999 | 旧的 13 种收敛为 7 档 |
| 控件高度 `--ui-control-*` | `sm` 24 / `md` 28 / `lg` 32 | 工具条按钮=md，主按钮=lg，行内小按钮=sm |
| 字号 `--ui-text-*` | `xs` 11 / `sm` 12 / `md` 13 / `lg` 15 / `xl` 17 | **8px 与 10px 废止**，最小 11px |
| 行高 `--ui-lh-*` | `tight` 1.25 / `base` 1.5 / `relaxed` 1.7 | — |
| 描边 `--ui-border-w` | 1 / 2 | 2 只用于焦点环与选中框 |
| 层级 `--ui-z-*` | `paper`0 / `edge`1 / `label`6 / `panel`20 / `solo`32 / `toolbar`34 / `toast`40 / `popover`45 / `modal`50 / `ask`55 / `rel`60 / `debug`9999 | 取代 10 个魔法数；确认弹层压在管理/设计器弹层之上 |
| 动效 `--ui-dur-*` | `fast` 120ms / `base` 180ms / `slow` 240ms；`--ui-ease` = `cubic-bezier(.2,.7,.3,1)` | 只做状态反馈 |

布局常量（同时供 JS 读取，禁止 JS 复制数值）：

| 令牌 | 值 | 使用方 |
|---|---|---|
| `--ui-gutter` | 12px | 浮窗/工具条外边距，JS 计算可见区 |
| `--ui-dock-w` | 280px | 左侧浮窗宽度 |
| `--ui-inspector-w` | 288px | 检查器宽度 |
| `--ui-toolbar-h` | 44px | 工具条高度（含外距=56 → `--ui-top-safe`） |
| `--ui-node-head-h` | 32px | 节点标题栏高度（`HEAD_H` 的唯一定义处） |

几何常量（尺寸/模糊/图标档位；`canvas.css` 与 JS 内联样式只引用，不写裸值）：

| 组 | 令牌 | 值 | 用途 |
|---|---|---|---|
| 模糊 | `--ui-blur-toolbar` / `--ui-blur-panel` | 10 / 12px | `backdrop-filter` |
| 图标 | `--ui-icon-12` … `--ui-icon-24` | 12 / 14 / 16 / 20 / 24px | `.ds-icon--*` |
| 网格 | `--ui-grid-cell` / `--ui-grid-bleed` | 40 / 2px | 画布网点与纸张网格 |
| 菜单 | `--ui-menu-min-w`、`--ui-tb-menu-*`、`--ui-tip-max-w` | 156 / 268·360·440 / 240px | 更多菜单、工具条下拉、气泡 |
| 弹层 | `--ui-modal-w` / `--ui-ask-w` / `--ui-toast-max-w` / `--ui-empty-max-w` | 580 / 420 / 560 / 440px | 模态、询问、toast、空态 |
| 单画布条 | `--ui-solo-max-w` / `--ui-solo-title-max-w` / `--ui-solo-proj-max-w` | 760 / 230 / 160px | 聚焦条 |
| 控件 | `--ui-control-xl` / `--ui-zoom-min-w` / `--ui-field-min-w` / `--ui-textarea-min-h` | 40 / 44 / 150 / 76px | 面板头、缩放标签、输入框 |
| 其他 | `--ui-edge-label-stroke` / `--ui-focus-offset` | 4 / 2px | 连线标签描边、焦点环偏移 |
| iframe 内联 | `--ui-inspect-border-w` / `--ui-inspect-label-max-w` / `--ui-note-min-h` | 1.5 / 260 / 120px | 标注层与便签（srcdoc 启动时内联） |

### 3.2 颜色令牌与对比度

| 令牌 | 浅色（纸） | 深色（墨纸） | 对比度 |
|---|---|---|---|
| `--ui-canvas` | `#EDE8DC` | `#14151A` | — |
| `--ui-surface` | `#FBF8F1` | `#1C1E24` | — |
| `--ui-elevated` | `#FFFDF8` | `#23262E` | — |
| `--ui-subtle` | `#F2EDE2` | `#242730` | — |
| `--ui-line` | `#DCD5C6` | `#2E313A` | — |
| `--ui-line-strong` | `#C9C0AC` | `#3D414C` | — |
| `--ui-ink` | `#16171A` | `#F2EFE7` | 16.17 / 15.60 |
| `--ui-ink-2` | `#57544C` | `#B9B5AA` | 6.82 / 8.13 |
| `--ui-ink-3` | `#6E6A60` | `#9A9588` | 4.87 / 5.57 |
| `--ui-brand` / `--ui-on-brand` | `#16171A` / `#F6F3EC` | `#F2EFE7` / `#16171A` | 16.17 / 15.60 |
| `--ui-brand-soft` | `#E7E1D2` | `#2A2D35` | — |
| `--ui-success-soft/ink` | `#E7F3EA` / `#1F7A3D` | `#1B2A22` / `#63D68E` | 4.71 / 8.24 |
| `--ui-warn-soft/ink` | `#FBEEDA` / `#8A5300` | `#2B2517` / `#F2B84B` | 5.53 / 8.50 |
| `--ui-danger-soft/ink` | `#FBE9E7` / `#B3261E` | `#2E1B1D` / `#FF8A93` | 5.58 / 7.21 |
| `--ui-focus` | `#16171A` | `#F2EFE7` | 焦点环 2px + 2px offset |
| `--ui-ins-a` / `--ui-ins-b` | `#0B5FFF` / `#C81E48` | `#6E9BFF` / `#FF7D96` | 5.13 / 5.62（白底） |
| `--ui-scrim` | `rgba(22,23,26,.36)` | `rgba(8,9,12,.62)` | — |
| `--ui-shadow-1/2/3` | 1px/8px/24px 三级 | 同上加深度 | 3 级取代 6 个散装阴影 |

> 「对比度」列按 WCAG 2.1 公式实测（`scripts` 见第 8 节门禁）；**改动任一亮度必须重跑门禁**。

### 3.3 状态与密度

- 编辑器级密度：正文 13px、说明 12px、微标 11px；行高 1.5。
- 控件统一 `height` + `padding-inline` + `border-radius` 三元组，禁止用 `line-height` 撑高度。
- 悬停只改 `background`/`border-color`/`color`，不改尺寸（避免抖动）。
- 过渡只挂在 `background/border-color/color/transform/opacity`，时长 ≤ 240ms。

---

## 4. 组件规格

| 组件 | 规格要点 |
|---|---|
| 工具条按钮 `.tb-btn` | 32×32（`md` 控件高），图标 16px，`radius-md`；默认透明底；hover `subtle`；激活 = 墨底纸字；`aria-label` + `data-tip` 气泡；focus-visible 2px 焦点环 |
| 品牌位 `.tb-brand` | 22×22 墨底纸字方块（去紫），字重 700 13px；窄屏隐藏 |
| 工具条 `.tb` | 面 `surface` + 1px `line` + `shadow-2`，`radius-2xl`；拖动时关闭 `backdrop-filter`（性能，保留） |
| 浮窗 `.dock` | 面 `surface`，`radius-2xl`，1px line；标题条高 40，标题 13/700 + 图标 14 灰；收起只留标题条 |
| 树行 `.tree-row` | 高 30，`radius-md`，hover `subtle`，当前项 `brand-soft` 底 + 墨字；`role="treeitem"`、`aria-expanded`、Tab 可达 |
| 体系卡 `.sys-card` | 面 `elevated`，1px line，`radius-xl`；hover 只加 `line-strong`（不再紫边） |
| chip `.chip` | 11px，`radius-full`，`subtle` 底 + `ink-2` 字；画板 chip 用 `brand-soft` + `ink` |
| 菜单 `.menu` | `elevated` + `shadow-3` + `radius-xl`，`padding-1`；行高 30；当前项 `brand-soft`；键盘 ↑↓ + Enter |
| 弹层 `.modal` | 遮罩 `scrim` + 模糊 8px；盒 `elevated`，`radius-2xl`，宽 `min(560px, 92vw)`，`shadow-3`；四要素（点外/Esc/Tab 陷阱/焦点归还） |
| 输入 `.field` | 高 32（`lg`），`subtle` 底 + 1px line，`radius-md`；focus 换 `brand` 描边 + 焦点环；placeholder 用 `ink-3` |
| 分段 `.seg` | 外层 line 描边 `radius-full`，内段 28 高；激活 = 墨底纸字 |
| toast `.toast` | 底部居中胶囊，`elevated` + line2；普通 2.4s / 错误 4.2s（UED §19.5.3）；`role=status`/`aria-live=polite`，错误用 `role=alert`；进出 `translateY(8px)`，`reduced-motion` 下只切 `display` |
| 检查器 `.insp` | 宽 `--ui-inspector-w`，面 `surface`，左侧 1px line；分区标题 11px/700 letter-spacing .6；规格标签等宽 11px |
| 空态 `.empty` | 图标 32 + 标题 15/700 + 说明 12/1.7 + 行动按钮（有项目时给「打开设计体系」） |
| 检查标注 A/B | 框 1.5px、`ins-a`/`ins-b`；胶囊 11px 白字压在 `ins-a`/`ins-b`（实测 ≥5.1）；跨卡关系字号 11 |

---

## 5. 交互与可访问性（本轮修）

1. **废除原生对话框**：新增 `uiAsk()` / `uiConfirm()`，用体系弹层（点外/Esc/焦点陷阱/焦点归还），
   `prompt` 文本域 + `confirm` 危险按钮；`canvas.js` 里 5 处调用全部替换。
2. **toast 语义化**：容器 `role="status" aria-live="polite"`，错误切 `role="alert"`；
   时长 2.4s/4.2s；`prefers-reduced-motion` 时不动画。
3. **焦点环全覆盖**：`*:focus-visible` 兜底 + 组件级 2px `--ui-focus` + 2px offset；
   树行/菜单项/分段/图标按钮全部进入焦点清单。
4. **键盘**：菜单 ↑↓/Enter/Esc；树行 Enter 切换、Space 展开；弹层 Tab 陷阱；Esc 就近关闭（沿用现有优先级链）。
5. **`prefers-reduced-motion: reduce`**：一条全局规则把 `--ui-dur-*` 归零并关闭 `backdrop-filter` 动画。
6. **`@media (hover:none)`**：常驻 `node__del`、`tree__more` 等 hover 才出现的操作。
7. **触屏/窄屏**：`<980px` 隐藏品牌文字（已有），新增 `<760px` 浮窗整体收为图标条。
8. **主题**：`-theme` 参数生效（缺省改 **light 纸面**），`localStorage adago.theme` 持久化，
   `index.html` 内联 bootstrap 在 CSS 之前应用主题（无闪白）；`btn-theme` 写 localStorage。
9. **单一布局真源**：JS 用 `getComputedStyle(document.documentElement)` 读 `--ui-dock-w`、
   `--ui-inspector-w`、`--ui-gutter`、`--ui-node-head-h`，删掉 `292/288/296/320/148/64/12/HEAD_H` 副本。
10. **截图跟随主题**：连线/标签/背景色从令牌读（`--ui-brand`/`--ui-ink`/`--ui-surface`），
    不再写死 `#7c5cff`/`#16171d`。

---

## 6. 实施批次与文件

| 批次 | 内容 | 文件 |
|---|---|---|
| A | 新增令牌层（明暗双主题 + 标尺 + 布局常量） | **新增** `internal/canvas/static/tokens.css` |
| B | `canvas.css` 全量改写为令牌驱动（零色值、零字号字面、圆角/字号全部走 var） | `static/canvas.css` |
| C | 结构：`tokens.css` 先于 `canvas.css`；主题 bootstrap；`role`/`aria`；弹层骨架 | `static/index.html` |
| D | 行为：主题持久化、`uiAsk/uiConfirm`、toast 语义与时长、布局度量读令牌、截图读令牌、详情面板去内联 style | `static/canvas.js`、`static/inspect.js` |
| E | 配置：`-theme` 真正生效（服务端注入 `data-theme`），默认改 light | `internal/config/config.go`、`internal/canvas/server.go`、`internal/app/app.go`、`main.go` |
| F | 门禁：令牌完整性 + 对比度 + 禁字面值 + 结构断言 | **新增** `internal/canvas/ui_tokens_test.go` |
| G | 文档：`DESIGN.md` §4.4 增「界面设计系统」、`README.md` 增令牌一节 | `DESIGN.md`、`README.md` |

不改动：`internal/design/*`（8 套体系与预览皮肤）、MCP 工具契约、HTTP API、数据结构。

---

## 7. 验收

### 7.1 视觉验收（真机截图）

| 用例 | 通过条件 |
|---|---|
| 浅色 1600×1000 / 1366×768 | 纸面底、墨字、单色 chrome；无紫/青/橙残留；主按钮墨底纸字 |
| 深色同上 | 墨纸底；浮窗与画布明度分层可见；连线/标签可读 |
| 项目标签（全部视图） | 中性胶囊，白字对比 ≥4.5（实测目标 ≥7） |
| 检查模式 A/B | 蓝/红编码色与 8 套体系肤色互不干扰；胶囊文字 ≥4.5 |
| 空态 | 图标 + 标题 + 说明 + 行动点；不再只有两行灰字 |
| 弹层 | 遮罩/圆角/阴影与体系一致；Esc 与点外可关；焦点可见 |
| 截图导出 | 浅色主题导出浅色连线/标签，深色导出深色（不再混色） |

### 7.2 机器门禁（`ui_tokens_test.go`）

```bash
GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...
```

断言清单：

1. `tokens.css` 同时定义明/暗两套，且第 3.2 节全部语义令牌齐全（缺一即 fail）。
2. 对比度：`ink/canvas`、`ink-2/surface`、`ink-3/surface`、`on-brand/brand`、
   `success-ink/success-soft`、`warn-ink/warn-soft`、`danger-ink/danger-soft` ≥ 4.5（Go 侧按 WCAG 公式验算）。
3. `canvas.css` 内不得出现 `#hex` / `rgb(` / `rgba(`；`font-size` 与 `border-radius` 不得出现 px 字面值。
4. `static/**/*.js`（含 `js/` 下 12 个职责模块与 `inspect.js`）不得出现 `#hex`
   （`icons.js` 为生成物，路径数据无颜色，一并纳入扫描）。
5. 画布 JS（聚合扫描）不得出现 `prompt(` / `confirm(`，且必须包含
   `uiAsk` / `uiConfirm` / `isAskOpen` 的定义。
6. `index.html` 中 `tokens.css` 必须先于 `canvas.css`；含 `data-theme` 初始值。
7. `canvas.css` 必须有 `prefers-reduced-motion` 与全局 `:focus-visible` 规则。

### 7.3 兼容性

- 类名与 DOM 结构、元素 id 全部保留（`canvas.js`/`inspect.js` 的行为不变）；
  新增只有：`tokens.css`、弹层 DOM（`#ask-modal`）、若干 `aria` 属性。
- `canvas.json` 数据结构不变。已生成节点 HTML 固化在内，不受界面主题影响。
- 默认主题由 dark 改为 light 属**有意的观感变更**，记录在本文与 `DESIGN.md`；
  既有用户的一次性切换可通过 `-theme dark` 或界面按钮恢复，并持久化。

---

## 8. 风险与取舍

| 风险 | 取舍 |
|---|---|
| 单色 chrome 可能显得「不够工具感」 | 用 2px 焦点环、墨底激活态、三级阴影补足层次；颜色留给预览内容 |
| 浅色为默认会让老用户觉得「变了」 | 与 UED 缺省态一致；一次切换即持久化；`-theme dark` 可固定 |
| 450 个 px 字面值无法一次全部令牌化 | 只强制「颜色/圆角/字号/层级」走令牌，间距归到偶数网格并在文档登记 |
| 门禁禁止 hex 可能误伤渐变/描边需要 | 允许 `color-mix(in srgb, var(--ui-*) …)` 与令牌组合，仍属令牌驱动 |

---

## 9. 完成定义（DoD）

- [x] 审计度量与对比度证据（第 1 节）
- [x] 令牌表（第 3 节，含实测对比度）
- [x] 组件与交互规格（第 4、5 节）
- [x] 批次 A–E 落地，`go build/vet/test` 全绿
- [x] 门禁 `ui_tokens_test.go` 通过（8 组断言）
- [x] 明暗 × 两种宽度真机截图通过（第 7.1 节）
- [x] `DESIGN.md` / `README.md` 同步，`19-ued.md` 追加变更记录

---

## 10. 实施结果（2026-11-09）

### 10.1 改动清单

| 文件 | 变更 |
|---|---|
| **新增** `internal/canvas/static/tokens.css` | 明暗两套语义令牌 + 排版/间距/圆角/控件/布局/层级/动效标尺 |
| `internal/canvas/static/canvas.css` | 全量重写为令牌驱动：**0 个十六进制色值、0 个 rgb 函数、0 个字号/圆角 px 字面值、0 个层级魔法数**（原 39 / 16 / 6 档 / 13 档 / 10 个）；`var(--ui-*)` 引用 864 处，裸 px 字面从 450 降到 48（余下全是 `1px` 描边与少数固定布局宽度） |
| `internal/canvas/static/index.html` | `tokens.css` 先于 `canvas.css`；主题 bootstrap 脚本；`data-theme` 占位；`role`/`aria` 补全；新增 `#ask-modal` 骨架 |
| `internal/canvas/static/canvas.js` | 新增 `tokenPx/tokenColor/UI` 令牌读取、`uiAsk/uiConfirm/isAskOpen`、`focusables`；5 处原生对话框全部替换；toast 语义与时长；`fitView/leftDockWidth/visibleRect` 改读令牌；截图/便签取色改读令牌；空态三件套；树行 `role/aria/tabindex` + 键盘导航 |
| `internal/canvas/static/inspect.js` | A/B 标注色改读令牌（含 `--ui-on-ins` 深色反白）；标注层样式去写死色值；换主题重挂（`adago:theme-change`） |
| `internal/canvas/server.go` | 新增 `Theme` 字段、`ServeIndex` 注入主题（占位符替换）、`NormalizeTheme` |
| `internal/app/app.go` | 装配时把 `cfg.Theme` 传给画布服务 |
| `internal/config/config.go` | `-theme` 缺省改 `light`（白名单收敛） |
| `main.go` | `-theme` 帮助文案更新 |
| **新增** `internal/canvas/ui_tokens_test.go` | 8 组机器门禁 |
| `DESIGN.md` / `README.md` / `19-ued.md` | 规范与变更记录同步 |

### 10.2 门禁实测

```bash
GOWORK=off go build ./...   # ok
GOWORK=off go vet ./...     # ok
GOWORK=off go test ./...    # ok（canvas 3.8s / design 0.4s / mcp 18.6s）
```

| 断言 | 结果 |
|---|---|
| `TestUITokensComplete` | 24 个语义令牌 × 明暗两套齐全 + 5 个布局常量 |
| `TestUITokenContrast` | 11 组前景/背景 × 2 主题全部 ≥ 4.5（含 `--ui-on-ins` 对 A/B） |
| `TestCanvasCSSUsesTokensOnly` | `canvas.css` 零色值/零字号字面/零圆角字面/零层级魔法数 |
| `TestCanvasCSSAccessibilityRules` | `:focus-visible`、`prefers-reduced-motion`、`.empty-hint` 在位 |
| `TestCanvasJSNoLiteralColors` | `canvas.js`/`inspect.js` 零色值（`icons.js` 生成物豁免） |
| `TestCanvasJSNoNativeDialogs` | 代码行零 `prompt(`/`confirm(`，且 `uiAsk/uiConfirm/isAskOpen` 存在 |
| `TestIndexLoadsTokensFirst` | `tokens.css` 先于 `canvas.css`、`data-theme` 占位、`#ask-modal` 在位 |
| `TestServeIndexInjectsTheme` | `NormalizeTheme` 白名单收敛（light/dark/空/DARK） |

> `staticcheck` 本机二进制仍是旧版（导出数据 v2 vs 工具链 v5），无法运行；`go vet` 通过。
> 与 `IMPROVEMENTS.md §8.4` 记录的遗留一致。

### 10.3 真机验证（无头 Chromium + CDP 真实点击）

| 用例 | 结果 |
|---|---|
| 浅色 1600×1000 / 1366×768 | 纸面底 + 墨字 + 单色 chrome；主按钮墨底纸字；无紫/青/橙残留 |
| 深色 1600×1000 | 墨纸底；浮窗/画布/卡片三级明度可辨；文字 15.6:1 |
| 画布下拉菜单 | 面 `elevated` + `shadow-3`；当前项 `brand-soft`；行高 28 |
| 项目管理弹层 | 遮罩 + 6px 模糊；`radius-2xl`；Tab 陷阱；Esc/点外可关 |
| 重命名（ask 弹层） | 单行输入 32 高；Inline 校验；焦点自动落在输入框并全选 |
| 清空（confirm 弹层） | 危险按钮 `danger-soft` 底 + `danger-ink` 字；`role=dialog` |
| 检查模式 A/B（深色） | A `#6E9BFF` / B `#FF7D96`，胶囊字 `#14151A`（6.77 / 7.49）；胶囊与八套体系肤色无关 |
| 项目分组标签 | 中性胶囊 + `ink` 字，取代原橙色块（2.15:1 → 16:1） |
| 体系详情 | 色卡改为「色点 + 名字 + 值」列表（白字压白块事故消除）；字阶显示真实字号 |
| 空态 | 图标 + 标题 + 说明 + 行动按钮（新建项目/查看设计体系） |
| toast | `role=status`→`role=alert`；2.4s/4.2s；`toast-in` 入场 |
| 截图导出（深色） | 节点光栅化正常；画布底色/连线/标签色读令牌（浅色导浅色、深色导深色） |
| 控制台 | 全程无异常、无 warning |
| `-theme dark` 启动 | `/` 返回 `data-theme="dark"`；localStorage 覆盖生效 |

### 10.4 已知偏差与遗留

| 项 | 说明 |
|---|---|
| 检查标注字号 10px → 11px | 胶囊更可读，但更长；已在 A/B 与间距标注上验证不遮挡 |
| 项目分组标签由橙改中性 | 观感变化最大的单点；理由是原配色 2.15:1 不可读且是界面第三彩度 |
| 缺省主题 dark → light | 与 `19-ued.md` 缺省态一致；`-theme dark` 或界面按钮可固定 |
| `staticcheck` 不可用 | 工具链版本问题，非代码问题（见 10.2） |
| 间距/尺寸已令牌化 | `TestCanvasCSSGeometryUsesTokens` 强制 `padding/margin/gap/width/height/min-*/max-*/inset/outline-offset/background-size/backdrop-filter` 不得写裸值；`TestUITokenGeometryComplete` 锁住几何令牌清单。`canvas.css` 仅剩 `1px` 发丝线、`translate` 光学微调与 `@media` 断点三类白名单字面量 |

---

## 11. 画布阅读行为（2026-11-14）

三条来自实际使用的反馈，落成可验证的界面契约（门禁在
`internal/canvas/project_io_test.go` 的 `TestCanvasContentAdaptiveContract`）。

### 11.1 减少文本选择，避免打断操作

画布是「物件」而不是文档：点卡片、拖画布、跨浮窗划选都不该留下蓝色文本选区
（选区不参与拖拽，但会压住卡片、看起来像卡住了）。

| 位置 | 规则 |
|---|---|
| 画布 / 工具条 / 两侧浮窗 / 检查器 / 聚焦条 / 纸面 / 分组标签 / 空态 / toast | `user-select:none`（`canvas.css` 的「文本选择」段） |
| `#proj-name`、`#proj-note`、`#tree-filter`、`#ask-input` | 保留 `user-select:text`（输入类控件不改语义） |
| 指针落点 | `nodes.js` 在捕获阶段清掉遗留选区，早于控件自身 `pointerdown` |

### 11.2 画布长度随内容

纸面（`#canvas-paper`）不再固定 = 项目版式：

- **自由画布（随内容）**：纸面矩形 = 内容包围盒 + `--ui-canvas-pad`；「全部项目」视图
  左侧额外留 `--ui-canvas-label-pad` 给行首标签。标签写「4424 × 11219 随内容」。
- **固定比例**：仍以版式尺寸为基准，但内容溢出时按溢出边界延长，标签补「· 随内容延长」
  —— 不再出现「纸面只有 1280×800、后面的稿子留在纸外」。
- **卡片高度**：`auto_height` 节点按正文实测回写（组件预览量 `.ds-page` 可增可减；
  整页稿只向上扩，避免「量小→变矮→再量」的追高振荡）。
- **默认视角**：首屏 / 切项目 / 导入后用 `focusContent()` —— zoom-to-fit，**内容一律全部可见**
  （zoom 不设「可读下限」：宁可 19% 也不裁掉半张稿）。要看清单张：卡片标题栏的
  「单独看这张稿」按钮 / 双击卡片 = 单画布聚焦并整张适配（`fitNode`/`fitView`），
  有选中时 `F` 也看全所选；`Esc` 回画布。
- **分组不互相遮挡**：`POST /api/projects/separate` 只在项目包围盒相交时把**整组**
  沿 y 错开，保留组内排版（`/api/arrange` 的整块重排会把 1440 宽卡片折成一列，
  长画布不重叠但内容被拆散）；一次可撤销，幂等。

### 11.3 项目导出 / 导入

| 入口 | 位置 |
|---|---|
| 导出当前项目 / 导出全部项目 / 导入项目… | 左栏「项目 · ⋯」菜单 |
| 每行「导出」 | 项目管理弹层（可连续导出多个项目） |

- 导出走服务端 bundle（`adago-design/project` v1：项目 + 节点正文内联 + 连线 + 画布版式），
  浏览器存成 `<项目名>-<时间戳>.json`；`GET /api/projects/{id}/export`、`GET /api/export`。
- 导入是**纯增量**：`POST /api/import` 永远新建项目，同名自动加「（导入）」后缀，
  节点/连线 ID 全部重新生成、坐标与尺寸原样保留；导入后自动切到新项目并按内容对焦。
- 一次 `Ctrl+Z` 可整体撤销导入（撤销后若作用域指向被撤销的项目，画布自动回落「全部项目」）。

---

*变更：v1（2026-11-08）—— 初版：审计、决策、令牌表、组件规格、交互与可访问性、批次、验收、风险。*
*变更：v1.1（2026-11-09）—— 落地：补充第 10 节实施结果（改动清单、门禁实测、真机验证、偏差与遗留）。*
*变更：v1.2（2026-11-14）—— 新增第 11 节：减少文本选择、画布随内容延长与可读视角、项目导出/导入。*

---

## 12. 原生窗口壳（2026-11-15）

`serve` 缺省不再打开系统浏览器，而是 mygo 原生窗口内嵌同一张画布页。壳沿用
本文件的 `--ui-*` 令牌（同名同值），**不新增强调色**；设计图见画布项目
「adago-design · 原生内嵌窗口设计图」（09 视觉规范 / 10 实现契约）。

### 12.1 尺寸与分区

| 部位 | 规格 |
|---|---|
| 标题条 | 40px，`surface` 底 + 1px `line` 下边；右侧留 `c.TitleBar().Right` 给系统窗口按钮 |
| 内嵌页 | 整幅 WebView，不描边、不圆角、不内缩（与标题条/状态条只隔 1px 发丝线） |
| 状态条 | 26px，`surface` 底 + 1px `line` 上边；左读数、右计数，等宽数字 |
| 控件 | 图标按钮 28×28（图标 16）、状态胶囊 24 高、圆角 6；间距 10/2，左右内边距 12 |

### 12.2 分工（一层 chrome）

- 标题条：身份（印章 + 名称 + CANVAS kicker）、**作用域只读胶囊**、MCP 胶囊
  （就绪/截图任务中/写盘失败）、主题/重载/浏览器/更多、系统窗口按钮。
- 状态条：服务地址、数据目录（`~` 缩写）、项目/稿数、rev、持久化状态。
- 页面：工具条、双浮窗、卡片、检查器、截图任务——全部留在页面，不搬进原生层。
- 页面在 `?embed=native` 下隐藏 `.tb-brand` 与 `#btn-theme`；主题按钮只保留
  原生一处，点击同时切换壳（`ui.Theme`）与页面（`localStorage` + `data-theme`）。

### 12.3 状态

| 状态 | 表现 |
|---|---|
| 冷启动 | 启动卡：数据目录 ✓ / 服务 ✓ / 页面加载中 + 2px 进度线 |
| 页面故障 | 故障卡：错误码 + URL + 三个出口（重载 / 开发者工具 / 浏览器）；自动重试 1 次 |
| 实例占用 | 故障卡：已有实例地址 + 出口；不启动服务、不写数据 |
| 持久化失败 | MCP 胶囊转 danger「写盘失败」，状态条显示最近一次错误 |
| 二次启动 | 窗口前置 + 原生 toast「已切换到正在运行的窗口」 |
| 沉浸全屏 | 隐藏两条，页面铺满；右上角提示胶囊「全屏 · F11 退出」 |

### 12.4 与页面的契约

`index.html` 在样式前读 `location.search` 写 `data-embed="native"`；
`js/native.js` 每秒回写 `data-scope` / `data-scope-count` / `data-project-count` /
`data-node-count` 与 `document.title`；`canvas.css` 只多两条 `display:none` 规则。
机器门禁：`internal/canvas/embed_test.go`。
---

## 13. 设计体系管理弹层 v2（列表 + 详情，2026-11-15）

v1 是「新增表单常驻的单栏 760」；v2 改为 **960×720 两栏**：左 360 负责找（搜索 + 过滤 +
列表），右 560 负责看与改（详情只读，编辑按需进入）。视觉仍只用纸墨与状态色，
彩色只作为每个体系的主色块（数据）出现，并始终配等宽色值。

### 13.1 结构与尺寸

```
┌ 设计体系 ── 9 ── [搜索] ──────── [＋新增体系] ─ × ┐ 56
│ 全部9 内置8 自定义1 已停用0                        │ 44
│ ┌ 列表 360 ────────┐ ┌ 详情 / 编辑 560 ──────────┐ │
│ │ ● Ant Design 5  ⏻ │ │ [色块] 名称 内置 已启用  [编辑] │ │
│ │ ● Apple HIG     ⏻ │ │ 厂商/类型/画板/组件/主色/来源   │ │
│ │ ● 山茶茶饮 自定义 ⏻ │ │ Token 概览（颜色/字阶/间距/圆角）│ │
│ │ ● …              ⏻ │ │ 显示信息 + 只读说明            │ │
│ └───────────────────┘ └────────────────────────────┘ │
│（footer 在右栏：只在右栏内 恢复默认/移除 + 完成/保存） │ 44
└──────────────────────────────────────────────────────┘
```

| 令牌 | 值 | 用途 |
|---|---|---|
| `--ui-modal-w-wide` / `--ui-modal-h-wide` | 960 / 720 | 弹层尺寸（82vh 上限） |
| `--ui-sysm-w-left` | 360 | 左栏宽 |
| `--ui-sysm-head-h` | 56 | 弹层头高 |
| `--ui-sysm-row-h` | 64 | 列表行高 |
| `--ui-sysm-sw-h` | 18 | 开关高度（宽 32 = control-lg） |
| `--ui-sysm-search-w` | 230 | 搜索框 |
| `--ui-sysm-json-h` | 240 | JSON 编辑区 |

### 13.2 组件与规则

| 部件 | 规格 |
|---|---|
| 弹层 | `elevated` 底、1px `line`、`radius-2xl`、`shadow-3`；宽高 `min(令牌, 94vw/82vh)` |
| 头 | 标题 15/700、计数 mono chip、搜索（`subtle` 底，:focus-within 转 `elevated`）、主按钮、关闭 |
| 过滤条 | 4 个分段（全部/内置/自定义/已停用）+ 等宽计数；激活 = 墨底纸字 |
| 列表行 | 28 色块 + 名称 13/500 + 内置/自定义/已停用 chip + `vendor·family·id`；右端 32×18 开关；选中行左 2px 墨条 + `brand-soft` |
| 详情 | 40 色块、17/600 名称、chips；kv 两列；4 个 Token 概览 tile；停用态 warn 横幅 + 启用 |
| 编辑 | 右侧面板（返回详情）；创建含基底/主色/10 阶色板预览；自定义多「完整 JSON」页签 |
| 错误 | 字段红边 + 行内 `.sysm-err`；JSON 错误用 `danger-soft` 代码块；保存失败不写盘 |
| 空态 | 左列表（搜索图标 + 两句）与右栏（ring + 引导）分开表达 |
| 强制 | 移除走 `uiConfirm`；Esc 分级；Tab 陷阱；关闭焦点回「管理」；不新增颜色 |

### 13.3 状态与键盘

| 状态 | 表达 |
|---|---|
| 启用 / 停用 | 开关 on/off；停用行降透明 + warn chip；详情横幅说明「只影响选择面」 |
| 内置 / 自定义 | 中性 chip / brand chip；内置无「移除」，显示「恢复默认」（无覆盖时 disabled） |
| 已改显示信息 | 详情 chip；「恢复默认」清除 overrides 并重新启用 |
| 选中 / 搜索无结果 | 墨条 + `brand-soft` / 双空态 |
| 键盘 | ↑↓ 选择、Enter 编辑、Space 启停、n 新增、/ 搜索、Esc 编辑→详情→关闭、Tab 循环 |

### 13.4 接线与门禁

- 任何变更成功后：重拉 `?all=1`、刷新左侧目录（`loadCatalog`）、收起体系详情；
  停用只影响目录/搜索/预览/MCP 工具，画布上的节点不回收。
- 管理 API 与 `design_system_manage` 不变；新增只读 `GET /api/catalog/palette` 供
  新增表单实时预览主色 10 阶色板。
- 门禁：`ui_tokens_test`（新样式零色值/零字号字面量、新令牌存在）、
  `module_graph_test`（systems-admin 依赖可达）、`admin_route_test`（路由/错误码/色板）。
## 14. SVG 图标库管理弹层 v1（2026-11-15）

与体系管理同壳的 **960×720 两栏**：左 360 = 搜索 + 过滤 + 图标列表；右 560 = 详情
（尺寸档 / 用法 / 元信息 / SVG 源码）或新增/编辑表单。视觉不新增颜色：glyph 一律
继承 `currentColor`，选中/停用/自定义只用纸墨与状态色表达；彩色只作为图标自身
fill 的数据出现。设计稿在画布项目「adago-design · SVG 图标管理」（7 张）。

### 14.1 结构与尺寸

```
┌ 图标库 ── 74 ── [搜索] ──────── [＋新增图标] ─ × ┐ 56
│ 全部74 内置74 自定义0 已停用0                     │ 44
│ ┌ 列表 360 ────────┐ ┌ 详情 / 表单 560 ──────────┐ │
│ │ ▣ chevron-right ⏻ │ │ [48 icon] 名称 内置 已启用 [编辑] │ │
│ │ × close         ⏻ │ │ 尺寸档 12/14/16/20/24          │ │
│ │ ⌕ search        ⏻ │ │ 用法（精灵 / 独立 SVG，复制）    │ │
│ │ ◆ brand-mark 自定义⏻│ │ 元信息：分组/别名/来源/viewBox   │ │
│ └───────────────────┘ │ SVG 源码（内置只读 / 自定义可编辑）│ │
│（footer 在右栏：恢复默认/删除 + 完成/保存）        │ 44
└──────────────────────────────────────────────────────┘
```

| 令牌 | 值 | 用途 |
|---|---|---|
| `--ui-icm-row-h` | 56 | 图标列表行（比体系行矮，glyph 更密） |
| `--ui-icm-glyph` | 32 | 列表图标格（glyph 20） |
| `--ui-icm-hero` | 48 | 详情 hero 图标格（glyph 24） |
| `--ui-icm-size-h` | 62 | 尺寸档格高 |
| `--ui-icm-src-h` | 118 | 详情 SVG 源码块 |
| `--ui-icm-code-h` | 148 | 表单 SVG 编辑区 |
| `--ui-icm-prev-h` | 84 | 表单实时预览区 |
| `--ui-icm-empty-max-w` | 320 | 空态文案宽度 |

### 14.2 组件与规则

| 部件 | 规格 |
|---|---|
| 列表行 | 32 图标格（`subtle` 底 + 1px line）+ 等宽名称 12/600 + 内置/自定义/已停用 chip + `group · 别名`；右端 32×18 开关；选中行左 2px 墨条 + `brand-soft` |
| 详情 | 48 图标格、17/600 等宽名称、chips；尺寸档 5 格；两种用法行（等宽 10 + 复制）；kv 两列（分组/别名/来源/viewBox）；SVG 源码 `pre`（内部滚动，不换行折行） |
| 新增 / 编辑 | 右侧面板；名称（内置/编辑态只读）+ 分组下拉 + 别名；SVG `textarea.icm-code` + 实时预览 + 允许/禁止清单；内置编辑只显示只读源码；标题条另有「设计图标」进入 §15 |
| 设计入口 | 详情头部：内置「以它为起点设计」、自定义「设计器编辑 / 用设计器改写」；有 design 的图标显示「可再设计」chip；表单底部「改用可视化设计」 |
| 预览 | `DOMParser` 先校验再注入；拦截 script/foreignObject/text/image/style/use、`on*`、`style`、外链 `href`/`url()`；失败显示原因而不是空白 |
| 错误 | 行内 `.sysm-err`：名称必填/格式/重名、SVG 必填/解析失败/不允许的元素 |
| 强制 | 移除走 `uiConfirm` 并提示引用会变空白；Esc 分级；Tab 陷阱；关闭焦点回「图标库」；不新增颜色 |

### 14.3 状态与键盘

| 状态 | 表达 |
|---|---|
| 启用 / 停用 | 开关 on/off；停用行降透明 + warn chip；详情横幅说明「只影响选择面，精灵仍保留」 |
| 内置 / 自定义 | 中性 chip / brand chip；内置无「删除」，显示「恢复默认」（无覆盖时 disabled） |
| 已改分组/别名 | 详情 chip；「恢复默认」清除 overrides 并重新启用 |
| 选中 / 搜索无结果 | 墨条 + `brand-soft` / 双空态 |
| 键盘 | Up/Down 选择、Enter 编辑、Space 启停、n 新增、/ 搜索、Esc 表单→详情→关闭、Tab 循环 |

### 14.4 接线与门禁

- 入口：工具条 `#icons-open`（栅格图标）；弹层 `#icons-modal`，DOM 为
  `#icons-search / #icons-design / #icons-new / #icons-segs / #icons-rows / #icons-right`。
- 「设计图标」先收起图标库再打开 `#icd-modal`（两个弹层互斥）；返回/保存通过
  `openIconManager(focusName)` 恢复并定位到刚编辑的图标。
- 每次变更成功后重拉 `GET /api/catalog/icons`；停用不影响精灵与已有模板引用。
- 与体系管理共用的 bug 修复：从「新增」返回时按 `selected` 重拉详情（不再停在
  「加载中…」）；列表滚动位置不随 `innerHTML` 丢失选中行（`scrollIntoView`）。
- 门禁：`ui_tokens_test`（零色值/零字号字面量、新令牌存在）、
  `module_graph_test`（`icons-admin.js` 从入口可达、导出齐全）、
  `admin_route_test`（图标路由/状态码/错误码映射）、`design/icons_manage_test`。

## 15. 图标设计器（24 网格画板，2026-11-16）

图标库从「粘贴 SVG」扩到「画图标」：同一份几何文档既是画板状态，也是落库的 `design`，
服务端再渲染出确定性 SVG。设计器是独立弹层，与图标库互斥打开，返回/保存后回到图标库。

### 15.1 结构与尺寸

`#icd-modal > .modal__box.icd-box > .icd`：头部 56、主体（左工具条 52 / 画板区 flex / 右属性栏 300）、底部操作条。

| 令牌 | 值 | 用途 |
|---|---|---|
| `--ui-icd-w` | 1060 | 设计器弹层宽度 |
| `--ui-icd-h` | 720 | 设计器弹层高度（视口 88vh 封顶） |
| `--ui-icd-rail-w` | 52 | 左侧工具条 |
| `--ui-icd-panel-w` | 300 | 右侧属性栏 |
| `--ui-icd-board` | 372 | 24 网格画板边长（15.5px/格） |
| `--ui-icd-tool` | 36 | 工具按钮 |
| `--ui-icd-row-h` | 32 | 图层行 |
| `--ui-icd-preview-tile` | 52 | 尺寸预览格 |
| `--ui-icd-code-h` | 132 | SVG 交付物预览块 |
| `--ui-icd-dark-bg/ink` | 恒深底/浅字 | 暗底检查，不随主题反转 |

### 15.2 组件与规则

| 部件 | 规格 |
|---|---|
| 工具条 | 52 宽竖排；V/L/R/O/P/G 六个等宽按键；选中 `brand` 底 + `on-brand` 字；`data-tip` 提示快捷键 |
| 画板 | 372 方形、`elevated` 底、1px `line-strong` 边、`radius-xl`；网格 1 单位（0.5px 发丝线）、主网格每 4 单位、安全区 20×20 虚线、光学圆 r=10；`vector-effect:non-scaling-stroke` 保证缩放不变粗 |
| 图形 | 与落库同一渲染语义：根 `<g fill=none stroke=currentColor stroke-width=1.8 cap/join=round>`；单图形可实心/无描边/透明度；`hidden` 不导出 |
| 选中 | 蓝色（`ins-a`）虚线包围盒 + 4 角圆点；直线额外显示两个端点；包围盒用 `getBBox()` 量取（path 也适用） |
| 属性栏 | 分段：图标信息（名称/分组/别名）、光学尺寸预览（12/16/20/24 + 暗底）、图形图层、选中图形（坐标/尺寸/圆角/实心/描边/不透明度/复制/层级/删除）、描边与网格、SVG 交付物、错误行 |
| 预览 | 客户端按同一几何即时渲染 CSS 尺寸 12–24；保存时服务端重新校验并生成落库 SVG，二者语义一致 |
| 底部 | 左说明（新建/编辑/以内置为起点三种文案）+ 取消 + 保存并启用/保存修改；错误同时进 toast 与行内 |

### 15.3 状态与键盘

| 状态 | 表达 |
|---|---|
| 新建 / 编辑 / 以内置为起点 | 头部 chip；编辑态名称只读；克隆预填 `<name>-custom` 且原图标不变 |
| imported | 「改写现有图标」+ 说明由 SVG 反推；复杂 SVG（渐变/裁剪/transform）打开失败，提示改用源码编辑 |
| 未保存 | `dirty` 时关闭/返回走 `uiConfirm`；保存成功后不确认，直接回图标库并定位 |
| 撤销/重做 | 40 步快照；头部按钮禁用态随 `history index` 变化 |
| 键盘 | V/L/R/O/P/G 切工具、Delete 删除选中、方向键 0.5（Shift 1）微移、Ctrl/⌘+Z 撤销、Shift+Z / Ctrl+Y 重做、Enter/双击结束折线、Esc 取消折线或关闭、Tab 陷阱 |

设计器打开时 Del/Backspace、Ctrl+Z 必须 `stopImmediatePropagation`，不能落到画布的节点删除 / 服务端历史；
Esc 要排在 main.js 的全局 Esc 之前（模块求值顺序 + capture）。

### 15.4 接线与门禁

- 文件：`static/js/icon-designer.js`（交互与客户端渲染）、`static/js/icons-admin.js`（入口/回调）、
  `static/index.html`（`#icd-modal` 骨架）、`canvas.css` + `tokens.css`（§15.1 令牌与样式）。
- 数据：`GET /api/catalog/icons/{name}/design` 打开；保存走 `POST/PUT /api/catalog/icons` 的 `design`；
  设计稿持久化在 `custom-icons/<name>.json`。
- 门禁：`ui_tokens_test`（令牌/零字面值/零原生对话框）、`module_graph_test`（新模块从入口可达）、
  `canvas/admin_route_test`（design 路由）、`design/icondesign_test`（渲染/导入/持久化）、
  `mcp/icons_test`（design 动作）。真机验证：ego-browser 画矩形+圆 → 保存 → 详情「可再设计」→
  再打开编辑 → 撤销/重做 → 内置 close「以它为起点」另存。

## 16. 品牌 Logo 工作台（2026-11-16）

与图标库/图标设计器同级的新能力：一个 **1120×720** 弹层内完成「资产库 + 设计 + 规范 + 导出」。
左侧是已保存 Logo，中间是三变体预览/编辑画板，右侧是检查器或表单；标记选择用浮层。

### 16.1 结构与尺寸

| 令牌 | 值 | 用途 |
|---|---|---|
| `--ui-lgm-w` | 1120 | 工作台宽度（视口 96vw、88vh 封顶） |
| `--ui-lgm-h` | 720 | 工作台高度 |
| `--ui-lgm-left-w` | 264 | 资产列表 |
| `--ui-lgm-panel-w` | 320 | 检查器/表单 |
| `--ui-lgm-row-h` | 64 | 列表行高（缩略 Logo + 名称 + 元信息） |
| `--ui-lgm-mark-cell` | 36 | 标记格 / 缩略格 |
| `--ui-lgm-preview-h` | 300 | 预览画布最小高 |
| `--ui-lgm-size-tile-h` | 72 | 光学尺寸档格高（含 `px` 标签） |
| `--ui-lgm-markpick-w/h` | 430 / 320 | 标记选择浮层 |
| `--ui-lgm-guide-inset` | 6 | 安全空间参考线内缩 |

### 16.2 组件与规则

| 部件 | 规格 |
|---|---|
| 列表行 | 36 缩略格（服务端正形 SVG，`max-width/height:100%`）+ 名称 12/500 + `layout · system · 更新日期`；选中行 `subtle-strong` 底 + 左侧 2px 墨条 |
| 变体标签 | 正形 / 反形 / 单色三枚；选中 `brand` 底；detail 与 edit 共用，切换同步大图与尺寸带 |
| 稳定预览 | 服务端 SVG（`width/height/viewBox` 显式），CSS 以 `max-width/max-height:100%` 适配 stage；不依赖页面精灵 |
| 光学尺寸带 | 64/40/24px 三档；每格左下 `px` 标签；宽度低于 spec.minWidth 时 `warn` 底 + 边框，tooltip 说明实际宽 |
| 编辑预览 | HTML/CSS 锁排：`lgm-lockup--horizontal/stacked/mark/wordmark`，padding=安全空间，gap/字号/字距/颜色来自文档；安全空间开关加 `lgm-guide` 虚线框 |
| 检查器（详情） | 资产元信息、安全空间/最小宽高/变体、字体授权、对比度警告、导出（SVG/PNG/复制）与摆上画布 |
| 表单（编辑） | 资产（名称/备注/体系+应用主色）、品牌文字、标记（选择/自绘）、布局、字体（字重/字号/字距/大小写/副标字号）、组合（标记尺寸/间距/安全空间）、颜色与透明底 |
| 标记选择浮层 | 搜索图标名/分组/别名；6 列图标格；底部「画新标记」直达图标设计器；选择后自动关闭 |
| 错误 | 面板底部 `lgm-err`（保存失败/校验），同时 toast；文件/表单不改动画布其它状态 |

### 16.3 状态与键盘

| 状态 | 表达 |
|---|---|
| empty / detail / edit | 空态双「新建 Logo」；详情三变体；编辑实时预览 + 未保存标记 |
| 未保存离开 | 关闭弹层/切换资产/新建/取消编辑都走 `uiConfirm`；保存成功直接回详情并定位新资产 |
| 标记缺失 | 图标被删除时编辑预览显示「标记缺失」；服务端加载会把 Logo 降级为纯字标而非丢资产 |
| 导出 | SVG 走 `/export` 附件；PNG 在浏览器 4× 光栅化（透明底）；SVG 字标为文本，提示转曲/授权 |
| 键盘 | Esc 关闭标记浮层/关闭工作台（未保存先确认）、Tab 陷阱；Delete/Backspace/Ctrl+Z 在设计器内被拦截，不落到画布节点/历史 |

### 16.4 接线与门禁

- 文件：`static/js/logo-workbench.js`、`index.html`（`#logos-open` + `#logos-modal`）、`canvas.css` + `tokens.css`（§16.1 令牌）。
- 数据：`GET /api/catalog/logos` / `{id}` / `{id}/canvas` / `{id}/export`，`/api/catalog/fonts`，`POST/PUT/DELETE /api/catalog/logos`。
- 与画布的桥：`POST /api/nodes` 写入自包含 Logo HTML（`kind=component`，标签 `logo` / `logo:<id>` / `variant:*`）。
- 门禁：`ui_tokens_test`（令牌/零字面值/零原生对话框）、`module_graph_test`（新模块从入口可达）、
  `canvas/admin_route_test`（Logo/字体路由）、`design/logodoc_test` + `logos_manage_test`（布局/变体/规范/持久化）、
  `mcp/logos_test`（design_logo 动作）。真机验证：新建 → 标记选择 → 横/竖排 → 保存 → 反形/单色 → SVG 导出 → 摆上画布。
