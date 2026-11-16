package design

// 图标设计文档：把「画图标」变成可序列化、可校验、可还原的几何数据。
//
// 与「粘贴 SVG」的区别：
//   - 设计文档由基础图形（line/rect/circle/ellipse/polyline/polygon/path）组成，
//     统一走 24 网格、currentColor 与描边参数，因此 12–24 光学尺寸下自动一致；
//   - 用户可以随时回到设计器继续改形状、图层、描边，而不是面对一段只能手改的源码；
//   - Agent 可以用 MCP 直接提交几何文档，拿到同一套确定性 SVG，再落库成自定义图标。
//
// 存放：自定义图标 JSON 的 design 字段（见 icons_manage.go），SVG 仍是交付物，
// design 只是「可再编辑的源」。两者不一致时以 SVG 为准（手改 SVG 会清掉 design）。
//
// 约定：
//   - ViewBox 恒为 0 0 <grid> <grid>，grid 默认 24；
//   - 图形按数组顺序绘制（后画的在上），hidden 只用于编辑期隐藏、不导出；
//   - 每个图形可单独 fill（currentColor 实心）或 no-stroke，默认继承统一描边；
//   - path 图形允许整体平移（dx/dy），导入内置图标时用它对已有路径做位移。

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	// IconDocVersion 是设计文档的当前版本。
	IconDocVersion = 1
	// IconDefaultGrid 是图标设计网格边长（24×24）。
	IconDefaultGrid = 24.0
	// IconDefaultStrokeWidth 与体系 .ds-icon 的 1.8px 描边对齐。
	IconDefaultStrokeWidth = 1.8
	// IconMinGrid / IconMaxGrid 是画板网格允许范围。
	IconMinGrid = 8.0
	IconMaxGrid = 64.0
	// IconMaxShapes 是一份设计稿的图形上限，防止单图标膨胀成插画。
	IconMaxShapes = 128
)

// 描边端点 / 连接方式白名单（与 SVG 名称一致）。
var (
	iconCaps  = map[string]bool{"butt": true, "round": true, "square": true}
	iconJoins = map[string]bool{"miter": true, "round": true, "bevel": true}
	// 图标 path 只允许绝对/相对命令、数字与分隔符；禁止 CSS/URL/函数式内容。
	iconPathDRe      = regexp.MustCompile(`^[MmLlHhVvCcSsQqTtAaZz0-9eE.,+\-\s]+$`)
	iconViewBoxNumRe = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
)

// 图形类型白名单。
var iconShapeTypes = map[string]bool{
	"line": true, "rect": true, "circle": true, "ellipse": true,
	"polyline": true, "polygon": true, "path": true,
}

// IconPoint 是折线/多边形的一个点；JSON 序列化为 [x, y]，便于前端直接读写。
type IconPoint [2]float64

// IconShape 是设计文档中的一个基础图形。
//
// 各类型使用的字段：
//
//	line      x1/y1/x2/y2
//	rect      x/y/width/height（radius 可选圆角）
//	circle    cx/cy/r
//	ellipse   cx/cy/rx/ry
//	polyline  points（≥2 点）
//	polygon   points（≥3 点，自动闭合）
//	path      d（SVG path 数据；可再用 dx/dy 整体平移）
type IconShape struct {
	Type string `json:"type"`
	// 几何：按类型取用，其余字段省略。
	X       float64     `json:"x,omitempty"`
	Y       float64     `json:"y,omitempty"`
	Width   float64     `json:"width,omitempty"`
	Height  float64     `json:"height,omitempty"`
	Radius  float64     `json:"radius,omitempty"`
	CX      float64     `json:"cx,omitempty"`
	CY      float64     `json:"cy,omitempty"`
	R       float64     `json:"r,omitempty"`
	RX      float64     `json:"rx,omitempty"`
	RY      float64     `json:"ry,omitempty"`
	X1      float64     `json:"x1,omitempty"`
	Y1      float64     `json:"y1,omitempty"`
	X2      float64     `json:"x2,omitempty"`
	Y2      float64     `json:"y2,omitempty"`
	Points  []IconPoint `json:"points,omitempty"`
	D       string      `json:"d,omitempty"`
	DX      float64     `json:"dx,omitempty"` // 整体平移（path 导入后也能拖动）
	DY      float64     `json:"dy,omitempty"`
	Fill    bool        `json:"fill,omitempty"`        // 实心填充 currentColor
	Stroke  *bool       `json:"stroke,omitempty"`      // nil=继承；false=只填充不描边
	Opacity float64     `json:"opacity,omitempty"`     // 0=默认 1；0–1
	StrokeW float64     `json:"strokeWidth,omitempty"` // 0=继承文档；否则覆盖
	Hidden  bool        `json:"hidden,omitempty"`      // 编辑期隐藏，不导出
}

// IconDoc 是一份可再编辑的图标设计文档。
type IconDoc struct {
	Version     int         `json:"version,omitempty"`
	Grid        float64     `json:"grid,omitempty"`
	StrokeWidth float64     `json:"strokeWidth,omitempty"`
	Cap         string      `json:"cap,omitempty"`
	Join        string      `json:"join,omitempty"`
	Shapes      []IconShape `json:"shapes"`
}

// NewIconDoc 返回一份空的 24 网格设计稿（新建图标时的初值）。
func NewIconDoc() IconDoc {
	return IconDoc{
		Version: IconDocVersion, Grid: IconDefaultGrid,
		StrokeWidth: IconDefaultStrokeWidth, Cap: "round", Join: "round",
		Shapes: []IconShape{},
	}
}

// NormalizeIconDoc 校验并补齐默认值；返回的副本可以直接渲染 / 落库。
func NormalizeIconDoc(doc IconDoc) (IconDoc, error) {
	doc.Version = IconDocVersion
	if doc.Grid == 0 {
		doc.Grid = IconDefaultGrid
	}
	if doc.Grid < IconMinGrid || doc.Grid > IconMaxGrid {
		return IconDoc{}, fmt.Errorf("%w: 网格必须是 %g–%g", ErrIconInvalid, IconMinGrid, IconMaxGrid)
	}
	if doc.StrokeWidth == 0 {
		doc.StrokeWidth = IconDefaultStrokeWidth
	}
	if doc.StrokeWidth < 0.5 || doc.StrokeWidth > 8 {
		return IconDoc{}, fmt.Errorf("%w: 描边宽度必须在 0.5–8", ErrIconInvalid)
	}
	doc.Cap = strings.ToLower(strings.TrimSpace(doc.Cap))
	if doc.Cap == "" {
		doc.Cap = "round"
	}
	if !iconCaps[doc.Cap] {
		return IconDoc{}, fmt.Errorf("%w: cap 只能是 butt/round/square", ErrIconInvalid)
	}
	doc.Join = strings.ToLower(strings.TrimSpace(doc.Join))
	if doc.Join == "" {
		doc.Join = "round"
	}
	if !iconJoins[doc.Join] {
		return IconDoc{}, fmt.Errorf("%w: join 只能是 miter/round/bevel", ErrIconInvalid)
	}
	if len(doc.Shapes) == 0 {
		return IconDoc{}, fmt.Errorf("%w: 至少需要一个图形", ErrIconInvalid)
	}
	if len(doc.Shapes) > IconMaxShapes {
		return IconDoc{}, fmt.Errorf("%w: 图形数量超过 %d", ErrIconInvalid, IconMaxShapes)
	}
	for i := range doc.Shapes {
		if err := normalizeIconShape(&doc.Shapes[i], doc.Grid, i); err != nil {
			return IconDoc{}, err
		}
	}
	return doc, nil
}

func normalizeIconShape(s *IconShape, grid float64, idx int) error {
	s.Type = strings.ToLower(strings.TrimSpace(s.Type))
	if !iconShapeTypes[s.Type] {
		return fmt.Errorf("%w: 第 %d 个图形类型 %q 不支持（line/rect/circle/ellipse/polyline/polygon/path）",
			ErrIconInvalid, idx+1, s.Type)
	}
	if s.Opacity < 0 || s.Opacity > 1 {
		return fmt.Errorf("%w: 第 %d 个图形的 opacity 必须在 0–1", ErrIconInvalid, idx+1)
	}
	if s.StrokeW != 0 && (s.StrokeW < 0.5 || s.StrokeW > 8) {
		return fmt.Errorf("%w: 第 %d 个图形的 strokeWidth 必须在 0.5–8", ErrIconInvalid, idx+1)
	}
	// 允许轻微出血（视觉修正常见），但拒绝把图形画到画板之外很远。
	limit := grid * 1.25
	lo := -grid * 0.25
	check := func(v float64) bool {
		return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= limit
	}
	checkPoint := func(x, y float64) error {
		if !check(x) || !check(y) {
			return fmt.Errorf("%w: 第 %d 个图形的坐标超出画板范围", ErrIconInvalid, idx+1)
		}
		return nil
	}
	switch s.Type {
	case "line":
		if err := checkPoint(s.X1, s.Y1); err != nil {
			return err
		}
		if err := checkPoint(s.X2, s.Y2); err != nil {
			return err
		}
		if s.X1 == s.X2 && s.Y1 == s.Y2 {
			return fmt.Errorf("%w: 第 %d 条线长度为 0", ErrIconInvalid, idx+1)
		}
	case "rect":
		if !check(s.X) || !check(s.Y) || s.Width <= 0 || s.Height <= 0 ||
			!check(s.X+s.Width) || !check(s.Y+s.Height) {
			return fmt.Errorf("%w: 第 %d 个矩形几何不合法", ErrIconInvalid, idx+1)
		}
		if s.Radius < 0 || s.Radius > math.Min(s.Width, s.Height)/2 {
			return fmt.Errorf("%w: 第 %d 个矩形圆角超出范围", ErrIconInvalid, idx+1)
		}
	case "circle":
		if !check(s.CX) || !check(s.CY) || s.R <= 0 || !check(s.CX+s.R) || !check(s.CY+s.R) {
			return fmt.Errorf("%w: 第 %d 个圆形几何不合法", ErrIconInvalid, idx+1)
		}
	case "ellipse":
		if !check(s.CX) || !check(s.CY) || s.RX <= 0 || s.RY <= 0 ||
			!check(s.CX+s.RX) || !check(s.CY+s.RY) {
			return fmt.Errorf("%w: 第 %d 个椭圆几何不合法", ErrIconInvalid, idx+1)
		}
	case "polyline", "polygon":
		min := 2
		if s.Type == "polygon" {
			min = 3
		}
		if len(s.Points) < min {
			return fmt.Errorf("%w: 第 %d 个%s至少需要 %d 个点", ErrIconInvalid, idx+1, s.Type, min)
		}
		for i := range s.Points {
			if err := checkPoint(s.Points[i][0], s.Points[i][1]); err != nil {
				return err
			}
		}
	case "path":
		s.D = strings.TrimSpace(s.D)
		if s.D == "" {
			return fmt.Errorf("%w: 第 %d 个 path 缺少 d", ErrIconInvalid, idx+1)
		}
		if len(s.D) > 4096 {
			return fmt.Errorf("%w: 第 %d 个 path 超过 4096 字符", ErrIconInvalid, idx+1)
		}
		if !iconPathDRe.MatchString(s.D) {
			return fmt.Errorf("%w: 第 %d 个 path 含不允许的字符", ErrIconInvalid, idx+1)
		}
		if !strings.ContainsAny(s.D, "Mm") {
			return fmt.Errorf("%w: 第 %d 个 path 缺少 M/m 起点", ErrIconInvalid, idx+1)
		}
	}
	// dx/dy 是整体平移，范围不再按单点校验，但仍需有限且不夸张。
	if math.IsNaN(s.DX) || math.IsNaN(s.DY) || math.IsInf(s.DX, 0) || math.IsInf(s.DY, 0) ||
		math.Abs(s.DX) > grid || math.Abs(s.DY) > grid {
		return fmt.Errorf("%w: 第 %d 个图形的位移超出范围", ErrIconInvalid, idx+1)
	}
	if s.Stroke != nil && !*s.Stroke && !s.Fill {
		return fmt.Errorf("%w: 第 %d 个图形既无描边也无填充（会不可见）", ErrIconInvalid, idx+1)
	}
	return nil
}

// IconDocBody 渲染设计稿的图形片段（不含 <svg> 外壳，可直接作为图标 Body）。
func IconDocBody(doc IconDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<g fill="none" stroke="currentColor" stroke-width="%s" stroke-linecap="%s" stroke-linejoin="%s">`,
		fnum(doc.StrokeWidth), doc.Cap, doc.Join)
	for _, s := range doc.Shapes {
		if s.Hidden {
			continue
		}
		b.WriteString(renderIconShape(s))
	}
	b.WriteString(`</g>`)
	return b.String()
}

// IconDocSVG 渲染完整 SVG（设计器导出、落库、MCP 预览共用同一条路径）。
func IconDocSVG(doc IconDoc) (string, error) {
	norm, err := NormalizeIconDoc(doc)
	if err != nil {
		return "", err
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` +
		fnum(norm.Grid) + ` ` + fnum(norm.Grid) + `">` + "\n" + IconDocBody(norm) + "\n</svg>", nil
}

func renderIconShape(s IconShape) string {
	var geo string
	switch s.Type {
	case "line":
		geo = fmt.Sprintf(`<line x1="%s" y1="%s" x2="%s" y2="%s"`,
			fnum(s.X1), fnum(s.Y1), fnum(s.X2), fnum(s.Y2))
	case "rect":
		geo = fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s"`,
			fnum(s.X), fnum(s.Y), fnum(s.Width), fnum(s.Height))
		if s.Radius > 0 {
			geo += ` rx="` + fnum(s.Radius) + `"`
		}
	case "circle":
		geo = fmt.Sprintf(`<circle cx="%s" cy="%s" r="%s"`, fnum(s.CX), fnum(s.CY), fnum(s.R))
	case "ellipse":
		geo = fmt.Sprintf(`<ellipse cx="%s" cy="%s" rx="%s" ry="%s"`,
			fnum(s.CX), fnum(s.CY), fnum(s.RX), fnum(s.RY))
	case "polyline", "polygon":
		geo = `<` + s.Type + ` points="` + renderPoints(s.Points) + `"`
	case "path":
		geo = `<path d="` + s.D + `"`
	}
	style := ""
	if s.StrokeW > 0 {
		style += ` stroke-width="` + fnum(s.StrokeW) + `"`
	}
	if s.Stroke != nil && !*s.Stroke {
		style += ` stroke="none"`
	}
	if s.Fill {
		style += ` fill="currentColor"`
	}
	if s.Opacity > 0 && s.Opacity < 1 {
		style += ` opacity="` + fnum(s.Opacity) + `"`
	}
	if s.DX != 0 || s.DY != 0 {
		return `<g transform="translate(` + fnum(s.DX) + ` ` + fnum(s.DY) + `)">` +
			geo + style + `/>` + `</g>`
	}
	return geo + style + `/>`
}

func renderPoints(pts []IconPoint) string {
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(fnum(p[0]))
		b.WriteByte(',')
		b.WriteString(fnum(p[1]))
	}
	return b.String()
}

// fnum 输出紧凑数字：最多 2 位小数，去掉尾随 0；避免 23.999999 这类浮点噪声。
func fnum(v float64) string {
	r := math.Round(v*100) / 100
	if r == 0 {
		return "0"
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// ---------------- SVG → 设计文档（导入） ----------------

// ImportIconDoc 把一段 SVG（完整或片段）解析成设计文档。
// 只接受基础图形；含渐变、裁剪、transform、嵌套 <svg> 等复杂结构时明确报错，
// 让 UI 退回源码编辑，而不是悄悄丢图形。
func ImportIconDoc(svg string) (IconDoc, error) {
	body, viewBox, err := parseIconSVG(svg)
	if err != nil {
		return IconDoc{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
	}
	grid, err := squareGridFromViewBox(viewBox)
	if err != nil {
		return IconDoc{}, err
	}
	doc := IconDoc{
		Version: IconDocVersion, Grid: grid,
		StrokeWidth: IconDefaultStrokeWidth, Cap: "round", Join: "round",
		Shapes: []IconShape{},
	}
	rootFill := false
	rootStrokeOff := false
	if a, ok := svgRootAttributes(svg); ok {
		if v, has, err := attrFloat(a, "stroke-width"); err != nil {
			return IconDoc{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
		} else if has && v > 0 {
			doc.StrokeWidth = v
		}
		if v, has := attrString(a, "stroke-linecap"); has && v != "" {
			doc.Cap = strings.ToLower(v)
		}
		if v, has := attrString(a, "stroke-linejoin"); has && v != "" {
			doc.Join = strings.ToLower(v)
		}
		// 根级 fill/stroke 是手写 SVG 的常见默认值，导入时下沉到每个图形，
		// 否则实心图标会被设计器画成描边图标。
		if v, has := attrString(a, "fill"); has {
			low := strings.ToLower(strings.TrimSpace(v))
			rootFill = low != "" && low != "none" && low != "transparent"
		}
		if v, has := attrString(a, "stroke"); has && strings.ToLower(strings.TrimSpace(v)) == "none" {
			rootStrokeOff = true
		}
	}

	raw := "<adago-icon>" + body + "</adago-icon>"
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = true
	depth := 0
	// 分组平移栈：设计器导出的 path 位移会包一层 <g transform="translate(x y)">，
	// 导入时折回图形的 dx/dy，保证「导出 → 再打开」位置不漂。
	translates := []IconPoint{}
	for {
		tok, tokErr := dec.Token()
		if tokErr != nil {
			if errors.Is(tokErr, io.EOF) {
				break
			}
			return IconDoc{}, fmt.Errorf("%w: 解析 SVG 结构失败：%v", ErrIconInvalid, tokErr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			depth++
			if depth == 1 { // <adago-icon> 包装层
				continue
			}
			if name == "adago-icon" {
				return IconDoc{}, fmt.Errorf("%w: 不支持嵌套 <svg>", ErrIconInvalid)
			}
			if name == "g" {
				// 顶层分组 = 设计器生成的统一描边层，可带描边参数；
				// 嵌套分组只接受 translate（path 位移），其余属性不猜，避免静默丢样式。
				for _, a := range t.Attr {
					if err := checkIconAttr(a); err != nil {
						return IconDoc{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
					}
				}
				tr, hasTr := attrString(t.Attr, "transform")
				off := IconPoint{0, 0}
				if hasTr {
					dx, dy, ok := parseTranslate(tr)
					if !ok {
						return IconDoc{}, fmt.Errorf("%w: 暂不支持分组 transform %q（设计器只还原 translate）", ErrIconInvalid, tr)
					}
					off = IconPoint{dx, dy}
				}
				translates = append(translates, off)
				if depth == 2 {
					if err := applyDocGroupAttrs(&doc, t.Attr); err != nil {
						return IconDoc{}, err
					}
				} else {
					for _, key := range []string{"stroke-width", "stroke", "fill", "opacity", "stroke-linecap", "stroke-linejoin"} {
						if _, ok := attrString(t.Attr, key); ok {
							return IconDoc{}, fmt.Errorf("%w: 暂不支持嵌套分组属性 %s（请展开到具体图形）", ErrIconInvalid, key)
						}
					}
				}
				continue
			}
			if !iconGraphicElements[name] {
				return IconDoc{}, fmt.Errorf("%w: 设计器不支持 <%s>（可改用源码编辑）", ErrIconInvalid, t.Name.Local)
			}
			for _, a := range t.Attr {
				if err := checkIconAttr(a); err != nil {
					return IconDoc{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
				}
			}
			shape, err := shapeFromSVGElement(name, t.Attr, rootFill, rootStrokeOff)
			if err != nil {
				return IconDoc{}, err
			}
			for _, off := range translates {
				shape.DX += off[0]
				shape.DY += off[1]
			}
			doc.Shapes = append(doc.Shapes, shape)
		case xml.EndElement:
			depth--
			if strings.EqualFold(t.Name.Local, "g") && len(translates) > 0 {
				translates = translates[:len(translates)-1]
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return IconDoc{}, fmt.Errorf("%w: 不支持文本节点", ErrIconInvalid)
			}
		}
	}
	if len(doc.Shapes) == 0 {
		return IconDoc{}, fmt.Errorf("%w: 没有可编辑的基础图形", ErrIconInvalid)
	}
	// 导入的图形可能有重叠、超出网格的路径，但仍需通过统一校验（除 path 外）。
	for i := range doc.Shapes {
		doc.Shapes[i].Type = strings.ToLower(doc.Shapes[i].Type)
	}
	if _, err := NormalizeIconDoc(doc); err != nil {
		return IconDoc{}, err
	}
	return doc, nil
}

// applyDocGroupAttrs 读取设计器统一描边层的参数（只认自己导出的那几种）。
func applyDocGroupAttrs(doc *IconDoc, attrs []xml.Attr) error {
	for _, a := range attrs {
		name := strings.ToLower(a.Name.Local)
		v := strings.TrimSpace(a.Value)
		switch name {
		case "transform":
			continue
		case "fill":
			if low := strings.ToLower(v); low != "" && low != "none" {
				return fmt.Errorf("%w: 暂不支持分组 fill=%s（请把填充放到具体图形）", ErrIconInvalid, v)
			}
		case "stroke":
			if low := strings.ToLower(v); low != "" && low != "currentcolor" {
				return fmt.Errorf("%w: 暂不支持分组 stroke=%s（请把描边放到具体图形）", ErrIconInvalid, v)
			}
		case "stroke-width":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f <= 0 {
				return fmt.Errorf("%w: 分组 stroke-width 不合法：%s", ErrIconInvalid, v)
			}
			doc.StrokeWidth = f
		case "stroke-linecap":
			doc.Cap = strings.ToLower(v)
		case "stroke-linejoin":
			doc.Join = strings.ToLower(v)
		case "opacity", "stroke-opacity", "fill-opacity":
			return fmt.Errorf("%w: 暂不支持分组 %s（请放到具体图形）", ErrIconInvalid, name)
		}
	}
	return nil
}

var translateRe = regexp.MustCompile(`^translate\s*\(\s*(-?[0-9.]+)(?:\s*[ ,]\s*(-?[0-9.]+))?\s*\)$`)

func parseTranslate(transform string) (dx, dy float64, ok bool) {
	m := translateRe.FindStringSubmatch(strings.TrimSpace(transform))
	if len(m) == 0 {
		return 0, 0, false
	}
	x, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, 0, false
	}
	y := 0.0
	if len(m) > 2 && m[2] != "" {
		y, err = strconv.ParseFloat(m[2], 64)
		if err != nil {
			return 0, 0, false
		}
	}
	return x, y, true
}

func shapeFromSVGElement(name string, attrs []xml.Attr, rootFill, rootStrokeOff bool) (IconShape, error) {
	if _, ok := attrString(attrs, "transform"); ok {
		return IconShape{}, fmt.Errorf("%w: 暂不支持图形 transform", ErrIconInvalid)
	}
	shape := IconShape{Type: name}
	var err error
	switch name {
	case "line":
		shape.X1, _, err = attrFloat(attrs, "x1")
		if err == nil {
			shape.Y1, _, err = attrFloat(attrs, "y1")
		}
		if err == nil {
			shape.X2, _, err = attrFloat(attrs, "x2")
		}
		if err == nil {
			shape.Y2, _, err = attrFloat(attrs, "y2")
		}
	case "rect":
		shape.X, _, err = attrFloat(attrs, "x")
		if err == nil {
			shape.Y, _, err = attrFloat(attrs, "y")
		}
		if err == nil {
			shape.Width, _, err = attrFloat(attrs, "width")
		}
		if err == nil {
			shape.Height, _, err = attrFloat(attrs, "height")
		}
		if err == nil {
			if rx, has, e := attrFloat(attrs, "rx"); e != nil {
				err = e
			} else if has {
				shape.Radius = rx
			}
		}
	case "circle":
		shape.CX, _, err = attrFloat(attrs, "cx")
		if err == nil {
			shape.CY, _, err = attrFloat(attrs, "cy")
		}
		if err == nil {
			shape.R, _, err = attrFloat(attrs, "r")
		}
	case "ellipse":
		shape.CX, _, err = attrFloat(attrs, "cx")
		if err == nil {
			shape.CY, _, err = attrFloat(attrs, "cy")
		}
		if err == nil {
			shape.RX, _, err = attrFloat(attrs, "rx")
		}
		if err == nil {
			shape.RY, _, err = attrFloat(attrs, "ry")
		}
	case "polyline", "polygon":
		if v, has := attrString(attrs, "points"); has {
			shape.Points, err = parseIconPoints(v)
		} else {
			err = errors.New("缺少 points")
		}
	case "path":
		if v, has := attrString(attrs, "d"); has {
			shape.D = strings.TrimSpace(v)
		} else {
			err = errors.New("缺少 d")
		}
	}
	if err != nil {
		return IconShape{}, fmt.Errorf("%w: 图形 <%s> 解析失败：%v", ErrIconInvalid, name, err)
	}

	// 样式：fill/stroke 显式覆盖；只记布尔，颜色统一走 currentColor。
	// 图形没写 fill/stroke 时继承根级默认值（手写 SVG 的常见写法）。
	if v, has := attrString(attrs, "fill"); has {
		low := strings.ToLower(strings.TrimSpace(v))
		shape.Fill = low != "" && low != "none" && low != "transparent"
	} else if rootFill {
		shape.Fill = true
	}
	if v, has := attrString(attrs, "stroke"); has && strings.ToLower(strings.TrimSpace(v)) == "none" {
		off := false
		shape.Stroke = &off
	} else if !has && rootStrokeOff {
		off := false
		shape.Stroke = &off
	}
	if v, has, e := attrFloat(attrs, "stroke-width"); e != nil {
		return IconShape{}, fmt.Errorf("%w: 图形 <%s> 的 stroke-width 不合法", ErrIconInvalid, name)
	} else if has && v > 0 {
		shape.StrokeW = v
	}
	if v, has, e := attrFloat(attrs, "opacity"); e != nil {
		return IconShape{}, fmt.Errorf("%w: 图形 <%s> 的 opacity 不合法", ErrIconInvalid, name)
	} else if has {
		shape.Opacity = v
	}
	return shape, nil
}

// svgRootAttributes 取原始 <svg> 外壳的根属性（parseIconSVG 只返回正文，会丢根样式）。
func svgRootAttributes(svg string) ([]xml.Attr, bool) {
	s := strings.TrimSpace(svg)
	if !strings.HasPrefix(strings.ToLower(s), "<svg") {
		return nil, false
	}
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		if start, ok := tok.(xml.StartElement); ok {
			if strings.ToLower(start.Name.Local) == "svg" {
				return start.Attr, true
			}
			return nil, false
		}
	}
}

func squareGridFromViewBox(viewBox string) (float64, error) {
	fields := strings.FieldsFunc(viewBox, func(r rune) bool {
		return r == ' ' || r == ','
	})
	if len(fields) != 4 {
		return 0, fmt.Errorf("%w: viewBox 需要 4 个数字", ErrIconInvalid)
	}
	nums := make([]float64, 4)
	for i, f := range fields {
		if !iconViewBoxNumRe.MatchString(f) {
			return 0, fmt.Errorf("%w: viewBox 数字不合法：%s", ErrIconInvalid, f)
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: viewBox 数字不合法：%s", ErrIconInvalid, f)
		}
		nums[i] = v
	}
	if nums[2] <= 0 || nums[2] != nums[3] {
		return 0, fmt.Errorf("%w: 设计器只支持正方形 viewBox（当前 %s）", ErrIconInvalid, viewBox)
	}
	if nums[0] != 0 || nums[1] != 0 {
		return 0, fmt.Errorf("%w: 设计器只支持 viewBox 从 0 0 开始（当前 %s）", ErrIconInvalid, viewBox)
	}
	if nums[2] < IconMinGrid || nums[2] > IconMaxGrid {
		return 0, fmt.Errorf("%w: 网格边长需在 %g–%g", ErrIconInvalid, IconMinGrid, IconMaxGrid)
	}
	return nums[2], nil
}

func parseIconPoints(raw string) ([]IconPoint, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == '\r'
	})
	if len(fields) < 4 || len(fields)%2 != 0 {
		return nil, errors.New("points 必须是偶数个坐标")
	}
	pts := make([]IconPoint, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		x, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, fmt.Errorf("points 第 %d 个 x 不合法", i/2+1)
		}
		y, err := strconv.ParseFloat(fields[i+1], 64)
		if err != nil {
			return nil, fmt.Errorf("points 第 %d 个 y 不合法", i/2+1)
		}
		pts = append(pts, IconPoint{x, y})
	}
	return pts, nil
}

func attrString(attrs []xml.Attr, name string) (string, bool) {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return strings.TrimSpace(a.Value), true
		}
	}
	return "", false
}

func attrFloat(attrs []xml.Attr, name string) (float64, bool, error) {
	v, ok := attrString(attrs, name)
	if !ok || v == "" {
		return 0, false, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, true, fmt.Errorf("%s 不是合法数字：%s", name, v)
	}
	return f, true, nil
}
