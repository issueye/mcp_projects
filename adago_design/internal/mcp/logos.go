package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

// design_logo：品牌 Logo 的生成 / 管理 / 导出 / 摆画布。
//
// 与画布 Logo 工作台共用 design 包的 Logo 资产函数：
//   - preview 是纯渲染，不落库，适合 Agent 先看 SVG/规范；
//   - create/update 才写 logos/<id>.json；
//   - place 把已有 Logo 生成独立 HTML 节点摆上项目画布。

type logoIn struct {
	Action string          `json:"action" jsonschema:"list|get|preview|create|update|remove|spec|export|place"`
	ID     string          `json:"id,omitempty" jsonschema:"Logo ID（get/update/remove/spec/export/place 必填）"`
	Name   string          `json:"name,omitempty" jsonschema:"资产名称；create 可省略（回退到 brand）"`
	Note   string          `json:"note,omitempty" jsonschema:"备注"`
	System string          `json:"system,omitempty" jsonschema:"设计体系 ID，用于默认品牌色"`
	Doc    *design.LogoDoc `json:"doc,omitempty" jsonschema:"完整 Logo 设计文档；提供时优先于 flat 字段"`

	Brand          string  `json:"brand,omitempty" jsonschema:"字标文本"`
	Tagline        string  `json:"tagline,omitempty" jsonschema:"副标文本"`
	MarkIcon       string  `json:"markIcon,omitempty" jsonschema:"标记图标名（design_icons 可检索）"`
	Layout         string  `json:"layout,omitempty" jsonschema:"horizontal|stacked|mark|wordmark"`
	Font           string  `json:"font,omitempty" jsonschema:"字体 ID（design_fonts 可查），默认 inter"`
	Weight         int     `json:"weight,omitempty" jsonschema:"字重 400/500/600/700"`
	Color          string  `json:"color,omitempty" jsonschema:"品牌色 #RRGGBB"`
	Background     string  `json:"background,omitempty" jsonschema:"transparent 或 #RRGGBB"`
	Casing         string  `json:"casing,omitempty" jsonschema:"none|upper"`
	MarkSize       float64 `json:"markSize,omitempty" jsonschema:"标记尺寸 px（默认 40）"`
	FontSize       float64 `json:"fontSize,omitempty" jsonschema:"字标字号 px（默认 36）"`
	TaglineSize    float64 `json:"taglineSize,omitempty" jsonschema:"副标字号 px（缺省 0.3×字标字号）"`
	TaglineWeight  int     `json:"taglineWeight,omitempty" jsonschema:"副标字重 100–900（缺省 500）"`
	TaglineSpacing float64 `json:"taglineSpacing,omitempty" jsonschema:"副标字距 px（缺省 0.12em 自动推算）"`
	TaglineGap     float64 `json:"taglineGap,omitempty" jsonschema:"字标与副标的行盒间距 px（缺省 0.2em 自动推算）"`
	Gap            float64 `json:"gap,omitempty" jsonschema:"标记与字标间距 px"`
	LetterSpacing  float64 `json:"letterSpacing,omitempty" jsonschema:"字距 px"`
	ClearSpace     float64 `json:"clearSpace,omitempty" jsonschema:"安全空间倍数（默认 0.5）"`
	WordmarkWidth  float64 `json:"wordmarkWidth,omitempty" jsonschema:"浏览器实测字宽；缺省按字符估算"`

	Variant     string `json:"variant,omitempty" jsonschema:"positive|inverse|mono，默认 positive"`
	Project     string `json:"project,omitempty" jsonschema:"place 动作必填：目标项目 ID"`
	AddToCanvas *bool  `json:"addToCanvas,omitempty" jsonschema:"create 后是否直接摆上画布（需 project）"`
}

type logoOut struct {
	Logos      []design.Logo     `json:"logos,omitempty"`
	Logo       *design.Logo      `json:"logo,omitempty"`
	Doc        *design.LogoDoc   `json:"doc,omitempty"`
	SVG        string            `json:"svg,omitempty"`
	Variants   map[string]string `json:"variants,omitempty"`
	Spec       *design.LogoSpec  `json:"spec,omitempty"`
	Width      int               `json:"width,omitempty"`
	Height     int               `json:"height,omitempty"`
	NodeID     string            `json:"nodeId,omitempty"`
	CanvasURL  string            `json:"canvasUrl,omitempty"`
	ExportPath string            `json:"exportPath,omitempty"`
}

func logoTool(d Deps) Handler[logoIn, logoOut] {
	return func(ctx context.Context, in logoIn) (*CallToolResult, logoOut, error) {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		fail := func(code, msg, hint string) (*CallToolResult, logoOut, error) {
			return failWith[logoOut](logoOut{}, code, msg, hint)
		}
		needID := action == "get" || action == "update" || action == "remove" ||
			action == "spec" || action == "export" || action == "place"
		if needID && strings.TrimSpace(in.ID) == "" {
			return fail("bad_request", "id 必填", "先用 action=list 查看已有 Logo")
		}

		switch action {
		case "list":
			list := design.AllLogos()
			out := logoOut{Logos: list}
			var sb strings.Builder
			fmt.Fprintf(&sb, "共 %d 个 Logo：\n", len(list))
			for _, l := range list {
				fmt.Fprintf(&sb, "- %s「%s」[%s] %s · 标记 %s · 更新 %s\n",
					l.ID, l.Name, l.System, l.Doc.Layout, logoMarkLabel(l.Doc.Mark), l.UpdatedAt.Format("2006-01-02 15:04"))
			}
			if len(list) == 0 {
				sb.WriteString("还没有 Logo；用 action=create 生成第一个品牌标识。")
			}
			return textResult(strings.TrimRight(sb.String(), "\n")), out, nil

		case "get", "spec":
			l, ok := design.GetLogo(in.ID)
			if !ok {
				return fail("unknown_logo", "未知 Logo "+in.ID, "action=list 查看现有 Logo")
			}
			spec, err := design.LogoDocSpec(l.Doc)
			if err != nil {
				return fail("invalid_logo", err.Error(), "action=update 修正设计稿")
			}
			variants, err := design.LogoVariants(l.Doc)
			if err != nil {
				return fail("invalid_logo", err.Error(), "")
			}
			m, _ := design.LogoLayoutMetrics(l.Doc)
			out := logoOut{Logo: &l, Doc: &l.Doc, Variants: variants, Spec: &spec,
				Width: int(m.Width + 0.5), Height: int(m.Height + 0.5)}
			if action == "get" {
				out.SVG = variants[design.LogoVariantPositive]
				return textResult(fmt.Sprintf("Logo %s「%s」：%s 布局，%dx%d；变体 %s。规范：最小宽 %dpx，安全空间 %g×。",
					l.ID, l.Name, l.Doc.Layout, out.Width, out.Height,
					strings.Join(spec.Variants, "/"), spec.MinWidth, l.Doc.ClearSpace)), out, nil
			}
			return textResult(fmt.Sprintf("Logo %s 规范：%dx%d，最小宽 %dpx，安全空间 %s；字体 %s。%s",
				l.ID, out.Width, out.Height, spec.MinWidth, spec.ClearSpaceRule,
				spec.FontLicense, logoSpecWarnings(spec))), out, nil

		case "preview":
			doc := logoDocFromInput(in)
			norm, err := design.NormalizeLogoDoc(doc)
			if err != nil {
				return fail("invalid_logo", err.Error(), logoDocHint)
			}
			variants, err := design.LogoVariants(norm)
			if err != nil {
				return fail("invalid_logo", err.Error(), logoDocHint)
			}
			spec, err := design.LogoDocSpec(norm)
			if err != nil {
				return fail("invalid_logo", err.Error(), logoDocHint)
			}
			m, err := design.LogoLayoutMetrics(norm)
			if err != nil {
				return fail("invalid_logo", err.Error(), "")
			}
			variant := logoVariantOrDefault(in.Variant)
			return textResult(fmt.Sprintf("已生成 %s 变体预览：%dx%d，最小宽 %dpx，安全空间 %g×。SVG 见 svg 字段。%s",
					variant, int(m.Width+0.5), int(m.Height+0.5), spec.MinWidth, spec.ClearSpace, logoSpecWarnings(spec))),
				logoOut{Doc: &norm, SVG: variants[variant], Variants: variants, Spec: &spec,
					Width: int(m.Width + 0.5), Height: int(m.Height + 0.5)}, nil

		case "create":
			doc := logoDocFromInput(in)
			l, err := design.CreateLogo(design.LogoInput{
				ID: strings.TrimSpace(in.ID), Name: strings.TrimSpace(in.Name),
				Note: strings.TrimSpace(in.Note), System: strings.TrimSpace(in.System), Doc: doc,
			}, d.DataDir)
			if err != nil {
				return fail(logoErrCode(err), err.Error(), logoDocHint)
			}
			spec, _ := design.LogoDocSpec(l.Doc)
			out := logoOut{Logo: &l, Doc: &l.Doc, Spec: &spec}
			text := fmt.Sprintf("已创建 Logo %s「%s」：%s 布局，最小宽 %dpx。",
				l.ID, l.Name, l.Doc.Layout, spec.MinWidth)
			place := in.Project != "" && (in.AddToCanvas == nil || *in.AddToCanvas)
			if place {
				node, nerr := addLogoNode(d, l, logoVariantOrDefault(in.Variant), in.Project)
				if nerr != nil {
					return projectFail[logoOut](out, nerr, in.Project)
				}
				out.NodeID, out.CanvasURL = node.ID, d.BaseURL()
				text += fmt.Sprintf("\n已摆上画布：节点 %s（%s）", node.ID, out.CanvasURL)
			}
			return textResult(text), out, nil

		case "update":
			l, ok := design.GetLogo(in.ID)
			if !ok {
				return fail("unknown_logo", "未知 Logo "+in.ID, "action=list 查看现有 Logo")
			}
			doc := l.Doc
			if in.Doc != nil {
				doc = *in.Doc
			} else {
				applyLogoFlatFields(&doc, in)
			}
			up, err := design.UpdateLogo(in.ID, design.LogoInput{
				Name: strings.TrimSpace(in.Name), Note: strings.TrimSpace(in.Note),
				System: strings.TrimSpace(in.System), Doc: doc,
			}, d.DataDir)
			if err != nil {
				return fail(logoErrCode(err), err.Error(), logoDocHint)
			}
			spec, _ := design.LogoDocSpec(up.Doc)
			return textResult(fmt.Sprintf("已更新 Logo %s「%s」（%s 布局，最小宽 %dpx）。",
					up.ID, up.Name, up.Doc.Layout, spec.MinWidth)),
				logoOut{Logo: &up, Doc: &up.Doc, Spec: &spec}, nil

		case "remove":
			if err := design.RemoveLogo(in.ID, d.DataDir); err != nil {
				return fail(logoErrCode(err), err.Error(), "action=list 查看现有 Logo")
			}
			return textResult("已移除 Logo " + in.ID + "。画布上已摆出的节点不会回收。"), logoOut{}, nil

		case "export":
			svg, filename, err := design.LogoVariantSVG(in.ID, logoVariantOrDefault(in.Variant))
			if err != nil {
				return fail(logoErrCode(err), err.Error(), "")
			}
			path := ""
			if d.DataDir != "" {
				dir := design.LogoExportDir(d.DataDir)
				_ = os.MkdirAll(dir, 0o755)
				path = filepath.Join(dir, filename)
				if werr := os.WriteFile(path, []byte(svg), 0o644); werr != nil {
					return fail("export_failed", "写入 SVG 失败: "+werr.Error(), "")
				}
			}
			return textResult(fmt.Sprintf("已导出 %s。SVG 文件: %s", filename, path)),
				logoOut{SVG: svg, ExportPath: path}, nil

		case "place":
			l, ok := design.GetLogo(in.ID)
			if !ok {
				return fail("unknown_logo", "未知 Logo "+in.ID, "action=list 查看现有 Logo")
			}
			if perr := checkProject(d, in.Project); perr != nil {
				return projectFail[logoOut](logoOut{}, perr, in.Project)
			}
			node, err := addLogoNode(d, l, logoVariantOrDefault(in.Variant), in.Project)
			if err != nil {
				return projectFail[logoOut](logoOut{}, err, in.Project)
			}
			return textResult(fmt.Sprintf("已把 Logo %s 摆上画布：节点 %s（%s）", l.ID, node.ID, d.BaseURL())),
				logoOut{Logo: &l, NodeID: node.ID, CanvasURL: d.BaseURL()}, nil

		default:
			return fail("bad_request", "未知 action: "+in.Action,
				"list|get|preview|create|update|remove|spec|export|place")
		}
	}
}

func logoDocFromInput(in logoIn) design.LogoDoc {
	doc := design.DefaultLogoDoc()
	if in.Doc != nil {
		doc = *in.Doc
		return doc
	}
	if sys, ok := design.GetSystemAny(strings.TrimSpace(in.System)); ok {
		if p := design.PrimaryColor(sys); p != "" {
			doc.Color = p
		}
	}
	applyLogoFlatFields(&doc, in)
	return doc
}

func applyLogoFlatFields(doc *design.LogoDoc, in logoIn) {
	// 任何字标/字体度量输入变化都作废旧字宽，交给服务端重新估算，
	// 否则 Agent 改完品牌名会沿用上一次的 wordmarkWidth 导致错位。
	textChanged := false
	if v := strings.TrimSpace(in.Brand); v != "" {
		doc.Brand = v
		textChanged = true
	}
	if v := strings.TrimSpace(in.Tagline); v != "" {
		doc.Tagline = v
		textChanged = true
	}
	if v := strings.TrimSpace(in.MarkIcon); v != "" {
		doc.Mark = design.LogoMark{Kind: "icon", Icon: strings.ToLower(v)}
	}
	if v := strings.TrimSpace(in.Layout); v != "" {
		doc.Layout = v
	}
	if v := strings.TrimSpace(in.Font); v != "" {
		doc.FontID = v
		textChanged = true
	}
	if in.Weight != 0 {
		doc.Weight = in.Weight
		textChanged = true
	}
	if v := strings.TrimSpace(in.Color); v != "" {
		doc.Color = v
	}
	if v := strings.TrimSpace(in.Background); v != "" {
		doc.Background = v
	}
	if v := strings.TrimSpace(in.Casing); v != "" {
		doc.Casing = v
		textChanged = true
	}
	if in.MarkSize != 0 {
		doc.MarkSize = in.MarkSize
	}
	if in.FontSize != 0 {
		doc.FontSize = in.FontSize
		textChanged = true
	}
	if in.TaglineSize != 0 {
		doc.TaglineSize = in.TaglineSize
		textChanged = true
	}
	if in.TaglineWeight != 0 {
		doc.TaglineWeight = in.TaglineWeight
		textChanged = true
	}
	if in.TaglineSpacing != 0 {
		doc.TaglineSpacing = in.TaglineSpacing
		textChanged = true
	}
	if in.TaglineGap != 0 {
		doc.TaglineGap = in.TaglineGap
	}
	if in.Gap != 0 {
		doc.Gap = in.Gap
	}
	if in.LetterSpacing != 0 {
		doc.LetterSpacing = in.LetterSpacing
		textChanged = true
	}
	if in.ClearSpace != 0 {
		doc.ClearSpace = in.ClearSpace
	}
	if textChanged {
		doc.WordmarkWidth, doc.TaglineWidth = 0, 0
	}
	if in.WordmarkWidth != 0 {
		doc.WordmarkWidth = in.WordmarkWidth
	}
}

func addLogoNode(d Deps, l design.Logo, variant, project string) (canvas.Node, error) {
	html, width, height, err := design.LogoCanvasHTML(l.ID, variant)
	if err != nil {
		return canvas.Node{}, err
	}
	title := "Logo · " + l.Name
	tags := []string{"logo", "logo:" + l.ID, "variant:" + variant, "project:" + project}
	if l.System != "" {
		tags = append(tags, l.System)
	}
	return d.Store.Add(canvas.Node{
		ProjectID: project, Title: title, System: l.System,
		Kind: canvas.KindComponent, HTML: html, Width: width, Height: height, Tags: tags,
	})
}

func logoErrCode(err error) string {
	switch {
	case errors.Is(err, design.ErrLogoNotFound):
		return "unknown_logo"
	case errors.Is(err, design.ErrLogoExists):
		return "logo_duplicate"
	case errors.Is(err, design.ErrLogoInvalid):
		return "invalid_logo"
	default:
		return "logo_failed"
	}
}

func logoVariantOrDefault(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case design.LogoVariantPositive, design.LogoVariantInverse, design.LogoVariantMono:
		return v
	}
	return design.LogoVariantPositive
}

func logoMarkLabel(m design.LogoMark) string {
	switch m.Kind {
	case "icon":
		return "icon:" + m.Icon
	case "shapes":
		return "自绘几何"
	default:
		return "无"
	}
}

func logoSpecWarnings(spec design.LogoSpec) string {
	if len(spec.Warnings) == 0 {
		return ""
	}
	return " [WARN] " + strings.Join(spec.Warnings, "；")
}

const logoDocHint = "LogoDoc: brand/tagline/layout(horizontal|stacked|mark|wordmark)/mark{kind:icon|shapes,icon,design}/" +
	"markSize/gap/font/fontSize/weight/taglineSize/taglineWeight/taglineSpacing/taglineGap/color/background/clearSpace；" +
	"可直接传 doc，也可用 flat 字段生成"
