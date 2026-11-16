package design

// 品牌 Logo 设计文档：把「一个 Logo」拆成可编辑、可校验、可多形态交付的数据。
//
// 与图标设计器的关系：
//   - 标记（mark）可以直接引用图标库（内置/自定义），也可以携带一份 IconDoc
//     自绘几何；画布 UI 的「去画新标记」会先存成自定义图标再引用，保证两端可编辑。
//   - 字标（wordmark）是真实文本 + 已登记字族；SVG 导出保留 <text>，
//     交付前应转曲/嵌入（spec 会提示字体授权与可复现性）。
//
// 布局在服务端计算，宽度依赖浏览器/近似字宽（WordmarkWidth/TaglineWidth）；
// 画布 UI 在每次改字后实测回填，MCP 未提供时用保守估算。这样导出 SVG 与
// HTML 预览不会因为「服务端没有字体度量」而错位。

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Logo 布局与变体。
const (
	LogoLayoutHorizontal = "horizontal" // 标记在左、字标在右
	LogoLayoutStacked    = "stacked"    // 标记在上、字标在下
	LogoLayoutMark       = "mark"       // 只用标记
	LogoLayoutWordmark   = "wordmark"   // 只用字标

	LogoVariantPositive = "positive" // 品牌色 / 指定底色
	LogoVariantInverse  = "inverse"  // 反白 / 品牌色底
	LogoVariantMono     = "mono"     // 单色（黑/白按底色）

	LogoDefaultVersion       = 1
	LogoDefaultColor         = "#16171A"
	LogoDefaultFont          = "inter"
	LogoDefaultFontSize      = 40.0
	LogoDefaultMarkSize      = 44.0
	LogoDefaultGap           = 12.0
	LogoDefaultTaglineSize   = 12.0
	LogoDefaultTaglineWeight = 500
	LogoDefaultClearSpace    = 0.5
	// 副标排印默认值：字距按 0.12em 自动推算，与字标的垂直间距按 0.2em 自动推算。
	LogoTaglineSpacingEm = 0.12
	LogoTaglineGapEm     = 0.20
	LogoMinMarkSize      = 8.0
	LogoMaxMarkSize      = 512.0
	LogoMinFontSize      = 8.0
	LogoMaxFontSize      = 256.0
	LogoMaxBrandRunes    = 64
	LogoMaxTaglineRunes  = 120
)

var (
	logoHexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	logoLayouts    = map[string]bool{
		LogoLayoutHorizontal: true, LogoLayoutStacked: true,
		LogoLayoutMark: true, LogoLayoutWordmark: true,
	}
	logoCasings     = map[string]bool{"none": true, "upper": true}
	logoVariants    = []string{LogoVariantPositive, LogoVariantInverse, LogoVariantMono}
	logoVariantSeen = map[string]bool{
		LogoVariantPositive: true, LogoVariantInverse: true, LogoVariantMono: true,
	}
)

// LogoMark 是 Logo 的标记部分：引用图标库，或携带一份自绘 IconDoc。
type LogoMark struct {
	Kind   string   `json:"kind"`             // icon | shapes
	Icon   string   `json:"icon,omitempty"`   // kind=icon 时的图标名
	Design *IconDoc `json:"design,omitempty"` // kind=shapes 时的几何文档
}

// LogoDoc 是一份品牌 Logo 设计稿。
type LogoDoc struct {
	Version int    `json:"version,omitempty"`
	Brand   string `json:"brand,omitempty"`   // 字标；纯标记布局可为空
	Tagline string `json:"tagline,omitempty"` // 副标（可选）

	Layout string   `json:"layout,omitempty"`
	Mark   LogoMark `json:"mark,omitempty"`

	MarkSize       float64 `json:"markSize,omitempty"` // px @1x
	Gap            float64 `json:"gap,omitempty"`      // 标记与文字的间距
	FontID         string  `json:"font,omitempty"`     // design 字体注册表 ID
	FontSize       float64 `json:"fontSize,omitempty"` // px @1x
	Weight         int     `json:"weight,omitempty"`   // 100–900
	TaglineSize    float64 `json:"taglineSize,omitempty"`
	TaglineWeight  int     `json:"taglineWeight,omitempty"`  // 100–900，默认 500
	TaglineSpacing float64 `json:"taglineSpacing,omitempty"` // px；缺省按 0.12em 推算
	TaglineGap     float64 `json:"taglineGap,omitempty"`     // px；字标与副标的行盒间距，缺省按 0.2em 推算
	LetterSpacing  float64 `json:"letterSpacing,omitempty"`  // px
	Casing         string  `json:"casing,omitempty"`         // none | upper

	Color      string  `json:"color,omitempty"`      // 正形品牌色 #RRGGBB
	Background string  `json:"background,omitempty"` // transparent | #RRGGBB
	ClearSpace float64 `json:"clearSpace,omitempty"` // 留白倍数（相对标记高度）

	// 浏览器实测/近似字宽；服务端用它计算 SVG 的精确横向位置。
	WordmarkWidth float64 `json:"wordmarkWidth,omitempty"`
	TaglineWidth  float64 `json:"taglineWidth,omitempty"`
}

// DefaultLogoDoc 返回一份可直接编辑的 Logo 初值。
func DefaultLogoDoc() LogoDoc {
	return LogoDoc{
		Version: LogoDefaultVersion, Layout: LogoLayoutHorizontal,
		Mark:     LogoMark{Kind: "icon", Icon: "star"},
		MarkSize: LogoDefaultMarkSize, Gap: LogoDefaultGap,
		FontID: LogoDefaultFont, FontSize: LogoDefaultFontSize,
		Weight: 700, TaglineSize: LogoDefaultTaglineSize, TaglineWeight: LogoDefaultTaglineWeight,
		Casing: "none", Color: LogoDefaultColor,
		Background: "transparent", ClearSpace: LogoDefaultClearSpace,
	}
}

// NormalizeLogoDoc 校验并补齐默认值；返回的副本可渲染 / 持久化。
func NormalizeLogoDoc(doc LogoDoc) (LogoDoc, error) {
	doc.Version = LogoDefaultVersion
	doc.Brand = strings.TrimSpace(doc.Brand)
	doc.Tagline = strings.TrimSpace(doc.Tagline)
	if runeLen(doc.Brand) > LogoMaxBrandRunes {
		return LogoDoc{}, fmt.Errorf("%w: 品牌名超过 %d 个字符", ErrLogoInvalid, LogoMaxBrandRunes)
	}
	if runeLen(doc.Tagline) > LogoMaxTaglineRunes {
		return LogoDoc{}, fmt.Errorf("%w: 副标超过 %d 个字符", ErrLogoInvalid, LogoMaxTaglineRunes)
	}
	doc.Layout = strings.ToLower(strings.TrimSpace(doc.Layout))
	if doc.Layout == "" {
		doc.Layout = LogoLayoutHorizontal
	}
	if !logoLayouts[doc.Layout] {
		return LogoDoc{}, fmt.Errorf("%w: layout 只能是 horizontal/stacked/mark/wordmark", ErrLogoInvalid)
	}
	doc.Casing = strings.ToLower(strings.TrimSpace(doc.Casing))
	if doc.Casing == "" {
		doc.Casing = "none"
	}
	if !logoCasings[doc.Casing] {
		return LogoDoc{}, fmt.Errorf("%w: casing 只能是 none/upper", ErrLogoInvalid)
	}
	if doc.MarkSize == 0 {
		doc.MarkSize = LogoDefaultMarkSize
	}
	if doc.MarkSize < LogoMinMarkSize || doc.MarkSize > LogoMaxMarkSize {
		return LogoDoc{}, fmt.Errorf("%w: markSize 必须在 %g–%g", ErrLogoInvalid, LogoMinMarkSize, LogoMaxMarkSize)
	}
	if doc.Gap == 0 {
		doc.Gap = LogoDefaultGap
	}
	if doc.Gap < 0 || doc.Gap > 256 {
		return LogoDoc{}, fmt.Errorf("%w: gap 必须在 0–256", ErrLogoInvalid)
	}
	if doc.FontSize == 0 {
		doc.FontSize = LogoDefaultFontSize
	}
	if doc.FontSize < LogoMinFontSize || doc.FontSize > LogoMaxFontSize {
		return LogoDoc{}, fmt.Errorf("%w: fontSize 必须在 %g–%g", ErrLogoInvalid, LogoMinFontSize, LogoMaxFontSize)
	}
	if doc.TaglineSize == 0 {
		doc.TaglineSize = math.Max(10, doc.FontSize*0.30)
	}
	if doc.TaglineSize < 6 || doc.TaglineSize > 128 {
		return LogoDoc{}, fmt.Errorf("%w: taglineSize 必须在 6–128", ErrLogoInvalid)
	}
	if doc.TaglineWeight == 0 {
		doc.TaglineWeight = LogoDefaultTaglineWeight
	}
	if doc.TaglineWeight < 100 || doc.TaglineWeight > 900 || doc.TaglineWeight%100 != 0 {
		return LogoDoc{}, fmt.Errorf("%w: taglineWeight 必须是 100–900 的百位整数", ErrLogoInvalid)
	}
	// 副标字距与字标间距都按 em 自动推算；显式值优先，便于字标优先/图形优先等预设微调。
	if doc.Tagline != "" && doc.TaglineSpacing == 0 {
		doc.TaglineSpacing = math.Round(doc.TaglineSize*LogoTaglineSpacingEm*100) / 100
	}
	if doc.TaglineSpacing < 0 || doc.TaglineSpacing > 50 {
		return LogoDoc{}, fmt.Errorf("%w: taglineSpacing 必须在 0–50", ErrLogoInvalid)
	}
	if doc.Tagline != "" && doc.TaglineGap == 0 {
		doc.TaglineGap = math.Round(doc.FontSize*LogoTaglineGapEm*100) / 100
	}
	if doc.TaglineGap < 0 || doc.TaglineGap > 256 {
		return LogoDoc{}, fmt.Errorf("%w: taglineGap 必须在 0–256", ErrLogoInvalid)
	}
	if doc.LetterSpacing < -20 || doc.LetterSpacing > 50 {
		return LogoDoc{}, fmt.Errorf("%w: letterSpacing 必须在 -20–50", ErrLogoInvalid)
	}
	doc.FontID = strings.ToLower(strings.TrimSpace(doc.FontID))
	if doc.FontID == "" {
		doc.FontID = LogoDefaultFont
	}
	font, ok := GetFont(doc.FontID)
	if !ok {
		return LogoDoc{}, fmt.Errorf("%w: 未登记字体 %s", ErrLogoInvalid, doc.FontID)
	}
	if doc.Weight == 0 {
		doc.Weight = 700
		if len(font.Weights) > 0 {
			doc.Weight = font.Weights[len(font.Weights)-1]
		}
	}
	if doc.Weight < 100 || doc.Weight > 900 || doc.Weight%100 != 0 {
		return LogoDoc{}, fmt.Errorf("%w: weight 必须是 100–900 的百位整数", ErrLogoInvalid)
	}
	if len(font.Weights) > 0 && !containsInt(font.Weights, doc.Weight) {
		return LogoDoc{}, fmt.Errorf("%w: 字体 %s 只提供 %v", ErrLogoInvalid, doc.FontID, font.Weights)
	}
	doc.Color = strings.TrimSpace(doc.Color)
	if doc.Color == "" {
		doc.Color = LogoDefaultColor
	}
	if !logoHexColorRe.MatchString(doc.Color) {
		return LogoDoc{}, fmt.Errorf("%w: color 必须是 #RRGGBB", ErrLogoInvalid)
	}
	doc.Background = strings.TrimSpace(doc.Background)
	if doc.Background == "" {
		doc.Background = "transparent"
	}
	if !strings.EqualFold(doc.Background, "transparent") && !logoHexColorRe.MatchString(doc.Background) {
		return LogoDoc{}, fmt.Errorf("%w: background 必须是 transparent 或 #RRGGBB", ErrLogoInvalid)
	}
	if strings.EqualFold(doc.Background, "transparent") {
		doc.Background = "transparent"
	}
	if doc.ClearSpace == 0 {
		doc.ClearSpace = LogoDefaultClearSpace
	}
	if doc.ClearSpace < 0 || doc.ClearSpace > 3 {
		return LogoDoc{}, fmt.Errorf("%w: clearSpace 必须在 0–3 倍", ErrLogoInvalid)
	}
	if doc.WordmarkWidth < 0 || doc.WordmarkWidth > 4000 {
		return LogoDoc{}, fmt.Errorf("%w: wordmarkWidth 超出范围", ErrLogoInvalid)
	}
	if doc.TaglineWidth < 0 || doc.TaglineWidth > 4000 {
		return LogoDoc{}, fmt.Errorf("%w: taglineWidth 超出范围", ErrLogoInvalid)
	}

	// 标记：icon 必须存在；shapes 必须通过 IconDoc 校验。
	doc.Mark.Kind = strings.ToLower(strings.TrimSpace(doc.Mark.Kind))
	if doc.Mark.Kind == "" {
		if doc.Mark.Icon != "" {
			doc.Mark.Kind = "icon"
		} else if doc.Mark.Design != nil {
			doc.Mark.Kind = "shapes"
		}
	}
	switch doc.Mark.Kind {
	case "icon":
		doc.Mark.Icon = strings.ToLower(strings.TrimSpace(doc.Mark.Icon))
		if doc.Mark.Icon == "" {
			return LogoDoc{}, fmt.Errorf("%w: 标记图标不能为空", ErrLogoInvalid)
		}
		if !HasIcon(doc.Mark.Icon) {
			return LogoDoc{}, fmt.Errorf("%w: 未知标记图标 %s", ErrLogoInvalid, doc.Mark.Icon)
		}
		doc.Mark.Design = nil
	case "shapes":
		if doc.Mark.Design == nil {
			return LogoDoc{}, fmt.Errorf("%w: kind=shapes 需要 design", ErrLogoInvalid)
		}
		norm, err := NormalizeIconDoc(*doc.Mark.Design)
		if err != nil {
			return LogoDoc{}, err
		}
		doc.Mark.Design = &norm
	case "none":
		// 仅 wordmark 布局允许无标记。
		if doc.Layout != LogoLayoutWordmark {
			return LogoDoc{}, fmt.Errorf("%w: mark.kind=none 只适用于 wordmark 布局", ErrLogoInvalid)
		}
		doc.Mark.Icon, doc.Mark.Design = "", nil
	default:
		return LogoDoc{}, fmt.Errorf("%w: mark.kind 只能是 icon/shapes/none", ErrLogoInvalid)
	}
	if doc.Layout != LogoLayoutWordmark && doc.Mark.Kind == "none" {
		return LogoDoc{}, fmt.Errorf("%w: %s 布局需要标记", ErrLogoInvalid, doc.Layout)
	}
	if doc.Layout == LogoLayoutWordmark && doc.Brand == "" {
		return LogoDoc{}, fmt.Errorf("%w: 纯字标布局需要品牌名", ErrLogoInvalid)
	}
	if doc.WordmarkWidth == 0 && doc.Brand != "" {
		doc.WordmarkWidth = estimateLogoTextWidth(logoCasedText(doc), doc.FontSize, doc.LetterSpacing, doc.Weight)
	}
	if doc.TaglineWidth == 0 && doc.Tagline != "" {
		doc.TaglineWidth = estimateLogoTextWidth(doc.Tagline, doc.TaglineSize, doc.TaglineSpacing, doc.TaglineWeight)
	}
	return doc, nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func runeLen(s string) int { return len([]rune(s)) }

func logoCasedText(doc LogoDoc) string {
	if doc.Casing == "upper" {
		return strings.ToUpper(doc.Brand)
	}
	return doc.Brand
}

// logoGlyphWidths 是 Inter 的无浏览器近似字宽（em）。之前统一按 0.5em 估算
// 拉丁小写，会让字标宽度少算 10–20%，导出的 SVG 右侧留白被吃掉、锁排看起来
// 偏窄。这里按常见字形给出分档值，再按字重做小幅放大。
var logoGlyphWidths = map[rune]float64{
	'a': 0.56, 'b': 0.60, 'c': 0.56, 'd': 0.60, 'e': 0.56, 'f': 0.34,
	'g': 0.60, 'h': 0.60, 'i': 0.26, 'j': 0.28, 'k': 0.54, 'l': 0.26,
	'm': 0.88, 'n': 0.60, 'o': 0.60, 'p': 0.60, 'q': 0.60, 'r': 0.40,
	's': 0.52, 't': 0.36, 'u': 0.60, 'v': 0.54, 'w': 0.80, 'x': 0.52,
	'y': 0.52, 'z': 0.50,
	'A': 0.68, 'B': 0.68, 'C': 0.74, 'D': 0.72, 'E': 0.60, 'F': 0.58,
	'G': 0.74, 'H': 0.74, 'I': 0.30, 'J': 0.50, 'K': 0.68, 'L': 0.56,
	'M': 0.88, 'N': 0.74, 'O': 0.76, 'P': 0.66, 'Q': 0.76, 'R': 0.68,
	'S': 0.62, 'T': 0.60, 'U': 0.72, 'V': 0.68, 'W': 0.96, 'X': 0.68,
	'Y': 0.64, 'Z': 0.62,
	'0': 0.62, '1': 0.44, '2': 0.58, '3': 0.58, '4': 0.60, '5': 0.58,
	'6': 0.58, '7': 0.56, '8': 0.60, '9': 0.60,
	' ': 0.26, '.': 0.26, ',': 0.26, ':': 0.26, ';': 0.26, '-': 0.36,
	'_': 0.52, '/': 0.36, '\\': 0.36, '|': 0.22, '!': 0.26, '?': 0.50,
	'\'': 0.20, '"': 0.36, '(': 0.36, ')': 0.36, '&': 0.68, '%': 0.84,
	'@': 0.86, '#': 0.62, '+': 0.62, '*': 0.46, '=': 0.62,
}

// estimateLogoTextWidth 是无浏览器字体度量时的保守估算：
// 中文/全角按 1em，拉丁按 Inter 的字形近似值，另按字重和 letter-spacing 修正。
func estimateLogoTextWidth(text string, size, spacing float64, weight int) float64 {
	if text == "" || size <= 0 {
		return 0
	}
	var w float64
	for _, r := range text {
		switch {
		case r >= 0x2E80:
			w += 1.0
		default:
			if v, ok := logoGlyphWidths[r]; ok {
				w += v
			} else {
				w += 0.55
			}
		}
	}
	factor := 1.0
	if weight > 400 {
		factor = 1 + float64(weight-400)/400*0.04
	}
	w *= size * factor
	w += spacing * float64(len([]rune(text))-1)
	return math.Round(w*100) / 100
}

// LogoMetrics 是布局计算结果（单位 px @1x）。
type LogoMetrics struct {
	Width, Height  float64
	Pad            float64
	ContentWidth   float64
	ContentHeight  float64
	MarkSize       float64
	TextWidth      float64
	TextHeight     float64
	WordmarkHeight float64
	TaglineHeight  float64
	TaglineGap     float64
	// MarkTop / TextTop 是相对内容区（不含安全空间）的纵向起点，
	// 横排时按字标的视觉中线对齐标记，而不是按「字标+副标」整块居中。
	MarkTop      float64
	TextTop      float64
	HasMark      bool
	HasTagline   bool
	WordmarkText string
}

// LogoLayoutMetrics 计算布局尺寸；供 SVG、HTML、规范共用。
func LogoLayoutMetrics(doc LogoDoc) (LogoMetrics, error) {
	doc, err := NormalizeLogoDoc(doc)
	if err != nil {
		return LogoMetrics{}, err
	}
	m := LogoMetrics{MarkSize: doc.MarkSize, WordmarkText: logoCasedText(doc)}
	m.HasMark = doc.Mark.Kind != "none" && doc.Layout != LogoLayoutWordmark
	m.HasTagline = doc.Tagline != "" && doc.Layout != LogoLayoutMark
	if !m.HasMark {
		m.MarkSize = 0
	}
	m.WordmarkHeight = math.Round(doc.FontSize*1.15*100) / 100
	if m.HasTagline {
		m.TaglineGap = math.Round(doc.TaglineGap*100) / 100
		m.TaglineHeight = math.Round(doc.TaglineSize*1.15*100) / 100
	}
	m.TextHeight = m.WordmarkHeight + m.TaglineGap + m.TaglineHeight
	m.TextWidth = doc.WordmarkWidth
	if doc.TaglineWidth > m.TextWidth {
		m.TextWidth = doc.TaglineWidth
	}
	if doc.Layout == LogoLayoutMark {
		m.TextWidth, m.TextHeight = 0, 0
	}
	if doc.Layout == LogoLayoutWordmark {
		m.MarkSize = 0
	}
	base := doc.MarkSize
	if base == 0 || doc.Layout == LogoLayoutWordmark {
		base = doc.FontSize
	}
	m.Pad = math.Round(doc.ClearSpace*base*100) / 100

	switch doc.Layout {
	case LogoLayoutHorizontal:
		m.ContentWidth = m.MarkSize + doc.Gap + m.TextWidth
		// 先按整块文字居中，再把标记抬到字标行的视觉中线。
		contentH := math.Max(m.MarkSize, m.TextHeight)
		textTop := (contentH - m.TextHeight) / 2
		markTop := textTop + (m.WordmarkHeight-m.MarkSize)/2
		if markTop < 0 {
			// 标记高于字标行：整体下移，保证内容仍在安全空间内。
			contentH += -markTop
			textTop += -markTop
			markTop = 0
		}
		if markTop+m.MarkSize > contentH {
			contentH = markTop + m.MarkSize
		}
		m.ContentHeight = contentH
		m.MarkTop, m.TextTop = markTop, textTop
	case LogoLayoutStacked:
		m.ContentWidth = math.Max(m.MarkSize, m.TextWidth)
		m.ContentHeight = m.MarkSize + doc.Gap + m.TextHeight
		m.MarkTop, m.TextTop = 0, m.MarkSize+doc.Gap
	case LogoLayoutMark:
		m.ContentWidth, m.ContentHeight = m.MarkSize, m.MarkSize
	case LogoLayoutWordmark:
		m.ContentWidth, m.ContentHeight = m.TextWidth, m.TextHeight
	}
	m.Width = m.ContentWidth + m.Pad*2
	m.Height = m.ContentHeight + m.Pad*2
	if m.Width < 4 || m.Height < 4 {
		return LogoMetrics{}, fmt.Errorf("%w: Logo 内容为空（尺寸过小）", ErrLogoInvalid)
	}
	return m, nil
}

// LogoSpec 是品牌 Logo 的交付规范（安全空间 / 最小尺寸 / 变体 / 授权）。
type LogoSpec struct {
	Width          float64            `json:"width"`
	Height         float64            `json:"height"`
	ClearSpace     float64            `json:"clearSpace"`
	ClearSpaceRule string             `json:"clearSpaceRule"`
	MinWidth       int                `json:"minWidth"`
	MinHeight      int                `json:"minHeight"`
	Variants       []string           `json:"variants"`
	FontID         string             `json:"font"`
	FontLicense    string             `json:"fontLicense"`
	Contrast       map[string]float64 `json:"contrast,omitempty"`
	Warnings       []string           `json:"warnings,omitempty"`
	Usage          []string           `json:"usage"`
}

// LogoDocSpec 计算规范。
func LogoDocSpec(doc LogoDoc) (LogoSpec, error) {
	norm, err := NormalizeLogoDoc(doc)
	if err != nil {
		return LogoSpec{}, err
	}
	m, err := LogoLayoutMetrics(norm)
	if err != nil {
		return LogoSpec{}, err
	}
	font, _ := GetFont(norm.FontID)
	small := math.Min(math.Max(norm.MarkSize, 1), math.Max(norm.FontSize, 1))
	if norm.Layout == LogoLayoutMark {
		small = norm.MarkSize
	}
	if norm.Layout == LogoLayoutWordmark {
		small = norm.FontSize
	}
	minW := int(math.Ceil(m.ContentWidth * 16 / math.Max(small, 1)))
	if minW < 24 {
		minW = 24
	}
	minH := int(math.Ceil(m.ContentHeight * 16 / math.Max(small, 1)))
	if minH < 8 {
		minH = 8
	}
	spec := LogoSpec{
		Width: m.Width, Height: m.Height, ClearSpace: m.Pad,
		ClearSpaceRule: fmt.Sprintf("四周留白 ≥ %g×标记高度（当前 %.0fpx）", norm.ClearSpace, m.Pad),
		MinWidth:       minW, MinHeight: minH, Variants: append([]string{}, logoVariants...),
		FontID: norm.FontID,
		Usage: []string{
			"最小宽度按「标记/字标 ≥16px」换算；更小尺寸请改用 mark-only 变体。",
			"正形用于浅底，反形用于品牌色/深底，单色用于单套印刷与低色域场景。",
			"副标默认 500 字重、0.12em 字距，与字标行盒间距 0.2em；可在设计器中微调。",
			"导出 SVG 的字标仍是文本；交付生产前请转曲，或确认字体已授权/嵌入。",
		},
	}
	if font.ID != "" {
		spec.FontLicense = fmt.Sprintf("%s · %s · 可商用=%v · 可嵌入=%v", font.License, font.Family, font.Commercial, font.Embeddable)
		if !font.Commercial {
			spec.Warnings = append(spec.Warnings, "字体未标记为可商用，交付前需替换或取得授权")
		}
	}
	bg := norm.Background
	if bg == "transparent" {
		bg = "#FBF8F1"
	}
	spec.Contrast = map[string]float64{
		LogoVariantPositive: logoContrast(norm.Color, bg),
		LogoVariantInverse:  logoContrast(logoInverseInk(norm.Color), norm.Color),
	}
	for _, v := range []string{LogoVariantPositive, LogoVariantInverse} {
		if spec.Contrast[v] < 3.0 {
			spec.Warnings = append(spec.Warnings, fmt.Sprintf("%s 变体对比度 %.2f < 3.0，请调整品牌色或底色", v, spec.Contrast[v]))
		}
	}
	// 比例走查：比例失衡是「看起来丑」的主要原因，给出可执行的下一步建议。
	if norm.FontSize > 0 && norm.Layout != LogoLayoutWordmark {
		if markRatio := norm.MarkSize / norm.FontSize; markRatio < 0.6 || markRatio > 1.8 {
			spec.Warnings = append(spec.Warnings, fmt.Sprintf(
				"标记/字标比例 %.2f 失衡；建议 0.9–1.3，当前可设标记约 %.0fpx",
				markRatio, norm.FontSize*1.1))
		}
	}
	if norm.FontSize > 0 && (norm.Layout == LogoLayoutHorizontal || norm.Layout == LogoLayoutStacked) {
		if gapRatio := norm.Gap / norm.FontSize; gapRatio < 0.15 || gapRatio > 0.75 {
			adj := "近"
			if gapRatio > 0.75 {
				adj = "远"
			}
			spec.Warnings = append(spec.Warnings, fmt.Sprintf(
				"标记与字标间距 %.2fem 过%s；建议 0.25–0.5em（约 %.0fpx）",
				gapRatio, adj, norm.FontSize*0.35))
		}
	}
	if norm.Tagline != "" && norm.Layout != LogoLayoutMark && norm.FontSize > 0 {
		if ratio := norm.TaglineSize / norm.FontSize; ratio < 0.18 || ratio > 0.45 {
			spec.Warnings = append(spec.Warnings, fmt.Sprintf(
				"副标/字标比例 %.2f 失衡；建议 0.25–0.35（当前约 %.1fpx）", ratio, norm.FontSize*0.3))
		}
	}
	if norm.FontSize > 0 && norm.LetterSpacing != 0 {
		if ratio := norm.LetterSpacing / norm.FontSize; ratio < -0.08 || ratio > 0.15 {
			spec.Warnings = append(spec.Warnings, fmt.Sprintf(
				"字标字距 %.2fem 偏离常规；建议 -0.05–0.1em。", ratio))
		}
	}
	if norm.Brand == "" && norm.Layout != LogoLayoutMark {
		spec.Warnings = append(spec.Warnings, "字标为空，当前组合布局只剩标记")
	}
	return spec, nil
}

// LogoVariants 渲染三个固定变体（正形 / 反形 / 单色）。
func LogoVariants(doc LogoDoc) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range logoVariants {
		svg, err := LogoSVG(doc, v)
		if err != nil {
			return nil, err
		}
		out[v] = svg
	}
	return out, nil
}

// LogoSVG 渲染指定变体的完整 SVG。
func LogoSVG(doc LogoDoc, variant string) (string, error) {
	norm, err := NormalizeLogoDoc(doc)
	if err != nil {
		return "", err
	}
	variant = strings.ToLower(strings.TrimSpace(variant))
	if variant == "" {
		variant = LogoVariantPositive
	}
	if !logoVariantSeen[variant] {
		return "", fmt.Errorf("%w: variant 只能是 positive/inverse/mono", ErrLogoInvalid)
	}
	m, err := LogoLayoutMetrics(norm)
	if err != nil {
		return "", err
	}
	ink, bg := logoVariantColors(norm, variant)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" role="img" aria-label="%s">`,
		fnum(m.Width), fnum(m.Height), fnum(m.Width), fnum(m.Height), xmlEscape(logoAriaLabel(norm)))
	if bg != "" && !strings.EqualFold(bg, "transparent") {
		fmt.Fprintf(&b, `<rect x="0" y="0" width="%s" height="%s" fill="%s"/>`, fnum(m.Width), fnum(m.Height), bg)
	}
	if m.HasMark {
		markX, markY, markSize := logoMarkBox(norm, m)
		mark, err := logoMarkElement(norm, ink, markX, markY, markSize)
		if err != nil {
			return "", err
		}
		b.WriteString(mark)
	}
	switch norm.Layout {
	case LogoLayoutHorizontal:
		textX := m.Pad + m.MarkSize + norm.Gap
		b.WriteString(logoTextElements(norm, m, ink, textX, m.Pad+m.TextTop, m.TextWidth, "start"))
	case LogoLayoutStacked:
		b.WriteString(logoTextElements(norm, m, ink, m.Width/2, m.Pad+m.TextTop, m.ContentWidth, "middle"))
	case LogoLayoutMark:
		// 无字标
	case LogoLayoutWordmark:
		b.WriteString(logoTextElements(norm, m, ink, m.Width/2, m.Pad+m.TextTop, m.ContentWidth, "middle"))
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func logoMarkBox(doc LogoDoc, m LogoMetrics) (x, y, size float64) {
	size = m.MarkSize
	switch doc.Layout {
	case LogoLayoutHorizontal:
		x = m.Pad
		y = m.Pad + m.MarkTop
	case LogoLayoutStacked:
		x = m.Pad + (m.ContentWidth-m.MarkSize)/2
		y = m.Pad + m.MarkTop
	case LogoLayoutMark:
		x = m.Pad + (m.ContentWidth-m.MarkSize)/2
		y = m.Pad + (m.ContentHeight-m.MarkSize)/2
	}
	return
}

func logoTextElements(doc LogoDoc, m LogoMetrics, ink string, x, top, width float64, anchor string) string {
	font, _ := GetFont(doc.FontID)
	family := logoFontStack(font)
	var b strings.Builder
	if doc.Brand != "" {
		baseline := top + doc.FontSize*0.82
		fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="%s" font-family="%s" font-size="%s" font-weight="%d" fill="%s"`,
			fnum(x), fnum(baseline), anchor, xmlEscape(family), fnum(doc.FontSize), doc.Weight, ink)
		if doc.LetterSpacing != 0 {
			fmt.Fprintf(&b, ` letter-spacing="%s"`, fnum(doc.LetterSpacing))
		}
		b.WriteString(`>` + xmlEscape(logoCasedText(doc)) + `</text>`)
	}
	if m.HasTagline {
		baseline := top + m.WordmarkHeight + doc.TaglineGap + doc.TaglineSize*0.82
		fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="%s" font-family="%s" font-size="%s" font-weight="%d" fill="%s"`,
			fnum(x), fnum(baseline), anchor, xmlEscape(family), fnum(doc.TaglineSize), doc.TaglineWeight, ink)
		if doc.TaglineSpacing != 0 {
			fmt.Fprintf(&b, ` letter-spacing="%s"`, fnum(doc.TaglineSpacing))
		}
		b.WriteString(`>` + xmlEscape(doc.Tagline) + `</text>`)
	}
	return b.String()
}

func logoMarkElement(doc LogoDoc, ink string, x, y, size float64) (string, error) {
	body := ""
	viewBox := IconDefaultViewBox
	strokeW := "1.8"
	switch doc.Mark.Kind {
	case "icon":
		ic, ok := GetIcon(doc.Mark.Icon)
		if !ok {
			return "", fmt.Errorf("%w: 未知标记图标 %s", ErrLogoInvalid, doc.Mark.Icon)
		}
		body, viewBox = ic.Body, ic.ViewBox
	case "shapes":
		if doc.Mark.Design == nil {
			return "", fmt.Errorf("%w: 标记缺少设计文档", ErrLogoInvalid)
		}
		norm, err := NormalizeIconDoc(*doc.Mark.Design)
		if err != nil {
			return "", err
		}
		body, viewBox = IconDocBody(norm), "0 0 "+fnum(norm.Grid)+" "+fnum(norm.Grid)
		strokeW = fnum(norm.StrokeWidth)
	default:
		return "", nil
	}
	return fmt.Sprintf(`<svg x="%s" y="%s" width="%s" height="%s" viewBox="%s" `+
		`fill="none" stroke="%s" color="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		fnum(x), fnum(y), fnum(size), fnum(size), viewBox, ink, ink, strokeW, body), nil
}

// LogoSVGHTML 给画布/MCP 用的独立 HTML 文档：中央展示一个变体，尺寸即 SVG 尺寸。
func LogoHTML(doc LogoDoc, variant string) (html string, width, height int, err error) {
	norm, err := NormalizeLogoDoc(doc)
	if err != nil {
		return "", 0, 0, err
	}
	svg, err := LogoSVG(norm, variant)
	if err != nil {
		return "", 0, 0, err
	}
	m, err := LogoLayoutMetrics(norm)
	if err != nil {
		return "", 0, 0, err
	}
	width = int(math.Ceil(m.Width))
	height = int(math.Ceil(m.Height))
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	bg := "#FBF8F1"
	if variant == LogoVariantInverse {
		bg = norm.Color
	}
	html = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><style>` +
		`*{box-sizing:border-box}html,body{margin:0;width:100%;height:100%}` +
		`body{display:flex;align-items:center;justify-content:center;background:` + bg + `;overflow:hidden}` +
		`svg{display:block;max-width:100%;max-height:100%}` +
		`</style></head><body>` + svg + `</body></html>`
	return html, width, height, nil
}

// LogoVariantsForHTML 生成三变体的画布 HTML（管理端预览/导出用）。
func LogoVariantsForHTML(doc LogoDoc) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range logoVariants {
		html, _, _, err := LogoHTML(doc, v)
		if err != nil {
			return nil, err
		}
		out[v] = html
	}
	return out, nil
}

func logoVariantColors(doc LogoDoc, variant string) (ink, bg string) {
	switch variant {
	case LogoVariantInverse:
		return logoInverseInk(doc.Color), doc.Color
	case LogoVariantMono:
		background := doc.Background
		if strings.EqualFold(background, "transparent") {
			return "#16171A", "transparent"
		}
		if logoLuminance(background) < 0.45 {
			return "#F6F3EC", background
		}
		return "#16171A", background
	default:
		return doc.Color, doc.Background
	}
}

func logoInverseInk(color string) string {
	if logoLuminance(color) > 0.55 {
		return "#16171A"
	}
	return "#F6F3EC"
}

func logoAriaLabel(doc LogoDoc) string {
	if doc.Brand != "" {
		return doc.Brand + " logo"
	}
	return "brand logo"
}

func logoFontStack(f FontSpec) string {
	family := strings.ReplaceAll(quoteFamily(f.Family), `"`, `'`)
	return family + ", sans-serif"
}

// ---------------- 颜色与对比 ----------------

func logoLuminance(hex string) float64 {
	if !logoHexColorRe.MatchString(hex) {
		return 0.5
	}
	h := strings.TrimPrefix(hex, "#")
	vals := make([]float64, 3)
	for i := 0; i < 3; i++ {
		v := parseHexByte(h[i*2 : i*2+2])
		vals[i] = v / 255.0
	}
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(vals[0]) + 0.7152*lin(vals[1]) + 0.0722*lin(vals[2])
}

func parseHexByte(s string) float64 {
	var v int
	for i := 0; i < 2; i++ {
		c := s[i]
		v *= 16
		switch {
		case c >= '0' && c <= '9':
			v += int(c - '0')
		case c >= 'a' && c <= 'f':
			v += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v += int(c-'A') + 10
		}
	}
	return float64(v)
}

func logoContrast(fg, bg string) float64 {
	l1, l2 := logoLuminance(fg), logoLuminance(bg)
	hi, lo := math.Max(l1, l2), math.Min(l1, l2)
	return math.Round(((hi+0.05)/(lo+0.05))*100) / 100
}

// ---------------- 小工具 ----------------

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// LogoSortedVariants 返回稳定顺序（测试/序列化用）。
func LogoSortedVariants() []string {
	out := append([]string{}, logoVariants...)
	sort.Strings(out)
	return out
}

// LogoVariantLabel 返回变体的中文标签。
func LogoVariantLabel(v string) string {
	switch v {
	case LogoVariantPositive:
		return "正形"
	case LogoVariantInverse:
		return "反形"
	case LogoVariantMono:
		return "单色"
	}
	return v
}
