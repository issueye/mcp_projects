package app

import (
	"errors"
	"strings"
	"testing"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

// Catalog 的图标管理实现：新增 / 详情 / 编辑 / 停用 / 内置保护 / 移除。
func TestCatalogIconLifecycle(t *testing.T) {
	_, _ = design.LoadCustomIcons("")
	_ = design.LoadIconSettings("")
	t.Cleanup(func() {
		_, _ = design.LoadCustomIcons("")
		_ = design.LoadIconSettings("")
	})

	c := Catalog{DataDir: t.TempDir()}
	svg := `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor"/></svg>`

	out, err := c.CreateIcon(canvas.IconInput{
		Name: "app-probe", Group: "status", Tags: []string{"app-probe"}, SVG: svg,
	})
	if err != nil {
		t.Fatal(err)
	}
	summary := out.(map[string]any)
	if summary["name"] != "app-probe" || summary["builtin"] != false || summary["disabled"] != false {
		t.Fatalf("创建摘要异常: %v", summary)
	}
	if usage, _ := summary["usage"].(string); !strings.Contains(usage, `#ds-ic-app-probe`) {
		t.Fatalf("usage 异常: %v", summary["usage"])
	}

	detail, err := c.IconDetail("app-probe")
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := detail.(map[string]any)["source"].(string); !strings.Contains(src, `viewBox="0 0 24 24"`) {
		t.Fatalf("详情 source 异常: %v", detail)
	}

	if _, err := c.IconDetail("nope"); err == nil {
		t.Fatal("未知图标应报错")
	} else {
		var ae *canvas.AdminError
		if !errors.As(err, &ae) || ae.Code != "unknown_icon" {
			t.Fatalf("错误码异常: %v", err)
		}
	}

	// 内置图标 SVG 只读、不可删除
	_, err = c.UpdateIcon("chevron-right", canvas.IconInput{Name: "chevron-right", SVG: `<path d="M0 0"/>`})
	var ae *canvas.AdminError
	if !errors.As(err, &ae) || ae.Code != "icon_builtin_protected" {
		t.Fatalf("内置编辑错误码异常: %v", err)
	}
	if err := c.RemoveIcon("chevron-right"); !errors.As(err, &ae) || ae.Code != "icon_builtin_protected" {
		t.Fatalf("内置删除错误码异常: %v", err)
	}

	// 停用 → 选择面消失
	out, err = c.SetIconDisabled("app-probe", true)
	if err != nil {
		t.Fatal(err)
	}
	if off, _ := out.(map[string]any)["disabled"].(bool); !off || design.IsIconEnabled("app-probe") {
		t.Fatal("停用未生效")
	}
	if _, err := c.ResetIcon("app-probe"); !errors.As(err, &ae) || ae.Code != "invalid_icon" {
		t.Fatalf("自定义图标重置错误码异常: %v", err)
	}

	// 启用后移除
	if _, err := c.SetIconDisabled("app-probe", false); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveIcon("app-probe"); err != nil {
		t.Fatal(err)
	}
	if _, ok := design.GetIcon("app-probe"); ok {
		t.Fatal("移除后注册表仍有该图标")
	}
}
