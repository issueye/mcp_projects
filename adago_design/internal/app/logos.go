package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

// 品牌 Logo 工作台：把 design 包的 Logo 资产能力接到画布 HTTP 端点。
// 与图标/体系管理同一套 AdminError 约定（稳定 code + 中文提示）。

func logoAdminErr(err error) error {
	if err == nil {
		return nil
	}
	code := "internal"
	hint := ""
	msg := err.Error()
	switch {
	case errors.Is(err, design.ErrLogoNotFound):
		code = "unknown_logo"
		msg = "未知 Logo：" + strings.TrimPrefix(msg, design.ErrLogoNotFound.Error()+": ")
		hint = "用 GET /api/catalog/logos 查看现有 Logo"
	case errors.Is(err, design.ErrLogoExists):
		code = "logo_duplicate"
		msg = "Logo ID 已被占用：" + strings.TrimPrefix(msg, design.ErrLogoExists.Error()+": ")
		hint = "换一个名称/ID，或先移除同 ID 的 Logo"
	case errors.Is(err, design.ErrLogoInvalid):
		code = "invalid_logo"
		msg = strings.TrimPrefix(msg, design.ErrLogoInvalid.Error()+": ")
		hint = "检查布局/字体/颜色/标记；字体必须来自 /api/catalog/fonts"
	default:
		return &canvas.AdminError{Code: code, Message: err.Error()}
	}
	return &canvas.AdminError{Code: code, Message: msg, Hint: hint}
}

func logoFromRaw(raw json.RawMessage) (design.LogoDoc, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return design.LogoDoc{}, fmt.Errorf("%w: doc 必填", design.ErrLogoInvalid)
	}
	var doc design.LogoDoc
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return design.LogoDoc{}, fmt.Errorf("%w: doc JSON 解析失败：%v", design.ErrLogoInvalid, err)
	}
	return doc, nil
}

// FontCatalog 返回字体注册表（Logo 字体选择与授权提示共用）。
func (Catalog) FontCatalog() any {
	out := make([]map[string]any, 0)
	for _, f := range design.Fonts() {
		out = append(out, map[string]any{
			"id": f.ID, "family": f.Family, "script": f.Script,
			"license": f.License, "commercial": f.Commercial,
			"embeddable": f.Embeddable, "redistributable": f.Redistributable,
			"attribution": f.Attribution, "weights": f.Weights, "homepage": f.Homepage,
		})
	}
	return out
}

func logoSummary(l design.Logo) map[string]any {
	svg, _ := design.LogoSVG(l.Doc, design.LogoVariantPositive)
	return map[string]any{
		"id":        l.ID,
		"name":      l.Name,
		"note":      l.Note,
		"system":    l.System,
		"brand":     l.Doc.Brand,
		"layout":    l.Doc.Layout,
		"mark":      l.Doc.Mark,
		"createdAt": l.CreatedAt,
		"updatedAt": l.UpdatedAt,
		"svg":       svg,
	}
}

// LogoSummaries 返回全部 Logo（按更新时间倒序）。
func (Catalog) LogoSummaries() any {
	out := make([]map[string]any, 0)
	for _, l := range design.AllLogos() {
		out = append(out, logoSummary(l))
	}
	return out
}

// LogoDetail 返回 Logo 完整上下文：doc + 三变体 + 规范 + 尺寸。
func (Catalog) LogoDetail(id string) (any, error) {
	l, ok := design.GetLogo(id)
	if !ok {
		return nil, logoAdminErr(fmt.Errorf("%w: %s", design.ErrLogoNotFound, id))
	}
	variants, err := design.LogoVariants(l.Doc)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	spec, err := design.LogoDocSpec(l.Doc)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	m, err := design.LogoLayoutMetrics(l.Doc)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	return map[string]any{
		"logo":     logoSummary(l),
		"doc":      l.Doc,
		"variants": variants,
		"spec":     spec,
		"width":    m.Width,
		"height":   m.Height,
	}, nil
}

// CreateLogo 新建 Logo 资产。
func (c Catalog) CreateLogo(in canvas.LogoInput) (any, error) {
	doc, err := logoFromRaw(in.Doc)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	l, err := design.CreateLogo(design.LogoInput{
		ID: strings.TrimSpace(in.ID), Name: strings.TrimSpace(in.Name),
		Note: strings.TrimSpace(in.Note), System: strings.TrimSpace(in.System), Doc: doc,
	}, c.DataDir)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	return logoSummary(l), nil
}

// UpdateLogo 全量更新 Logo。
func (c Catalog) UpdateLogo(id string, in canvas.LogoInput) (any, error) {
	doc, err := logoFromRaw(in.Doc)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	l, err := design.UpdateLogo(id, design.LogoInput{
		Name: strings.TrimSpace(in.Name), Note: strings.TrimSpace(in.Note),
		System: strings.TrimSpace(in.System), Doc: doc,
	}, c.DataDir)
	if err != nil {
		return nil, logoAdminErr(err)
	}
	return logoSummary(l), nil
}

// RemoveLogo 删除 Logo 资产。
func (c Catalog) RemoveLogo(id string) error {
	if err := design.RemoveLogo(id, c.DataDir); err != nil {
		return logoAdminErr(err)
	}
	return nil
}

// LogoExport 返回 SVG 文件内容与文件名。
func (Catalog) LogoExport(id, variant string) (string, string, error) {
	svg, filename, err := design.LogoVariantSVG(id, variant)
	if err != nil {
		return "", "", logoAdminErr(err)
	}
	return svg, filename, nil
}

// LogoCanvas 返回可直接摆上画布的独立 HTML。
func (Catalog) LogoCanvas(id, variant string) (string, int, int, error) {
	html, w, h, err := design.LogoCanvasHTML(id, variant)
	if err != nil {
		return "", 0, 0, logoAdminErr(err)
	}
	return html, w, h, nil
}
