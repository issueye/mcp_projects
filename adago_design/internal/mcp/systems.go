package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"adagodesign/internal/design"
)

// design_system_manage：设计体系的管理面（与画布弹层同一套 design 函数）。
// 停用只影响「选择面」（目录/搜索/预览）；已上画布的设计稿不回收。

type systemManageIn struct {
	Action      string `json:"action" jsonschema:"list|create|update|disable|enable|remove|reset"`
	ID          string `json:"id,omitempty" jsonschema:"体系 ID（create 可省略；update/disable/enable/remove/reset 必填）"`
	Name        string `json:"name,omitempty" jsonschema:"体系名称（create/update）"`
	Vendor      string `json:"vendor,omitempty" jsonschema:"厂商/来源（create/update）"`
	Family      string `json:"family,omitempty" jsonschema:"mobile|web|cross（create）"`
	Description string `json:"description,omitempty" jsonschema:"描述（create/update）"`
	Summary     string `json:"summary,omitempty" jsonschema:"一句话风格（update）"`
	Base        string `json:"base,omitempty" jsonschema:"create 的结构基底体系 ID（须已启用）"`
	Primary     string `json:"primary,omitempty" jsonschema:"create 的品牌主色 #RRGGBB"`
	JSON        string `json:"json,omitempty" jsonschema:"update：自定义体系完整 JSON（整体替换）；内置体系不允许"`
}

type systemStateOut struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Vendor     string   `json:"vendor,omitempty"`
	Family     string   `json:"family,omitempty"`
	Themes     []string `json:"themes,omitempty"`
	Primary    string   `json:"primary,omitempty"`
	Builtin    bool     `json:"builtin"`
	Disabled   bool     `json:"disabled"`
	Overridden bool     `json:"overridden,omitempty"`
}

type systemManageOut struct {
	Systems []systemStateOut `json:"systems,omitempty"`
	System  *systemStateOut  `json:"system,omitempty"`
}

func stateOut(sys *design.System) systemStateOut {
	builtin, off, over, _ := design.SystemState(sys.ID)
	return systemStateOut{
		ID: sys.ID, Name: sys.Name, Vendor: sys.Vendor, Family: sys.Family,
		Themes: sys.Themes, Primary: design.PrimaryColor(sys),
		Builtin: builtin, Disabled: off, Overridden: over,
	}
}

func systemManageTool(d Deps) Handler[systemManageIn, systemManageOut] {
	return func(ctx context.Context, in systemManageIn) (*CallToolResult, systemManageOut, error) {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		fail := func(code, msg, hint string) (*CallToolResult, systemManageOut, error) {
			return failWith[systemManageOut](systemManageOut{}, code, msg, hint)
		}
		switch action {
		case "list":
			all := design.AllSystems()
			out := systemManageOut{Systems: make([]systemStateOut, 0, len(all))}
			for _, sys := range all {
				out.Systems = append(out.Systems, stateOut(sys))
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "共 %d 个体系（启用 %d）：\n", len(all), len(design.Systems()))
			for _, s := range out.Systems {
				flags := make([]string, 0, 2)
				if s.Builtin {
					flags = append(flags, "内置")
				} else {
					flags = append(flags, "自定义")
				}
				if s.Disabled {
					flags = append(flags, "已停用")
				}
				if s.Overridden {
					flags = append(flags, "已改显示信息")
				}
				fmt.Fprintf(&sb, "- %s · %s（%s）%s\n", s.ID, s.Name, s.Vendor, strings.Join(flags, " / "))
			}
			return textResult(strings.TrimRight(sb.String(), "\n")), out, nil

		case "create":
			sys, err := design.CreateSystem(design.CreateInput{
				ID: in.ID, Name: in.Name, Vendor: in.Vendor, Family: in.Family,
				Description: in.Description, Base: in.Base, Primary: in.Primary,
			}, d.DataDir)
			if err != nil {
				return fail(manageErrCode(err), err.Error(), "base 用 design_systems 里已启用的体系；primary 用 #RRGGBB")
			}
			out := systemManageOut{System: ptr(stateOut(sys))}
			return textResult(fmt.Sprintf("已新增体系「%s」（%s），基底 %s，主色 %s。可直接用 design_tokens / design_preview。",
				sys.Name, sys.ID, design.SanitizeID(in.Base), design.PrimaryColor(sys))), out, nil

		case "update":
			if strings.TrimSpace(in.ID) == "" {
				return fail("bad_request", "id 必填", "")
			}
			if strings.TrimSpace(in.JSON) != "" {
				sys, err := design.UpdateCustomSystemJSON(in.ID, d.DataDir, in.JSON)
				if err != nil {
					return fail(manageErrCode(err), err.Error(), "只有自定义体系可整体替换 JSON；内置体系改 name/vendor/description/summary")
				}
				return textResult(fmt.Sprintf("已整体更新自定义体系 %s（%s）。", sys.ID, sys.Name)),
					systemManageOut{System: ptr(stateOut(sys))}, nil
			}
			var patch design.MetaPatch
			if v := strings.TrimSpace(in.Name); v != "" {
				patch.Name = &v
			}
			if v := strings.TrimSpace(in.Vendor); v != "" {
				patch.Vendor = &v
			}
			if v := strings.TrimSpace(in.Description); v != "" {
				patch.Description = &v
			}
			if v := strings.TrimSpace(in.Summary); v != "" {
				patch.Summary = &v
			}
			sys, err := design.UpdateSystemMeta(in.ID, d.DataDir, patch)
			if err != nil {
				return fail(manageErrCode(err), err.Error(), "内置体系改的是显示信息（Token 不变）；空字段回落默认值")
			}
			return textResult(fmt.Sprintf("已更新体系 %s 的显示信息（%s）。", sys.ID, sys.Name)),
				systemManageOut{System: ptr(stateOut(sys))}, nil

		case "disable", "enable":
			if strings.TrimSpace(in.ID) == "" {
				return fail("bad_request", "id 必填", "")
			}
			off := action == "disable"
			sys, err := design.SetSystemDisabled(in.ID, d.DataDir, off)
			if err != nil {
				return fail(manageErrCode(err), err.Error(), "用 action=list 查看现有体系")
			}
			verb := "停用"
			if !off {
				verb = "启用"
			}
			return textResult(fmt.Sprintf("已%s体系「%s」（%s）。停用后目录/搜索/预览不再出现它。", verb, sys.Name, sys.ID)),
				systemManageOut{System: ptr(stateOut(sys))}, nil

		case "remove":
			if strings.TrimSpace(in.ID) == "" {
				return fail("bad_request", "id 必填", "")
			}
			if err := design.RemoveSystem(in.ID, d.DataDir); err != nil {
				if errors.Is(err, design.ErrSystemBuiltin) {
					return fail("system_builtin_protected", err.Error(), "内置体系请用 action=disable 停用")
				}
				return fail(manageErrCode(err), err.Error(), "")
			}
			return textResult(fmt.Sprintf("已移除自定义体系 %s。", in.ID)), systemManageOut{}, nil

		case "reset":
			if strings.TrimSpace(in.ID) == "" {
				return fail("bad_request", "id 必填", "")
			}
			sys, err := design.ResetSystem(in.ID, d.DataDir)
			if err != nil {
				return fail(manageErrCode(err), err.Error(), "reset 只用于内置体系：恢复默认显示信息并重新启用")
			}
			return textResult(fmt.Sprintf("已恢复内置体系 %s 的默认显示信息并重新启用。", sys.ID)),
				systemManageOut{System: ptr(stateOut(sys))}, nil

		default:
			return fail("bad_request", "未知 action: "+in.Action, "list|create|update|disable|enable|remove|reset")
		}
	}
}

func manageErrCode(err error) string {
	switch {
	case errors.Is(err, design.ErrSystemNotFound):
		return "unknown_system"
	case errors.Is(err, design.ErrSystemBuiltin):
		return "system_builtin_protected"
	case errors.Is(err, design.ErrSystemExists):
		return "system_duplicate"
	case errors.Is(err, design.ErrSystemInvalid):
		return "invalid_system"
	default:
		return "manage_failed"
	}
}

func ptr[T any](v T) *T { return &v }
