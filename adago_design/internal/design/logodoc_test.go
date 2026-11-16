package design

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleLogoDoc() LogoDoc {
	doc := DefaultLogoDoc()
	doc.Brand = "Acme Cloud"
	doc.Tagline = "design system"
	doc.Color = "#0B5FFF"
	doc.Background = "#FBF8F1"
	return doc
}

func TestLogoDocRenderLayouts(t *testing.T) {
	doc := sampleLogoDoc()
	for _, layout := range []string{LogoLayoutHorizontal, LogoLayoutStacked, LogoLayoutMark, LogoLayoutWordmark} {
		t.Run(layout, func(t *testing.T) {
			d := doc
			d.Layout = layout
			if layout == LogoLayoutWordmark {
				d.Mark = LogoMark{Kind: "none"}
			}
			norm, err := NormalizeLogoDoc(d)
			if err != nil {
				t.Fatal(err)
			}
			svg, err := LogoSVG(norm, LogoVariantPositive)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`<svg`, `viewBox=`, `<rect`} {
				if !strings.Contains(svg, want) {
					t.Errorf("%s SVG 缺少 %q\n%s", layout, want, svg)
				}
			}
			if layout != LogoLayoutWordmark && !strings.Contains(svg, `<svg x=`) {
				t.Errorf("%s 布局缺标记: %s", layout, svg)
			}
			if layout != LogoLayoutMark && !strings.Contains(svg, ">Acme Cloud</text>") {
				t.Errorf("%s 布局缺字标: %s", layout, svg)
			}
			if layout == LogoLayoutMark && strings.Contains(svg, "<text") {
				t.Errorf("纯标记布局不应有文字节点: %s", svg)
			}
			m, err := LogoLayoutMetrics(norm)
			if err != nil {
				t.Fatal(err)
			}
			if m.Width <= 0 || m.Height <= 0 {
				t.Fatalf("%s 尺寸异常: %+v", layout, m)
			}
		})
	}
}

func TestLogoDocVariants(t *testing.T) {
	doc := sampleLogoDoc()
	variants, err := LogoVariants(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 3 {
		t.Fatalf("变体数量 = %d", len(variants))
	}
	if !strings.Contains(variants[LogoVariantPositive], "#0B5FFF") {
		t.Fatal("正形应使用品牌色")
	}
	if !strings.Contains(variants[LogoVariantInverse], `fill="#0B5FFF"`) {
		t.Fatal("反形应有品牌色底")
	}
	if !strings.Contains(variants[LogoVariantMono], "#16171A") {
		t.Fatal("单色应为墨色")
	}
	// 反形在深底上必须用浅色墨，不能把品牌色字压在品牌色底上
	light := doc
	light.Color = "#FBF8F1"
	svg, err := LogoSVG(light, LogoVariantInverse)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, "#16171A") {
		t.Fatalf("浅色品牌色的反形应回落墨色:\n%s", svg)
	}
}

func TestLogoDocSpec(t *testing.T) {
	spec, err := LogoDocSpec(sampleLogoDoc())
	if err != nil {
		t.Fatal(err)
	}
	if spec.MinWidth <= 0 || spec.MinHeight <= 0 || spec.ClearSpace <= 0 {
		t.Fatalf("规范尺寸异常: %+v", spec)
	}
	if len(spec.Variants) != 3 || spec.FontID != "inter" {
		t.Fatalf("规范变体/字体异常: %+v", spec)
	}
	if !strings.Contains(spec.ClearSpaceRule, "留白") {
		t.Fatalf("安全空间说明缺失: %s", spec.ClearSpaceRule)
	}
	if spec.Contrast[LogoVariantPositive] <= 0 {
		t.Fatalf("对比度缺失: %+v", spec.Contrast)
	}
	if len(spec.Usage) == 0 {
		t.Fatal("缺少使用规范")
	}
}

func TestLogoDocValidation(t *testing.T) {
	base := sampleLogoDoc()
	cases := []struct {
		name string
		edit func(*LogoDoc)
	}{
		{"未知布局", func(d *LogoDoc) { d.Layout = "circle" }},
		{"坏颜色", func(d *LogoDoc) { d.Color = "blue" }},
		{"坏底色", func(d *LogoDoc) { d.Background = "#FFF" }},
		{"未知字体", func(d *LogoDoc) { d.FontID = "comic-sans" }},
		{"字体不含该字重", func(d *LogoDoc) { d.FontID = "noto-sans-sc"; d.Weight = 600 }},
		{"标记为空", func(d *LogoDoc) { d.Mark = LogoMark{} }},
		{"未知标记图标", func(d *LogoDoc) { d.Mark.Icon = "nope-icon" }},
		{"纯字标缺品牌名", func(d *LogoDoc) { d.Layout = LogoLayoutWordmark; d.Mark = LogoMark{Kind: "none"}; d.Brand = "" }},
		{"字号过小", func(d *LogoDoc) { d.FontSize = 4 }},
		{"标记过大", func(d *LogoDoc) { d.MarkSize = 999 }},
		{"负间距", func(d *LogoDoc) { d.Gap = -1 }},
		{"留白过大", func(d *LogoDoc) { d.ClearSpace = 9 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := base
			c.edit(&doc)
			if _, err := NormalizeLogoDoc(doc); !errors.Is(err, ErrLogoInvalid) {
				t.Fatalf("%s 应返回 ErrLogoInvalid，得到 %v", c.name, err)
			}
		})
	}
	// 自绘几何标记
	doc := base
	shapes := NewIconDoc()
	shapes.Shapes = []IconShape{{Type: "circle", CX: 12, CY: 12, R: 8}}
	doc.Mark = LogoMark{Kind: "shapes", Design: &shapes}
	norm, err := NormalizeLogoDoc(doc)
	if err != nil {
		t.Fatal(err)
	}
	svg, err := LogoSVG(norm, LogoVariantPositive)
	if err != nil || !strings.Contains(svg, "<circle") {
		t.Fatalf("自绘标记渲染失败: %v\n%s", err, svg)
	}
}

func TestLogoHTML(t *testing.T) {
	html, w, h, err := LogoHTML(sampleLogoDoc(), LogoVariantInverse)
	if err != nil {
		t.Fatal(err)
	}
	if w <= 0 || h <= 0 || !strings.Contains(html, "<!DOCTYPE html>") || !strings.Contains(html, "<svg") {
		t.Fatalf("LogoHTML 异常: %dx%d\n%s", w, h, html)
	}
	if !strings.Contains(html, `background:#0B5FFF`) && !strings.Contains(html, "background:#0B5FFF") {
		t.Fatalf("反形画布底色缺失:\n%s", html)
	}
}

func TestLogoStoreLifecycle(t *testing.T) {
	_, _ = LoadLogos("")
	dir := t.TempDir()
	t.Cleanup(func() { _, _ = LoadLogos("") })

	doc := sampleLogoDoc()
	l, err := CreateLogo(LogoInput{Name: "Acme Cloud", Note: "主标识", System: "ant", Doc: doc}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != "acme-cloud" || l.Name != "Acme Cloud" || l.System != "ant" {
		t.Fatalf("创建异常: %+v", l)
	}
	if raw, err := os.ReadFile(filepath.Join(LogoDir(dir), l.ID+".json")); err != nil ||
		!strings.Contains(string(raw), `"brand": "Acme Cloud"`) {
		t.Fatalf("Logo 文件未写入: %v", err)
	}
	if _, err := CreateLogo(LogoInput{ID: l.ID, Doc: doc}, dir); !errors.Is(err, ErrLogoExists) {
		t.Fatalf("显式重复 ID 应报错，得到 %v", err)
	}
	// 同名自动加序号
	l2, err := CreateLogo(LogoInput{Name: "Acme Cloud", Doc: doc}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if l2.ID == l.ID || l2.ID != "acme-cloud-2" {
		t.Fatalf("自动 ID 异常: %s", l2.ID)
	}

	// 全量更新 + 详情
	doc2 := doc
	doc2.Brand = "Acme Cloud 2"
	doc2.Layout = LogoLayoutStacked
	up, err := UpdateLogo(l.ID, LogoInput{Doc: doc2, System: "apple"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if up.Doc.Brand != "Acme Cloud 2" || up.System != "apple" || up.Name != l.Name {
		t.Fatalf("更新异常: %+v", up)
	}
	if _, err := UpdateLogo("nope", LogoInput{Doc: doc}, dir); !errors.Is(err, ErrLogoNotFound) {
		t.Fatalf("未知 Logo 更新应报错，得到 %v", err)
	}

	// SVG / HTML / 规范
	svg, filename, err := LogoVariantSVG(l.ID, LogoVariantInverse)
	if err != nil || filename != l.ID+"-inverse.svg" || !strings.Contains(svg, "#0B5FFF") {
		t.Fatalf("变体导出异常: %v %s\n%s", err, filename, svg)
	}
	html, w, h, err := LogoCanvasHTML(l.ID, LogoVariantPositive)
	if err != nil || w <= 0 || h <= 0 || !strings.Contains(html, "<svg") {
		t.Fatalf("画布 HTML 异常: %v %dx%d", err, w, h)
	}
	if _, err := LogoSpecFor(up); err != nil {
		t.Fatal(err)
	}

	// 重新加载（含中文名称自动兜底）
	if err := os.MkdirAll(LogoDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(Logo{
		ID: "logo-cn", Name: "品牌标志", Doc: doc,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(LogoDir(dir), "logo-cn.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = LoadLogos("")
	n, err := LoadLogos(dir)
	if err != nil || n != 3 {
		t.Fatalf("LoadLogos = %d, %v", n, err)
	}
	if _, ok := GetLogo(l.ID); !ok {
		t.Fatal("重新加载后丢失 Logo")
	}
	if list := AllLogos(); len(list) != 3 {
		t.Fatalf("AllLogos = %d", len(list))
	}

	if err := RemoveLogo(l.ID, dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetLogo(l.ID); ok {
		t.Fatal("移除后仍存在")
	}
	if _, err := os.Stat(filepath.Join(LogoDir(dir), l.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("移除后文件应删除: %v", err)
	}
	if err := RemoveLogo("nope", dir); !errors.Is(err, ErrLogoNotFound) {
		t.Fatalf("未知 Logo 移除应报错，得到 %v", err)
	}
}

func TestLogoDegradesWhenMarkIconMissing(t *testing.T) {
	_, _ = LoadLogos("")
	dir := t.TempDir()
	t.Cleanup(func() { _, _ = LoadLogos("") })
	doc := sampleLogoDoc()
	doc.Mark = LogoMark{Kind: "icon", Icon: "missing-mark"}
	// normalize 本身应拒绝未知图标
	if _, err := NormalizeLogoDoc(doc); !errors.Is(err, ErrLogoInvalid) {
		t.Fatalf("未知标记应被拒绝，得到 %v", err)
	}
	// 磁盘上的旧资产：标记消失后降级为纯字标而不是丢资产
	raw, _ := json.MarshalIndent(Logo{ID: "legacy-logo", Name: "Legacy", Doc: doc}, "", "  ")
	if err := os.MkdirAll(LogoDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(LogoDir(dir), "legacy-logo.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := LoadLogos(dir)
	if n != 1 {
		t.Fatalf("缺失标记的 Logo 应降级保留，n=%d err=%v", n, err)
	}
	l, ok := GetLogo("legacy-logo")
	if !ok || l.Doc.Layout != LogoLayoutWordmark || l.Doc.Mark.Kind != "none" {
		t.Fatalf("降级结果异常: %+v", l)
	}
}
