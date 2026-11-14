package canvas

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 项目导出 / 导入：格式契约、往返保真、同名去重、撤销与 HTTP 入口。

func seedProject(t *testing.T, s *Store) (string, Node, Node) {
	t.Helper()
	pid := newTestProject(t, s)
	if _, err := s.SetProjectCanvas(pid, &ProjectCanvas{Ratio: "16:9"}); err != nil {
		t.Fatalf("SetProjectCanvas: %v", err)
	}
	a := addNode(t, s, pid, newTestNode("首页"))
	b := addNode(t, s, pid, newTestNode("详情"))
	if _, err := s.Update(a.ID, func(n *Node) { n.X, n.Y, n.Artboard = 120, 240, "desktop-app" }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := s.AddEdge(a.ID, b.ID, "点击进入"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	return pid, a, b
}

func TestExportProjectBundle(t *testing.T) {
	s, _ := OpenStore("")
	pid, a, _ := seedProject(t, s)

	bundle, err := s.ExportProject(pid)
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	if bundle.Format != ProjectBundleFormat || bundle.Version != ProjectBundleVersion {
		t.Fatalf("格式标识异常: %s v%d", bundle.Format, bundle.Version)
	}
	if bundle.Project.ID != pid || bundle.Project.Name != "测试项目" {
		t.Fatalf("项目字段异常: %+v", bundle.Project)
	}
	if bundle.Project.Canvas == nil || bundle.Project.Canvas.Width != 1920 {
		t.Fatalf("画布版式未随导出: %+v", bundle.Project.Canvas)
	}
	if len(bundle.Nodes) != 2 || len(bundle.Edges) != 1 {
		t.Fatalf("节点/连线数量异常: nodes=%d edges=%d", len(bundle.Nodes), len(bundle.Edges))
	}
	// 正文必须真的在文件里（索引剥 HTML 是列表接口的行为，导出不能跟着剥）
	found := false
	for _, n := range bundle.Nodes {
		if n.ID == a.ID {
			found = true
			if !strings.Contains(n.HTML, "<p>首页</p>") {
				t.Fatalf("导出应携带正文: %q", n.HTML)
			}
			if n.X != 120 || n.Y != 240 || n.Artboard != "desktop-app" {
				t.Fatalf("坐标/画板未保真: %+v", n)
			}
		}
	}
	if !found {
		t.Fatal("导出缺少目标节点")
	}
	// 顺序稳定：同一份数据导出两次字节应一致（便于 diff 与快照）
	again, _ := s.ExportProject(pid)
	if len(again.Nodes) != len(bundle.Nodes) || again.Nodes[0].ID != bundle.Nodes[0].ID {
		t.Fatal("导出顺序应稳定")
	}

	// 未知项目
	if _, err := s.ExportProject("p-nope"); err != ErrProjectNotFound {
		t.Fatalf("未知项目应 ErrProjectNotFound，得到 %v", err)
	}
}

func TestImportProjectRoundtrip(t *testing.T) {
	src, _ := OpenStore("")
	pid, _, _ := seedProject(t, src)
	bundle, err := src.ExportProject(pid)
	if err != nil {
		t.Fatal(err)
	}

	// 导入进另一个库（真实场景：换机器 / 交给同事）
	dir := t.TempDir()
	dst, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sm, err := dst.ImportProject(bundle)
	if err != nil {
		t.Fatalf("ImportProject: %v", err)
	}
	if sm.NodeCount != 2 || sm.FrameCount != 0 {
		t.Fatalf("导入计数异常: %+v", sm)
	}
	if sm.Canvas == nil || sm.Canvas.Height != 1080 {
		t.Fatalf("画布版式未还原: %+v", sm.Canvas)
	}

	nodes := dst.ListByProject(sm.ID)
	if len(nodes) != 2 {
		t.Fatalf("导入节点数 = %d", len(nodes))
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		if ids[n.ID] {
			t.Fatalf("导入后节点 ID 重复: %s", n.ID)
		}
		ids[n.ID] = true
		if n.ProjectID != sm.ID {
			t.Fatalf("节点未归属新项目: %+v", n)
		}
		if !strings.Contains(n.HTML, "<p>") {
			t.Fatalf("正文丢失: %q", n.Title)
		}
	}
	edges := dst.ListEdges()
	if len(edges) != 1 || edges[0].Label != "点击进入" {
		t.Fatalf("连线未还原: %+v", edges)
	}
	if !ids[edges[0].From] || !ids[edges[0].To] {
		t.Fatalf("连线端点未重映射到新节点: %+v", edges[0])
	}

	// 落盘可重开
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(s2.ListByProject(sm.ID)) != 2 {
		t.Fatal("重开后导入的节点应还在")
	}
}

func TestImportProjectDedupNameAndUndo(t *testing.T) {
	s, _ := OpenStore("")
	pid, _, _ := seedProject(t, s)
	bundle, err := s.ExportProject(pid)
	if err != nil {
		t.Fatal(err)
	}

	// 同名导入 → 自动加后缀，原项目不受影响
	sm, err := s.ImportProject(bundle)
	if err != nil {
		t.Fatalf("ImportProject: %v", err)
	}
	if sm.ID == pid {
		t.Fatal("导入应新建项目，不能复用已有 ID")
	}
	if sm.Name == "测试项目" || !strings.Contains(sm.Name, "导入") {
		t.Fatalf("同名项目应加「导入」后缀: %q", sm.Name)
	}
	if len(s.ListProjects()) != 2 {
		t.Fatalf("项目数 = %d，期望 2", len(s.ListProjects()))
	}

	// 节点 ID 不冲突：原项目仍是 2 个节点
	if got := len(s.ListByProject(pid)); got != 2 {
		t.Fatalf("原项目节点数被改动: %d", got)
	}

	// 一次撤销整体退回
	desc, ok := s.Undo()
	if !ok || !strings.Contains(desc, "导入") {
		t.Fatalf("导入应可撤销，得到 %q %v", desc, ok)
	}
	if len(s.ListProjects()) != 1 {
		t.Fatalf("撤销后项目数 = %d，期望 1", len(s.ListProjects()))
	}
	if got := len(s.List()); got != 2 {
		t.Fatalf("撤销后节点数 = %d，期望 2", got)
	}
	if _, ok := s.Redo(); !ok {
		t.Fatal("导入应可重做")
	}
	if len(s.ListProjects()) != 2 {
		t.Fatalf("重做后项目数 = %d，期望 2", len(s.ListProjects()))
	}
}

func TestImportRejectsInvalidBundles(t *testing.T) {
	s, _ := OpenStore("")
	ok := ProjectBundle{Project: Project{Name: "合法"}, Nodes: []Node{{Title: "n", HTML: "<html></html>"}}}

	cases := []struct {
		name   string
		bundle ProjectBundle
		want   error
	}{
		{"未知格式", ProjectBundle{Format: "other", Project: Project{Name: "x"}}, ErrBundleFormat},
		{"过高版本", ProjectBundle{Format: ProjectBundleFormat, Version: 99, Project: Project{Name: "x"}}, ErrBundleVersion},
		{"空名", ProjectBundle{Project: Project{Name: "   "}}, ErrBundleInvalid},
		{"非法 kind", ProjectBundle{Project: Project{Name: "x"}, Nodes: []Node{{Kind: "bogus"}}}, ErrBundleInvalid},
		{"非法画布", ProjectBundle{Project: Project{Name: "x", Canvas: &ProjectCanvas{Ratio: "21:9"}}}, ErrBundleInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.ImportProject(tc.bundle); err == nil {
				t.Fatalf("应拒绝：%+v", tc.bundle)
			} else if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("错误 = %v，期望 %v", err, tc.want)
			}
		})
	}
	if len(s.ListProjects()) != 0 {
		t.Fatalf("非法导入不应落库: %+v", s.ListProjects())
	}

	if _, err := s.ImportProjects(nil); err != ErrBundleEmpty {
		t.Fatalf("空列表应 ErrBundleEmpty，得到 %v", err)
	}
	if _, err := s.ImportProject(ok); err != nil {
		t.Fatalf("合法 bundle 应通过: %v", err)
	}
}

func TestExportWorkspaceRoundtrip(t *testing.T) {
	s, _ := OpenStore("")
	seedProject(t, s)
	p2, err := s.AddProject("第二个项目", "备注")
	if err != nil {
		t.Fatal(err)
	}
	addNode(t, s, p2.ID, newTestNode("页面"))

	ws, err := s.ExportWorkspace()
	if err != nil {
		t.Fatalf("ExportWorkspace: %v", err)
	}
	if ws.Format != WorkspaceBundleFormat || len(ws.Projects) != 2 {
		t.Fatalf("工作区导出异常: %s %d", ws.Format, len(ws.Projects))
	}
	nodeTotal := 0
	for _, b := range ws.Projects {
		nodeTotal += len(b.Nodes)
	}
	if nodeTotal != 3 {
		t.Fatalf("工作区节点总数 = %d，期望 3", nodeTotal)
	}

	dst, _ := OpenStore("")
	sms, err := dst.ImportProjects(ws.Projects)
	if err != nil {
		t.Fatalf("ImportProjects: %v", err)
	}
	if len(sms) != 2 || len(dst.List()) != 3 {
		t.Fatalf("工作区导入异常: projects=%d nodes=%d", len(sms), len(dst.List()))
	}
	var second *ProjectSummary
	for i := range dst.ListProjects() {
		if dst.ListProjects()[i].Name == "第二个项目" {
			second = &dst.ListProjects()[i]
		}
	}
	if second == nil {
		t.Fatalf("第二个项目未还原: %+v", dst.ListProjects())
	}
	if second.Note != "备注" || second.Canvas != nil || second.NodeCount != 1 {
		t.Fatalf("项目备注/画布未还原: %+v", second)
	}
}

func TestProjectIOHTTP(t *testing.T) {
	srv, ts := newTestServer(t)
	pid := newHTTPProject(t, ts)
	res0(t, ts, "POST", "/api/nodes", map[string]any{
		"project": pid, "title": "卡片", "kind": "screen",
		"html": "<html><body>hi</body></html>", "width": 800, "height": 600,
	})

	// 单项目导出：带 Content-Disposition，内容是合法 bundle
	res, err := http.Get(ts.URL + "/api/projects/" + pid + "/export")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d", res.StatusCode)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
		!strings.Contains(cd, ".json") {
		t.Fatalf("导出应作为附件下载: %q", cd)
	}
	var bundle ProjectBundle
	if err := json.NewDecoder(res.Body).Decode(&bundle); err != nil {
		t.Fatalf("导出不是合法 bundle: %v", err)
	}
	if bundle.Project.ID != pid || len(bundle.Nodes) != 1 {
		t.Fatalf("导出内容异常: %+v", bundle)
	}

	// 导入：新建项目，原项目不变
	res2, data := doJSON(t, "POST", ts.URL+"/api/import", bundle)
	if res2.StatusCode != http.StatusCreated {
		t.Fatalf("import status = %d: %v", res2.StatusCode, data)
	}
	if data["imported"] != float64(1) || data["node_count"] != float64(1) {
		t.Fatalf("导入回包异常: %v", data)
	}
	if got := len(srv.store.ListProjects()); got != 2 {
		t.Fatalf("导入后项目数 = %d", got)
	}

	// 工作区导出 + 导入
	res3, err := http.Get(ts.URL + "/api/export")
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	var ws WorkspaceBundle
	if err := json.NewDecoder(res3.Body).Decode(&ws); err != nil {
		t.Fatalf("工作区导出非法: %v", err)
	}
	if len(ws.Projects) != 2 {
		t.Fatalf("工作区项目数 = %d", len(ws.Projects))
	}
	res4, data4 := doJSON(t, "POST", ts.URL+"/api/import", ws)
	if res4.StatusCode != http.StatusCreated {
		t.Fatalf("workspace import status = %d: %v", res4.StatusCode, data4)
	}
	if got := len(srv.store.ListProjects()); got != 4 {
		t.Fatalf("工作区导入后项目数 = %d", got)
	}

	// 错误分支：未知格式 / 缺字段 / 未知项目导出
	if got := statusCode(t, ts, "POST", "/api/import", map[string]any{"format": "x"}); got != http.StatusBadRequest {
		t.Fatalf("未知格式应 400, got %d", got)
	}
	if got := statusCode(t, ts, "GET", "/api/projects/p-nope/export", nil); got != http.StatusNotFound {
		t.Fatalf("未知项目导出应 404, got %d", got)
	}
}

// 导出/导入是「能直接看到设计内容」的前置条件：正文不能只留引用。
func TestExportBundleCarriesNodeFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	pid := newTestProject(t, s)
	n := addNode(t, s, pid, newTestNode("唯一内容标记"))

	// 正文确实落在 nodes/ 分文件里（v4 格式）
	if _, err := os.Stat(filepath.Join(dir, "nodes", n.ID+".html")); err != nil {
		t.Fatalf("正文文件应存在: %v", err)
	}
	bundle, err := s.ExportProject(pid)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(bundle)
	if !strings.Contains(string(raw), "唯一内容标记") {
		t.Fatal("导出的 JSON 必须内联正文，否则换机器后只剩空卡片")
	}
}

// 前端入口契约：菜单/项目管理必须有导出导入，且隐藏文件选择器存在。
func TestIndexHasProjectIOEntry(t *testing.T) {
	html := readStatic(t, "index.html")
	for _, want := range []string{
		`data-menu="export"`, `data-menu="export-all"`, `data-menu="import"`, `id="import-file"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 缺少导入导出入口 %s", want)
		}
	}
	js := readStatic(t, "js/project-io.js")
	for _, want := range []string{"exportProject", "exportAllProjects", "importProjectFile"} {
		if !strings.Contains(js, want) {
			t.Errorf("project-io.js 缺少 %s", want)
		}
	}
	if !strings.Contains(readStatic(t, "js/dock.js"), "importProjectFile") {
		t.Error("dock.js 应把导入入口接到 project-io.js")
	}
}

// 画布「随内容自适应」与「减少文本选择」的前端契约。
func TestCanvasContentAdaptiveContract(t *testing.T) {
	css := readStatic(t, "canvas.css")
	for _, want := range []string{"user-select:none", "user-select:text"} {
		if !strings.Contains(css, want) {
			t.Errorf("canvas.css 缺少 %s（画布不应随手拖出文本选区）", want)
		}
	}
	view := readStatic(t, "js/view.js")
	if !strings.Contains(view, "function contentBounds(") {
		t.Error("view.js 需要 contentBounds()：纸面尺寸随内容包围盒推导")
	}
	if !strings.Contains(view, `ratioText = "随内容"`) {
		t.Error("view.js 的自由画布应标注为「随内容」")
	}
	if !strings.Contains(view, "paperEl.style.left") {
		t.Error("纸面需要跟随内容起点移动，而不是固定在原点")
	}
	if !strings.Contains(view, "function focusContent(") {
		t.Error("view.js 需要 focusContent()：首屏把作用域内容整体放进视野")
	}
	if !strings.Contains(view, "function fitNode(") {
		t.Error("view.js 需要 fitNode()：单张稿必须能一键看全")
	}
	if strings.Contains(view, "--ui-min-read-scale") {
		t.Error("view.js 不应再有「可读下限」：默认视角必须让内容全部可见（用户反馈被裁切）")
	}
	nodes := readStatic(t, "js/nodes.js")
	if !strings.Contains(nodes, "renderCanvas()") {
		t.Error("nodes.js 在增删/拖动/测高后应刷新纸面（renderCanvas）")
	}
	if !strings.Contains(nodes, "function clearTextSelection(") {
		t.Error("nodes.js 需要在画布上落指针时清掉遗留选区")
	}
	if !strings.Contains(nodes, "node__zoom") || !strings.Contains(nodes, "enterSolo(n.id") {
		t.Error("nodes.js 的卡片标题栏需要「单独看这张稿」按钮（整张可见）")
	}
	sync := readStatic(t, "js/sync.js")
	if !strings.Contains(sync, "renderCanvas") {
		t.Error("sync.js 轮询合并后应刷新纸面，Agent 新增节点时长度才能跟上")
	}
	if !strings.Contains(sync, "fallbackToAllScope") {
		t.Error("sync.js 需要在当前项目消失时回落到「全部项目」（撤销导入会让作用域悬空）")
	}
	main := readStatic(t, "js/main.js")
	if !strings.Contains(main, "focusContent()") {
		t.Error("main.js 首屏应使用 focusContent()")
	}
	if !strings.Contains(main, "ensureGroupLayout()") {
		t.Error("main.js 首屏应把相互遮挡的项目分组分开")
	}
}

// 分组错开（/api/projects/separate）：只在重叠时移动，且保留组内排版。
func TestSeparateGroupsHTTP(t *testing.T) {
	_, ts := newTestServer(t)
	mkProject := func(name string) string {
		res, data := doJSON(t, "POST", ts.URL+"/api/projects", map[string]any{"name": name})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("建项目 status = %d: %v", res.StatusCode, data)
		}
		return data["project"].(map[string]any)["id"].(string)
	}
	p1 := mkProject("分组一")
	p2 := mkProject("分组二")
	add := func(pid, title string, x, y float64) {
		res, data := doJSON(t, "POST", ts.URL+"/api/nodes", map[string]any{
			"project": pid, "html": "<html></html>", "title": title,
			"width": 400, "height": 300, "x": x, "y": y,
		})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("add node status = %d: %v", res.StatusCode, data)
		}
	}
	// 两个项目都摆在同一起点：真实场景里不同项目各自从原点铺开
	add(p1, "a", 10, 10)
	add(p2, "b", 10, 10)

	before := res0(t, ts, "GET", "/api/nodes", nil)["nodes"].([]any)
	yOf := func(list []any, title string) float64 {
		for _, raw := range list {
			m := raw.(map[string]any)
			if m["title"] == title {
				return m["y"].(float64)
			}
		}
		t.Fatalf("找不到节点 %s", title)
		return 0
	}
	y0 := yOf(before, "b")

	data := res0(t, ts, "POST", "/api/projects/separate", map[string]any{})
	if data["moved"] != float64(1) {
		t.Fatalf("moved = %v，期望 1", data["moved"])
	}
	after := res0(t, ts, "GET", "/api/nodes", nil)["nodes"].([]any)
	if yOf(after, "b") <= y0 {
		t.Fatalf("第二组应下移：b.y %v -> %v", y0, yOf(after, "b"))
	}
	if y := yOf(after, "a"); y != 10 {
		t.Fatalf("第一组不应移动：a.y = %v", y)
	}
	// 幂等
	if again := res0(t, ts, "POST", "/api/projects/separate", map[string]any{}); again["moved"] != float64(0) {
		t.Fatalf("二次移动 = %v，期望 0", again["moved"])
	}
}
