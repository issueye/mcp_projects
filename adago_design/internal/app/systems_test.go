package app

import (
	"errors"
	"strings"
	"testing"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

func TestCatalogSystemAdmin(t *testing.T) {
	_ = design.LoadSystemSettings("")
	t.Cleanup(func() { _ = design.LoadSystemSettings("") })

	dir := t.TempDir()
	c := Catalog{DataDir: dir}
	t.Cleanup(func() {
		_, _ = c.ResetSystem("xiaomi")
		_ = c.RemoveSystem("app-probe")
	})

	// 管理面列表包含内置与自定义
	all, ok := c.SystemSummariesAll().([]map[string]any)
	if !ok || len(all) < 8 {
		t.Fatalf("SystemSummariesAll 异常: %T len=%d", c.SystemSummariesAll(), len(all))
	}

	// 停用内置体系：目录摘要里消失，管理列表里 disabled=true
	if _, err := c.SetSystemDisabled("xiaomi", true); err != nil {
		t.Fatal(err)
	}
	enabled, _ := c.SystemSummaries().([]map[string]any)
	for _, m := range enabled {
		if m["id"] == "xiaomi" {
			t.Fatal("停用体系不应出现在 SystemSummaries")
		}
	}
	all, _ = c.SystemSummariesAll().([]map[string]any)
	var xiaomi map[string]any
	for _, m := range all {
		if m["id"] == "xiaomi" {
			xiaomi = m
		}
	}
	if xiaomi == nil || xiaomi["disabled"] != true || xiaomi["builtin"] != true {
		t.Fatalf("小米体系状态异常: %+v", xiaomi)
	}
	if _, err := c.ResetSystem("xiaomi"); err != nil {
		t.Fatal(err)
	}

	// 内置体系不可删，错误码稳定
	var ae *canvas.AdminError
	err := c.RemoveSystem("apple")
	if !errors.As(err, &ae) || ae.Code != "system_builtin_protected" {
		t.Fatalf("内置删除错误 = %v", err)
	}

	// 新增自定义体系（品牌派生）
	created, err := c.CreateSystem(canvas.SystemInput{
		ID: "app-probe", Name: "App Probe", Vendor: "测试", Family: "web",
		Description: "管理集成测试", Base: "volcano", Primary: "#C8412B",
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := created.(map[string]any)
	if m["id"] != "app-probe" || m["builtin"] != false {
		t.Fatalf("新增摘要异常: %+v", m)
	}

	// 编辑显示信息
	updated, err := c.UpdateSystem("app-probe", canvas.SystemInput{Name: "App Probe 2", Vendor: "测试 2"})
	if err != nil {
		t.Fatal(err)
	}
	if mm := updated.(map[string]any); mm["name"] != "App Probe 2" {
		t.Fatalf("编辑未生效: %+v", mm)
	}

	// 完整 JSON 可读
	raw, err := c.SystemJSON("app-probe")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"id": "app-probe"`) && !strings.Contains(raw, `"id":"app-probe"`) {
		t.Fatalf("SystemJSON 缺 id: %s", raw[:min(len(raw), 120)])
	}

	// 自定义体系不支持 reset（只对内置），错误码 invalid_system
	if _, err := c.ResetSystem("app-probe"); !errors.As(err, &ae) || ae.Code != "invalid_system" {
		t.Fatalf("自定义 reset 错误 = %v", err)
	}

	// 移除自定义体系
	if err := c.RemoveSystem("app-probe"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveSystem("app-probe"); !errors.As(err, &ae) || ae.Code != "unknown_system" {
		t.Fatalf("重复移除错误 = %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
