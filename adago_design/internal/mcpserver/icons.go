package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"adagodesign/internal/design"
)

// design_icon_manage：SVG 图标库的管理面（与画布弹层同一套 design 函数）。
// 停用只影响选择面（design_icons / 目录 / 检索）；精灵与已有模板仍保留该图标。

type iconManageIn struct {
	Action string   `json:"action" jsonschema:"list|create|update|disable|enable|remove|reset"`
	Name   string   `json:"name,omitempty" jsonschema:"图标名（kebab-case；create 必填，其余操作必填）"`
	Group  string   `json:"group,omitempty" jsonschema:"分组: arrow|action|status|nav|media|data"`
	Tags   []string `json:"tags,omitempty" jsonschema:"别名/检索词；update 时整体替换，传空数组即清空"`
	SVG    string   `json:"svg,omitempty" jsonschema:"create/update：完整 SVG 或路径片段（≤64KB）"`
	Query  string   `json:"query,omitempty" jsonschema:"list 时按名称/分组/别名过滤"`
}

type iconStateOut struct {
	Name       string   `json:"name"`
	Group      string   `json:"group"`
	Tags       []string `json:"tags,omitempty"`
	ViewBox    string   `json:"viewBox,omitempty"`
	Builtin    bool     `json:"builtin"`
	Disabled   bool     `json:"disabled"`
	Overridden bool     `json:"overridden,omitempty"`
	Usage      string   `json:"usage,omitempty"`
}

type iconManageOut struct {
	Icons []iconStateOut `json:"icons,omitempty"`
	Icon  *iconStateOut  `json:"icon,omitempty"`
}

func iconState(ic design.Icon) iconStateOut {
	builtin, off, over, _ := design.IconState(ic.Name)
	return iconStateOut{
		Name: ic.Name, Group: ic.Group, Tags: ic.Tags, ViewBox: ic.ViewBox,
		Builtin: builtin, Disabled: off, Overridden: over,
		Usage: design.IconUsage(ic.Name),
	}
}

func iconManageErrCode(err error) string {
	switch {
	case errors.Is(err, design.ErrIconNotFound):
		return "unknown_icon"
	case errors.Is(err, design.ErrIconBuiltin):
		return "icon_builtin_protected"
	case errors.Is(err, design.ErrIconExists):
		return "icon_duplicate"
	case errors.Is(err, design.ErrIconInvalid):
		return "invalid_icon"
	default:
		return "manage_failed"
	}
}

func matchIconQuery(ic design.Icon, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(ic.Name), q) || strings.Contains(strings.ToLower(ic.Group), q) {
		return true
	}
	for _, t := range ic.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

func iconManageTool(d Deps) mcp.ToolHandlerFor[iconManageIn, iconManageOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in iconManageIn) (*mcp.CallToolResult, iconManageOut, error) {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		fail := func(code, msg, hint string) (*mcp.CallToolResult, iconManageOut, error) {
			return failWith[iconManageOut](iconManageOut{}, code, msg, hint)
		}
		if action != "list" && action != "create" && strings.TrimSpace(in.Name) == "" {
			return fail("bad_request", "name 必填", "list 可查看现有图标名")
		}
		if d.DataDir == "" && action != "list" {
			return fail("bad_request", "数据目录未配置（-data-dir 或默认 <config>/adago-design）", "")
		}

		switch action {
		case "list":
			q := strings.ToLower(strings.TrimSpace(in.Query))
			out := iconManageOut{Icons: make([]iconStateOut, 0)}
			for _, ic := range design.AllIcons() {
				if !matchIconQuery(ic, q) {
					continue
				}
				out.Icons = append(out.Icons, iconState(ic))
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "共 %d 个图标（启用 %d，自定义 %d）：\n",
				len(out.Icons), len(design.Icons()), countCustomIcons(out.Icons))
			for _, ic := range out.Icons {
				flags := make([]string, 0, 2)
				if ic.Builtin {
					flags = append(flags, "内置")
				} else {
					flags = append(flags, "自定义")
				}
				if ic.Disabled {
					flags = append(flags, "已停用")
				}
				fmt.Fprintf(&sb, "- %s [%s]（%s）%s\n", ic.Name, ic.Group, ic.ViewBox, strings.Join(flags, " / "))
			}
			if q != "" && len(out.Icons) == 0 {
				sb.WriteString("没有匹配的图标；用 action=list 不带 query 查看全部。")
			}
			return textResult(strings.TrimRight(sb.String(), "\n")), out, nil

		case "create":
			ic, err := design.CreateIcon(design.IconInput{
				Name: in.Name, Group: in.Group, Tags: in.Tags, SVG: in.SVG,
			}, d.DataDir)
			if err != nil {
				return fail(iconManageErrCode(err), err.Error(),
					"name 用 kebab-case；svg 只允许 path/circle/rect/line/polyline/polygon/ellipse/g/defs 渐变")
			}
			return textResult(fmt.Sprintf("已新增自定义图标 %s（%s，viewBox %s）。design_icons 可直接检索到。",
				ic.Name, ic.Group, ic.ViewBox)), iconManageOut{Icon: ptr(iconState(ic))}, nil

		case "update":
			if strings.TrimSpace(in.Group) == "" && in.Tags == nil && strings.TrimSpace(in.SVG) == "" {
				return fail("bad_request", "update 至少提供 group/tags/svg 之一", "")
			}
			var patch design.IconPatch
			patch.SVG = in.SVG
			if g := strings.TrimSpace(in.Group); g != "" {
				patch.Group = &g
			}
			if in.Tags != nil {
				patch.Tags = in.Tags
				patch.TagsSet = true
			}
			ic, err := design.UpdateIcon(in.Name, d.DataDir, patch)
			if err != nil {
				if errors.Is(err, design.ErrIconBuiltin) {
					return fail("icon_builtin_protected", err.Error(), "内置图标只能停用或改分组/别名")
				}
				return fail(iconManageErrCode(err), err.Error(), "")
			}
			return textResult(fmt.Sprintf("已更新图标 %s（分组 %s，别名 %d 个）。",
				ic.Name, ic.Group, len(ic.Tags))), iconManageOut{Icon: ptr(iconState(ic))}, nil

		case "disable", "enable":
			off := action == "disable"
			ic, err := design.SetIconDisabled(in.Name, d.DataDir, off)
			if err != nil {
				return fail(iconManageErrCode(err), err.Error(), "用 action=list 查看现有图标")
			}
			verb := "停用"
			if !off {
				verb = "启用"
			}
			return textResult(fmt.Sprintf("已%s图标 %s。停用后 design_icons 不再返回它（精灵仍保留，旧模板不受影响）。", verb, ic.Name)),
				iconManageOut{Icon: ptr(iconState(ic))}, nil

		case "remove":
			if err := design.RemoveIcon(in.Name, d.DataDir); err != nil {
				if errors.Is(err, design.ErrIconBuiltin) {
					return fail("icon_builtin_protected", err.Error(), "内置图标请用 action=disable 停用")
				}
				return fail(iconManageErrCode(err), err.Error(), "")
			}
			return textResult(fmt.Sprintf("已移除自定义图标 %s。", in.Name)), iconManageOut{}, nil

		case "reset":
			ic, err := design.ResetIcon(in.Name, d.DataDir)
			if err != nil {
				return fail(iconManageErrCode(err), err.Error(), "reset 只用于内置图标：恢复默认分组/别名并重新启用")
			}
			return textResult(fmt.Sprintf("已恢复内置图标 %s 的默认分组/别名并重新启用。", ic.Name)),
				iconManageOut{Icon: ptr(iconState(ic))}, nil

		default:
			return fail("bad_request", "未知 action: "+in.Action, "list|create|update|disable|enable|remove|reset")
		}
	}
}

func countCustomIcons(list []iconStateOut) int {
	n := 0
	for _, ic := range list {
		if !ic.Builtin {
			n++
		}
	}
	return n
}
