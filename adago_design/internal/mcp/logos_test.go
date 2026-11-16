package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"adagodesign/internal/design"
)

func TestDesignLogoTool(t *testing.T) {
	_, _ = design.LoadLogos("")
	_ = design.LoadIconSettings("")
	t.Cleanup(func() {
		_, _ = design.LoadLogos("")
		_ = design.LoadIconSettings("")
	})

	sess, d := newTestSession(t)
	const id = "mcp-logo-probe"
	t.Cleanup(func() { _ = design.RemoveLogo(id, d.DataDir) })

	res := callTool(t, sess, "design_logo", map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("list 失败: %s", contentText(t, res))
	}

	// preview：flat 字段生成；三变体 + 规范
	res = callTool(t, sess, "design_logo", map[string]any{
		"action": "preview", "brand": "MCP Cloud", "tagline": "design system",
		"markIcon": "star", "layout": "horizontal", "font": "inter", "weight": 700,
		"color": "#0B5FFF", "background": "#FBF8F1",
	})
	if res.IsError {
		t.Fatalf("preview 失败: %s", contentText(t, res))
	}
	var out logoOut
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("preview 结构化输出解析失败: %v", err)
	}
	if out.Doc == nil || out.Doc.Brand != "MCP Cloud" || out.SVG == "" {
		t.Fatalf("preview 输出异常: %+v", out)
	}
	if len(out.Variants) != 3 || out.Spec == nil || out.Spec.MinWidth <= 0 {
		t.Fatalf("preview 变体/规范异常: %+v", out)
	}
	if !strings.Contains(contentText(t, res), "最小宽") {
		t.Fatalf("preview 文案异常: %s", contentText(t, res))
	}

	// create：落库 + 自动摆上画布
	pid := testProject(t, d)
	before := len(d.Store.List())
	res = callTool(t, sess, "design_logo", map[string]any{
		"action": "create", "id": id, "name": "MCP Cloud", "system": "ant",
		"brand": "MCP Cloud", "tagline": "design system", "markIcon": "star",
		"layout": "stacked", "font": "inter", "weight": 700, "color": "#0B5FFF",
		"project": pid, "addToCanvas": true,
	})
	if res.IsError {
		t.Fatalf("create 失败: %s", contentText(t, res))
	}
	out = logoOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Logo == nil || out.Logo.ID != id || out.NodeID == "" {
		t.Fatalf("create 输出异常: %+v", out)
	}
	if len(d.Store.List()) != before+1 {
		t.Fatalf("画布节点数应 +1: %d", len(d.Store.List()))
	}
	if got, ok := design.GetLogo(id); !ok || got.Doc.Brand != "MCP Cloud" {
		t.Fatalf("Logo 未落库: %+v", got)
	}

	// get / spec
	res = callTool(t, sess, "design_logo", map[string]any{"action": "get", "id": id})
	if res.IsError || !strings.Contains(contentText(t, res), "MCP Cloud") {
		t.Fatalf("get 失败: %s", contentText(t, res))
	}
	out = logoOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Variants) != 3 || out.Spec == nil {
		t.Fatalf("get 输出异常: %+v", out)
	}
	res = callTool(t, sess, "design_logo", map[string]any{"action": "spec", "id": id})
	if res.IsError || !strings.Contains(contentText(t, res), "安全空间") {
		t.Fatalf("spec 失败: %s", contentText(t, res))
	}

	// update：flat 覆盖
	res = callTool(t, sess, "design_logo", map[string]any{
		"action": "update", "id": id, "layout": "horizontal", "tagline": "cloud platform",
		"markIcon": "globe",
	})
	if res.IsError {
		t.Fatalf("update 失败: %s", contentText(t, res))
	}
	if got, _ := design.GetLogo(id); got.Doc.Layout != "horizontal" || got.Doc.Tagline != "cloud platform" {
		t.Fatalf("update 未生效: %+v", got.Doc)
	}

	// export
	res = callTool(t, sess, "design_logo", map[string]any{"action": "export", "id": id, "variant": "inverse"})
	if res.IsError {
		t.Fatalf("export 失败: %s", contentText(t, res))
	}
	out = logoOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ExportPath == "" || !strings.Contains(out.SVG, "#0B5FFF") {
		t.Fatalf("export 输出异常: %+v", out)
	}

	// place 已有 Logo
	res = callTool(t, sess, "design_logo", map[string]any{
		"action": "place", "id": id, "project": pid, "variant": "mono",
	})
	if res.IsError {
		t.Fatalf("place 失败: %s", contentText(t, res))
	}

	// 非法：未知 action / 空 doc / 缺 id
	res = callTool(t, sess, "design_logo", map[string]any{"action": "nope"})
	if !res.IsError {
		t.Fatal("未知 action 应报错")
	}
	res = callTool(t, sess, "design_logo", map[string]any{"action": "get"})
	if !res.IsError || !strings.Contains(contentText(t, res), "bad_request") {
		t.Fatalf("缺 id 应报 bad_request: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_logo", map[string]any{"action": "preview", "doc": map[string]any{"version": 1}})
	if !res.IsError || !strings.Contains(contentText(t, res), "invalid_logo") {
		t.Fatalf("空设计稿应报 invalid_logo: %s", contentText(t, res))
	}

	// remove
	if res = callTool(t, sess, "design_logo", map[string]any{"action": "remove", "id": id}); res.IsError {
		t.Fatalf("remove 失败: %s", contentText(t, res))
	}
	if _, ok := design.GetLogo(id); ok {
		t.Fatal("移除后仍存在")
	}
}
