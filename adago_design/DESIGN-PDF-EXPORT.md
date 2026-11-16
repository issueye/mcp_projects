# 项目设计文稿导出（PDF）· 系统设计文稿

> 范围：adago-design 画布「项目 → 系统设计文稿 PDF」。
> 版本：v1 · 2026-11-16 · 先设计后实现
> 关联：`internal/canvas/print.go`、`internal/native/print.go`、`static/js/project-pdf.js`
> 设计稿：`.design/pdf-export/`（7 张：设计说明 / 入口 / 流程 / 封面 / 文稿内页 / 视觉规范 / 实现契约）
> PDF 版：[DESIGN-PDF-EXPORT.pdf](DESIGN-PDF-EXPORT.pdf)（由本功能从 `.design/pdf-export`
> 设计稿项目导出；10 页 A4 横向）

---

## 1. 背景与目标

现有项目导出只有两种形态：

| 形态 | 产物 | 适合 | 不适合 |
|---|---|---|---|
| 项目 JSON（`GET /api/projects/{id}/export`） | 可再导入的工程文件 | 换机器、交给同事继续编辑 | 评审、汇报、归档 |
| 画布 HTML（`canvas_export` MCP） | 一张带坐标的 HTML 板 | 在浏览器里看整块画布 | 打印、逐页批注、发给不装工具的人 |

缺的是**交付物**：一个项目里的设计稿要按阅读顺序成册，带项目身份、体系说明、页码，能直接发出去评审。本次新增：

1. **项目设计导出为 PDF**：一个项目 → 一份多页 PDF 设计文稿；
2. **系统设计文稿**：PDF 内含封面、项目概览、设计体系摘要、设计索引、逐稿页；
3. 原生窗口下**直接下载 PDF**；浏览器/无窗口形态退化为**打印页 + 另存为 PDF**；
4. 同一套只读文稿页 `GET /projects/{id}/print` 同时服务上述两种形态。

### 非目标

- 不做在线 PDF 预览器 / 批注器：PDF 交给系统阅读器。
- 不做「导出全部项目」的合并文稿：那是另一个版式问题（多个封面、多个目录），
  本轮只做单项目；全部项目仍走 JSON / HTML。
- 不做可编辑的 InDesign / Figma 交换格式。
- 不改变 `canvas.json` 数据格式、项目/节点 API 与已有 JSON 导出。
- 不在服务端光栅化设计稿：文稿直接嵌入原 HTML 预览，保持矢量文字与 1:1 结构。

---

## 2. 用户流程

### 2.1 原生窗口（缺省）

```
项目面板 ⋯ / 项目管理行「PDF」
        │
        ▼
panel JS：POST /api/projects/{id}/pdf
        │   （请求前 toast 常驻：正在生成…）
        ▼
canvas HTTP：校验项目 → 过滤 frame → 计算纸张方向
        │
        ▼
native.Shell.PrintPDF(pageURL, landscape)
        ├─ mygo.NewWindow({Hidden:true, URL:/projects/{id}/print})
        ├─ 轮询 window.__adagoPrintReady（全部 iframe + 字体就绪）
        ├─ Page.PrintToPDF(A4, landscape, Background:true, Margins:0)
        └─ 关窗，返回 PDF 字节
        │
        ▼
HTTP 200 application/pdf → 前端 Blob → 下载到系统下载目录
```

- 生成期间不打扰画布：打印窗口隐藏、`SkipTaskbar`，主窗口可继续编辑；
- 同时只允许一个 PDF 渲染任务（`Shell.pdfMu`）；
- 文件名：`<项目名>-系统设计文稿-YYYYMMDD-HHmm.pdf`。
- 下载完成由原生壳 `OnDownloadDone` 给标题条提示（文件名）；找不到文件时可用
  标题栏 `⋯ →「打开下载目录（导出文件）」` 或托盘「打开下载目录」直接打开。

### 2.4 文件落点与命名

| 项 | 规则 |
|---|---|
| 目录 | 系统下载目录；Windows 通常是 `%USERPROFILE%\Downloads`（本机实测 `C:\Users\User\Downloads`） |
| 文件名 | `<项目名>-系统设计文稿-YYYYMMDD-HHmm.pdf`；项目名清掉路径分隔符 / 控制字符，截断 48 字 |
| 重名 | mygo 下载行为自动加 ` (2)`、` (3)`，不覆盖旧文件 |
| 查看 | 原生：标题栏 `⋯ → 打开下载目录（导出文件）`、托盘「打开下载目录」；浏览器：下载记录 / 默认下载目录 |
| 提示 | 原生下载完成标题条提示文件名；前端成功 toast 同时提示「标题栏 ⋯ → 打开下载目录」 |

### 2.2 浏览器 / 无内嵌渲染器

```
项目面板 ⋯ / 项目管理行「PDF」
        │
        ▼
window.open("/projects/{id}/print?print=1")
        │
        ▼
文稿页 script：iframe + 字体就绪 → window.print()
        │
        ▼
浏览器打印对话框 →「另存为 PDF」→ 勾选「背景图形」
```

- 页面顶部的 `.printbar` 在屏幕上提供「打印 / 另存为 PDF」按钮与同一句提示；
  打印时该条被 `@media print` 隐藏，不会进 PDF。
- 若原生形态返回 `501 pdf_unavailable`（例如装配层未注入渲染器），前端也走这条回退。

### 2.3 状态与反馈

| 状态 | 反馈 | 出口 |
|---|---|---|
| 未选项目（「全部」作用域） | toast：先在项目树里选一个项目 | 选项目后重试 |
| 生成中（原生） | 常驻 toast：「正在生成「项目名」的系统设计文稿…」 | 等待；主窗口仍可操作 |
| 成功（原生） | 下载完成标题条提示「已保存到下载目录：文件名」；toast：已导出…KB | 文件在系统下载目录；标题栏 ⋯ / 托盘「打开下载目录」 |
| 成功（浏览器） | 打开新标签 + toast：在打印对话框选择「另存为 PDF」 | 打印对话框 |
| 项目为空 / 只有分组框 | `409 empty_project` + hint（分组框不单独成页） | 先添加设计稿 |
| 没有 PDF 渲染器 | `501 pdf_unavailable` | 自动回退打印页 |
| 渲染失败 / 超时 | `502 pdf_failed`，带原因与「改用打印页」提示 | 重试；浏览器打印 |

---

## 3. 文稿信息架构

```
第 1 页   封面        项目身份 + 四格统计 + 生成时间
第 2 页   项目概览    项目信息 / 设计体系 / 设计稿构成 / 文稿结构 / 项目备注
第 3..k   设计索引    # / 设计稿 / 体系 / 类型 / 画板 / 尺寸 / 文稿页（可分页）
第 k+1.. 逐稿页      一稿一页：编号 / 标题 / 体系 / 类型 / 画板 / 尺寸 / 预览 / 来源
```

### 页数公式

- 有设计稿：`总页数 = 2 + 索引页数 + 设计稿数`
  - 索引页数 = `ceil(设计稿数 / 每页索引行数)`；
  - 每页索引行数：横向 14、纵向 18。
- 空项目：`总页数 = 3`（封面 + 概览 + 空态页），空态页给「先添加设计稿」引导。
- 逐稿页页码 = `2 + 索引页数 + 该稿在文稿中的序号`；索引最后一列就是它。

### 收录规则

- **分组框（kind=frame）不单独成页**：它是成员卡片的背板，收进文稿只会得到一张空框；
  概览里以「分组框 N 个」统计，封面也有计数。
- 其它 kind 全部收录：整页稿 / 组件 / 色板 / 便签。
- 阅读顺序按**画布位置**：先按 64px 高度分桶（同一行），桶内按 X，最后按 Y / ID；
  与「创建时间」无关，避免后补的稿插到文稿中间。
- 只收录当前项目的节点；跨项目连线不进入文稿（文稿是项目内文档）。

---

## 4. 页面规格

### 4.1 纸张与方向

| 场景 | 纸张 | 页面盒 |
|---|---|---|
| 横稿占多数 | A4 横向（297×210mm） | 296×209mm |
| 竖稿不少于横稿 / 空项目 | A4 纵向（210×297mm） | 209×296mm |

方向算法（`PrintOrientation`）：统计非 frame 稿件的宽高比，`高 > 宽` 记为竖稿，
其余记横稿；`竖稿 < 横稿` 才用横向，否则纵向（平票取纵向，手机稿优先按 100% 印出）。

> 取舍：PDF 每页只能有一种纸张；混排项目必有一类稿被缩小。按多数决定，
> 少数派仍等比缩放、不裁切。索引/概览与逐稿页同向。

### 4.2 预览缩放

```
scale = min(可用宽 / 稿宽, 可用高 / 稿高, 1.0)
```

| 方向 | 预览可用框（逻辑像素） |
|---|---|
| 横向 | 1019 × 579 |
| 纵向 | 635 × 850 |

- 只缩不放：小于可用框的稿保持 100%，居中留白；
- 缩放作用在 iframe 外层 `.frame` 的 `transform: scale(k)`，iframe 内部仍按原画板宽高排版，
  因此 1:1 的结构、断点与字号关系不变；
- 逐稿页右上角显示「预览 44%」这类读数，读者知道当前印刷尺度；
- 稿件正文为空时显示「只有元数据，没有可渲染的正文」，不输出空白框。

### 4.3 版面与排版

| 元素 | 规格 |
|---|---|
| 版心 | 页内 padding 12/14mm（上下/左右），页脚贴底 |
| 页头 | kicker 8pt 大写 + 标题 20–21pt + 右侧项目名；下边框 0.35mm |
| 页脚 | 品牌 · 项目名 · 「第 x / N 页」；上边框 0.35mm |
| 封面 | 墨底纸字；标题按项目名长度自适应 36 / 32 / 27 / 22pt；四格统计 |
| 概览 | 两栏面板（1.08fr / .92fr）+ 全宽项目备注 + 元信息条 |
| 索引 | 固定列宽表格；偶数行浅底；末行下方一句收录说明 |
| 逐稿页 | 编号 27pt + 标题 18pt + 元信息 chips；舞台点阵底；页脚来源/时间/页码 |
| 字体 | Inter + Noto Sans SC + 系统中文兜底；等宽仅用于 ID |
| 颜色 | 纸面 `#FBF8F1`、浅底 `#FFFDF8`、画布 `#EDE8DC`、墨 `#16171A`、线 `#DCD5C6` |
| 体系色 | 只用体系主色点（已做 `#RGB/#RRGGBB/#RRGGBBAA` 形状校验） |

分页：每张 `.sheet` 固定页面盒 + `break-after: page`，最后一张 `auto`；
`@page { size: A4 portrait|landscape; margin: 0 }`。原生 PrintToPDF 使用
`preferCSSPageSize: false`，纸张尺寸由 Go 侧选项决定，与页面方向保持一致。

---

## 5. 数据与摘要

### 5.1 节点字段

| 字段 | 用途 |
|---|---|
| `title` | 索引 / 逐稿页标题 |
| `system` | 体系 ID；经目录映射成名称 / 厂商 / 主色 |
| `kind` | 类型文案；frame 过滤 |
| `artboard` | 画板契约（`iphone-15` / `web-1440` …） |
| `width` / `height` | 尺寸、缩放与纸张方向 |
| `html` | 逐稿页预览 iframe 的 `srcdoc`（全量 HTML 转义） |
| `tags` | 逐稿页页脚标签 |
| `id` / `updated_at` | 逐稿页来源信息 |
| `x` / `y` | 阅读顺序（不进入 PDF 坐标） |

### 5.2 体系摘要

`Server.Catalog.SystemSummaries()`（app.Catalog 桥接 design 包）→ `PrintSystem`：
`ID / Name / Vendor / Primary / DefaultArtboard`。目录未注入时退化为体系 ID；
已停用体系若仍被旧稿引用，也按 ID 显示，不丢稿。

### 5.3 安全边界

- 标题、备注、标签、体系名、项目名全部 `html.EscapeString`；
- 节点 HTML 只作为 `srcdoc` 属性值写入，**不做字符串拼接**，不执行脚本；
- iframe 带 `sandbox="allow-same-origin"`，节点自带 CSP，外链资源仍被阻断；
- 文稿页本身无外链 CSS / JS / 字体，单文件可离线打印。

---

## 6. 接口契约

| 方法 / 路径 | 说明 |
|---|---|
| `GET /projects/{id}/print` | 只读文稿页（完整 HTML）。`?print=1` 在就绪后自动调 `window.print()` |
| `POST /api/projects/{id}/pdf` | 生成并下载 PDF。请求体为空；`Content-Type: application/pdf` |

### 6.1 PDF 响应头

```
Content-Type: application/pdf
Content-Disposition: attachment; filename="<ASCII 兜底>"; filename*=UTF-8''<RFC 5987 编码>
Cache-Control: no-store
```

文件名 `<项目名>-系统设计文稿-YYYYMMDD-HHmm.pdf`；项目名清掉路径分隔符与控制字符，
截断到 48 个字符；ASCII 兜底保留可读字母数字，中文一律 `_`。

### 6.2 错误码

| 状态 | code | 含义 |
|---|---|---|
| 404 | `unknown_project` | 项目不存在（沿用项目错误码） |
| 409 | `empty_project` | 项目里没有可成页的稿件（空白或只有 frame） |
| 501 | `pdf_unavailable` | 当前形态没有 PDF 渲染器（浏览器 / 无窗口 / 未注入） |
| 502 | `pdf_failed` | 打印窗口加载失败、就绪超时、PrintToPDF 失败或返回空 PDF |
| 500 | `internal` | 其它未预期错误（panic 兜底） |

`pdf_unavailable` 带 hint：浏览器模式会打开打印页，请选择「另存为 PDF」。

---

## 7. 原生渲染链路（internal/native/print.go）

1. **入口**：`app.WithPDFPrinter(sh.PrintPDF)` 注入到 `canvas.Server.PDF`；
   `serve` 与 `mcp -window` 都注入，`serve -browser` / `mcp` 不注入。
2. **隐藏窗口**：`mygo.NewWindow({URL: pageURL, Hidden: true, SkipTaskbar: true,
   Width: 1122/794, Height: 794/1122, BackgroundColor:"#FFFFFF"})`；
   URL 是仅回环可达的文稿页，窗口不抢焦点。
3. **就绪等待**：轮询 `document.documentElement.getAttribute("data-print-ready")`；
   文稿页在「全部 iframe load + `document.fonts.ready` + 180ms」后置位，
   不依赖 `rAF`（隐藏窗口会被节流）。上限 20 秒，`context` 可取消。
4. **打印**：`PrintToPDF({PageSize: A4, Landscape: landscape, Background: true,
   Margins: 0})` —— `Background` 必须为 true，否则封面墨底与纸面底色会丢；
   `Margins: 0` 让页内 padding 成为唯一版心，避免浏览器默认 0.4 英寸再叠一层。
5. **清理**：`defer Close()`；`pdfMu` 串行化；失败时同样关窗，不泄漏窗口。
6. **超时**：HTTP handler 用 `context.WithTimeout(r.Context(), 45s)`；
   正常 1–3 秒（10 页内），大项目按稿件数线性增长。

> 为什么不在主窗口打印：主窗口是编辑器，打印会把工具条、浮窗、画布背景一起印进去；
> 临时铺打印层再还原会打断用户状态。隐藏窗口加载同一套只读文稿页，互不干扰。

---

## 8. 前端交互（static/js/project-pdf.js）

| 入口 | 行为 |
|---|---|
| 项目面板「⋯ → 导出项目 PDF（系统设计文稿）」 | 当前作用域项目；「全部」作用域提示先选项目 |
| 项目管理弹层行内「PDF」按钮 | 指定项目；不关闭弹层，可连续导出多个项目 |

- `isEmbedded()`（`<html data-embed="native">`）决定路径：原生 POST 下载，浏览器打印页；
- 原生 `fetch` 不用 `api()`（它按 JSON 解析）；直接判断 200 / 501 / 其它错误；
- 文件名与 JSON 导出同风格：`safeName(项目名)-系统设计文稿-stamp.pdf`；
- 常驻 toast 用 `toastStay()`，成功/失败由下一次 `toast()` 接管；
- 不做原生 `confirm/prompt`，不新增颜色字面量（`ui_tokens_test` 门禁）。

---

## 9. 兼容与降级

| 场景 | 行为 |
|---|---|
| 原生窗口（缺省） | 隐藏窗口 PrintToPDF，自动下载 |
| `serve -browser` / `mcp` 无窗口 | 打印页 + 浏览器「另存为 PDF」 |
| 浏览器打开原生实例的 URL | 页面不是 `embed=native` → 走打印页（不打扰原生窗口） |
| 原生渲染器 501 / 失败 | 前端回退打印页；PDF 端点返回稳定错误码 |
| 旧项目 / 旧数据 | 文稿只读节点元数据，不改任何文件；v1–v4 数据同样可导出 |
| 体系被停用 | 节点仍显示体系 ID / 仍可成稿；目录只影响选择面 |
| 深色主题 | 文稿固定纸面 + 墨封面，不跟随界面主题（打印交付物应与屏幕解耦） |

---

## 10. 验收与测试

### 10.1 机器门禁

| 测试 | 覆盖 |
|---|---|
| `TestPrintOrientation` | 空画布默认、竖稿、横稿、混排平票、分组框不参与 |
| `TestPrintableNodesOrderAndFilter` | frame 过滤、行优先/行内 X、返回副本 |
| `TestPrintDocumentCoverOverviewIndex` | 封面统计、体系摘要、索引页码、sheet 数、预览与就绪脚本 |
| `TestPrintDocumentIndexPaginationAndEmpty` | 20 张稿 → 2 页索引 / 24 页；空项目 3 页 |
| `TestPrintDocumentEscapesPreviewAttribute` | 标题 / 正文转义，原始标签不泄漏 |
| `TestPrintDocumentRejectsInvalidPrimary` | 非 hex 主色不进入样式，体系名仍显示 |
| `TestPDFFileNameAndDisposition` | 中文文件名、非法字符清洗、RFC 5987 编码 |
| `TestPrintRoutes` | 文稿页 200 / Content-Type / 未知项目 404 |
| `TestProjectPDFEndpoint` | 501 → 200 → 502 → 409 全路径、URL/方向透传、响应头 |
| `TestPrintRouteUsesCatalogSystems` | 目录体系名/主色进入文稿 |
| `TestPrintDocumentIsSelfContained` | 无外链 CSS/JS，单文件可打印 |

### 10.2 真机验证（2026-11-16）

| 用例 | 数据 | 结果 |
|---|---|---|
| 混排项目 | 390×880 手机稿 + 1440×936 桌面稿 | A4 纵向 5 页，616 KB，约 1.7–2.5s；封面/概览/索引/两页逐稿页齐全 |
| 桌面项目 | 1440×900 单稿 | A4 横向 4 页，369 KB，约 1.6s；逐稿页预览 64% |
| 空项目 | 无节点 | `409 empty_project`；`/print` 仍给空态页 |
| 只有分组框 | frame 1 个 | `409 empty_project` |
| 文本层 | 中文标题/正文 | PDF 可复制、可搜索（PyMuPDF 抽取与屏幕一致） |
| 打印页 | 浏览器直接打开 `/print` | `__adagoPrintReady === true`，5 张 sheet，顶部打印条可点 |

---

## 11. 风险与后续

| 风险 | 现在怎么处理 | 后续 |
|---|---|---|
| 稿件很多时渲染慢 | 45s 超时 + 错误提示；隐藏窗口一次装载 | 分批渲染 / 只导出选中稿 |
| 混排方向只能选一种 | 多数票决定，少数等比缩小不裁切 | 分卷（横向卷 + 纵向卷）再合并 PDF |
| 预览依赖 WebView2 可用 | 无窗口形态走打印页；504/502 有回退 | `headless` 服务端渲染（需要浏览器引擎） |
| PDF 内没有目录书签 | 索引页已给页码；阅读器可搜索标题 | 生成 PDF outline（需扩展 PrintToPDF 后处理） |
| 文件落到下载目录 | mygo 默认下载行为，toast 提示 | 增加「打开下载目录」按钮 / 指定路径 |
| 找不到导出文件 | 已加标题栏 ⋯「打开下载目录」、托盘「打开下载目录」与下载完成提示；路径见 §2.4 | 在项目面板里常驻一个「最近导出」入口 |
| 字体子集嵌入 | WebView2 打印会嵌入实际使用字体（Windows 上中文常落到 Microsoft YaHei 子集）。生成物是用户文档，应用本身不打包/分发字体文件；对 OFL 有严格要求的交付可在机器上安装 Noto Sans SC（字体栈首选）或用浏览器打印，行为一致 | 随包自带 OFL 中文字体并改为 `@font-face`（会显著增大体积） |

---

## 12. 文件清单

| 文件 | 改动 |
|---|---|
| `internal/canvas/print.go` | 文稿生成、方向算法、节点排序、PDF 端点、文件名与响应头 |
| `internal/canvas/print_test.go` | 上述 11 组门禁 |
| `internal/canvas/server.go` | `PDF` 字段与两条路由 |
| `internal/app/app.go` | `WithPDFPrinter` 装配点 |
| `internal/native/print.go` | 隐藏窗口 + 就绪轮询 + PrintToPDF |
| `internal/native/shell.go` | `pdfMu` |
| `main.go` | `serve` / `mcp -window` 注入 `sh.PrintPDF` |
| `internal/canvas/static/index.html` | 项目菜单新增 PDF 项 |
| `internal/canvas/static/js/panels.js` | 项目管理行 PDF 按钮 |
| `internal/canvas/static/js/dock.js` | 两处入口接线 |
| `internal/canvas/static/js/project-pdf.js` | 导出流程、下载、打印回退、toast |
| `internal/canvas/static/js/ui.js` | `toastStay()` 常驻反馈 |
| `DESIGN-PDF-EXPORT.md` | 本文稿 |
| `.design/pdf-export/` | 设计稿与构建脚本 |
| `README.md` / `DESIGN.md` | 功能索引与设计文档 |

---

## 13. 交付顺序

1. 文稿生成器（纯函数 + 门禁）——先保证页码、方向、转义可测；
2. 路由与 PDF 端点（无渲染器时 501）；
3. 原生隐藏窗口 PrintToPDF（真机确认 PDF 可复制、可打开）；
4. 前端入口与两条运行形态；
5. 真机验证 + 设计文稿（本文件与 `.design/pdf-export/`）。

**先设计 · 后实现 · 2026-11-16**
