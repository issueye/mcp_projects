package app

import "github.com/egoist/mygo/ui"

// 「纸页 Inkwell」是 browser-use 的界面体系：暖纸底、石墨字、青铜作为
// 「代理在场」的专属色，发丝线分栏，等宽字体承载机器信息。设计稿见
// mcp_projects/adago_design 的画布项目「browser-use · 界面重设计」。
//
// 这里把体系 Token（light / dark 两套）映射成 mygo 的 Theme：mygo 的控件
// 只认 Background/Surface/Border/Text/Accent 这些语义槽，1:1 对应 Inkwell
// 的 paper / surface / line / ink / bronze。
var (
	inkwellLight = inkwellTheme(false)
	inkwellDark  = inkwellTheme(true)
)

// inkwellTheme 返回指定外观下的 Inkwell 主题。
func inkwellTheme(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Dark = true
		t.Background = ui.Hex("#1F1C18")     // paper
		t.Surface = ui.Hex("#29251F")        // paper-2：当前标签页/控件纸面
		t.SurfaceHover = ui.Hex("#332E26")   // paper-3 上浮
		t.SurfacePressed = ui.Hex("#3B3630") // border
		t.Border = ui.Hex("#3B3630")         // line
		t.Text = ui.Hex("#EDEAE3")           // ink
		t.TextMuted = ui.Hex("#A79E92")      // ink-3
		t.Accent = ui.Hex("#E0895C")         // bronze（夜色）
		t.AccentHover = ui.Hex("#EAA078")
		t.AccentPressed = ui.Hex("#C9754B")
		t.AccentText = ui.Hex("#1F1C18")
		t.Danger = ui.Hex("#E08A78")
		t.Warning = ui.Hex("#DFB164")
		t.Success = ui.Hex("#6FBF95")
		t.Selection = ui.RGBA(224, 137, 92, 0.32)
		t.Focus = ui.RGBA(224, 137, 92, 0.55)
		t.Inverse = ui.Hex("#EDEAE3")
		t.InverseText = ui.Hex("#1F1C18")
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
		t.ScrollbarWidth = 8
		t.Radius = 6
		t.Spacing = 4
		t.FontSize = 13
		return t
	}
	t := ui.LightTheme()
	t.Background = ui.Hex("#F7F5F0")
	t.Surface = ui.Hex("#FBFAF7")
	t.SurfaceHover = ui.Hex("#F1EEE7")
	t.SurfacePressed = ui.Hex("#E3DED4")
	t.Border = ui.Hex("#E3DED4")
	t.Text = ui.Hex("#1B1917")
	t.TextMuted = ui.Hex("#6F675E")
	t.Accent = ui.Hex("#9C4A22")
	t.AccentHover = ui.Hex("#873E1C")
	t.AccentPressed = ui.Hex("#703216")
	t.AccentText = ui.Hex("#FFFFFF")
	t.Danger = ui.Hex("#A33A2B")
	t.Warning = ui.Hex("#8A5D0B")
	t.Success = ui.Hex("#1F6B4E")
	t.Selection = ui.RGBA(156, 74, 34, 0.20)
	t.Focus = ui.RGBA(156, 74, 34, 0.45)
	t.Inverse = ui.Hex("#1B1917")
	t.InverseText = ui.Hex("#FBFAF7")
	t.Scrollbar = ui.RGBA(27, 25, 23, 0.22)
	t.ScrollbarWidth = 8
	t.Radius = 6
	t.Spacing = 4
	t.FontSize = 13
	return t
}

// 设计 Token 的直接引用，供 view 里画自有部件时取色。
func (a *App) ink(dark bool) inkTokens {
	if dark {
		return inkTokens{
			paper: ui.Hex("#1F1C18"), paper2: ui.Hex("#29251F"), paper3: ui.Hex("#181512"),
			surface: ui.Hex("#2E2A23"), line: ui.Hex("#3B3630"), line2: ui.Hex("#514A40"),
			ink: ui.Hex("#EDEAE3"), ink2: ui.Hex("#C4BCAE"), ink3: ui.Hex("#A79E92"), ink4: ui.Hex("#7C746A"),
			bronze: ui.Hex("#E0895C"), bronzeBg: ui.Hex("#33251C"), bronzeLine: ui.Hex("#4E382A"),
			onBronze: ui.Hex("#1F1C18"),
			success:  ui.Hex("#6FBF95"), successBg: ui.Hex("#1B2D25"),
			warn: ui.Hex("#DFB164"), warnBg: ui.Hex("#332B1A"),
			danger: ui.Hex("#E08A78"), dangerBg: ui.Hex("#3A221D"),
			info: ui.Hex("#8FB3CB"), infoBg: ui.Hex("#1D2830"),
		}
	}
	return inkTokens{
		paper: ui.Hex("#F7F5F0"), paper2: ui.Hex("#FBFAF7"), paper3: ui.Hex("#F1EEE7"),
		surface: ui.Hex("#FFFFFF"), line: ui.Hex("#E3DED4"), line2: ui.Hex("#CFC8BB"),
		ink: ui.Hex("#1B1917"), ink2: ui.Hex("#5A534B"), ink3: ui.Hex("#6F675E"), ink4: ui.Hex("#9A9287"),
		bronze: ui.Hex("#9C4A22"), bronzeBg: ui.Hex("#F6EBE3"), bronzeLine: ui.Hex("#E7CDBC"),
		onBronze: ui.Hex("#FFFFFF"),
		success:  ui.Hex("#1F6B4E"), successBg: ui.Hex("#E6F1EA"),
		warn: ui.Hex("#8A5D0B"), warnBg: ui.Hex("#F7EEDC"),
		danger: ui.Hex("#A33A2B"), dangerBg: ui.Hex("#F8E7E3"),
		info: ui.Hex("#33556F"), infoBg: ui.Hex("#E8EEF3"),
	}
}

// inkTokens 是 Inkwell 语义色的 Go 侧镜像（与 kit.css 变量一一对应）。
type inkTokens struct {
	paper, paper2, paper3, surface, line, line2        ui.Color
	ink, ink2, ink3, ink4                              ui.Color
	bronze, bronzeBg, bronzeLine, onBronze             ui.Color
	success, successBg, warn, warnBg, danger, dangerBg ui.Color
	info, infoBg                                       ui.Color
}
