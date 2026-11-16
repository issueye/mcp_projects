package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"adagodesign/internal/design"
)

func mcpDesignDoc() map[string]any {
	return map[string]any{
		"grid": 24, "strokeWidth": 2, "cap": "round", "join": "round",
		"shapes": []any{
			map[string]any{"type": "rect", "x": 4, "y": 4, "width": 16, "height": 16, "radius": 4},
			map[string]any{"type": "circle", "cx": 12, "cy": 12, "r": 3, "fill": true, "stroke": false},
			map[string]any{"type": "line", "x1": 8, "y1": 12, "x2": 16, "y2": 12},
		},
	}
}

// design_icon_manage 与画布弹层共用同一套 design 函数：
// 新增 / 检索 / 停用 / 启用 / 内置保护 / 移除 走一遍 MCP 契约。
// design_icon_manage 的 design 动作：几何文档 → 确定性 SVG；已有图标 / SVG 可反推；
// create/update 带 design 落库后仍能取回设计稿。
func TestDesignIconDesignTool(t *testing.T) {
	_, _ = design.LoadCustomIcons("")
	_ = design.LoadIconSettings("")
	t.Cleanup(func() {
		_, _ = design.LoadCustomIcons("")
		_ = design.LoadIconSettings("")
	})

	sess, d := newTestSession(t)
	const name = "mcp-design-probe"
	t.Cleanup(func() { _ = design.RemoveIcon(name, d.DataDir) })

	res := callTool(t, sess, "design_icon_manage", map[string]any{"action": "design", "design": mcpDesignDoc()})
	if res.IsError {
		t.Fatalf("design 失败: %s", contentText(t, res))
	}
	var out iconManageOut
	if raw, _ := json.Marshal(res.StructuredContent); len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("结构化输出解析失败: %v (%s)", err, raw)
		}
	}
	if out.SVG == "" || !strings.Contains(out.SVG, "<rect") || !strings.Contains(out.SVG, "stroke-width=\"2\"") {
		t.Fatalf("design 未返回预期 SVG: %s", out.SVG)
	}
	if out.Design == nil || len(out.Design.Shapes) != 3 {
		t.Fatalf("design 未返回规范化文档: %+v", out.Design)
	}
	if txt := contentText(t, res); !strings.Contains(txt, "3 个图形") {
		t.Fatalf("design 文案异常: %s", txt)
	}

	// 从 SVG 反推
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "design",
		"svg":    `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/></svg>`,
	})
	if res.IsError {
		t.Fatalf("design 反推失败: %s", contentText(t, res))
	}
	raw, _ := json.Marshal(res.StructuredContent)
	out = iconManageOut{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Imported || out.Design == nil || len(out.Design.Shapes) != 1 {
		t.Fatalf("反推结果异常: %+v", out)
	}

	// 打开内置图标：只读，可另存
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "design", "name": "close"})
	if res.IsError {
		t.Fatalf("打开内置图标失败: %s", contentText(t, res))
	}
	if strings.Contains(contentText(t, res), "只读") == false {
		t.Fatalf("内置图标文案应提示只读: %s", contentText(t, res))
	}
	out = iconManageOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Builtin || out.Editable {
		t.Fatalf("内置图标标记异常: %+v", out)
	}

	// 缺少来源参数
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "design"})
	if !res.IsError || !strings.Contains(contentText(t, res), "bad_request") {
		t.Fatalf("design 缺参数应报 bad_request: %s", contentText(t, res))
	}

	// 带 design 新增，再取回
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "create", "name": name, "group": "action", "tags": []string{"design-probe"},
		"design": mcpDesignDoc(),
	})
	if res.IsError {
		t.Fatalf("create design 失败: %s", contentText(t, res))
	}
	if got, _ := design.GetIcon(name); !got.Designed {
		t.Fatal("create 后 Designed 应为 true")
	}
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "design", "name": name})
	if res.IsError {
		t.Fatalf("取回设计稿失败: %s", contentText(t, res))
	}
	out = iconManageOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Imported || out.Design == nil || len(out.Design.Shapes) != 3 {
		t.Fatalf("取回的设计稿异常: %+v", out)
	}

	// 用设计稿更新
	next := map[string]any{
		"grid": 24, "strokeWidth": 2,
		"shapes": []any{map[string]any{"type": "circle", "cx": 12, "cy": 12, "r": 7}},
	}
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "update", "name": name, "design": next,
	})
	if res.IsError {
		t.Fatalf("update design 失败: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "design", "name": name})
	out = iconManageOut{}
	raw, _ = json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Design == nil || len(out.Design.Shapes) != 1 {
		t.Fatalf("update 后设计稿异常: %+v", out)
	}

	// 非法设计稿：空 shapes
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "create", "name": "mcp-design-bad",
		"design": map[string]any{"shapes": []any{}},
	})
	if !res.IsError || !strings.Contains(contentText(t, res), "invalid_icon") {
		t.Fatalf("空设计稿应报 invalid_icon: %s", contentText(t, res))
	}
}

func TestDesignIconManageTool(t *testing.T) {
	_, _ = design.LoadCustomIcons("")
	_ = design.LoadIconSettings("")
	t.Cleanup(func() {
		_, _ = design.LoadCustomIcons("")
		_ = design.LoadIconSettings("")
	})

	sess, d := newTestSession(t)
	const name = "mcp-icon-probe"
	t.Cleanup(func() { _ = design.RemoveIcon(name, d.DataDir) })

	res := callTool(t, sess, "design_icon_manage", map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("list 失败: %s", contentText(t, res))
	}
	if txt := contentText(t, res); !strings.Contains(txt, "个图标") {
		t.Fatalf("list 文案异常: %s", txt)
	}

	svg := `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor"/></svg>`
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "create", "name": name, "group": "status",
		"tags": []string{"mcp-probe"}, "svg": svg,
	})
	if res.IsError {
		t.Fatalf("create 失败: %s", contentText(t, res))
	}
	if _, ok := design.GetIcon(name); !ok {
		t.Fatal("新增后 GetIcon 找不到")
	}

	// design_icons 应能按别名检索到自定义图标
	res = callTool(t, sess, "design_icons", map[string]any{"query": "mcp-probe"})
	if res.IsError || !strings.Contains(contentText(t, res), name) {
		t.Fatalf("design_icons 未检索到自定义图标: %s", contentText(t, res))
	}

	// 部分更新：只改分组
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "update", "name": name, "group": "nav",
	})
	if res.IsError {
		t.Fatalf("update 失败: %s", contentText(t, res))
	}
	if got, _ := design.GetIcon(name); got.Group != "nav" || len(got.Tags) != 1 {
		t.Fatalf("部分更新异常: %+v", got)
	}

	// 停用后 design_icons 不再返回
	if res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "disable", "name": name}); res.IsError {
		t.Fatalf("disable 失败: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_icons", map[string]any{"query": "mcp-probe"})
	if strings.Contains(contentText(t, res), name) {
		t.Fatal("停用图标仍出现在 design_icons")
	}
	if res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "enable", "name": name}); res.IsError {
		t.Fatalf("enable 失败: %s", contentText(t, res))
	}

	// 内置图标不可删 / SVG 只读
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "remove", "name": "close"})
	if !res.IsError || !strings.Contains(contentText(t, res), "icon_builtin_protected") {
		t.Fatalf("内置删除应报 protected: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "update", "name": "close", "svg": `<path d="M0 0"/>`,
	})
	if !res.IsError || !strings.Contains(contentText(t, res), "icon_builtin_protected") {
		t.Fatalf("内置 SVG 编辑应报 protected: %s", contentText(t, res))
	}

	// 非法 SVG
	res = callTool(t, sess, "design_icon_manage", map[string]any{
		"action": "create", "name": "mcp-bad-icon", "svg": `<svg><script>alert(1)</script><path d="M0 0"/></svg>`,
	})
	if !res.IsError || !strings.Contains(contentText(t, res), "invalid_icon") {
		t.Fatalf("非法 SVG 应报 invalid_icon: %s", contentText(t, res))
	}

	// 移除自定义图标
	if res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "remove", "name": name}); res.IsError {
		t.Fatalf("remove 失败: %s", contentText(t, res))
	}
	if _, ok := design.GetIcon(name); ok {
		t.Fatal("移除后仍存在")
	}

	// 未知 action
	res = callTool(t, sess, "design_icon_manage", map[string]any{"action": "nope"})
	if !res.IsError {
		t.Fatal("未知 action 应报错")
	}
}
