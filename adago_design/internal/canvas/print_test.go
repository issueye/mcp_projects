package canvas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func wideNode(title string) Node {
	return Node{
		Title: title, Kind: KindScreen, System: "apple", Artboard: "iphone-15",
		HTML:  "<!DOCTYPE html><html><body><p>" + title + "</p></body></html>",
		Width: 390, Height: 844, X: 0, Y: 0,
	}
}

func TestPrintOrientation(t *testing.T) {
	cases := []struct {
		name      string
		nodes     []Node
		landscape bool
	}{
		{"空画布默认横向", nil, true},
		{"竖稿", []Node{{Width: 390, Height: 844}}, false},
		{"横稿", []Node{{Width: 1440, Height: 900}}, true},
		{"竖稿多", []Node{{Width: 390, Height: 844}, {Width: 430, Height: 932}, {Width: 1440, Height: 900}}, false},
		{"横稿多", []Node{{Width: 390, Height: 844}, {Width: 1440, Height: 900}, {Width: 1920, Height: 1080}}, true},
		{"分组框不参与", []Node{{Kind: KindFrame, Width: 100, Height: 900}, {Width: 390, Height: 844}}, false},
	}
	for _, c := range cases {
		if got := PrintOrientation(c.nodes); got != c.landscape {
			t.Errorf("%s: PrintOrientation = %v，期望 %v", c.name, got, c.landscape)
		}
	}
}

func TestPrintableNodesOrderAndFilter(t *testing.T) {
	nodes := []Node{
		{ID: "a", Title: "后行右", Kind: KindNote, X: 500, Y: 80, Width: 200, Height: 120},
		{ID: "b", Title: "前行右", Kind: KindNote, X: 500, Y: 10, Width: 200, Height: 120},
		{ID: "c", Title: "前行左", Kind: KindNote, X: 100, Y: 12, Width: 200, Height: 120},
		{ID: "f", Title: "分组框", Kind: KindFrame, X: -50, Y: -50, Width: 900, Height: 600},
	}
	got := PrintableNodes(nodes)
	if len(got) != 3 {
		t.Fatalf("PrintableNodes 数量 = %d，期望 3（分组框不入稿）", len(got))
	}
	want := []string{"c", "b", "a"} // 同一行桶内按 X；下一行靠后
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("第 %d 个 = %s，期望 %s（行优先、行内按 X）", i, got[i].ID, id)
		}
	}
	// 返回的是副本：改文稿用的节点不影响 Store 里的数据
	got[0].Tags = append(got[0].Tags, "x")
	if len(nodes[2].Tags) != 0 {
		t.Fatal("PrintableNodes 应返回副本，不能与入参共享切片")
	}
}

func TestPrintDocumentCoverOverviewIndex(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 30, 0, 0, time.Local)
	p := Project{
		ID: "p-1", Name: `官网改版 <A&B>`, Note: "首屏信息层级重排；验收看对比度。",
		Canvas:    &ProjectCanvas{Width: 1920, Height: 1080, Ratio: "16:9"},
		CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now,
	}
	nodes := []Node{
		{ID: "n1", Title: "iPhone 首页", Kind: KindScreen, System: "apple", Artboard: "iphone-15",
			HTML: "<!DOCTYPE html><html><body>alpha</body></html>", Width: 390, Height: 844, X: 0, Y: 0},
		{ID: "n2", Title: "桌面首页", Kind: KindScreen, System: "ant", Artboard: "web-1440",
			HTML: "<!DOCTYPE html><html><body>beta</body></html>", Width: 1440, Height: 900, X: 0, Y: 900},
		{ID: "n3", Title: "组件：按钮", Kind: KindComponent, System: "apple", Artboard: "iphone-15",
			HTML: "<!DOCTYPE html><html><body>gamma</body></html>", Width: 390, Height: 320, X: 450, Y: 0},
		{ID: "n4", Title: "背景分组框 XYZ", Kind: KindFrame, Width: 1200, Height: 800},
	}
	systems := map[string]PrintSystem{
		"apple": {ID: "apple", Name: "Apple HIG", Vendor: "Apple", Primary: "#007AFF"},
		"ant":   {ID: "ant", Name: "Ant Design", Vendor: "Ant", Primary: "#1677FF"},
		"bad":   {ID: "bad", Name: "脏数据", Primary: "red;}</style><script>alert(1)</script>"},
	}
	doc := PrintDocument(p, nodes, systems, now)

	// 三张稿 → 竖稿 1：横稿 2，纸张应为横向
	if !strings.Contains(doc, "size:A4 landscape") {
		t.Error("文稿纸张应为横向（横稿占多数）")
	}
	if strings.Contains(doc, "背景分组框 XYZ") {
		t.Error("frame 节点不应出现在文稿里")
	}
	// 标题 / 备注都要转义
	if !strings.Contains(doc, "官网改版 &lt;A&amp;B&gt;") {
		t.Error("项目名未转义")
	}
	if !strings.Contains(doc, "首屏信息层级重排；验收看对比度。") {
		t.Error("项目备注未进入封面")
	}
	// 封面统计：3 张稿、2 个体系、1 个分组框
	if !strings.Contains(doc, `<span class="cover-stat__v">3</span>`) {
		t.Error("封面设计稿计数错误")
	}
	if !strings.Contains(doc, `<span class="cover-stat__v">2</span>`) {
		t.Error("封面体系计数错误")
	}
	if !strings.Contains(doc, `<span class="cover-stat__v">1</span>`) {
		t.Error("封面分组框计数错误")
	}
	// 体系名与主色点
	if !strings.Contains(doc, "Apple HIG") || !strings.Contains(doc, "background:#007AFF") {
		t.Error("体系摘要未进入文稿")
	}
	// 非法主色值不应写进样式（只允许 #hex）
	if strings.Contains(doc, "alert(1)") {
		t.Error("非法体系主色未做形状校验，脏数据进入了文稿")
	}
	// 索引：两页？这里 3 行 → 1 页索引；首稿页码 = 2 + 1 + 1 = 4
	if !strings.Contains(doc, `<td class="idx-page">4</td>`) {
		t.Error("索引里的文稿页码不正确")
	}
	// 逐稿页：标题、画板、尺寸、来源
	for _, want := range []string{"iPhone 首页", "画板 iphone-15", "390 × 844", "桌面首页", "组件：按钮"} {
		if !strings.Contains(doc, want) {
			t.Errorf("文稿缺少 %q", want)
		}
	}
	// 一页一稿：sheet 数与总页数一致（封面 1 + 概览 1 + 索引 1 + 稿 3 = 6）
	if got := strings.Count(doc, `<section class="sheet`); got != 6 {
		t.Errorf("sheet 数 = %d，期望 6", got)
	}
	// 就绪标记与打印脚本
	for _, want := range []string{"__adagoPrintReady", "window.print()"} {
		if !strings.Contains(doc, want) {
			t.Errorf("文稿缺少 %q", want)
		}
	}
	if !strings.Contains(doc, `class="preview"`) || !strings.Contains(doc, `<iframe`) {
		t.Error("文稿缺少预览 iframe")
	}
}

func TestPrintDocumentIndexPaginationAndEmpty(t *testing.T) {
	now := time.Now()
	p := Project{ID: "p", Name: "大量稿"}
	var nodes []Node
	for i := 0; i < 20; i++ {
		nodes = append(nodes, Node{
			ID: fmt.Sprintf("n%02d", i), Title: fmt.Sprintf("稿 %02d", i), Kind: KindComponent,
			System: "apple", Width: 420, Height: 320, HTML: "<html><body>x</body></html>",
			X: float64(i * 10),
		})
	}
	doc := PrintDocument(p, nodes, nil, now)
	// 横稿 → 每页索引 14 行 → 2 页索引；总页数 2 + 2 + 20 = 24
	if got := strings.Count(doc, `<section class="sheet`); got != 24 {
		t.Errorf("sheet 数 = %d，期望 24（2 页索引 + 20 张稿 + 封面概览）", got)
	}
	if !strings.Contains(doc, `02 · 设计索引 2 / 2`) {
		t.Error("索引分页标题缺失")
	}
	// 第一张稿的页码 = 2 + 2 + 1 = 5
	if !strings.Contains(doc, `<td class="idx-page">5</td>`) {
		t.Error("第二页索引仍应从第 1 张稿开始编页")
	}

	empty := PrintDocument(Project{ID: "p-empty", Name: "空项目"}, nil, nil, now)
	if got := strings.Count(empty, `<section class="sheet`); got != 3 {
		t.Errorf("空项目 sheet 数 = %d，期望 3（封面 + 概览 + 空态）", got)
	}
	if !strings.Contains(empty, "这个项目还没有设计稿") {
		t.Error("空项目缺少空态说明")
	}
}

func TestPrintDocumentRejectsInvalidPrimary(t *testing.T) {
	doc := PrintDocument(Project{Name: "脏色"}, []Node{{
		Title: "稿", Kind: KindComponent, System: "bad", Width: 420, Height: 320,
		HTML: "<html><body>ok</body></html>",
	}}, map[string]PrintSystem{
		"bad": {ID: "bad", Name: "脏体系", Primary: `red;}</style><script>alert(1)</script>`},
	}, time.Now())
	if strings.Contains(doc, "alert(1)") {
		t.Error("非法主色被写入了文稿样式")
	}
	if !strings.Contains(doc, "脏体系") {
		t.Error("体系名仍应显示（只有主色被丢弃）")
	}
	if !strings.Contains(doc, "sys-dot is-unknown") {
		t.Error("非法主色应退化为无主色的体系点")
	}
}

func TestPrintDocumentEscapesPreviewAttribute(t *testing.T) {
	p := Project{Name: "转义"}
	n := Node{
		Title: `题"名`, Kind: KindComponent, System: "x", Width: 420, Height: 320,
		HTML: `<html><body><img src="x" onerror="alert(1)"><script>alert(2)</script></body></html>`,
	}
	doc := PrintDocument(p, []Node{n}, nil, time.Now())
	if strings.Contains(doc, `<img src="x"`) || strings.Contains(doc, `<script>alert(2)`) {
		t.Error("预览正文未作为 srcdoc 属性转义，原始标签泄漏进了文稿")
	}
	if !strings.Contains(doc, "&lt;img src=") {
		t.Error("预览正文没有按 HTML 转义写入 srcdoc")
	}
	if !strings.Contains(doc, `题&#34;名`) {
		t.Error("稿件标题未转义")
	}
}

func TestPDFFileNameAndDisposition(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 4, 0, 0, time.Local)
	got := PDFFileName(Project{Name: `官网 / 改版: "v2"`}, now)
	if !strings.HasSuffix(got, "-系统设计文稿-20260708-1504.pdf") {
		t.Errorf("文件名后缀不对: %q", got)
	}
	for _, bad := range []string{"/", ":", `"`} {
		if strings.Contains(got, bad) {
			t.Errorf("文件名里残留非法字符 %q: %q", bad, got)
		}
	}
	if got := PDFFileName(Project{}, now); !strings.HasPrefix(got, "项目-系统设计文稿") {
		t.Errorf("空项目名兜底不对: %q", got)
	}

	d := PDFDisposition("项目-文稿.pdf")
	if !strings.HasPrefix(d, `attachment; filename="`) || !strings.Contains(d, "filename*=UTF-8''") {
		t.Errorf("Content-Disposition 不合规: %q", d)
	}
	if strings.ContainsAny(d, "\r\n") {
		t.Error("Content-Disposition 不应含换行")
	}
	if strings.Contains(d, `filename="项目`) {
		t.Error("ASCII 兜底文件名不应含非 ASCII 字符")
	}
	if got := PDFDisposition("a'b c.pdf"); !strings.Contains(got, "a%27b%20c.pdf") {
		t.Errorf("RFC 5987 编码没有处理单引号/空格: %q", got)
	}
}

func TestPrintRoutes(t *testing.T) {
	srv, ts := newTestServer(t)
	id := newHTTPProject(t, ts)
	// 通过 Store 直接放一张稿：竖稿
	addNode(t, srv.store, id, Node{
		Title: "手机首页", Kind: KindScreen, System: "apple", Artboard: "iphone-15",
		HTML: "<html><body>手机首页</body></html>", Width: 390, Height: 844,
	})

	res, err := http.Get(ts.URL + "/projects/" + id + "/print")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("文稿页 status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("文稿页 Content-Type = %q", ct)
	}
	html := string(body)
	for _, want := range []string{"手机首页", "系统设计文稿", "preview"} {
		if !strings.Contains(html, want) {
			t.Errorf("文稿页缺少 %q", want)
		}
	}

	// 未知项目
	res2, _ := http.Get(ts.URL + "/projects/nope/print")
	b2, _ := io.ReadAll(res2.Body)
	res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound || !strings.Contains(string(b2), "unknown_project") {
		t.Errorf("未知项目文稿页 = %d %s", res2.StatusCode, b2)
	}

	// 空项目：文稿页仍应 200，并给出空态而不是空白页
	empty, err := srv.store.AddProject("空文稿页", "")
	if err != nil {
		t.Fatal(err)
	}
	resEmpty, err := http.Get(ts.URL + "/projects/" + empty.ID + "/print")
	if err != nil {
		t.Fatal(err)
	}
	bodyEmpty, _ := io.ReadAll(resEmpty.Body)
	resEmpty.Body.Close()
	if resEmpty.StatusCode != http.StatusOK || !strings.Contains(string(bodyEmpty), "这个项目还没有设计稿") {
		t.Errorf("空项目文稿页 = %d，缺少空态引导", resEmpty.StatusCode)
	}
}

func TestProjectPDFEndpoint(t *testing.T) {
	srv, ts := newTestServer(t)
	id := newHTTPProject(t, ts)
	addNode(t, srv.store, id, Node{
		Title: "手机首页", Kind: KindScreen, System: "apple",
		HTML: "<html><body>手机首页</body></html>", Width: 390, Height: 844,
	})

	post := func() (*http.Response, []byte) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/projects/"+id+"/pdf", nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, body
	}

	// 未注入 PDF 渲染器：501 + 稳定错误码
	res, body := post()
	if res.StatusCode != http.StatusNotImplemented || !strings.Contains(string(body), "pdf_unavailable") {
		t.Fatalf("无打印机时 PDF 端点 = %d %s", res.StatusCode, body)
	}

	var gotURL string
	var gotLandscape bool
	srv.PDF = func(ctx context.Context, pageURL string, landscape bool) ([]byte, error) {
		gotURL, gotLandscape = pageURL, landscape
		return []byte("%PDF-1.4\n%fake pdf\n"), nil
	}
	res, body = post()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PDF 端点 status = %d: %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
		!strings.Contains(cd, "filename*=UTF-8''") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Errorf("PDF 响应体不是 PDF: %q", body)
	}
	if !strings.HasPrefix(gotURL, "http://") || !strings.HasSuffix(gotURL, "/projects/"+id+"/print") {
		t.Errorf("打印机收到的页面地址 = %q", gotURL)
	}
	// 390×844 竖稿 → 纵向
	if gotLandscape {
		t.Error("竖稿应将 landscape=false 传给打印机")
	}

	// 打印机报错 → 502 pdf_failed
	srv.PDF = func(context.Context, string, bool) ([]byte, error) {
		return nil, fmt.Errorf("boom")
	}
	res, body = post()
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "pdf_failed") {
		t.Errorf("打印机失败时 = %d %s", res.StatusCode, body)
	}

	// 空项目 → 409 empty_project
	empty, err := srv.store.AddProject("空项目", "")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/projects/"+empty.ID+"/pdf", nil)
	res3, _ := http.DefaultClient.Do(req)
	b3, _ := io.ReadAll(res3.Body)
	res3.Body.Close()
	if res3.StatusCode != http.StatusConflict || !strings.Contains(string(b3), "empty_project") {
		t.Errorf("空项目 PDF = %d %s", res3.StatusCode, b3)
	}

	// 只有分组框也算空
	frameOnly, err := srv.store.AddProject("分组项目", "")
	if err != nil {
		t.Fatal(err)
	}
	addNode(t, srv.store, frameOnly.ID, Node{Title: "分组框", Kind: KindFrame, Width: 900, Height: 600})
	req, _ = http.NewRequest("POST", ts.URL+"/api/projects/"+frameOnly.ID+"/pdf", nil)
	res4, _ := http.DefaultClient.Do(req)
	b4, _ := io.ReadAll(res4.Body)
	res4.Body.Close()
	if res4.StatusCode != http.StatusConflict || !strings.Contains(string(b4), "empty_project") {
		t.Errorf("只有分组框的项目 PDF = %d %s", res4.StatusCode, b4)
	}
}

// fakePrintCatalog 只实现文稿需要的 SystemSummaries；其余方法不影响本用例。
type fakePrintCatalog struct{}

func (fakePrintCatalog) SystemSummaries() any {
	return []map[string]any{
		{"id": "apple", "name": "Apple HIG", "vendor": "Apple", "primary": "#007AFF", "default_artboard": "iphone-15"},
	}
}
func (fakePrintCatalog) SystemDetail(string) (any, bool) { return nil, false }
func (fakePrintCatalog) ComposePreview(string, []string, string, string, string) (string, int, int, string, error) {
	return "", 0, 0, "", nil
}

func TestPrintRouteUsesCatalogSystems(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.Catalog = fakePrintCatalog{}
	id := newHTTPProject(t, ts)
	addNode(t, srv.store, id, Node{
		Title: "首页", Kind: KindScreen, System: "apple",
		HTML: "<html><body>首页</body></html>", Width: 390, Height: 844,
	})
	res, err := http.Get(ts.URL + "/projects/" + id + "/print")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "Apple HIG") || !strings.Contains(string(body), "#007AFF") {
		t.Error("文稿页没有使用目录里的体系名称与主色")
	}
}

// 防止误删的契约：文稿页必须允许浏览器打印（不依赖外链脚本）。
func TestPrintDocumentIsSelfContained(t *testing.T) {
	doc := PrintDocument(Project{Name: "自包含"}, []Node{{
		Title: "稿", Kind: KindComponent, Width: 420, Height: 320,
		HTML: "<html><body>ok</body></html>",
	}}, nil, time.Now())
	for _, bad := range []string{"http://", "https://", "@import", "<link"} {
		if strings.Contains(doc, bad) {
			t.Errorf("文稿不应依赖外部资源，发现 %q", bad)
		}
	}
	if !strings.Contains(doc, "<style>") {
		t.Error("文稿样式应内联")
	}
}
