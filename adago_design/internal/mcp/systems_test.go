package mcp

import (
	"strings"
	"testing"

	"adagodesign/internal/design"
)

// design_system_manage 与画布弹层共用同一套 design 管理函数：
// 列表/新增/停用/启用/内置保护/移除 走一遍 MCP 契约。
func TestDesignSystemManageTool(t *testing.T) {
	_ = design.LoadSystemSettings("")
	t.Cleanup(func() { _ = design.LoadSystemSettings("") })

	sess, d := newTestSession(t)
	const id = "mcp-probe"
	t.Cleanup(func() { _ = design.RemoveSystem(id, d.DataDir) })

	res := callTool(t, sess, "design_system_manage", map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("list 失败: %s", contentText(t, res))
	}
	if txt := contentText(t, res); !strings.Contains(txt, "个体系") || !strings.Contains(txt, "内置") {
		t.Fatalf("list 文案异常: %s", txt)
	}

	// 新增（品牌派生）
	res = callTool(t, sess, "design_system_manage", map[string]any{
		"action": "create", "id": id, "name": "MCP Probe",
		"vendor": "测试", "family": "web", "base": "volcano", "primary": "#3366CC",
	})
	if res.IsError {
		t.Fatalf("create 失败: %s", contentText(t, res))
	}
	if _, ok := design.GetSystem(id); !ok {
		t.Fatal("新增后 GetSystem 找不到")
	}

	// 部分更新：只给名称，不应清掉其它字段
	res = callTool(t, sess, "design_system_manage", map[string]any{
		"action": "update", "id": id, "name": "MCP Probe Renamed",
	})
	if res.IsError {
		t.Fatalf("update 失败: %s", contentText(t, res))
	}
	if got, _ := design.GetSystem(id); got == nil || got.Name != "MCP Probe Renamed" || got.Vendor != "测试" {
		t.Fatalf("部分更新异常: %+v", got)
	}

	// 停用后 design_systems 不再列出
	if res = callTool(t, sess, "design_system_manage", map[string]any{"action": "disable", "id": id}); res.IsError {
		t.Fatalf("disable 失败: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_systems", nil)
	if strings.Contains(contentText(t, res), id) {
		t.Fatal("停用体系仍出现在 design_systems")
	}
	// design_tokens 也应视为不存在
	res = callTool(t, sess, "design_tokens", map[string]any{"system": id})
	if !res.IsError {
		t.Fatal("停用体系仍可查 Token")
	}

	// 启用回来
	if res = callTool(t, sess, "design_system_manage", map[string]any{"action": "enable", "id": id}); res.IsError {
		t.Fatalf("enable 失败: %s", contentText(t, res))
	}
	res = callTool(t, sess, "design_systems", nil)
	if !strings.Contains(contentText(t, res), id) {
		t.Fatal("启用后 design_systems 未恢复")
	}

	// 内置体系不可删
	res = callTool(t, sess, "design_system_manage", map[string]any{"action": "remove", "id": "apple"})
	if !res.IsError || !strings.Contains(contentText(t, res), "system_builtin_protected") {
		t.Fatalf("内置删除应报 protected: %s", contentText(t, res))
	}

	// 移除自定义体系
	if res = callTool(t, sess, "design_system_manage", map[string]any{"action": "remove", "id": id}); res.IsError {
		t.Fatalf("remove 失败: %s", contentText(t, res))
	}
	if _, ok := design.GetSystem(id); ok {
		t.Fatal("移除后仍存在")
	}

	// 参数校验
	res = callTool(t, sess, "design_system_manage", map[string]any{"action": "nope"})
	if !res.IsError {
		t.Fatal("未知 action 应报错")
	}
}
