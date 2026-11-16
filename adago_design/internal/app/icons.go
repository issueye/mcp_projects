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

// SVG 图标管理：把 design 包的图标注册表管理能力接到画布 HTTP 管理端点。
// 与设计体系管理同一套 AdminError 约定（稳定 code + 中文提示）。

func iconAdminErr(err error) error {
	code := "internal"
	hint := ""
	msg := err.Error()
	switch {
	case errors.Is(err, design.ErrIconNotFound):
		code = "unknown_icon"
		msg = strings.TrimPrefix(msg, design.ErrIconNotFound.Error()+": ")
		hint = "用 GET /api/catalog/icons 查看现有图标"
	case errors.Is(err, design.ErrIconBuiltin):
		code = "icon_builtin_protected"
		msg = strings.TrimPrefix(msg, design.ErrIconBuiltin.Error()+": ")
		hint = "内置图标 SVG 只读、不可删除；可停用或改分组/别名"
	case errors.Is(err, design.ErrIconExists):
		code = "icon_duplicate"
		msg = "名称已被占用：" + strings.TrimPrefix(msg, design.ErrIconExists.Error()+": ")
		hint = "换一个名称，或先移除同名自定义图标"
	case errors.Is(err, design.ErrIconInvalid):
		code = "invalid_icon"
		msg = strings.TrimPrefix(msg, design.ErrIconInvalid.Error()+": ")
	default:
		return &canvas.AdminError{Code: code, Message: err.Error()}
	}
	return &canvas.AdminError{Code: code, Message: msg, Hint: hint}
}

// iconSummary 生成管理界面与预览共用的一份图标摘要。
func iconSummary(ic design.Icon) map[string]any {
	builtin, off, over, _ := design.IconState(ic.Name)
	return map[string]any{
		"name":       ic.Name,
		"group":      ic.Group,
		"tags":       ic.Tags,
		"viewBox":    ic.ViewBox,
		"builtin":    builtin,
		"disabled":   off,
		"overridden": over,
		"svg":        design.IconInline(ic.Name, 24),
		"usage":      design.IconUsage(ic.Name),
		"designed":   ic.Designed,
		"filled":     ic.Filled,
	}
}

// designFromRaw 把 HTTP 入参里的 design 原样解析成设计文档（缺省 / null 视为未提供）。
func designFromRaw(raw json.RawMessage) (*design.IconDoc, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var doc design.IconDoc
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return nil, fmt.Errorf("%w: design JSON 解析失败：%v", design.ErrIconInvalid, err)
	}
	return &doc, nil
}

// IconSummariesAll 返回全部图标（含停用），管理弹层用。
func (Catalog) IconSummariesAll() any {
	out := make([]map[string]any, 0)
	for _, ic := range design.AllIcons() {
		out = append(out, iconSummary(ic))
	}
	return out
}

// IconDetail 返回单个图标的详情（含 SVG 源码）。
func (c Catalog) IconDetail(name string) (any, error) {
	ic, ok := design.GetIcon(name)
	if !ok {
		return nil, iconAdminErr(fmt.Errorf("%w: %s", design.ErrIconNotFound, name))
	}
	src, err := design.IconSource(name)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	detail := iconSummary(ic)
	detail["source"] = src
	if info, derr := design.IconDesign(ic.Name, c.DataDir); derr == nil {
		detail["design"] = info.Doc
		detail["designImported"] = info.Imported
		detail["designEditable"] = !info.Builtin
		detail["designed"] = info.Designed
	} else {
		detail["designError"] = derr.Error()
	}
	return detail, nil
}

// CreateIcon 新增一个自定义图标。
func (c Catalog) CreateIcon(in canvas.IconInput) (any, error) {
	doc, err := designFromRaw(in.Design)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	ic, err := design.CreateIcon(design.IconInput{
		Name:   strings.TrimSpace(in.Name),
		Group:  in.Group,
		Tags:   in.Tags,
		SVG:    in.SVG,
		Design: doc,
	}, c.DataDir)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	return iconSummary(ic), nil
}

// UpdateIcon 编辑图标：内置只改分组/别名，自定义可整体替换 SVG。
func (c Catalog) UpdateIcon(name string, in canvas.IconInput) (any, error) {
	doc, err := designFromRaw(in.Design)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	patch := design.IconPatch{}
	patch.SVG = in.SVG
	patch.Design = doc
	if g := strings.TrimSpace(in.Group); g != "" {
		patch.Group = &g
	}
	if in.Tags != nil {
		patch.Tags = in.Tags
		patch.TagsSet = true
	}
	ic, err := design.UpdateIcon(name, c.DataDir, patch)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	return iconSummary(ic), nil
}

// IconDesign 返回设计器上下文（几何文档 + 只读/反推标记）。
func (c Catalog) IconDesign(name string) (any, error) {
	info, err := design.IconDesign(name, c.DataDir)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	ic, _ := design.GetIcon(strings.TrimSpace(name))
	return map[string]any{
		"name":     ic.Name,
		"group":    ic.Group,
		"tags":     ic.Tags,
		"design":   info.Doc,
		"imported": info.Imported,
		"builtin":  info.Builtin,
		"designed": info.Designed,
		"editable": !info.Builtin,
	}, nil
}

// SetIconDisabled 停用 / 启用图标。
func (c Catalog) SetIconDisabled(name string, disabled bool) (any, error) {
	ic, err := design.SetIconDisabled(name, c.DataDir, disabled)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	return iconSummary(ic), nil
}

// ResetIcon 内置图标恢复默认分组/别名并重新启用。
func (c Catalog) ResetIcon(name string) (any, error) {
	ic, err := design.ResetIcon(name, c.DataDir)
	if err != nil {
		return nil, iconAdminErr(err)
	}
	return iconSummary(ic), nil
}

// RemoveIcon 删除自定义图标（内置图标受保护）。
func (c Catalog) RemoveIcon(name string) error {
	if err := design.RemoveIcon(name, c.DataDir); err != nil {
		return iconAdminErr(err)
	}
	return nil
}
