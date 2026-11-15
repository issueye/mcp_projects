package mcpserver

import (
	"strings"
	"testing"

	"adagodesign/internal/design"
)

// design_icon_manage 与画布弹层共用同一套 design 函数：
// 新增 / 检索 / 停用 / 启用 / 内置保护 / 移除 走一遍 MCP 契约。
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
