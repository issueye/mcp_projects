package canvas

import (
	"context"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// PDFPrinter 是装配层注入的「把打印文稿页渲染成 PDF」能力。
// 原生窗口由 native.Shell 提供（隐藏窗口 + WebView2 PrintToPDF）；
// 浏览器 / 无窗口模式下为 nil，PDF 端点返回 pdf_unavailable，页面改用打印。
// landscape 决定纸张方向，与 /projects/{id}/print 的 @page 保持一致。
type PDFPrinter func(ctx context.Context, pageURL string, landscape bool) ([]byte, error)

// 项目设计文稿（PDF 的宿主页面）。
//
// 一条设计稿 → 一页文稿：封面（项目身份）、概览（项目 / 体系 / 构成）、
// 索引（逐稿页码）、逐稿页（标题 / 体系 / 画板 / 尺寸 / 预览 / 来源）。
// 文稿由服务端生成纯 HTML，页面自身只负责排版；PDF 由原生窗口的
// WebView2 PrintToPDF 渲染（浏览器模式退化为打印页 + 另存为 PDF）。
//
// 设计取舍：
//   - 一页一稿而不是把画布缩略图印成一张：设计文稿要能逐页评审、逐页批注，
//     画布上的相对位置只保留阅读顺序（自上而下、自左而右），不保留画布坐标。
//   - 预览用沙箱 iframe + 等比缩放：设计稿本来就是固定画板的 HTML 文档，
//     重新排版会破坏设计；缩放只影响印刷尺度，不影响 1:1 还原。
//   - 纸张方向由稿件的宽高比决定（竖稿多 → A4 纵向），而不是写死横向：
//     手机稿在横纸里只能缩到 66%，字会糊；纵向能按 100% 印出来。
//   - 文稿只依赖节点自身与体系摘要，不读磁盘上的任何额外文件：换机器、
//     只带一个项目导出 JSON 也能得到同一份文稿。

// PrintSystem 是文稿里一套设计体系的可读摘要（缺省时只显示体系 ID）。
type PrintSystem struct {
	ID              string
	Name            string
	Vendor          string
	Primary         string // #RRGGBB；非法值不参与渲染
	DefaultArtboard string
}

// PrintOrientation 决定纸张方向：竖稿（高 > 宽）不少于横稿时用纵向。
// 空画布默认横向（更接近画布本身的阅读形态）。
func PrintOrientation(nodes []Node) bool {
	portrait, landscape := 0, 0
	for _, n := range nodes {
		if n.Kind == KindFrame || n.Width <= 0 || n.Height <= 0 {
			continue
		}
		if n.Height > n.Width {
			portrait++
		} else {
			landscape++
		}
	}
	if portrait == 0 && landscape == 0 {
		return true // landscape
	}
	return portrait < landscape
}

// PrintableNodes 返回文稿收录的节点：跳过分组框（背板），按画布位置行优先
// 排序（上 → 下、左 → 右）。同一行按 64px 高度分桶，避免 1px 抖动把同一行
// 拆成两行；桶内仍以 X 为准，保证排序可传递、输出稳定。
func PrintableNodes(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Kind == KindFrame {
			continue
		}
		out = append(out, n.Clone())
	}
	sort.SliceStable(out, func(i, j int) bool {
		bi, bj := math.Floor(out[i].Y/64), math.Floor(out[j].Y/64)
		if bi != bj {
			return bi < bj
		}
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// kindLabels 是文稿里对节点种类的读法（与 canvas 的 kind 值一一对应）。
var kindLabels = map[string]string{
	KindScreen:    "整页稿",
	KindComponent: "组件",
	KindPalette:   "色板",
	KindNote:      "便签",
	KindFrame:     "分组框",
}

func printKindLabel(kind string) string {
	if label, ok := kindLabels[kind]; ok {
		return label
	}
	if strings.TrimSpace(kind) == "" {
		return "设计稿"
	}
	return kind
}

// validHexColor 只接受 #RGB / #RRGGBB / #RRGGBBAA：体系主色来自数据文件，
// 但文稿把它写进 CSS，必须先做形状校验，避免脏数据注入样式。
func validHexColor(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return false
	}
	h := s[1:]
	if len(h) != 3 && len(h) != 6 && len(h) != 8 {
		return false
	}
	for _, r := range h {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func printTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

// printLayout 是纸张方向的版面常量（单位：mm 给排版、px 给预览缩放）。
// 预览框宽高已扣掉页头、页脚、舞台内边距；缩放只按这两个框算，
// 因此每一页的预览都恰好放得下，不会被裁切。
type printLayout struct {
	landscape   bool
	pageCSS     string // @page 的 size 值
	sheetW      string
	sheetH      string
	boxW        float64
	boxH        float64
	indexRows   int
	paperLabel  string
	orientation string
}

func printLayoutFor(landscape bool) printLayout {
	if landscape {
		return printLayout{
			landscape: true, pageCSS: "A4 landscape",
			sheetW: "296mm", sheetH: "209mm",
			boxW: 1019, boxH: 579, indexRows: 14,
			paperLabel: "A4 · 横向", orientation: "landscape",
		}
	}
	return printLayout{
		landscape: false, pageCSS: "A4 portrait",
		sheetW: "209mm", sheetH: "296mm",
		boxW: 635, boxH: 850, indexRows: 18,
		paperLabel: "A4 · 纵向", orientation: "portrait",
	}
}

// PrintDocument 生成完整的设计文稿 HTML。
//
// 调用方负责把 nodes 前置过滤（PrintableNodes）；本函数仍会再过滤一次，
// 保证 frame 节点永远不会进入文稿。systems 可为 nil（未知体系只显示 ID）。
func PrintDocument(p Project, nodes []Node, systems map[string]PrintSystem, now time.Time) string {
	designs := PrintableNodes(nodes)
	frames := 0
	for _, n := range nodes {
		if n.Kind == KindFrame {
			frames++
		}
	}
	lay := printLayoutFor(PrintOrientation(designs))

	indexPages := 0
	totalPages := 3 // 封面 + 概览 + 空态页
	if len(designs) > 0 {
		indexPages = (len(designs) + lay.indexRows - 1) / lay.indexRows
		totalPages = 2 + indexPages + len(designs)
	}

	var b strings.Builder
	b.Grow(32 * 1024)
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<meta name=\"color-scheme\" content=\"light\">\n")
	b.WriteString("<title>" + esc(p.Name) + " · 系统设计文稿 · adago-design</title>\n")
	b.WriteString(printStyle(lay))
	b.WriteString("</head>\n<body class=\"doc\">\n")
	b.WriteString(printBar(p, designs, totalPages, lay))
	b.WriteString("<div class=\"sheet-wrap\">\n")
	b.WriteString(coverSheet(p, designs, frames, totalPages, now, lay))
	b.WriteString(overviewSheet(p, designs, systems, frames, indexPages, totalPages, now, lay))
	if len(designs) == 0 {
		b.WriteString(emptySheet(p, 3, totalPages, lay))
	} else {
		b.WriteString(indexSheets(p.Name, designs, systems, indexPages, totalPages, lay))
		for i, n := range designs {
			pageNo := 2 + indexPages + i + 1
			b.WriteString(designSheet(n, i+1, len(designs), pageNo, totalPages, systems, lay))
		}
	}
	b.WriteString("</div>\n")
	b.WriteString(printReadyScript())
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// esc 是文稿唯一的转义入口（正文 / 属性都用它）。
func esc(s string) string { return html.EscapeString(s) }

// printStyle 返回文稿的全部样式。纸张尺寸通过占位符替换，避免 CSS 里的
// 百分号与 fmt 冲突；颜色是本文稿自己的印刷色板（纸墨），不复用画布 UI 令牌。
func printStyle(lay printLayout) string {
	return strings.NewReplacer(
		"__PAGE__", lay.pageCSS,
		"__SHEET_W__", lay.sheetW,
		"__SHEET_H__", lay.sheetH,
	).Replace(printCSS)
}

func printBar(p Project, designs []Node, totalPages int, lay printLayout) string {
	return `<header class="printbar">` +
		`<div class="printbar__lead"><b>系统设计文稿</b>` +
		`<span class="printbar__hint">` + esc(p.Name) + ` · ` + fmt.Sprint(len(designs)) + ` 张设计稿 · ` +
		esc(lay.paperLabel) + ` · 共 ` + fmt.Sprint(totalPages) + ` 页</span></div>` +
		`<div class="printbar__acts">` +
		`<span class="printbar__hint">浏览器模式请在打印对话框选择「另存为 PDF」，并勾选「背景图形」。</span>` +
		`<button type="button" onclick="window.print()">打印 / 另存为 PDF</button>` +
		`</div></header>` + "\n"
}

// sheetOpen 统一页面的外壳，便于页码与版式调试。
func sheetOpen(class string) string {
	return `<section class="sheet ` + class + `">` + "\n"
}

func sheetClose() string { return "</section>\n" }

// pageHead 是概览 / 索引页共用的页头（编号 + 标题 + 项目名）。
func pageHead(kicker, title, project string) string {
	return `<header class="page-head">` +
		`<div><div class="page-kicker">` + esc(kicker) + `</div>` +
		`<h1 class="page-title">` + esc(title) + `</h1></div>` +
		`<div class="page-proj">` + esc(project) + `</div></header>` + "\n"
}

// pageFoot 是页脚：品牌、项目名、页码。
func pageFoot(project string, page, total int) string {
	return `<footer class="page-foot"><span>adago-design · 系统设计文稿</span>` +
		`<span class="page-foot__proj">` + esc(project) + `</span>` +
		`<span class="page-foot__no">第 ` + fmt.Sprint(page) + ` / ` + fmt.Sprint(total) + ` 页</span></footer>` + "\n"
}

func coverSheet(p Project, designs []Node, frames, totalPages int, now time.Time, lay printLayout) string {
	note := strings.TrimSpace(p.Note)
	noteClass := "cover-note"
	if note == "" {
		note = "这个项目还没有填写备注；文稿按设计稿的画布顺序沉淀成册。"
		noteClass += " is-muted"
	}
	systemCount := countSystems(designs)
	// 标题字号按项目名长度收敛：长名字不把封面撑成两三行。
	titleSize := 36
	switch n := len([]rune(p.Name)); {
	case n > 26:
		titleSize = 22
	case n > 18:
		titleSize = 27
	case n > 10:
		titleSize = 32
	}
	var b strings.Builder
	b.WriteString(sheetOpen("cover"))
	b.WriteString(`<div class="cover-top"><span class="brandmark">D</span>` +
		`<span class="brandword">adago-design</span>` +
		`<span class="cover-kicker">系统设计文稿 · SYSTEM DESIGN DOCUMENT</span></div>` + "\n")
	b.WriteString(`<div class="cover-main">` +
		`<div class="cover-eyebrow">项目设计 · ` + esc(lay.paperLabel) + `</div>` +
		`<h1 class="cover-title" style="font-size:` + fmt.Sprint(titleSize) + `pt">` + esc(p.Name) + `</h1>` +
		`<p class="` + noteClass + `">` + esc(note) + `</p></div>` + "\n")
	b.WriteString(`<div class="cover-stats">` +
			coverStat("设计稿", fmt.Sprint(len(designs))) +
			coverStat("设计体系", fmt.Sprint(systemCount)) +
			coverStat("分组框", fmt.Sprint(frames)) +
			coverStat("文稿页", fmt.Sprint(totalPages)) +
			`</div>` + "\n")
	b.WriteString(`<div class="cover-foot"><span>adago-design · 设计体系 MCP</span>` +
		`<span>生成于 ` + esc(printTime(now)) + `</span></div>` + "\n")
	b.WriteString(sheetClose())
	return b.String()
}

func coverStat(label, value string) string {
	return `<div class="cover-stat"><span class="cover-stat__k">` + esc(label) +
		`</span><span class="cover-stat__v">` + esc(value) + `</span></div>`
}

func overviewSheet(p Project, designs []Node, systems map[string]PrintSystem, frames, indexPages, totalPages int, now time.Time, lay printLayout) string {
	canvasLabel := "自由画布"
	if p.Canvas != nil {
		canvasLabel = p.Canvas.Caption()
	}
	// 体系使用情况：按稿件数量排序，数量相同时按名称排，输出稳定。
	usage := systemUsage(designs, systems)
	kinds := kindUsage(designs)
	composition := ""
	for _, kv := range kinds {
		composition += `<span class="chip"><b>` + esc(kv.label) + `</b>` + fmt.Sprint(kv.count) + `</span>`
	}
	if composition == "" {
		composition = `<span class="chip is-muted">暂无设计稿</span>`
	}
	var sys strings.Builder
	if len(usage) == 0 {
		sys.WriteString(`<div class="sys-empty">还没有体系信息；设计稿会按画布顺序收录。</div>`)
	}
	for _, u := range usage {
		dot := ""
		if validHexColor(u.info.Primary) {
			dot = `<span class="sys-dot" style="background:` + esc(u.info.Primary) + `"></span>`
		} else {
			dot = `<span class="sys-dot is-unknown"></span>`
		}
		name := u.info.Name
		if name == "" {
			name = u.id
			if name == "" {
				name = "未标注体系"
			}
		}
		meta := u.info.Vendor
		if meta == "" {
			meta = u.info.ID
		}
		sys.WriteString(`<div class="sys-row">` + dot +
			`<span class="sys-name">` + esc(name) + `</span>` +
			`<span class="sys-meta">` + esc(meta) + `</span>` +
			`<span class="sys-count">` + fmt.Sprint(u.count) + `</span></div>`)
	}

	designsLabel := fmt.Sprintf("%d 张", len(designs))
	if frames > 0 {
		designsLabel += fmt.Sprintf("（另有 %d 个分组框不单独成页）", frames)
	}
	note := strings.TrimSpace(p.Note)
	noteClass := "note-text"
	if note == "" {
		note = "暂未填写项目备注。"
		noteClass += " is-muted"
	}

	var b strings.Builder
	b.WriteString(sheetOpen("overview"))
	b.WriteString(pageHead("01 · 文稿概览", "项目概览", p.Name))
	b.WriteString(`<div class="ov">` + "\n")
	b.WriteString(`<div class="ov-top">` + "\n")
	b.WriteString(`<div class="panel"><div class="panel-h">项目信息</div>` + "\n")
	b.WriteString(kvRow("项目名", p.Name))
	b.WriteString(kvRow("画布", canvasLabel))
	b.WriteString(kvRow("设计稿", designsLabel))
	b.WriteString(kvRow("创建", printTime(p.CreatedAt)))
	b.WriteString(kvRow("更新", printTime(p.UpdatedAt)))
	b.WriteString(kvRow("项目 ID", p.ID))
	b.WriteString(`</div>` + "\n")

	b.WriteString(`<div class="panel"><div class="panel-h">设计体系</div>` + "\n")
	b.WriteString(`<div class="sys-list">` + sys.String() + `</div>` + "\n")
	b.WriteString(`<div class="panel-h panel-h--sub">设计稿构成</div><div class="chips">` + composition + `</div>` + "\n")
	b.WriteString(`<div class="panel-h panel-h--sub">文稿结构</div>` + "\n")
	indexStep := fmt.Sprint(indexPages) + ` 页`
	if indexPages == 0 {
		indexStep = `空态页`
	}
	b.WriteString(`<div class="flow">` +
		`<span class="step"><b>1</b>封面</span><span class="arw">→</span>` +
		`<span class="step"><b>2</b>概览</span><span class="arw">→</span>` +
		`<span class="step"><b>3</b>索引 ` + indexStep + `</span><span class="arw">→</span>` +
		`<span class="step"><b>4</b>设计稿 ` + fmt.Sprint(len(designs)) + ` 页</span>` +
		`</div>` + "\n")
	b.WriteString(`</div>` + "\n")
	b.WriteString(`</div>` + "\n")

	b.WriteString(`<div class="panel ov-note"><div class="panel-h">项目备注</div>` +
		`<p class="` + noteClass + `">` + esc(note) + `</p></div>` + "\n")
	b.WriteString(`<div class="ov-meta">` +
		`<span>` + esc(lay.paperLabel) + `</span>` +
		`<span>共 ` + fmt.Sprint(totalPages) + ` 页</span>` +
		`<span>生成于 ` + esc(printTime(now)) + `</span>` +
		`<span>数据来源：项目数据（不含外部资源）</span>` +
		`</div>` + "\n")
	b.WriteString(`</div>` + "\n")
	b.WriteString(`<p class="read-note">阅读约定：一张设计稿一页，按画布位置从上到下、从左到右收录；` +
		`预览按比例缩放以完整放入纸面，标注尺寸为画板逻辑像素；分组框只作为背板，不单独成页。` +
		`本文稿由项目数据直接生成，不包含外部引用资源。</p>` + "\n")
	b.WriteString(pageFoot(p.Name, 2, totalPages))
	b.WriteString(sheetClose())
	return b.String()
}

func kvRow(k, v string) string {
	if strings.TrimSpace(v) == "" {
		v = "-"
	}
	return `<div class="kv-row"><span class="k">` + esc(k) + `</span><span class="v">` + esc(v) + `</span></div>`
}

func emptySheet(p Project, page, total int, lay printLayout) string {
	return sheetOpen("empty") +
		pageHead("02 · 设计索引", "设计索引", p.Name) +
		`<div class="empty-body"><div class="empty-mark">D</div>` +
		`<div class="empty-title">这个项目还没有设计稿</div>` +
		`<p class="empty-note">先在画布上添加设计稿，再导出 PDF；文稿会按画布位置自动编页，` +
		`一张稿一页，并生成封面、概览与索引。</p></div>` +
		pageFoot(p.Name, page, total) + sheetClose()
}

type systemUse struct {
	id    string
	info  PrintSystem
	count int
}

func systemUsage(designs []Node, systems map[string]PrintSystem) []systemUse {
	byID := map[string]*systemUse{}
	order := make([]string, 0, len(systems))
	for _, n := range designs {
		id := n.System
		u, ok := byID[id]
		if !ok {
			u = &systemUse{id: id, info: systems[id]}
			if u.id == "" {
				u.id = "未标注体系"
			}
			byID[id] = u
			order = append(order, id)
		}
		u.count++
	}
	out := make([]systemUse, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		ni, nj := out[i].info.Name, out[j].info.Name
		if ni == "" {
			ni = out[i].id
		}
		if nj == "" {
			nj = out[j].id
		}
		return ni < nj
	})
	return out
}

func countSystems(designs []Node) int {
	set := map[string]bool{}
	for _, n := range designs {
		if strings.TrimSpace(n.System) == "" {
			continue
		}
		set[n.System] = true
	}
	return len(set)
}

type kindUse struct {
	label string
	count int
}

func kindUsage(designs []Node) []kindUse {
	counts := map[string]int{}
	for _, n := range designs {
		counts[n.Kind]++
	}
	// 固定顺序，避免 map 遍历抖动：整页 → 组件 → 色板 → 便签 → 其它。
	keys := []string{KindScreen, KindComponent, KindPalette, KindNote}
	out := make([]kindUse, 0, len(counts))
	for _, k := range keys {
		if c := counts[k]; c > 0 {
			out = append(out, kindUse{label: printKindLabel(k), count: c})
			delete(counts, k)
		}
	}
	if len(counts) > 0 {
		rest := make([]string, 0, len(counts))
		for k := range counts {
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range rest {
			out = append(out, kindUse{label: printKindLabel(k), count: counts[k]})
		}
	}
	return out
}

func indexSheets(project string, designs []Node, systems map[string]PrintSystem, indexPages, totalPages int, lay printLayout) string {
	var b strings.Builder
	for page := 0; page < indexPages; page++ {
		from := page * lay.indexRows
		to := from + lay.indexRows
		if to > len(designs) {
			to = len(designs)
		}
		pageNo := 2 + page + 1
		b.WriteString(sheetOpen("index"))
		b.WriteString(pageHead(fmt.Sprintf("02 · 设计索引 %d / %d", page+1, indexPages), "设计索引", project))
		b.WriteString(`<table class="idx"><thead><tr>` +
			`<th class="idx-no">#</th><th class="idx-title">设计稿</th><th class="idx-sys">体系</th>` +
			`<th class="idx-kind">类型</th><th class="idx-ab">画板</th><th class="idx-size">尺寸</th>` +
			`<th class="idx-page">文稿页</th></tr></thead><tbody>` + "\n")
		for i := from; i < to; i++ {
			n := designs[i]
			pageOf := 2 + indexPages + i + 1
			dot := ""
			if info, ok := systems[n.System]; ok && validHexColor(info.Primary) {
				dot = `<span class="sys-dot sys-dot--sm" style="background:` + esc(info.Primary) + `"></span>`
			}
			b.WriteString(`<tr>` +
				`<td class="idx-no">` + fmt.Sprint(i+1) + `</td>` +
				`<td class="idx-title">` + dot + `<span>` + esc(n.Title) + `</span></td>` +
				`<td class="idx-sys">` + esc(printSystemName(systems, n.System)) + `</td>` +
				`<td class="idx-kind">` + esc(printKindLabel(n.Kind)) + `</td>` +
				`<td class="idx-ab">` + esc(dash(n.Artboard)) + `</td>` +
				`<td class="idx-size">` + fmt.Sprintf("%d × %d", n.Width, n.Height) + `</td>` +
				`<td class="idx-page">` + fmt.Sprint(pageOf) + `</td></tr>` + "\n")
		}
		b.WriteString(`</tbody></table>` + "\n")
		firstDesignPage := 2 + indexPages + 1
		b.WriteString(`<p class="idx-note">共 ` + fmt.Sprint(len(designs)) + ` 张设计稿 · 逐稿页从第 ` +
			fmt.Sprint(firstDesignPage) + ` 页开始 · 按画布位置从上到下、从左到右排序</p>` + "\n")
		b.WriteString(pageFoot(project, pageNo, totalPages))
		b.WriteString(sheetClose())
	}
	return b.String()
}

func printSystemName(systems map[string]PrintSystem, id string) string {
	if info, ok := systems[id]; ok && strings.TrimSpace(info.Name) != "" {
		return info.Name
	}
	if strings.TrimSpace(id) == "" {
		return "未标注体系"
	}
	return id
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func designSheet(n Node, index, count, pageNo, totalPages int, systems map[string]PrintSystem, lay printLayout) string {
	scale := 1.0
	if n.Width > 0 && n.Height > 0 {
		scale = math.Min(lay.boxW/float64(n.Width), lay.boxH/float64(n.Height))
		if scale > 1 {
			scale = 1
		}
		if scale <= 0 || math.IsNaN(scale) {
			scale = 1
		}
	}
	sw := int(math.Round(float64(n.Width) * scale))
	sh := int(math.Round(float64(n.Height) * scale))
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}

	info := systems[n.System]
	sysName := printSystemName(systems, n.System)
	dot := ""
	if validHexColor(info.Primary) {
		dot = `<span class="meta-dot" style="background:` + esc(info.Primary) + `"></span>`
	}
	tags := ""
	if len(n.Tags) > 0 {
		tags = `<span class="design-tags">` + esc(strings.Join(n.Tags, " · ")) + `</span>`
	}
	preview := `<div class="no-preview">这张稿只有元数据，没有可渲染的正文。</div>`
	if strings.TrimSpace(n.HTML) != "" {
		preview = `<div class="preview" style="width:` + fmt.Sprint(sw) + `px;height:` + fmt.Sprint(sh) + `px">` +
			`<div class="frame" style="width:` + fmt.Sprint(n.Width) + `px;height:` + fmt.Sprint(n.Height) +
			`px;transform:scale(` + fmt.Sprintf("%.3f", scale) + `)">` +
			`<iframe sandbox="allow-same-origin" scrolling="no" srcdoc="` + esc(n.HTML) + `"></iframe>` +
			`</div></div>`
	}

	var b strings.Builder
	b.WriteString(sheetOpen("design"))
	b.WriteString(`<header class="design-head">` +
		`<div class="design-no">` + fmt.Sprintf("%02d", index) + `</div>` +
		`<div class="design-title-wrap"><h2 class="design-title">` + esc(n.Title) + `</h2>` +
		`<div class="design-meta">` +
		`<span class="meta-chip">` + dot + esc(sysName) + `</span>` +
		`<span class="meta-chip">` + esc(printKindLabel(n.Kind)) + `</span>` +
		`<span class="meta-chip">画板 ` + esc(dash(n.Artboard)) + `</span>` +
		`<span class="meta-chip">` + fmt.Sprintf("%d × %d", n.Width, n.Height) + `</span>` +
		`</div></div>` +
		`<div class="design-head__right"><span class="design-seq">设计稿 ` + fmt.Sprint(index) + ` / ` + fmt.Sprint(count) + `</span>` +
		`<span class="design-scale">预览 ` + fmt.Sprintf("%d%%", int(math.Round(scale*100))) + `</span></div>` +
		`</header>` + "\n")
	b.WriteString(`<div class="stage">` + preview + `</div>` + "\n")
	b.WriteString(`<footer class="design-foot">` + tags +
		`<span class="design-foot__id">` + esc(n.ID) + `</span>` +
		`<span class="design-foot__time">更新 ` + esc(printTime(n.UpdatedAt)) + `</span>` +
		`<span class="design-foot__no">第 ` + fmt.Sprint(pageNo) + ` / ` + fmt.Sprint(totalPages) + ` 页</span>` +
		`</footer>` + "\n")
	b.WriteString(sheetClose())
	return b.String()
}

// printReadyScript 在文档、字体与全部预览 iframe 就绪后置位
// window.__adagoPrintReady；原生壳轮询这个标记，避免打印出半张空白页。
// 不依赖 requestAnimationFrame：隐藏窗口会被节流，定时器兜底更可靠。
func printReadyScript() string {
	return `<script>
(function () {
  var frames = Array.prototype.slice.call(document.querySelectorAll("iframe"));
  function waitFrame(f) {
    return new Promise(function (res) {
      try {
        var d = f.contentDocument;
        if (d && d.readyState === "complete") { res(); return; }
      } catch (_) { /* 跨域或文档未就绪：继续等 load */ }
      f.addEventListener("load", function () { res(); }, { once: true });
      setTimeout(res, 5000);
    });
  }
  Promise.all(frames.map(waitFrame))
    .then(function () { return document.fonts && document.fonts.ready; })
    .then(function () { return new Promise(function (res) { setTimeout(res, 180); }); })
    .then(function () {
      window.__adagoPrintReady = true;
      document.documentElement.setAttribute("data-print-ready", "1");
      if ((location.search || "").indexOf("print=1") >= 0) {
        setTimeout(function () { window.print(); }, 260);
      }
    });
})();
</script>
` + "\n"
}

// PDFFileName 是下载时的建议文件名（保留中文，去掉路径分隔符与控制字符）。
func PDFFileName(p Project, now time.Time) string {
	base := strings.TrimSpace(p.Name)
	if base == "" {
		base = "项目"
	}
	base = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		case '\r', '\n', '\t':
			return ' '
		default:
			return r
		}
	}, base)
	// 控制字符也去掉（文件名在 Windows / macOS 上都要合法）
	base = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, base)
	runes := []rune(strings.TrimSpace(base))
	if len(runes) > 48 {
		runes = runes[:48]
	}
	return string(runes) + "-系统设计文稿-" + now.Format("20060102-1504") + ".pdf"
}

// PDFDisposition 生成兼容 RFC 6266 / RFC 5987 的 Content-Disposition：
// ASCII 兜底给老客户端，filename* 给现代浏览器保留中文项目名。
func PDFDisposition(filename string) string {
	var ascii strings.Builder
	for _, r := range filename {
		if r < 0x80 && r != '"' && r != '\\' && r != '\r' && r != '\n' {
			ascii.WriteRune(r)
		} else {
			ascii.WriteByte('_')
		}
	}
	fallback := ascii.String()
	if fallback == "" || strings.Trim(fallback, "._- ") == "" {
		fallback = "adago-design.pdf"
	}
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + encodeRFC5987(filename)
}

// encodeRFC5987 按 RFC 5987 的 attr-char 集合编码：只保留字母数字与
// !#$&+-.^_`|~，其余（含空格、引号、百分号、中文）一律 %XX。
// 不能用 url.PathEscape：它保留单引号与部分 RFC 5987 之外的字符。
func encodeRFC5987(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// ---------------- HTTP ----------------

// printSystems 从目录拿到体系摘要，供文稿显示名称 / 厂商 / 主色。
// 目录未注入（单测或极简装配）时返回空表，文稿退化为显示体系 ID。
func (s *Server) printSystems() map[string]PrintSystem {
	out := map[string]PrintSystem{}
	if s.Catalog == nil {
		return out
	}
	list, ok := s.Catalog.SystemSummaries().([]map[string]any)
	if !ok {
		return out
	}
	for _, m := range list {
		id := mapString(m, "id")
		if id == "" {
			continue
		}
		out[id] = PrintSystem{
			ID: id, Name: mapString(m, "name"), Vendor: mapString(m, "vendor"),
			Primary: mapString(m, "primary"), DefaultArtboard: mapString(m, "default_artboard"),
		}
	}
	return out
}

func mapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// handlePrintDocument 输出文稿 HTML（浏览器直接打开、原生壳隐藏窗口加载）。
// 它是只读端点：不写盘、不改项目，任何一份项目数据都能重复生成同一份文稿。
func (s *Server) handlePrintDocument(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	bundle, err := s.store.ExportProject(id)
	if err != nil {
		writeErr(w, statusForProjectErr(err), projectErr(err, id))
		return
	}
	doc := PrintDocument(bundle.Project, bundle.Nodes, s.printSystems(), time.Now())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(doc))
}

// handleProjectPDF 生成项目设计文稿 PDF。
//
// 浏览器模式没有 PrintToPDF：返回 501 pdf_unavailable，页面据此打开
// /projects/{id}/print?print=1，在打印对话框里「另存为 PDF」。
func (s *Server) handleProjectPDF(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	bundle, err := s.store.ExportProject(id)
	if err != nil {
		writeErr(w, statusForProjectErr(err), projectErr(err, id))
		return
	}
	designs := PrintableNodes(bundle.Nodes)
	if len(designs) == 0 {
		writeErr(w, http.StatusConflict, APIError("empty_project",
			"项目里还没有设计稿，无法生成文稿",
			"先在画布上添加至少一张设计稿；分组框（frame）不单独成页"))
		return
	}
	if s.PDF == nil {
		writeErr(w, http.StatusNotImplemented, APIError("pdf_unavailable",
			"当前运行形态没有 PDF 渲染能力",
			"浏览器模式会打开打印页，请在打印对话框选择「另存为 PDF」"))
		return
	}
	landscape := PrintOrientation(designs)
	pageURL := "http://" + r.Host + "/projects/" + url.PathEscape(id) + "/print"
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	pdf, err := s.PDF(ctx, pageURL, landscape)
	if err != nil {
		writeErr(w, http.StatusBadGateway, APIError("pdf_failed",
			"PDF 渲染失败: "+err.Error(),
			"确认原生窗口已打开；浏览器模式下改用打印页「另存为 PDF」"))
		return
	}
	if len(pdf) == 0 {
		writeErr(w, http.StatusBadGateway, APIError("pdf_failed", "PDF 渲染结果为空", "重试一次；持续失败请查看日志"))
		return
	}
	filename := PDFFileName(bundle.Project, time.Now())
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", PDFDisposition(filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// printCSS 是文稿的印刷样式。它以 mm / pt 排版，屏幕与纸面共用一套结构；
// 屏幕多一条打印提示条，纸面隐藏。分页只用 break-after，页边距为 0，
// 由每页自己的内边距形成版心（浏览器打印对话框也不会再叠一层页边距）。
const printCSS = `<style>
:root{
  --p-ink:#16171A; --p-ink2:#57544C; --p-ink3:#6E6A60;
  --p-paper:#FBF8F1; --p-canvas:#EDE8DC; --p-elevated:#FFFDF8;
  --p-subtle:#F2EDE2; --p-line:#DCD5C6; --p-line2:#C9C0AC;
  --p-brand:#16171A; --p-brand-soft:#E7E1D2;
}
*{box-sizing:border-box}
html,body{margin:0;padding:0}
body{background:var(--p-canvas);color:var(--p-ink);font-family:"Inter","Noto Sans SC",system-ui,-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;font-size:10.5pt;line-height:1.55;-webkit-print-color-adjust:exact;print-color-adjust:exact}
h1,h2{font-weight:700;letter-spacing:-.01em}
@page{size:__PAGE__;margin:0}
.printbar{position:sticky;top:0;z-index:20;display:flex;align-items:center;justify-content:space-between;gap:8mm;padding:8mm 16mm 5mm;background:var(--p-canvas)}
.printbar__lead{display:flex;align-items:baseline;gap:4mm;font-size:11pt}
.printbar__hint{color:var(--p-ink3);font-size:9pt}
.printbar__acts{display:flex;align-items:center;gap:5mm}
.printbar button{font:inherit;font-size:9.5pt;color:var(--p-paper);background:var(--p-ink);border:1px solid var(--p-ink);border-radius:999px;padding:2mm 6mm;cursor:pointer}
.sheet-wrap{padding:0 0 12mm}
.sheet{position:relative;display:flex;flex-direction:column;width:__SHEET_W__;height:__SHEET_H__;margin:0 auto 10mm;padding:12mm 14mm 10mm;background:var(--p-paper);overflow:hidden;box-shadow:0 4mm 12mm rgba(22,23,26,.16)}
.sheet+.sheet{margin-top:0}
.page-head{display:flex;align-items:flex-end;justify-content:space-between;gap:8mm;padding-bottom:3mm;border-bottom:.35mm solid var(--p-line2);margin-bottom:6mm}
.page-kicker{font-size:8pt;letter-spacing:.14em;text-transform:uppercase;color:var(--p-ink3)}
.page-title{font-size:21pt;line-height:1.1;margin:1.5mm 0 0}
.page-proj{font-size:9.5pt;color:var(--p-ink2);text-align:right;max-width:55%}
.page-foot{display:flex;align-items:center;justify-content:space-between;gap:6mm;margin-top:auto;padding-top:3mm;border-top:.35mm solid var(--p-line);font-size:8pt;color:var(--p-ink3)}
.page-foot__proj{flex:1;text-align:center;color:var(--p-ink3)}
.page-foot__no{white-space:nowrap;color:var(--p-ink2)}

/* 封面 */
.cover{background:var(--p-ink);color:var(--p-paper);padding:14mm 16mm 12mm}
.cover-top{display:flex;align-items:center;gap:4mm}
.brandmark{width:9mm;height:9mm;border-radius:1.5mm;display:flex;align-items:center;justify-content:center;background:var(--p-paper);color:var(--p-ink);font-size:13pt;font-weight:700}
.brandword{font-size:11pt;font-weight:600;letter-spacing:.02em}
.cover-kicker{margin-left:auto;font-size:8pt;letter-spacing:.16em;color:#BDB8AD;text-transform:uppercase}
.cover-main{flex:1;display:flex;flex-direction:column;justify-content:center;gap:5mm}
.cover-eyebrow{font-size:9pt;letter-spacing:.12em;color:#BDB8AD;text-transform:uppercase}
.cover-title{font-size:36pt;line-height:1.06;margin:0;max-width:86%;overflow-wrap:anywhere}
.cover-note{font-size:11pt;line-height:1.7;color:#D8D3C8;max-width:70%;margin:0;white-space:pre-wrap}
.cover-note.is-muted{color:#9E998E;font-style:italic}
.cover-stats{display:grid;grid-template-columns:repeat(4,1fr);gap:0;border-top:.35mm solid rgba(246,243,236,.26)}
.cover-stat{display:flex;flex-direction:column;gap:1mm;padding:5mm 0 4mm}
.cover-stat:not(:last-child){border-right:.2mm solid rgba(246,243,236,.18);padding-right:5mm}
.cover-stat__k{font-size:8pt;letter-spacing:.12em;color:#BDB8AD;text-transform:uppercase}
.cover-stat__v{font-size:18pt;font-weight:700;color:var(--p-paper)}
.cover-foot{display:flex;justify-content:space-between;gap:6mm;margin-top:7mm;font-size:8pt;color:#9E998E}

/* 概览 */
.ov{display:flex;flex-direction:column;gap:5mm}
.ov-top{display:grid;grid-template-columns:1.08fr .92fr;gap:7mm;align-items:start}
.panel{border:.3mm solid var(--p-line);border-radius:2.5mm;background:var(--p-elevated);padding:5mm 6mm;display:flex;flex-direction:column;gap:3mm;min-height:0}
.panel-h{font-size:8.5pt;letter-spacing:.12em;text-transform:uppercase;color:var(--p-ink3);margin:0}
.panel-h--sub{margin-top:1mm}
.ov-note{padding:5mm 6mm;gap:2mm}
.note-text{font-size:10.5pt;line-height:1.75;color:var(--p-ink2);margin:0;white-space:pre-wrap}
.note-text.is-muted{color:var(--p-ink3);font-style:italic}
.ov-meta{display:flex;flex-wrap:wrap;gap:2mm 7mm;font-size:8.5pt;color:var(--p-ink3);padding-top:2.5mm;border-top:.2mm solid var(--p-line)}
.kv-row{display:grid;grid-template-columns:24mm 1fr;gap:3mm;padding:1.8mm 0;border-bottom:.2mm dashed var(--p-line);font-size:9.5pt}
.kv-row .k{color:var(--p-ink3)}
.kv-row .v{overflow-wrap:anywhere}
.sys-list{display:flex;flex-direction:column;gap:0}
.sys-row{display:flex;align-items:center;gap:3mm;padding:2.2mm 0;border-bottom:.2mm dashed var(--p-line);font-size:9.5pt}
.sys-row:last-child{border-bottom:0}
.sys-dot{width:4mm;height:4mm;border-radius:1mm;background:var(--p-brand-soft);flex:none}
.sys-dot.is-unknown{background:transparent;border:.25mm solid var(--p-line2)}
.sys-dot--sm{width:2.6mm;height:2.6mm;border-radius:.7mm;display:inline-block;vertical-align:-.4mm;margin-right:1.5mm}
.sys-name{font-weight:600;overflow-wrap:anywhere}
.sys-meta{margin-left:auto;color:var(--p-ink3);font-size:8.5pt}
.sys-count{font-variant-numeric:tabular-nums;color:var(--p-ink2);min-width:6mm;text-align:right}
.sys-empty{font-size:9pt;color:var(--p-ink3);font-style:italic}
.chips{display:flex;flex-wrap:wrap;gap:2mm}
.chip{display:inline-flex;align-items:center;gap:1.5mm;padding:1.6mm 3mm;border:.25mm solid var(--p-line2);border-radius:999px;font-size:8.5pt;background:var(--p-paper)}
.chip b{font-weight:600}
.chip.is-muted{color:var(--p-ink3);font-style:italic}
.flow{display:flex;align-items:center;flex-wrap:wrap;gap:2mm;font-size:8.5pt;color:var(--p-ink2)}
.step{display:inline-flex;align-items:center;gap:1.5mm;padding:1.4mm 2.6mm;border-radius:1.2mm;background:var(--p-subtle)}
.step b{font-size:7.5pt;color:var(--p-ink3)}
.arw{color:var(--p-line2)}
.read-note{font-size:8.5pt;line-height:1.7;color:var(--p-ink3);margin:5mm 0 0;border-left:.8mm solid var(--p-line2);padding-left:4mm}

/* 索引 */
.idx{width:100%;border-collapse:collapse;font-size:9.5pt;table-layout:fixed;font-variant-numeric:tabular-nums}
.idx thead th{text-align:left;font-size:8pt;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:var(--p-ink3);border-bottom:.35mm solid var(--p-line2);padding:0 2mm 2mm;white-space:nowrap}
.idx tbody tr{border-bottom:.2mm solid var(--p-line)}
.idx tbody tr:nth-child(even){background:rgba(242,237,226,.45)}
.idx td{padding:2.3mm 2mm;vertical-align:top;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.idx-no{width:8mm;color:var(--p-ink3)}
.idx-title{width:auto}
.idx-sys{width:30mm}
.idx-kind{width:16mm}
.idx-ab{width:24mm;font-size:8.5pt;color:var(--p-ink2)}
.idx-size{width:28mm;font-size:8.5pt;color:var(--p-ink2)}
.idx-page{width:16mm;text-align:right;color:var(--p-ink2)}
.idx-note{margin:5mm 0 0;font-size:8.5pt;color:var(--p-ink3);border-left:.8mm solid var(--p-line2);padding-left:3mm}
.empty-body{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4mm;text-align:center}
.empty-mark{width:14mm;height:14mm;border-radius:2.5mm;border:.4mm solid var(--p-line2);display:flex;align-items:center;justify-content:center;font-size:18pt;font-weight:700;color:var(--p-ink3)}
.empty-title{font-size:17pt;font-weight:700}
.empty-note{max-width:105mm;font-size:9.5pt;color:var(--p-ink3);margin:0}

/* 逐稿页 */
.design-head{display:flex;align-items:flex-start;gap:5mm;padding-bottom:3mm;border-bottom:.35mm solid var(--p-line2);margin-bottom:4mm}
.design-no{font-size:27pt;font-weight:700;line-height:.9;color:var(--p-ink3);min-width:16mm;font-variant-numeric:tabular-nums}
.design-title-wrap{min-width:0}
.design-title{font-size:18pt;line-height:1.15;margin:0;overflow-wrap:anywhere}
.design-meta{display:flex;flex-wrap:wrap;gap:1.5mm 4mm;margin-top:2mm;font-size:8.5pt;color:var(--p-ink2)}
.meta-chip{display:inline-flex;align-items:center;gap:1.5mm}
.meta-dot{width:2.6mm;height:2.6mm;border-radius:.7mm;flex:none;background:var(--p-brand-soft)}
.design-head__right{margin-left:auto;text-align:right;display:flex;flex-direction:column;gap:1mm;font-size:8.5pt;color:var(--p-ink3);white-space:nowrap}
.design-seq{font-variant-numeric:tabular-nums}
.stage{flex:1;min-height:0;display:flex;align-items:center;justify-content:center;padding:3mm;border:.3mm solid var(--p-line);border-radius:2mm;background-color:var(--p-canvas);background-image:radial-gradient(var(--p-line2) .28mm,transparent .28mm);background-size:4mm 4mm;overflow:hidden}
.preview{position:relative;flex:none;overflow:hidden;border-radius:1.5mm;background:#FFF;box-shadow:0 2mm 6mm rgba(22,23,26,.18)}
.frame{position:absolute;left:0;top:0;transform-origin:top left;background:#FFF}
.frame iframe{display:block;width:100%;height:100%;border:0;background:#FFF}
.no-preview{font-size:9pt;color:var(--p-ink3);font-style:italic}
.design-foot{display:flex;align-items:center;gap:5mm;margin-top:3mm;padding-top:3mm;border-top:.2mm dashed var(--p-line);font-size:8pt;color:var(--p-ink3)}
.design-tags{color:var(--p-ink2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:35%}
.design-foot__id{font-family:"JetBrains Mono",ui-monospace,monospace;color:var(--p-ink3)}
.design-foot__time{margin-left:auto;white-space:nowrap}
.design-foot__no{white-space:nowrap;color:var(--p-ink2)}

@media print{
  body{background:#FFF}
  .printbar{display:none}
  .sheet-wrap{padding:0}
  .sheet{margin:0 auto;box-shadow:none;break-after:page;page-break-after:always}
  .sheet:last-child{break-after:auto;page-break-after:auto}
}
</style>
`
