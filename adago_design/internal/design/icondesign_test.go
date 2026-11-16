package design

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func sampleIconDoc() IconDoc {
	doc := NewIconDoc()
	doc.Shapes = []IconShape{
		{Type: "line", X1: 4, Y1: 12, X2: 20, Y2: 12},
		{Type: "rect", X: 4, Y: 4, Width: 16, Height: 16, Radius: 4},
		{Type: "circle", CX: 12, CY: 12, R: 3, Fill: true, Stroke: boolPtr(false)},
		{Type: "ellipse", CX: 8, CY: 17, RX: 3, RY: 2},
		{Type: "polyline", Points: []IconPoint{{3, 18}, {8, 22}, {13, 18}}},
		{Type: "polygon", Points: []IconPoint{{14, 9}, {16, 13}, {20, 13}}},
		{Type: "path", D: "M2 2l4 4l-4 4z", DX: 1, DY: 1},
	}
	return doc
}

func TestIconDocRenderPrimitives(t *testing.T) {
	doc, err := NormalizeIconDoc(sampleIconDoc())
	if err != nil {
		t.Fatal(err)
	}
	svg, err := IconDocSVG(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`viewBox="0 0 24 24"`,
		`stroke-width="1.8"`,
		`stroke-linecap="round"`,
		`stroke-linejoin="round"`,
		`<line x1="4" y1="12" x2="20" y2="12"`,
		`<rect x="4" y="4" width="16" height="16" rx="4"`,
		`<circle cx="12" cy="12" r="3" stroke="none" fill="currentColor"`,
		`<ellipse cx="8" cy="17" rx="3" ry="2"`,
		`<polyline points="3,18 8,22 13,18"`,
		`<polygon points="14,9 16,13 20,13"`,
		`<g transform="translate(1 1)"><path d="M2 2l4 4l-4 4z"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG 缺少 %q\n%s", want, svg)
		}
	}
	// 渲染结果必须还能被现有图标校验器接受（保证落库路径不绕过安全检查）。
	if _, _, err := parseIconSVG(svg); err != nil {
		t.Fatalf("设计稿渲染结果被 parseIconSVG 拒绝: %v\n%s", err, svg)
	}

	body := IconDocBody(doc)
	if strings.Count(body, `<g fill=`) != 1 || !strings.Contains(body, "</g>") {
		t.Fatalf("Body 应只有一个统一描边分组: %s", body)
	}
	// hidden 图形不导出
	hidden := sampleIconDoc()
	hidden.Shapes[1].Hidden = true
	hiddenSVG, err := IconDocSVG(hidden)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hiddenSVG, `<rect`) {
		t.Fatal("hidden 图形不应导出")
	}
}

func TestIconDocValidation(t *testing.T) {
	base := sampleIconDoc()
	cases := []struct {
		name string
		edit func(*IconDoc)
	}{
		{"空文档", func(d *IconDoc) { d.Shapes = nil }},
		{"未知类型", func(d *IconDoc) { d.Shapes[0].Type = "star" }},
		{"网格过小", func(d *IconDoc) { d.Grid = 4 }},
		{"描边过粗", func(d *IconDoc) { d.StrokeWidth = 9 }},
		{"线长为零", func(d *IconDoc) { d.Shapes[0].X2, d.Shapes[0].Y2 = d.Shapes[0].X1, d.Shapes[0].Y1 }},
		{"矩形反尺寸", func(d *IconDoc) { d.Shapes[1].Width = -1 }},
		{"圆半径为零", func(d *IconDoc) { d.Shapes[2].R = 0 }},
		{"折线点太少", func(d *IconDoc) { d.Shapes[4].Points = d.Shapes[4].Points[:1] }},
		{"多边形点太少", func(d *IconDoc) { d.Shapes[5].Points = d.Shapes[5].Points[:2] }},
		{"path 注入", func(d *IconDoc) { d.Shapes[6].D = `M0 0" onload="x` }},
		{"path 无起点", func(d *IconDoc) { d.Shapes[6].D = "l1 1" }},
		{"不可见图形", func(d *IconDoc) {
			d.Shapes[1].Fill = false
			d.Shapes[1].Stroke = boolPtr(false)
		}},
		{"透明度越界", func(d *IconDoc) { d.Shapes[0].Opacity = 1.5 }},
		{"坐标越界", func(d *IconDoc) { d.Shapes[2].CX = 99 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := base
			doc.Shapes = append([]IconShape(nil), base.Shapes...)
			c.edit(&doc)
			if _, err := NormalizeIconDoc(doc); !errors.Is(err, ErrIconInvalid) {
				t.Fatalf("%s 应返回 ErrIconInvalid，得到 %v", c.name, err)
			}
		})
	}
}

func TestIconDocImportBuiltinAndRoundTrip(t *testing.T) {
	for _, name := range []string{"close", "search", "chevron-right", "check-circle"} {
		info, err := IconDesign(name, t.TempDir())
		if err != nil {
			t.Fatalf("%s 反推失败: %v", name, err)
		}
		if !info.Imported || !info.Builtin || len(info.Doc.Shapes) == 0 {
			t.Fatalf("%s 反推标记异常: %+v", name, info)
		}
		svg, err := IconDocSVG(info.Doc)
		if err != nil {
			t.Fatalf("%s 反推后渲染失败: %v", name, err)
		}
		again, err := ImportIconDoc(svg)
		if err != nil {
			t.Fatalf("%s 往返导入失败: %v\n%s", name, err, svg)
		}
		if len(again.Shapes) != len(info.Doc.Shapes) {
			t.Fatalf("%s 往返图形数变化: %d → %d", name, len(info.Doc.Shapes), len(again.Shapes))
		}
	}
}

func TestIconDocImportRejectsComplexSVG(t *testing.T) {
	cases := map[string]string{
		"渐变":     `<svg viewBox="0 0 24 24"><defs><linearGradient id="g"><stop offset="0"/></linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`,
		"变换":     `<svg viewBox="0 0 24 24"><rect width="10" height="10" transform="rotate(45 12 12)"/></svg>`,
		"嵌套分组样式": `<svg viewBox="0 0 24 24"><g><g stroke-width="3"><path d="M2 2h20"/></g></g></svg>`,
		"非正方形":   `<svg viewBox="0 0 24 12"><path d="M2 2h20"/></svg>`,
		"文本":     `<svg viewBox="0 0 24 24"><text x="2" y="2">x</text><path d="M2 2h20"/></svg>`,
	}
	for label, svg := range cases {
		if _, err := ImportIconDoc(svg); !errors.Is(err, ErrIconInvalid) {
			t.Errorf("%s 应被拒绝，得到 %v", label, err)
		}
	}
	// 只有基础图形时正常导入，并继承根描边参数
	doc, err := ImportIconDoc(`<svg viewBox="0 0 24 24" stroke-width="2.5" stroke-linecap="square"><circle cx="12" cy="12" r="8"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Grid != 24 || doc.StrokeWidth != 2.5 || doc.Cap != "square" || len(doc.Shapes) != 1 {
		t.Fatalf("导入参数异常: %+v", doc)
	}
	// 根级 fill/stroke 要下沉到图形，否则实心图标会被画成描边图标
	filled, err := ImportIconDoc(`<svg viewBox="0 0 24 24" fill="currentColor" stroke="none">` +
		`<path d="M4 4h16v16H4z"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	sh := filled.Shapes[0]
	if !sh.Fill || sh.Stroke == nil || *sh.Stroke {
		t.Fatalf("根级 fill/stroke 未下沉: %+v", sh)
	}
}

func TestIconDesignPersistence(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	doc := sampleIconDoc()
	const name = "design-probe"

	ic, err := CreateIcon(IconInput{Name: name, Group: "action", Tags: []string{"设计稿"}, Design: &doc}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ic.Designed || ic.ViewBox != "0 0 24 24" {
		t.Fatalf("设计稿图标状态异常: %+v", ic)
	}
	raw, err := os.ReadFile(filepath.Join(CustomIconsDir(dir), name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var f iconFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Design == nil || len(f.Design.Shapes) != len(doc.Shapes) {
		t.Fatalf("design 未随图标持久化: %s", raw)
	}
	if !strings.Contains(f.SVG, "translate(1 1)") || !strings.Contains(f.SVG, `<circle`) {
		t.Fatalf("落库 SVG 与设计稿不同源: %s", f.SVG)
	}

	info, err := IconDesign(name, dir)
	if err != nil || info.Imported || !info.Designed || info.Builtin {
		t.Fatalf("IconDesign = %+v, %v", info, err)
	}
	if len(info.Doc.Shapes) != len(doc.Shapes) {
		t.Fatalf("读取的设计稿图形数 = %d", len(info.Doc.Shapes))
	}

	// 重新加载磁盘后仍能继续设计
	if _, err := LoadCustomIcons(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetIcon(name); !got.Designed {
		t.Fatal("重新加载后应保留 Designed 标记")
	}

	// 用设计稿整体更新：只留一个矩形
	next := NewIconDoc()
	next.Shapes = []IconShape{{Type: "rect", X: 3, Y: 3, Width: 18, Height: 18, Radius: 3}}
	if _, err := UpdateIcon(name, dir, IconPatch{Design: &next}); err != nil {
		t.Fatal(err)
	}
	info, err = IconDesign(name, dir)
	if err != nil || info.Imported || len(info.Doc.Shapes) != 1 {
		t.Fatalf("更新后设计稿异常: %+v, %v", info, err)
	}
	// 手改 SVG 会清掉设计稿（见 TestIconDesignClearedByManualSVG）
}

func TestIconDesignClearedByManualSVG(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	doc := NewIconDoc()
	doc.Shapes = []IconShape{{Type: "circle", CX: 12, CY: 12, R: 8}}
	const name = "design-manual"
	if _, err := CreateIcon(IconInput{Name: name, Design: &doc}, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateIcon(name, dir, IconPatch{
		SVG: `<svg viewBox="0 0 24 24"><path d="M4 4h16v16H4z"/></svg>`,
	}); err != nil {
		t.Fatal(err)
	}
	ic, _ := GetIcon(name)
	if ic.Designed {
		t.Fatal("手改 SVG 后 Designed 应为 false")
	}
	raw, err := os.ReadFile(filepath.Join(CustomIconsDir(dir), name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"design"`) {
		t.Fatalf("手改 SVG 后 design 字段应清除: %s", raw)
	}
	info, err := IconDesign(name, dir)
	if err != nil || !info.Imported || info.Designed {
		t.Fatalf("手改后 IconDesign 应回退为反推: %+v, %v", info, err)
	}
}

// 内置图标全部要能在设计器里打开（以它为起点），否则 UI 的按钮会时灵时不灵。
func TestAllBuiltinIconsOpenInDesigner(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	for _, ic := range AllIcons() {
		if !ic.Builtin {
			continue
		}
		info, err := IconDesign(ic.Name, dir)
		if err != nil {
			t.Errorf("%s 无法在设计器打开: %v", ic.Name, err)
			continue
		}
		if !info.Imported || !info.Builtin || len(info.Doc.Shapes) == 0 {
			t.Errorf("%s 设计稿标记异常: %+v", ic.Name, info)
		}
		if _, err := IconDocSVG(info.Doc); err != nil {
			t.Errorf("%s 设计稿渲染失败: %v", ic.Name, err)
		}
	}
}

func TestBuiltinDesignIsReadOnly(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	info, err := IconDesign("close", dir)
	if err != nil || !info.Imported || !info.Builtin || info.Designed {
		t.Fatalf("内置设计稿异常: %+v, %v", info, err)
	}
	doc := info.Doc
	if _, err := UpdateIcon("close", dir, IconPatch{Design: &doc}); !errors.Is(err, ErrIconBuiltin) {
		t.Fatalf("内置图标写设计稿应被拒绝，得到 %v", err)
	}
	// 以内置图标为起点另存为新自定义图标（设计器里的「以它为起点」）
	if _, err := CreateIcon(IconInput{Name: "close-custom", Group: "action", Design: &doc}, dir); err != nil {
		t.Fatal(err)
	}
}
