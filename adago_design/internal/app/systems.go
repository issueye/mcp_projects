package app

import (
	"errors"
	"strings"

	"adagodesign/internal/canvas"
	"adagodesign/internal/design"
)

// 设计体系管理：把 design 包的管理能力接到画布 HTTP 管理端点上。
// 错误统一翻译成 canvas.AdminError（稳定 code + 中文提示），由 canvas 映射状态码。

func adminErr(err error) error {
	code := "internal"
	hint := ""
	msg := err.Error()
	switch {
	case errors.Is(err, design.ErrSystemNotFound):
		code = "unknown_system"
		msg = strings.TrimPrefix(msg, design.ErrSystemNotFound.Error()+": ")
		hint = "用 GET /api/catalog/systems?all=1 查看现有体系"
	case errors.Is(err, design.ErrSystemBuiltin):
		code = "system_builtin_protected"
		msg = strings.TrimPrefix(msg, design.ErrSystemBuiltin.Error()+": ")
		hint = "内置体系不可删除；可停用或改显示信息（POST .../state、PUT ...）"
	case errors.Is(err, design.ErrSystemExists):
		code = "system_duplicate"
		msg = strings.TrimPrefix(msg, design.ErrSystemExists.Error()+": ")
	case errors.Is(err, design.ErrSystemInvalid):
		code = "invalid_system"
		msg = strings.TrimPrefix(msg, design.ErrSystemInvalid.Error()+": ")
	default:
		return &canvas.AdminError{Code: code, Message: err.Error()}
	}
	return &canvas.AdminError{Code: code, Message: msg, Hint: hint}
}

// SystemSummariesAll 返回全部体系（含停用），管理弹层用。
func (Catalog) SystemSummariesAll() any {
	out := make([]map[string]any, 0)
	for _, sys := range design.AllSystems() {
		out = append(out, systemSummary(sys))
	}
	return out
}

// SystemJSON 返回自定义体系的完整 JSON（高级编辑）。
func (Catalog) SystemJSON(id string) (string, error) {
	raw, err := design.SystemJSON(id)
	if err != nil {
		return "", adminErr(err)
	}
	return raw, nil
}

// Palette 返回品牌主色的 10 阶色板（新增表单的实时预览）。
func (Catalog) Palette(primary string) ([]string, error) {
	hex := strings.TrimSpace(primary)
	if hex == "" {
		return nil, &canvas.AdminError{Code: "invalid_system", Message: "primary 必填"}
	}
	if !strings.HasPrefix(hex, "#") {
		hex = "#" + hex
	}
	scale, ok := design.BrandPalette(hex)
	if !ok {
		return nil, &canvas.AdminError{Code: "invalid_system", Message: "主色必须是 #RRGGBB，得到 " + primary}
	}
	return scale, nil
}

// CreateSystem 新增一个品牌派生体系（结构继承 base，主色重着色）。
func (c Catalog) CreateSystem(in canvas.SystemInput) (any, error) {
	sys, err := design.CreateSystem(design.CreateInput{
		ID:          in.ID,
		Name:        in.Name,
		Vendor:      in.Vendor,
		Family:      in.Family,
		Description: in.Description,
		Base:        in.Base,
		Primary:     in.Primary,
	}, c.DataDir)
	if err != nil {
		return nil, adminErr(err)
	}
	return systemSummary(sys), nil
}

// UpdateSystem 编辑体系：JSON 非空时整体替换（仅自定义），否则改显示信息。
func (c Catalog) UpdateSystem(id string, in canvas.SystemInput) (any, error) {
	if strings.TrimSpace(in.JSON) != "" {
		sys, err := design.UpdateCustomSystemJSON(id, c.DataDir, in.JSON)
		if err != nil {
			return nil, adminErr(err)
		}
		return systemSummary(sys), nil
	}
	sys, err := design.UpdateSystemMeta(id, c.DataDir, metaPatchFromInput(in))
	if err != nil {
		return nil, adminErr(err)
	}
	return systemSummary(sys), nil
}

// metaPatchFromInput 只把非空字段当成「要改」：空串保留现值（清空覆盖用
// 内置体系的「恢复默认」，自定义体系用 JSON 整体编辑）。
func metaPatchFromInput(in canvas.SystemInput) design.MetaPatch {
	var p design.MetaPatch
	if v := strings.TrimSpace(in.Name); v != "" {
		p.Name = &v
	}
	if v := strings.TrimSpace(in.Vendor); v != "" {
		p.Vendor = &v
	}
	if v := strings.TrimSpace(in.Description); v != "" {
		p.Description = &v
	}
	if v := strings.TrimSpace(in.Summary); v != "" {
		p.Summary = &v
	}
	return p
}

// SetSystemDisabled 停用 / 启用体系。
func (c Catalog) SetSystemDisabled(id string, disabled bool) (any, error) {
	sys, err := design.SetSystemDisabled(id, c.DataDir, disabled)
	if err != nil {
		return nil, adminErr(err)
	}
	return systemSummary(sys), nil
}

// ResetSystem 内置体系恢复默认显示信息并重新启用。
func (c Catalog) ResetSystem(id string) (any, error) {
	sys, err := design.ResetSystem(id, c.DataDir)
	if err != nil {
		return nil, adminErr(err)
	}
	return systemSummary(sys), nil
}

// RemoveSystem 删除自定义体系（内置体系受保护）。
func (c Catalog) RemoveSystem(id string) error {
	if err := design.RemoveSystem(id, c.DataDir); err != nil {
		return adminErr(err)
	}
	return nil
}
