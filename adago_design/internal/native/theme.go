// Package native 是 adago-design 的原生壳：一枚 mygo 窗口，内嵌画布页
// （WebView 指向已在本进程跑起来的 HTTP 服务），标题条与状态条由 Go 直接画。
//
// 设计稿见画布项目「adago-design · 原生内嵌窗口设计图」（09 视觉规范 /
// 10 实现契约）。壳只负责窗口最外一圈：身份、作用域、服务读数、托盘与
// 窗口记忆；画布交互（拖拽 / 检查 / 缩放 / 截图）仍然全部在页面里。
package native

import "github.com/egoist/mygo/ui"

// tokens 是画布页 tokens.css 的 Go 侧镜像（同名同值，唯一真源仍是 CSS）。
// 壳只用 surface / line / ink 三档 + 成对状态色，不新增第二种强调色。
type tokens struct {
	canvas     ui.Color
	surface    ui.Color
	elevated   ui.Color
	subtle     ui.Color
	line       ui.Color
	lineStrong ui.Color
	ink        ui.Color
	ink2       ui.Color
	ink3       ui.Color
	brand      ui.Color
	onBrand    ui.Color
	brandSoft  ui.Color
	successBg  ui.Color
	successInk ui.Color
	warnBg     ui.Color
	warnInk    ui.Color
	dangerBg   ui.Color
	dangerInk  ui.Color
	focus      ui.Color
}

// tokensFor 返回指定外观的语义色；值与 tokens.css 一一对应。
func tokensFor(dark bool) tokens {
	if dark {
		return tokens{
			canvas:     ui.Hex("#14151A"),
			surface:    ui.Hex("#1C1E24"),
			elevated:   ui.Hex("#23262E"),
			subtle:     ui.Hex("#242730"),
			line:       ui.Hex("#2E313A"),
			lineStrong: ui.Hex("#3D414C"),
			ink:        ui.Hex("#F2EFE7"),
			ink2:       ui.Hex("#B9B5AA"),
			ink3:       ui.Hex("#9A9588"),
			brand:      ui.Hex("#F2EFE7"),
			onBrand:    ui.Hex("#16171A"),
			brandSoft:  ui.Hex("#2A2D35"),
			successBg:  ui.Hex("#1B2A22"),
			successInk: ui.Hex("#63D68E"),
			warnBg:     ui.Hex("#2B2517"),
			warnInk:    ui.Hex("#F2B84B"),
			dangerBg:   ui.Hex("#2E1B1D"),
			dangerInk:  ui.Hex("#FF8A93"),
			focus:      ui.Hex("#F2EFE7"),
		}
	}
	return tokens{
		canvas:     ui.Hex("#EDE8DC"),
		surface:    ui.Hex("#FBF8F1"),
		elevated:   ui.Hex("#FFFDF8"),
		subtle:     ui.Hex("#F2EDE2"),
		line:       ui.Hex("#DCD5C6"),
		lineStrong: ui.Hex("#C9C0AC"),
		ink:        ui.Hex("#16171A"),
		ink2:       ui.Hex("#57544C"),
		ink3:       ui.Hex("#6E6A60"),
		brand:      ui.Hex("#16171A"),
		onBrand:    ui.Hex("#F6F3EC"),
		brandSoft:  ui.Hex("#E7E1D2"),
		successBg:  ui.Hex("#E7F3EA"),
		successInk: ui.Hex("#1F7A3D"),
		warnBg:     ui.Hex("#FBEEDA"),
		warnInk:    ui.Hex("#8A5300"),
		dangerBg:   ui.Hex("#FBE9E7"),
		dangerInk:  ui.Hex("#B3261E"),
		focus:      ui.Hex("#16171A"),
	}
}

// themeFor 把纸墨令牌映射成 mygo 的语义槽（原生控件、菜单、滚动条共用）。
func themeFor(dark bool) *ui.Theme {
	tk := tokensFor(dark)
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
	} else {
		t = ui.LightTheme()
	}
	t.Dark = dark
	t.Background = tk.canvas
	t.Surface = tk.surface
	t.SurfaceHover = tk.subtle
	t.SurfacePressed = tk.brandSoft
	t.Border = tk.line
	t.Text = tk.ink
	t.TextMuted = tk.ink3
	// 单色品牌：强调色就是墨（深色主题是纸），没有第二种彩色。
	t.Accent = tk.brand
	t.AccentHover = tk.ink2
	t.AccentPressed = tk.ink3
	t.AccentText = tk.onBrand
	t.Danger = tk.dangerInk
	t.Warning = tk.warnInk
	t.Success = tk.successInk
	t.Selection = ui.RGBA(0, 0, 0, 0)
	t.Focus = tk.focus
	t.Inverse = tk.ink
	t.InverseText = tk.onBrand
	if dark {
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
	} else {
		t.Scrollbar = ui.RGBA(22, 23, 26, 0.22)
	}
	t.ScrollbarWidth = 8
	t.Radius = 6
	t.Spacing = 4
	t.FontSize = 13
	return t
}

// bgHex 是窗口与 WebView 的底色（页面加载前不留白闪）。
func bgHex(dark bool) string {
	if dark {
		return "#14151A"
	}
	return "#EDE8DC"
}
