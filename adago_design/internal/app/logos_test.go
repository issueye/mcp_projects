package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

func TestCatalogLogoLifecycle(t *testing.T) {
	_, _ = design.LoadLogos("")
	t.Cleanup(func() { _, _ = design.LoadLogos("") })

	c := Catalog{DataDir: t.TempDir()}
	doc := design.DefaultLogoDoc()
	doc.Brand = "Acme Cloud"
	doc.Tagline = "design system"
	doc.Color = "#0B5FFF"
	doc.Background = "#FBF8F1"
	raw, _ := json.Marshal(doc)

	out, err := c.CreateLogo(canvas.LogoInput{Name: "Acme Cloud", Note: "主标识", System: "ant", Doc: raw})
	if err != nil {
		t.Fatal(err)
	}
	summary := out.(map[string]any)
	if summary["id"] != "acme-cloud" || summary["brand"] != "Acme Cloud" {
		t.Fatalf("创建摘要异常: %v", summary)
	}
	if svg, _ := summary["svg"].(string); !strings.Contains(svg, "Acme Cloud") {
		t.Fatalf("摘要 SVG 异常: %v", summary["svg"])
	}

	detail, err := c.LogoDetail("acme-cloud")
	if err != nil {
		t.Fatal(err)
	}
	d := detail.(map[string]any)
	vars, _ := d["variants"].(map[string]string)
	if len(vars) != 3 {
		t.Fatalf("变体数量异常: %v", d["variants"])
	}
	if _, ok := d["spec"].(design.LogoSpec); !ok {
		t.Fatalf("spec 类型异常: %T", d["spec"])
	}
	if w, _ := d["width"].(float64); w <= 0 {
		t.Fatalf("宽度异常: %v", d["width"])
	}

	// 导出 / 画布 HTML
	svg, filename, err := c.LogoExport("acme-cloud", "inverse")
	if err != nil || filename != "acme-cloud-inverse.svg" || !strings.Contains(svg, "#0B5FFF") {
		t.Fatalf("导出异常: %v %s\n%s", err, filename, svg)
	}
	html, w, h, err := c.LogoCanvas("acme-cloud", "positive")
	if err != nil || w <= 0 || h <= 0 || !strings.Contains(html, "<!DOCTYPE html>") {
		t.Fatalf("画布 HTML 异常: %v %dx%d", err, w, h)
	}

	// 更新
	doc2 := doc
	doc2.Layout = design.LogoLayoutStacked
	doc2.Brand = "Acme Cloud 2"
	raw2, _ := json.Marshal(doc2)
	up, err := c.UpdateLogo("acme-cloud", canvas.LogoInput{Name: "Acme Cloud", System: "apple", Doc: raw2})
	if err != nil {
		t.Fatal(err)
	}
	if up.(map[string]any)["brand"] != "Acme Cloud 2" {
		t.Fatalf("更新摘要异常: %v", up)
	}

	// 未知 / 非法
	if _, err := c.LogoDetail("nope"); err == nil {
		t.Fatal("未知 Logo 应报错")
	} else {
		var ae *canvas.AdminError
		if !errors.As(err, &ae) || ae.Code != "unknown_logo" {
			t.Fatalf("错误码异常: %v", err)
		}
	}
	if _, err := c.CreateLogo(canvas.LogoInput{Name: "Bad", Doc: json.RawMessage(`{"brand":`)}); err == nil {
		t.Fatal("坏 doc JSON 应报错")
	} else {
		var ae *canvas.AdminError
		if !errors.As(err, &ae) || ae.Code != "invalid_logo" {
			t.Fatalf("坏 doc 错误码异常: %v", err)
		}
	}
	if _, err := c.CreateLogo(canvas.LogoInput{Name: "Bad", Doc: json.RawMessage(`{"doc":1}`)}); err == nil {
		t.Fatal("错误 doc 结构应报错")
	}

	// 不会因为详情失败而丢资产；列表仍在
	if list := c.LogoSummaries().([]map[string]any); len(list) != 1 {
		t.Fatalf("列表数量异常: %d", len(list))
	}
	if err := c.RemoveLogo("acme-cloud"); err != nil {
		t.Fatal(err)
	}
	if _, ok := design.GetLogo("acme-cloud"); ok {
		t.Fatal("移除后仍存在")
	}
}

func TestCatalogFontCatalog(t *testing.T) {
	fonts := Catalog{}.FontCatalog().([]map[string]any)
	if len(fonts) < 5 {
		t.Fatalf("字体数量异常: %d", len(fonts))
	}
	found := false
	for _, f := range fonts {
		if f["id"] == "inter" {
			found = true
			if f["license"] == "" || f["commercial"] != true {
				t.Fatalf("Inter 授权元数据异常: %v", f)
			}
		}
	}
	if !found {
		t.Fatal("字体目录缺少 Inter")
	}
}
