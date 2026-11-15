package native

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/icon.png
var windowIconPNG []byte

//go:embed assets/tray.png
var trayIconPNG []byte

// monoFamily 是机器读数（端口、数据目录、rev、耗时）的等宽字族。
const monoFamily = "Cascadia Mono, Consolas, JetBrains Mono, SFMono-Regular, monospace"

// 图标与画布页共用同一套 24 网格线性语言（1.7 描边、currentColor）。
var (
	iconFolder     = svgIcon(`<path d="M4 7.5A1.5 1.5 0 015.5 6h3.2c.4 0 .8.2 1 .5L11 8h7.5A1.5 1.5 0 0120 9.5v8A1.5 1.5 0 0118.5 19h-13A1.5 1.5 0 014 17.5z"/>`)
	iconTheme      = svgIcon(`<circle cx="12" cy="12" r="8"/><path d="M12 4a8 8 0 010 16z" fill="currentColor" stroke="none"/>`)
	iconRefresh    = svgIcon(`<path d="M20 11a8 8 0 10-2.3 6.3"/><path d="M20 5v6h-6"/>`)
	iconExternal   = svgIcon(`<path d="M7 17L17 7"/><path d="M9 7h8v8"/>`)
	iconMore       = svgIcon(`<circle cx="6" cy="12" r="1.3" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.3" fill="currentColor" stroke="none"/><circle cx="18" cy="12" r="1.3" fill="currentColor" stroke="none"/>`)
	iconCheck      = svgIcon(`<path d="M4 12.5l5 5L20 6.5"/>`)
	iconWarning    = svgIcon(`<path d="M12 4l8.5 15h-17z"/><path d="M12 10v4"/><circle cx="12" cy="17.2" r="1" fill="currentColor" stroke="none"/>`)
	iconClose      = svgIcon(`<path d="M6 6l12 12M18 6L6 18"/>`)
	iconHome       = svgIcon(`<path d="M4 11l8-6 8 6"/><path d="M6 10v9h12v-9"/>`)
	iconMinus      = svgIcon(`<path d="M5 12h14"/>`)
	iconLink       = svgIcon(`<path d="M10.5 13.5a3.5 3.5 0 005 0l2.5-2.5a3.5 3.5 0 00-5-5l-1 1"/><path d="M13.5 10.5a3.5 3.5 0 00-5 0L6 13a3.5 3.5 0 005 5l1-1"/>`)
	iconFullscreen = svgIcon(`<path d="M4 9V4h5"/><path d="M20 9V4h-5"/><path d="M4 15v5h5"/><path d="M20 15v5h-5"/>`)
	iconDevtools   = svgIcon(`<path d="M9 8l-4 4 4 4"/><path d="M15 8l4 4-4 4"/><path d="M13 5l-2 14"/>`)
)

// svgIcon 用统一的描边参数包装 24 网格路径数据。
func svgIcon(inner string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + inner + `</svg>`))
}
