# adago_design — 设计体系 MCP（原生窗口 · 内嵌无限画布）

一个「设计体系 + 无限画布」的 MCP 服务器：AI Agent 通过 MCP 工具读取
多套设计体系的 Design Token 与组件规范，把组件预览（真实 HTML+CSS 渲染）以
卡片形式摆到无限画布上，做多体系并排对比、组合页面、截图回传。
画布跑在 **mygo 原生窗口内嵌的 WebView2** 里（缺省形态），也可以用
`-browser` 回到系统浏览器。

内置八套体系（Token 基于各体系公开设计规范整理，为近似实现）：

| id | 体系 | 主色 | 主题 |
|---|---|---|---|
| `apple` | Apple HIG (iOS) | systemBlue `#007AFF` | light / dark |
| `volcano` | 火山引擎 Arco Design | arcoblue-6 `#165DFF` | light / dark |
| `douyin` | 抖音设计体系 | 抖音红 `#FE2C55` + 电光青 `#25F4EE` | light / dark |
| `xiaomi` | 小米 MIUI / HyperOS | 小米橙 `#FF6900` | light / dark |
| `element` | Element Plus | primary `#409EFF` | light / dark |
| `ant` | Ant Design 5 | daybreak `#1677FF` | light / dark |
| `tdesign` | 腾讯 TDesign | brand `#0052D9` | light / dark |
| `material` | Material Design 3 | 基线紫 `#6750A4` | light / dark |

组件（一份 HTML 结构 × 每体系一份皮肤 CSS）：button / input / select / switch /
checkbox / radio / slider / card / tag / badge / avatar / alert / progress /
navbar / tabbar / list / dialog / toast / tabs / table / pagination。

体系数据的单一真源是 [`internal/design/data/*.json`](internal/design/data)
（8 套内置体系，`gen_export` 可校验/回写）；新增或修订体系改 JSON 即可，
颜色合法、字体可商用、组件可渲染等不变量仍由门禁测试把关。

## 项目 → 规范 → 预览（设计稿必须归属项目）

一个**项目**对应一块画布作用域：画布只显示当前项目的节点，导出也按项目分。
设计稿**不允许**挂空项目 —— 这条约束拦在 `canvas.Store.Add` 里，HTTP 直连与 MCP 工具
两条写入路径都绕不过去；未建项目就新增会返回 `project_required`。

```
project_create（或 project_list 选已有）
      └─► design_preview / design_screen / canvas_add / design_audit …（都要带 project）
              └─► 画布只显示该项目；canvas_export 也只导该项目
```

> 升级说明：`canvas.json` v2→v3 引入项目归属；v3→v4（当前）把节点正文拆到
> `nodes/<id>.html` 分文件、索引只存元数据。旧数据在首次加载时自动迁移
> （无主节点归入固定项目 `p-default`「默认项目（历史数据）」），不会丢稿。

### 项目导出 / 导入（换机器、交给同事）

左栏「项目 · ⋯」菜单里有 **导出当前项目 / 导出全部项目 / 导入项目…**，
项目管理弹层每行还有「导出」。导出是单文件 JSON（项目 + 节点正文内联 + 连线 +
画布版式），导入是**纯增量**：永远新建项目，同名自动加「（导入）」后缀，
节点/连线 ID 重新生成而坐标、尺寸、画板原样保留，导入后自动切到新项目。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/projects/{id}/export` | 导出单个项目（`Content-Disposition: attachment`） |
| GET | `/api/export` | 导出全部项目（workspace bundle） |
| POST | `/api/import` | 导入单项目或 workspace bundle；返回新建项目的 id/名称/节点数 |
| POST | `/api/projects/separate` | 只在项目分组互相遮挡时把整组沿 y 错开（保留组内排版，可撤销） |

格式标识 `adago-design/project` / `adago-design/workspace`，`version: 1`；
版本不符或 `kind`、画布版式非法时整批拒绝（`bad_bundle`），不会留下半个项目。
一次 `Ctrl+Z` 可整体撤销导入。

### 项目设计文稿（PDF）

左栏「项目 · ⋯ → **导出项目 PDF（系统设计文稿）**」与项目管理弹层行内的 **PDF**
按钮，把当前项目生成一份可评审、可归档的多页 PDF：

| 页 | 内容 |
|---|---|
| 封面 | 项目名、备注、设计稿/体系/分组框/文稿页计数、生成时间；墨底纸字 |
| 概览 | 项目信息（画布/创建/更新/ID）、设计体系（名称/厂商/主色/稿数）、设计稿构成、项目备注 |
| 设计索引 | # / 设计稿 / 体系 / 类型 / 画板 / 尺寸 / **文稿页**；稿多时自动分页 |
| 逐稿页 | 一稿一页：编号、标题、体系/类型/画板/尺寸、等比预览、标签/ID/更新时间/页码 |

- **一稿一页**，按画布位置（先上下、再左右）收录；**分组框不单独成页**（只作背板计数）。
- 纸张按稿件宽高比自动选 **A4 横向 / 纵向**（竖稿不少于横稿时用纵向）；预览只缩不放，
  逐稿页标出「预览 44%」这类印刷尺度，不裁切、不改画板结构。
- 原生窗口（缺省）下直接下载 PDF：隐藏窗口加载只读文稿页 → WebView2
  `PrintToPDF` → 前端 Blob 存到**系统下载目录**（Windows 通常是
  `%USERPROFILE%\Downloads`），文件名为 `<项目名>-系统设计文稿-YYYYMMDD-HHmm.pdf`；
  重名自动加 ` (2)`。生成时主画布仍可编辑，下载完成标题条会提示文件名；
  找不到文件时用标题栏 `⋯ → 打开下载目录（导出文件）`，或托盘里的「打开下载目录」。
- 浏览器 / 无窗口形态没有内嵌渲染器：打开 `/projects/{id}/print?print=1` 打印页，
  在打印对话框选「另存为 PDF」（勾选「背景图形」）。
- 空项目（或只有分组框）返回 `409 empty_project`；当前形态无渲染器返回
  `501 pdf_unavailable`；渲染失败/超时 `502 pdf_failed`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/projects/{id}/print` | 只读设计文稿页（HTML）；`?print=1` 就绪后自动 `window.print()` |
| POST | `/api/projects/{id}/pdf` | 生成并下载 PDF（原生形态）；无渲染器时 501 |

文稿页与 PDF 生成都不写盘、不改项目数据；体系名称与主色来自目录摘要，
非法主色值不会进入样式。完整设计见 [DESIGN-PDF-EXPORT.md](DESIGN-PDF-EXPORT.md)
（PDF 版见 [DESIGN-PDF-EXPORT.pdf](DESIGN-PDF-EXPORT.pdf)）；设计稿在
`.design/pdf-export/`（设计说明 / 入口与反馈 / 渲染流程 / 封面 / 内页 /
视觉规范 / 实现契约）。

## 三层规范（先定尺寸 → 定体系 → 出预览）

| 规范 | 入口 | 说明 |
|---|---|---|
| 画板（尺寸） | `design_artboard` | 8 块画板：iPhone 15 / SE、Android 360、微信小程序、iPad、Web 1280/1440、桌面应用壳。给出内容区、安全区、栅格、导出倍率与「尺寸契约」文本 |
| 字体（授权） | `design_fonts` | 字体注册表带商用/嵌入/再分发三个标记；默认栈是 Inter + Noto Sans SC（OFL），全平台可复现 |
| 图标 | `design_icons` | 74 个内置 24 网格线性图标 + 运行期自定义 SVG；画布内可视化设计器可画图形并落库为可再编辑的 design + SVG；`{{icon:name:size}}` 占位符在渲染时展开为内联精灵 |

三条硬规则：

1. **先定画板**。每个预览节点都带 `artboard` 字段，宽高由画板推导，不再有 420/390/780 这类魔法数。
2. **禁止 emoji 当图标**。预览文档内联 `<symbol>` 精灵，使用处写 `<svg class="ds-icon"><use href="#ds-ic-close"/></svg>`；
   `design_audit` 会扫描 emoji 与 `▾ › ✕ ◉` 等字符符号并判 fail。
3. **字体必须可商用**。系统字体只能作尾部兜底（`design_audit` 给 warn），
   `Redistributable=false` 的品牌字体（如 MiSans）不得随包分发字体文件。

图标库改动后跑一次 `GOWORK=off go generate ./internal/design`，
会从 Go 侧路径表生成画布 UI 用的 `internal/canvas/static/icons.js`，两侧永不漂移。
内置图标仍走这条构建期同步；**自定义 SVG 图标**在运行期从
`<data>/custom-icons/*.json` 加载，不需要改代码（见下一节）。

## 设计体系管理（动态）

左侧「设计体系」浮窗标题条上的 **管理** 打开管理弹层（原生窗口/浏览器里都可）。
v2 弹层为 **960×720 两栏**：左 360 = 搜索 + 过滤（全部/内置/自定义/已停用）+ 列表；
右 560 = 详情，点「编辑」才进入表单；新增/编辑共用右侧面板。

| 能力 | 说明 |
|---|---|
| 新增 | 品牌派生：名称 + 结构基底 + 品牌主色（#RRGGBB），可填厂商/落地类型/ID/描述；**主色旁实时预览 10 阶色板**，字型/间距/圆角/阴影/动效继承基底 |
| 编辑 | **内置**：改显示信息（名称/厂商/描述/一句话风格），Token 不变；用「恢复默认」清除覆盖；**自定义**：显示信息 / 完整 JSON 两个页签（校验 colors/type/radius） |
| 停用 / 启用 | 行内开关；只影响「选择面」：目录、搜索、`design_preview` 与 MCP 工具不再出现该体系；**画布上已生成的稿不回收** |
| 恢复默认 | 仅内置体系：清掉显示信息覆盖并重新启用 |
| 移除 | 仅自定义体系；内置体系受保护（提示改为停用），避免误删 8 套内置规范 |
| 键盘 | ↑↓ 选择、Enter 编辑、Space 停用/启用、n 新增、/ 搜索、Esc 分级返回/关闭、Tab 陷阱 |

- 持久化：自定义体系在 `<data>/custom-systems/<id>.json`；停用列表与内置显示信息覆盖在 `<data>/system-settings.json`（tmp→rename 原子写）。启动顺序：先加载自定义体系，再套用设置。
- 同一套能力也暴露给 Agent：MCP 工具 **`design_system_manage`**（`list / create / update / disable / enable / remove / reset`）。
- 管理 API（全部回环 + 写操作同源校验）：

| 方法/路径 | 说明 |
|---|---|
| `GET /api/catalog/systems` | 已启用体系摘要；`?all=1` 含停用（管理面） |
| `GET /api/catalog/systems/{id}` | 体系详情（含停用，管理面需要） |
| `GET /api/catalog/systems/{id}/json` | 自定义体系完整 JSON |
| `GET /api/catalog/palette?primary=#RRGGBB` | 主色的 10 阶派生色板（新增预览） |
| `POST /api/catalog/systems` | 新增 `{name,vendor?,family?,base,primary,id?,description?}` |
| `PUT /api/catalog/systems/{id}` | 编辑显示信息，或自定义 `{json}` 整体替换 |
| `POST /api/catalog/systems/{id}/state` | `{disabled: true\|false}` |
| `POST /api/catalog/systems/{id}/reset` | 内置恢复默认 |
| `DELETE /api/catalog/systems/{id}` | 移除自定义体系 |

设计稿：画布项目「adago-design · 设计体系管理弹层 v2」（说明 / 默认 / 停用 / 新增 /
JSON / 空态 / 视觉规范 / 实现契约）。

错误码：`unknown_system` / `system_duplicate` / `system_builtin_protected` / `invalid_system`。

## SVG 图标管理与设计（动态）

画布工具条上的 **图标库** 按钮（栅格图标，主题切换左侧）打开管理弹层。
与体系管理同壳：**960×720 两栏**，左 360 = 搜索 + 过滤（全部/内置/自定义/已停用）+ 列表，
右 560 = 详情（尺寸档 / 用法 / 元信息 / SVG 源码）或新增/编辑表单。
标题条的 **设计图标** 打开独立的 **24 网格图标设计器**（见下节）。

| 能力 | 说明 |
|---|---|
| 浏览 | 每行 32 图标格 + 等宽名称 + 分组/别名 + 行内开关；glyph 始终跟随 currentColor；有设计稿的图标带「可再设计」 |
| 详情 | 尺寸档 12 / 14 / 16 / 20 / 24、精灵引用与独立 SVG 两种用法（一键复制）、分组/别名/来源/viewBox、SVG 源码；「设计器编辑 / 以它为起点设计」 |
| 设计 | 24 网格画板：直线 / 矩形 / 圆 / 椭圆 / 折线 / 多边形 / 路径；0.5 吸附、Shift 约束、Alt 细调；图层顺序/显隐；40 步撤销；12–24 实时预览与暗底检查 |
| 新增 | 两种入口：**设计图标**（画图形，保存 design + SVG）或 **粘贴 SVG**（完整 SVG / 路径片段 + 实时预览并拦截 script/text/外链/on*） |
| 编辑 | **内置**：只能改分组/别名（写覆盖），SVG 只读；**自定义**：分组/别名/SVG 都可改；有 design 的用设计器继续编辑，手改 SVG 会清掉 design |
| 停用 / 启用 | 行内开关；只影响「选择面」（`design_icons`、列表计数、检索），**精灵仍包含**，旧模板与已有设计稿引用不会变空白 |
| 恢复默认 | 仅内置图标：清掉分组/别名覆盖并重新启用 |
| 移除 | 仅自定义图标；内置图标受保护（提示改为停用）；移除前确认，并说明画布上引用会变空白 |
| 键盘 | Up/Down 选择、Enter 编辑、Space 停用/启用、n 粘贴 SVG、d 设计图标、/ 搜索、Esc 分级返回/关闭、Tab 陷阱 |

### 图标设计器（24 网格）

- 入口：图标库标题条 **设计图标**（新建）或详情 **设计器编辑 / 以它为起点设计**。
- 画板：24×24 网格、2 单位安全区、视觉直径 20 的光学圆；所有图形统一 `currentColor` + 1.8 描边（可调 1.2–3）。
- 工具：选择/移动（拖动 + 方向键 0.5 微移）、直线、矩形、圆、椭圆、折线/多边形（逐点点击，Enter/双击结束，点回起点闭合）。
- 图层：后画的在上；可显隐、复制、上移/下移、删除；选中后在右侧改坐标/尺寸/圆角/实心/描边/不透明度。
- 预览：12 / 16 / 20 / 24 四档 + 暗底检查；SVG 交付物实时生成，所见即落库内容。
- 保存：`POST /api/catalog/icons`（新建）或 `PUT /api/catalog/icons/{name}`（编辑），请求体带 `design`；
  内置图标只读，保存会以 `name-custom` 新建，原图标不变。
- 打开已有图标：优先读 `custom-icons/*.json` 的 `design`；没有 design 的（含全部内置）从 SVG 反推为几何图形（标记 `imported`），含渐变/裁剪/transform 的复杂 SVG 会拒绝并提示改用源码编辑。

- 持久化：自定义图标在 `<data>/custom-icons/<name>.json`（名称 + 分组 + 别名 + SVG 原文 + 可选 `design`）；
  内置图标的停用列表与分组/别名覆盖在 `<data>/icon-settings.json`（tmp→rename 原子写）。
  启动顺序：先加载自定义图标，再套用设置。
- 同一套能力也暴露给 Agent：MCP 工具 **`design_icon_manage`**
  （`list / design / create / update / disable / enable / remove / reset`）；
  `design` 动作接受几何文档并返回确定性 SVG，也可传 `name` 打开已有图标或传 `svg` 反推文档。
- `design_icons` 自动包含自定义图标、隐藏已停用图标；`format=inline` 保留自定义 viewBox。
- 管理 API（全部回环 + 写操作同源校验）：

| 方法/路径 | 说明 |
|---|---|
| `GET /api/catalog/icons` | 全部图标摘要（含停用/自定义/designed，管理面） |
| `GET /api/catalog/icons/{name}` | 图标详情（含归一化 SVG 源码；有 design 时附带几何文档） |
| `GET /api/catalog/icons/{name}/design` | 设计器上下文：`{design, imported, builtin, designed, editable}` |
| `POST /api/catalog/icons` | 新增 `{name, group?, tags?, svg?, design?}`（design 优先） |
| `PUT /api/catalog/icons/{name}` | 编辑 `{group?, tags?, svg?, design?}`；内置传 `svg`/`design` 返回 409 |
| `POST /api/catalog/icons/{name}/state` | `{disabled: true\|false}` |
| `POST /api/catalog/icons/{name}/reset` | 内置恢复默认分组/别名并启用 |
| `DELETE /api/catalog/icons/{name}` | 移除自定义图标 |

设计稿：画布项目「adago-design · SVG 图标管理」（说明 / 默认 / 自定义 / 新增 / 空态 /
视觉规范 / 实现契约）；设计器为 2.0 新增能力，布局规范见 DESIGN-SYSTEM-UI.md §15。

错误码：`unknown_icon` / `icon_duplicate` / `icon_builtin_protected` / `invalid_icon`。

## 品牌 Logo 工作台（设计 + 规范 + 交付）

工具条上的 **品牌 Logo**（星形图标）打开 1120×720 工作台：左 264 = 资产列表，
中 = 三变体预览 / 编辑画板，右 320 = 检查器或设计表单。

| 能力 | 说明 |
|---|---|
| 资产库 | `<data>/logos/<id>.json`；列表按更新时间倒序，可搜索名称/品牌/体系 |
| 组合 | 标记（图标库任意内置/自定义图标，或 MCP 的 `mark.design` 自绘几何）+ 字标 + 副标；横排 / 竖排 / 纯标记 / 纯字标 |
| 设计 | 品牌名、副标、字体（登记字体 + 授权提示）、字重、字号/字距/大小写、标记尺寸、间距、安全空间倍数、品牌色、透明/自定底色 |
| 变体 | 正形（品牌色）、反形（品牌色底 + 自动对比墨色）、单色（按底色自动黑/白）；实时切换 |
| 预览 | 详情用服务端确定性 SVG；编辑用 HTML/CSS 锁排 + `canvas.measureText` 实测字宽，保存时回填 `wordmarkWidth/taglineWidth`，落库 SVG 与预览对位一致 |
| 规范 | 安全空间（≥ 0.5×标记高度）、按「标记/字标 ≥16px」换算的最小宽/高、三变体清单、字体授权（可商用/可嵌入）、正/反形对比度警告 |
| 导出 | SVG（服务端附件下载）、PNG（浏览器 4× 光栅化、透明底）、复制 SVG |
| 摆上画布 | 生成自包含 HTML 节点（kind=component，标签 `logo`/`logo:<id>`/`variant:*`），归属当前项目 |
| 标记自绘 | 「画新标记」直接打开图标设计器；保存后新图标自动回填为 Logo 标记（内联 SVG，不依赖页面精灵） |

- 同一套能力也暴露给 Agent：MCP 工具 **`design_logo`**
  （`list / get / preview / create / update / remove / spec / export / place`）。
  `preview` 接受完整 `doc` 或 flat 字段（brand/tagline/markIcon/layout/font/color/...），返回三份 SVG + 规范；
  `create/update` 落库；`place` 把已有 Logo 摆到项目画布；`export` 写 `<data>/logo-exports/<id>-<variant>.svg`。
- HTTP：

| 方法/路径 | 说明 |
|---|---|
| `GET /api/catalog/fonts` | 字体注册表（Logo 字体选择与授权标签） |
| `GET /api/catalog/logos` | Logo 摘要（含正形 SVG 缩略） |
| `GET /api/catalog/logos/{id}` | 完整详情：doc + 三变体 SVG + spec + 尺寸 |
| `POST /api/catalog/logos` | 新增 `{name?, note?, system?, doc}` |
| `PUT /api/catalog/logos/{id}` | 全量更新 `{name?, note?, system?, doc}` |
| `DELETE /api/catalog/logos/{id}` | 移除资产（画布节点不回收） |
| `GET /api/catalog/logos/{id}/export?variant=` | 附件下载 SVG |
| `GET /api/catalog/logos/{id}/canvas?variant=` | 自包含 HTML + 宽高（摆画布用） |

- Logo 的字标在 SVG 中保留为 `<text>`；生产交付前应转曲或确认字体授权（详情/规范会提示）。
- 错误码：`unknown_logo` / `logo_duplicate` / `invalid_logo` / `logo_failed`。

## 画布界面（设计系统）

画布工具蒙皮走**单色品牌 + 令牌单源**：纸面（浅色，缺省）× 墨纸（深色）两套主题，
颜色/字号/圆角/间距/层级全部来自 `internal/canvas/static/tokens.css` 的 `--ui-*` 令牌，
组件样式 `canvas.css` 里不写任何原始色值。完整规范见 [DESIGN-SYSTEM-UI.md](DESIGN-SYSTEM-UI.md)。

| 维度 | 规则 |
|---|---|
| 品牌 | 单色：浅色墨底纸字、深色纸底墨字；彩色只用于状态（成功/警告/危险）与检查标注 A/B |
| 主题 | `data-theme=light`（缺省纸面）/ `dark`（墨纸）；`-theme` 只是缺省值，界面切换写浏览器 localStorage |
| 字号 | 11 / 12 / 13 / 15 / 17（8px、10px 已废止） |
| 圆角 | 2 / 4 / 6 / 8 / 10 / 12 / full（7 档取代原 13 种） |
| 弹层 | `uiAsk()` / `uiConfirm()` 替代原生 prompt/confirm：点外关闭、Esc、Tab 陷阱、焦点归还 |
| 门禁 | `ui_tokens_test.go`：令牌完整性、明暗对比度 ≥4.5、零字面值、零原生对话框 |

## 构建

```powershell
# 推荐走仓库根脚本（GOWORK=off）
.\scripts\build-mcp.cmd

# 或手动
cd mcp_projects/adago_design
GOWORK=off go build -trimpath -ldflags="-s -w -H windowsgui" -o bin/adago-design.exe .
```

依赖 `github.com/egoist/mygo v0.7.4`（Windows 上用系统 WebView2 运行时；
Win10/11 自带）。纯 Go 构建，`CGO_ENABLED=0` 可用，产物仍是单个 exe。

构建用 `-ldflags "-H windowsgui"` 链接：**不弹黑色控制台**。日志同时写
`<数据目录>\adago-design.log`（4MB 后轮换为 `.1`）与 stderr（控制台可用时）。
GUI 子系统从终端启动 `serve` 会立即返回、进程常驻；要停止请关窗口 / 托盘退出，
`-no-open` 没有窗口时用任务管理器结束进程。

**图标**：`assets/icon.png`（512 母版）→ `tools/icopack` 生成 `assets/icon.ico`
与 `internal/native/assets/{icon,tray}.png`；`windres` 把 ico 编进
`rsrc_windows_amd64.syso`（Explorer/任务栏的 exe 图标）。改图标后：

```powershell
pwsh -File tools/make-icon.ps1   # 需要 MinGW-w64 的 windres
```

窗口图标与托盘图标在运行时由 Go 设置，不依赖 syso；当前 binutils 没有
`pe-aarch64`，arm64 构建暂时只有窗口/托盘图标、exe 文件图标缺失。

## 运行形态（一个二进制，三种输入面）

```powershell
# 人工查看：原生窗口（标题条 + 内嵌画布页 + 状态条），同端口暴露 HTTP MCP
bin\adago-design.exe serve            # 窗口 + http://127.0.0.1:8788/mcp

# 回退：系统默认浏览器打开画布（旧形态，排障用）
bin\adago-design.exe serve -browser

# 只跑服务，不打开任何界面
bin\adago-design.exe serve -no-open

# Agent 接入：stdio MCP + 无窗口画布服务（同样带 /mcp）
bin\adago-design.exe mcp

# Agent 接入 + 原生窗口（截图超时会聚焦这个窗口，而不是弹浏览器）
bin\adago-design.exe mcp -window
```

公共参数：`-port 8788`（被占用自动 +1 探测）、`-data <目录>`（默认
`%APPDATA%\adago-design`，可被 `ADAGO_DESIGN_DATA` 覆盖）、`-open`、
`-browser`、`-window`、`-no-open`、`-theme dark|light`；
环境变量 `ADAGO_DESIGN_UI=native|browser|none` 压过标志（CI/脚本用）。

## 原生窗口（native shell）

```
┌ mygo 窗口 ─────────────────────────────────────────────┐
│ 标题条 40：品牌 · 作用域 · MCP 胶囊 · 主题/重载/浏览器/更多 │
│ ┌ 内嵌画布页（/?embed=native）─────────────────────────┐ │
│ │ 工具条 / 双浮窗 / 卡片 / 检查器 / 截图任务全在页面里   │ │
│ └─────────────────────────────────────────────────────┘ │
│ 状态条 26：服务地址 · 数据目录 · 项目/稿数 · rev · 持久化  │
└────────────────────────────────────────────────────────┘
```

- **设计图**：画布项目「adago-design · 原生内嵌窗口设计图」（11 张稿：
  主窗浅/深、冷启动、二次启动、页面故障、实例占用、沉浸全屏、托盘、视觉规范、实现契约）。
- **一层 chrome**：壳只画标题条与状态条；画布操作（缩放/比例/检查/清空）仍在页面里，
  不搬两遍。页面在 `?embed=native` 下隐藏自己的品牌与主题按钮（身份和主题归原生层），
  其余 DOM、类名、交互不变。
- **状态自解释**：启动卡（服务绑定→页面加载）→ 就绪；页面加载失败/渲染进程退出进入
  故障卡（重载 / 开发者工具 / 浏览器三个出口）；数据目录被无窗口实例占用时只给出口、
  不抢服务。持久化失败与截图任务在 MCP 胶囊与状态条上显示，不弹对话框。
- **二次启动**：单实例锁按数据目录分区（锁名含 `-data` 哈希）；同目录再启动会把命令行
  转交给已运行实例并把窗口前置，然后退出。不同 `-data` 可并存。
- **窗口记忆**：`<data>/window.json` 记宽高/位置/最大化/主题；关闭或拖动后 700ms 落盘。
- **日志**：`<data>/adago-design.log`（4MB 轮换，同时打 stderr）；GUI 构建没有控制台也能排障。
- **托盘**：显示/隐藏窗口、在浏览器打开、复制 MCP 配置（HTTP）、打开数据目录、
  **打开下载目录**、重载、退出。
- **快捷键**：`Ctrl+R` 重载、`Ctrl+Shift+I` 开发者工具、`F11` 全屏、`Ctrl+Shift+L` 换肤、
  `Ctrl+Shift+O` 浏览器打开、`Ctrl+Q` 退出；页面快捷键（空格平移、`0`/`F`、`Ctrl+Z`）照旧。
- **截图不变**：`canvas_screenshot` 仍由页面里的离屏 iframe 光栅化；超时且 `AutoOpen` 时，
  原生模式先聚焦窗口，没有窗口才退浏览器（每进程至多一次）。

**单实例防护**：绑定端口前先探测 `port..port+20` 是否已有**同数据目录**的
实例（`/api/health` 回显 `server`/`data_dir`/`rev` 供比对）。原生窗口模式在
mygo 单实例锁之后运行；命中「无窗口实例」时窗口进故障卡并给出出口。
`-browser` / `-no-open` 路径命中时沿用旧行为（委派浏览器 / 直接退出）。
`mcp` 模式快速失败，提示关闭旧实例或换 `-data` —— 两个实例同时写同一份
`canvas.json` 会互相覆盖。不同数据目录的实例互不冲突，各占各的端口共存。

**持久化加固**：`canvas.json` 紧凑序列化；写盘走「tmp → 旧文件轮换进
`.bak`/`.bak2` → 就位」三步，任一步失败记入 `lastPersistErr` 并打日志
（`/api/health` 的 `persist_error` 字段可见），不再静默丢错误；
主文件缺失或损坏时启动自动从备份降级恢复。

## MCP 工具

| 工具 | 用途 |
|---|---|
| `project_create` | **创建设计项目**（设计稿的归属容器）；同名拒绝，重复调用请改用 `project_list` |
| `project_list` | 列出项目与各自的设计稿数量；不确定 `project` 填什么时先调它 |
| `project_remove` | 删除**空**项目（`confirm=true`）；项目下仍有设计稿会被拒绝 |
| `project_canvas` | **画布版式**：按比例预设或自定义宽高设置当前画布，`free=true` 恢复自由画布；读取用 `project_list` |
| `canvas_resync_artboards` | **按画板显式重算宽高**（可撤销，面板一次回退）：画板/比例切换不会自动迁移已有节点；组件只纠正宽度，整页稿同时纠正宽高 |
| `design_artboard` | **画板（尺寸规范）**：列表 / 按 ID 取推导值；返回内容区、安全区、栅格、导出倍率与尺寸契约 |
| `design_fonts` | **字体授权**：可商用 / 可嵌入 / 可再分发标记，各脚本默认字族栈，授权策略 |
| `design_icons` | **图标库**：按名称/分组/别名检索（内置 + 自定义，停用隐藏），返回 `<use>` 引用或独立 `<svg>` |
| `design_icon_manage` | SVG 图标管理与设计：`list/design/create/update/disable/enable/remove/reset`；`design` 动作接受 24 网格几何文档并返回确定性 SVG（也可从 name/svg 反推）；create/update 可带 `design` 落库为可再编辑图标；内置可停用/改分组别名、SVG 只读（可另存） |
| `design_logo` | 品牌 Logo：`list/get/preview/create/update/remove/spec/export/place`；支持 flat 字段或完整 LogoDoc（标记图标/自绘几何、布局、字体、颜色、安全空间）；preview 返回三变体 SVG + 规范；create/place 可把 Logo 摆上项目画布 |
| `design_systems` | 列出全部设计体系（含默认画板） |
| `design_tokens` | 查询 Token（system/theme/category：color/type/spacing/radius/shadow/motion）；`brief=true` 精简输出：关键语义色的值、字阶尺寸/字重、间距与圆角阶梯，省略阴影/动效/变体色 |
| `design_components` | 组件清单；指定 component 返回详情与 HTML 片段 |
| `design_search` | 跨体系搜索体系/组件/Token |
| `design_derive` | 从品牌主色派生自定义体系（10 阶色板 + 语义色，结构继承基底），持久化注册 |
| `design_register` | 注册自定义体系（Token JSON，结构缺省从基底兜底） |
| `design_system_manage` | 设计体系管理：`list/create/update/disable/enable/remove/reset`；停用只影响目录/搜索/预览，内置不可删 |
| `design_audit` | 设计走查：体系级（WCAG 对比度/字号/阶梯/状态色 + 字体授权 + 图标一致性 + 尺寸规范，评分 + 报告节点）；传 `node_id` 改为**节点级走查**（外链资源 / 图标与符号 / 尺寸契约 / 字体授权，走查已生成节点的正文） |
| `design_preview` | 用某体系把组件渲染成真实预览页（`artboard` 决定宽高、`project` 决定归属），默认自动摆上画布 |
| `design_screen` | 按模板（login/settings/feed/profile）生成整页真实预览，高度取画板 body 高 |
| `design_compare` | 两体系 Token 对比（结构化 diff）；可选生成并排对比板节点 |
| `canvas_add` | 添加任意 HTML 预览卡片（可带 `artboard` 取画板尺寸） |
| `canvas_list` / `canvas_move` / `canvas_remove` / `canvas_clear` | 画布节点管理 |
| `canvas_undo` / `canvas_redo` | 撤销 / 重做最近一次画布内容操作（节点增改删、清空、连线；50 步，仅进程内存） |
| `canvas_arrange` | 节点自动网格排版（体系分组优先） |
| `canvas_restore` | 恢复最近一次清空（内存快照） |
| `canvas_export` | 导出整块画布为独立 HTML 文件 |
| `canvas_connect` | 节点间连线（带标签的贝塞尔曲线，持久化） |
| `canvas_group` | 分组框（自动包住成员节点，垫底虚线框） |
| `canvas_screenshot` | 截图画布或单个节点，返回 PNG（需画布页已打开：原生窗口或浏览器；超时且允许时先聚焦原生窗口、否则拉起浏览器并重试一次，每进程至多一次） |
| `canvas_url` | 获取画布访问地址 |

错误以 `isError` 结果返回并携带稳定错误码：`project_required` / `unknown_project` /
`project_duplicate` / `project_not_empty` / `edge_cross_project` / `unknown_ratio` /
`unknown_system` /
`unknown_icon` / `icon_duplicate` / `icon_builtin_protected` / `invalid_icon` /
`unknown_artboard` / `unknown_component` / `capture_timeout` / `confirm_required` /
`bad_request` / `screen_failed` / `compare_failed` / `empty_canvas`。
项目 PDF 端点另用 `empty_project` / `pdf_unavailable` / `pdf_failed`（见上一节）。

## 接入 ada-go（stdio）

`.adago/mcp.json`（或 `~/.adago/mcp.json`）：

```json
{
  "id": "adago_design",
  "transport": "stdio",
  "command": "E:\\codes\\go_projects\\ada_go\\mcp_projects\\adago_design\\bin\\adago-design.exe",
  "args": ["mcp"],
  "enabled": true
}
```

写入后重载 mcp-client 插件即可。也可以用根脚本注册：

```powershell
.\scripts\build-mcp.cmd -Register        # 构建 + 注册 computer-use（原有行为）
.\scripts\build-mcp.cmd -Register -IncludeAdagoDesignMcp
```

## 接入（Streamable HTTP，推荐）

`serve` / `mcp` 两种模式都会在画布服务上暴露 `POST <BaseURL>/mcp`
（Streamable HTTP，**无状态**：每个请求一个临时会话，响应用
`application/json` 而不是 SSE）。Agent 直接连 URL，不必 spawn 子进程，也
不会和已运行的画布服务抢「同数据目录唯一实例」：

```powershell
# 先起服务（已在跑就跳过）；不带 -no-open 则是原生窗口 + 同一端点
bin\adago-design.exe serve -no-open

# 探活 + 握手（Accept 必须同时含 application/json 与 text/event-stream）
curl.exe -s http://127.0.0.1:8788/mcp ^
  -H "Content-Type: application/json" ^
  -H "Accept: application/json, text/event-stream" ^
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-03-26\"}}"
```

DSH（`~/.dsh/profiles/desktop/cordis.patch.yml`）：

```yaml
- insert:
    - id: mcp-adago-design
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: adago_design
        transport: streamable-http
        url: http://127.0.0.1:8788/mcp
        toolCallTimeoutMs: 180000
```

工具随后以 `mcp__adago_design__<tool>` 出现在工具目录（需重载该行或重启
DSH）。ada-go 侧同一端点写成 `"transport": "http", "url": ".../mcp"` 即可。

端点为无状态实现，所以 `GET /mcp`（旧 HTTP+SSE）与 `DELETE /mcp`（结束会话）
都返回 405；客户端不需要处理 `Mcp-Session-Id`。

## 检查模式（蓝湖式元素标注 + 双元素关联）

工具条「检查」开启元素检查模式（`body.mode-inspect` 会把预览 iframe 的
`pointer-events` 从 `none` 放开，否则父页面收不到元素事件）：

- **悬停即出信息**：虚线高亮框 + 红色元素名胶囊（`标签.类 · 宽×高`）、
  到最近兄弟元素的间距线（最多两条，如 `12px`），右侧检查器同步显示该元素规格
- **点选 + hover 对比**：单击定锚点 A 后，**移入**任意其它元素即实时得到 A↔B
  （临时 B 随鼠标走，不需要第二次点击，可跨卡片）；再点一次才把 B 固定
- 固定 B 后 hover 只更新信息、不改对比对象；再点第三个元素则锚点前移（连续对比）
- `Shift`+单击：强制替换 B；`Esc` 逐级退出（B → A → 关闭检查），`C` 清空配对

**两层标注**：

| 层 | 位置 | 内容 |
|---|---|---|
| 卡片内 | 各自 iframe 的 `.dsx-layer` | 元素边框、A/B 角标、悬停名称胶囊、就近间距、到画板四边距离 |
| 卡片间 | 父页面 `#rel-layer`（世界坐标） | A↔B 水平/垂直间距、边线对齐虚线、包含/重叠、尺寸差 |

世界坐标 = `节点 x/y + 元素在 iframe 视口内的坐标`，与画布缩放/平移完全解耦，
所以两张卡片上的元素也能连起来量。

规格面板：尺寸 / 到画板（含宽度利用率）/ 内边距 / 边框 / 圆角 / 阴影 / 背景 / 字体，
数值带 **Token 合规标记**（✓ 落在阶梯、✓≈ 2px 内近似、⚠ 偏离并给最近规范值）；
配对时切换为关联面板（间距 / 对齐 / 尺寸差 / 位移 / 包含）。面板渲染带 `panelGen`
代际号：hover 高频切换 + `ensureCatalog` 异步，旧请求回来时若代际已变就丢弃，
避免「鼠标停在 C、面板还显示 B」。

## 同步机制（SSE 推送 + 轻量轮询兜底）

- 画布页连接 `GET /api/events`（SSE）：任何变更 150ms 合帧后立刻拉一次节点列表；
  节点列表默认**剥掉 html 大字段**（只传元数据 + `html_rev`），Agent 写入的
  变更即时出现在其它打开的画布页里，空闲流量从「N 节点 × 10KB / 2s」降到 KB 级
- 新节点 / `html_rev` 变化的节点才按需 `GET /api/nodes/{id}` 取正文；
  首屏与切项目用 `?include=html` 一次拿全；2s 轮询保留作 SSE 断线兜底
- Agent 修改画布版式后，画布 UI 即时同步（不再有约 30s 的轮询滞后）

## 交互性能

- 节点位置走 `transform: translate3d`（`left/top` 恒为 0），位移交给合成器，
  避免每帧 layout 与 iframe 表面重新光栅化
- 拖动用 `pointerdown` + `setPointerCapture` + rAF 合帧；指针离开窗口也不丢
- 画布平移两种入口：**空白处拖动**（一直可用）与 **按住空格后任意位置拖动**
  （卡片正文、iframe 上都行）；空格 + 滚轮同样缩放。空格模式在
  `pointerdown` 捕获阶段接管，卡片不会被误选/误拖，光标变成抓手
- 预览 iframe 默认 `pointer-events:none`，卡片整块可拖可选中；空格模式下
  即便在检查模式（iframe 反向放开了事件）也会临时关掉，保证拖动不断线
- 连线渲染按「脏边增量」更新（元素池化，只改 `d` 属性），空画布零开销
- 拖动期间关闭面板 `backdrop-filter`，松开后用服务端回包同步 `updated_at`
  （避免轮询误判为远端修改而重载 iframe）
- 预览高度自适应：`auto_height` 节点的卡片高度跟正文走（组件预览量 `.ds-page` 可增可减，
  整页稿只在溢出时向上扩），消除卡片内滚动条与截图裁切
- 画布长度随内容：纸面（`#canvas-paper`）自由画布取内容包围盒，固定比例画布在内容溢出时
  按溢出边界延长（标签补「· 随内容延长」）；标签分别写「N × M 随内容」/「16:9 · 随内容延长」
- 默认视角 = zoom-to-fit，**不裁切**：首屏 / 切项目 / 导入后 `focusContent()` 把当前作用域
  的全部设计稿完整放进视野（19% 也好过内容被切一半）；要看清单张，用卡片标题栏的
  「单独看这张稿」（整张适配 + 隐藏其它卡）或双击，`F` 有选中时也看全所选；`Esc` 回画布
- 画布上不产生文本选区：画布/浮窗/工具条 `user-select:none`，输入类控件保留 `text`，
  指针落到画布时清掉遗留选区（跨区域划选后不会留一片蓝底压住卡片）
- 左侧拆成两块独立浮窗（`#left-dock`）：项目面板在上、设计体系面板在下，可分别收起；
  收起后只留标题条，状态分别记在 `localStorage`（`adago.projCollapsed` / `adago.sysCollapsed`）
- 滚动条统一成「透明轨道 + 6px 悬浮滑块」（面板与预览文档各一套规则）：
  默认滚动条在深色主题下是一整条灰带，还会挤占卡片版心宽度

## 项目管理（左侧项目浮窗）

左侧「项目」浮窗里是项目树，参照设计稿工具的分组列表：

```
全部        4   ⋯        ← 点「全部」回到跨项目视图；⋯ 里是管理/重排/刷新
＋ 新建项目   ⇧N
⌕ 搜索项目
› 📁 默认项目（历史数据） 2   ← 点行 = 只看该项目；点 › = 展开该项目的设计稿
   · 旧设计稿A                ← 单击定位；双击 = 单独只展示这一张
› 📁 移动端改版          2
› 📁 运营活动页          0
```

- **展开**：点项目行左侧的 `›` 展开/收起该项目的设计稿子项（子项懒加载，收起再展开
  走缓存）；`Space` / `→` 展开、`←` 收起、`Enter` 切到该项目。**点行本身**仍是「只看该项目」，
  两个动作互不抢占 —— 之前 `›` 点不开是漏了 `toggleTreeRow` 的导入（现在是机器门禁）。
- **全部视图**：一次看所有项目的稿子，按项目分行排布，每行左侧一个橙色标签
  （`项目名 + 数量`，点它只看该项目）。标签做反向缩放，画布缩到 20% 也看得清。
- **单项目视图**：点项目行即过滤到该项目，标签隐藏；再点「全部」回来。
- **双击单独展示**：双击项目下的设计稿子项（或双击画布卡片标题栏）进入
  **单画布聚焦** —— 只留这一张稿，隐藏其它卡片、连线和分组标签并放大居中；
  顶部出现「← 返回画布 · 标题 · 项目名」条，树里对应子项高亮。
  `Esc` / 双击空白处 / 点「返回画布」退出；切换项目、清空画布、删除该稿时自动退出。
- **新建**：`＋ 新建项目` 或 `⇧N` 打开项目管理面板（名称必填、同名拒绝、可填备注），
  建完自动切到新项目。
- **管理**：`⋯ → 项目管理…` 打开面板，支持重命名、导出、删除（仅空项目）、切换。
- **导出 / 导入**：`⋯ → 导出当前项目 / 导出全部项目 / 导入项目…`；导入选择
  `adago-design` 导出的 `.json`，确认后新建项目（同名加「（导入）」后缀）并自动切过去。
- **重排**：`⋯ → 按项目重新排列`，把各项目排成行（行首留 200px 标签位）。
  进入「全部」时若检测到项目包围盒重叠，会自动把整组错开（`/api/projects/separate`，
  保留组内排版、可撤销），之后仍可自由拖动。

## 画布版式（选择 / 比例 / 居中）

工具栏靠左是画布控制组。**画布 = 项目作用域 + 一份版式**，三件事都收在这里：

> 工具条**只显示图标**：画布名、比例、适配/检查/清空等文字与当前状态都放在悬停
> tooltip 里（键盘聚焦同样可见），避免长项目名把工具条撑成多行。

- **选择画布**：文件夹图标 →「全部项目 / 项目名」下拉，每行带该项目的版式与稿数；选中即切换作用域。
  与左侧项目树共用同一状态（`localStorage` 记住上次选择），两边互相同步。
- **修改比例**：比例图标 → 自由画布 / 16:9 / 16:10 / 4:3 / 3:2 / 1:1 / 3:4 / 2:3 / 9:16 / 自定义宽高。
  比例落成具体宽高（预设长边 1080–1920，自定义 320–8192），存进项目的
  `canvas` 字段（`canvas.json`）；Agent 侧对应 `project_canvas` 工具。
- **居中展示**：准星图标；把画布矩形对齐到可视区中心（自动扣掉左侧浮窗、检查器、工具条），
  放不下时缩放到刚好放下；自由画布按全部节点包围盒居中。快捷键 `0`。

画布纸张（`#canvas-paper`）只作版式参考：**不裁剪、不拦截事件**，节点仍可拖到边界外
当草稿区。单画布聚焦（双击卡片/项目树子项）时纸张自动隐藏，居中改为对准该节点。
Agent 修改画布版式后，画布 UI 经 SSE 即时同步（无需手动刷新页面）。

## 截图原理（无 CDP）

`canvas_screenshot` 创建截图任务 → 已打开的画布页面长轮询领取 → 把目标
预览 HTML 放进离屏 iframe → `SVG foreignObject` 序列化 → `<canvas>` 光栅化
→ PNG 回传给 MCP。8 秒内没有页面响应则返回 `capture_timeout`；生产模式
（`serve` / `mcp`）会在超时后自动把画布露出来再重试一次：原生窗口先
`Focus()`，没有窗口才退系统浏览器（每进程至多一次，测试/CI 可关）。
预览内容是纯 CSS（无外链资源、图标为内联 `<symbol>` 精灵），渲染安全不失真；
所有入库节点 HTML 统一注入 CSP
（`default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:`，
见 `internal/canvas/sanitize.go`）—— Agent 经 `canvas_add` 塞入的外链资源会被
浏览器阻断，不再污染 `foreignObject` 光栅化（改前一张外链图会让整张截图失败），
内联样式不受影响，设计预览渲染不变。

## 测试与门禁

```powershell
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go generate ./internal/design   # 改了图标后同步 icons.js
node --test internal/canvas/testdata/geometry.test.mjs   # 前端几何纯函数（可选，需 Node）
```

画布前端是 ES Modules（`internal/canvas/static/js/`，入口 `main.js`），无构建步骤：
浏览器直接加载源码，因此静态资源必须带正确的 `Content-Type`（服务端已显式设置）。
`geometry.test.mjs` 也由 `go test ./internal/canvas` 代理执行，PATH 中没有 node 时自动跳过。

**模块图门禁**（`module_graph_test.go`，纯正则、无需 Node）把三类「浏览器才报错」的
问题变成红测：① 具名 import 必须是对面真实导出；② `js/**` 每个模块都必须从 `main.js`
可达（`keyboard.js` 这类只有副作用的模块要显式 `import "…"`，漏挂会静默失效）；
③ 调用了跨模块导出名却没导入的兜底扫描（典型症状：项目树的 `›` 点不开、画布拖不动 /
滚轮不缩放，控制台 `ReferenceError`，而 Go 测试与 `go vet` 全绿）。
同一个文件里还有空格临时平移模式的契约检查（state 开关 / keyboard 按下松开 /
nodes 捕获阶段接管 / CSS 光标规则）。

- `internal/canvas`：Store CRUD/持久化/并发、HTTP API 全路径、截图任务闭环、
  **P0 加固**（备份轮换与降级恢复、写盘失败可见、列表剥 HTML、SSE 推送、
  部分更新指针语义、多实例探测，见 `hardening_test.go`）、
  **CSP 注入**（注入位置/幂等/旧快照自愈，见 `sanitize_test.go`）、
  **页面 embed 契约**（bootstrap 先于样式、两条隐藏规则、dataset 字段与接线，
  见 `embed_test.go`）、
  **系统设计文稿**（方向/排序/页码/转义/非法主色/文件名/两条路由/501 与 409 路径，
  见 `print_test.go`）。
- `internal/config`：界面形态矩阵（serve/mcp × `-browser`/`-window`/`-no-open`/
  `-open` × `ADAGO_DESIGN_UI`，见 `config_test.go`）。
- `internal/native`：无 GUI 的纯函数单测（锁名、`window.json` 归一化与 round trip、
  `/api/health` 解析、端口标签、年龄文案、MCP 配置片段，见 `native_test.go`）；
  窗口本身靠真机冒烟（启动/二次启动/冲突卡/截图回传）。
- `internal/design`：8 体系 Token 完备性（数据源 `data/*.json`）、全部组件可渲染
  （21 个，含 tabs/table/pagination）、图标库不变量、字体授权（全部体系可商用）、
  画板推导值、**渲染产物无 emoji/字符图标**、规格阶梯走查（CSS 字面值须落阶梯）、
  **节点级走查**（外链/符号/尺寸契约/字体授权，见 `nodeaudit_test.go`）。
- `internal/mcp`：内存 MCP 会话走 `tools/list` + `tools/call` 契约，
  含 `design_icons` / `design_fonts` / `design_artboard` 与画板透传、
  `design_audit` 节点级走查与 `design_tokens` brief（`enhance3_test.go`）、
  `OpenCanvas` 注入点（`open_test.go`）、`design_system_manage` 契约
  （`systems_test.go`：停用后 design_systems/design_tokens 拒绝、内置保护、自定义增删）。
- 体系管理：`internal/design/manage_test.go`（停用持久化/内置覆盖与恢复/内置保护/
  自定义增改删/非法 JSON）、`internal/app/systems_test.go`（Catalog 集成）、
  `internal/canvas/admin_route_test.go`（管理端点路由与错误码）。

设计细节见 [DESIGN.md](DESIGN.md)，项目 PDF 文稿见
[DESIGN-PDF-EXPORT.md](DESIGN-PDF-EXPORT.md)；六项历史问题的分析与改造记录见
[IMPROVEMENTS.md](IMPROVEMENTS.md)。
