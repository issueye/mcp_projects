package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"browseruse/internal/audit"
	"browseruse/internal/browser"

	"github.com/egoist/mygo/ui"
)

// view 按「纸页 Inkwell」体系绘制主窗：标题栏（traffic lights · 品牌 · 折页标签）、
// 工具栏（导航 · 分段地址栏 · 代理胶囊）、页面视口（元信息条 + 网页）、状态条。
func (a *App) view(c *ui.Context) {
	dark := false
	if c.Theme() != nil {
		dark = c.Theme().Dark
	}
	if dark {
		c.SetTheme(inkwellDark)
	} else {
		c.SetTheme(inkwellLight)
	}
	tk := inkTokensFor(dark)
	if a.mgr == nil {
		ui.Column(c).Fill().Background(tk.paper)
		return
	}
	ui.Column(c).Fill().Background(tk.paper).AlignItems(ui.Stretch).Children(func() {
		a.titleBar(c, tk)
		a.toolbar(c, tk)
		a.page(c, tk, dark)
		a.statusBar(c, tk)
	})
}

// inkTokensFor 返回指定外观的 Token；与 theme.go 的 inkwellTheme 共用取值。
func inkTokensFor(dark bool) inkTokens {
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

// ---- 标题栏 ----------------------------------------------------------------

// chromeTop 是标题栏 + 工具栏的总高度：蒙版与抽屉从这条线开始。
func chromeTop(c *ui.Context) float32 {
	bar := c.TitleBar()
	h := bar.Height
	if h < 44 {
		h = 44
	}
	return h + 46
}

func (a *App) titleBar(c *ui.Context, tk inkTokens) {
	bar := c.TitleBar()
	h := bar.Height
	if h < 44 {
		h = 44
	}
	tabH := h - 8
	ui.Row(c).Height(h).Padding(0, bar.Right+4, 0, 0).Gap(12).
		AlignItems(ui.Center).Background(tk.paper).DragWindow().Children(func() {
		// traffic lights：装饰性，与 Windows 原生按钮并存，表达「这是应用窗口」。
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			for _, col := range []ui.Color{ui.Hex("#E0695B"), ui.Hex("#E2AC4B"), ui.Hex("#5FA467")} {
				ui.Row(c).Size(11, 11).Radius(6).Background(col)
			}
		})
		a.brandMark(c, tk)
		ui.Column(c).Grow(1).FillWidth().AlignSelf(ui.Stretch).AlignItems(ui.Start).ClipX().Children(func() {
			ui.Row(c).Height(h).Gap(2).AlignItems(ui.Center).Children(func() {
				for _, info := range a.mgr.Tabs() {
					a.tabItem(c, tk, info, tabH)
				}
				if a.iconButton(c, tk, iconPlus, "新建标签页", false, 28) {
					a.openTab("about:blank")
				}
			})
		})
	})
}

// brandMark 画「折角方块」品牌标记与应用名；高度固定，视觉上像一枚印章。
func (a *App) brandMark(c *ui.Context, tk inkTokens) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(24, 24).Radius(7).Background(tk.ink).Center().Children(func() {
			ui.Icon(c, iconCube).Size(14, 14).TextColor(tk.paper2)
		})
		ui.Text(c, "browser-use").FontSize(12).Bold().TextColor(tk.ink)
		ui.Row(c).Height(12).Width(1).Background(tk.line).MarginX(1)
		ui.Text(c, "AGENT").Font(monoFamily).FontSize(9).TextColor(tk.ink4)
	})
}

func (a *App) tabItem(c *ui.Context, tk inkTokens, info browser.TabInfo, height float32) {
	closed := false
	tab := ui.ButtonBase(c).Label(info.Title).Height(height).Width(208).Padding(4, 8).Gap(8).Radius(7).
		Cursor(ui.CursorPointer)
	switch {
	case info.Active:
		tab.Background(tk.paper2).Border(1, tk.line)
	case tab.Hovered():
		tab.Background(tk.paper3)
	}
	tab.Children(func() {
		ui.Row(c).Size(16, 16).Radius(5).Background(tabColor(info.ID)).Center().Children(func() {
			ui.Text(c, titleInitial(info.Title)).FontSize(9).Bold().TextColor(ui.RGB(255, 255, 255))
		})
		titleColor := tk.ink3
		if info.Active {
			titleColor = tk.ink
		}
		ui.Text(c, info.Title).SingleLine().Grow(1).FontSize(12).TextColor(titleColor)
		if info.Loading {
			sp := ui.Spinner(c).Size(12, 12)
			_ = sp
		}
		if info.Locked {
			ui.Icon(c, iconLock).Size(11, 11).TextColor(tk.info)
		}
		if info.Active || tab.Hovered() {
			cb := ui.ButtonBase(c).Label("关闭标签页").Size(18, 18).Radius(5).
				Cursor(ui.CursorPointer).Tooltip("关闭标签页")
			if cb.Hovered() {
				cb.Background(tk.paper3)
			}
			cb.Children(func() {
				ui.Icon(c, iconClose).Size(10, 10).TextColor(tk.ink3)
			})
			if cb.Clicked() {
				closed = true
			}
		}
		if info.Active {
			// 折页关系：当前页底部压一条青铜线，替身工具栏的发丝线。
			ui.Row(c).Absolute().Bottom(0).Left(8).Right(8).Height(2).Radius(1).Background(tk.bronze)
		}
	})
	if closed {
		a.closeTab(info.ID)
		return
	}
	if tab.Clicked() {
		if err := a.mgr.Activate(info.ID); err == nil {
			a.addressAuto = true
		}
	}
}

var tabPalette = []ui.Color{
	ui.Hex("#1B5E6B"),
	ui.Hex("#5A534B"),
	ui.Hex("#7A4B2A"),
	ui.Hex("#3F5B47"),
	ui.Hex("#5A4A6B"),
	ui.Hex("#6B3F3A"),
}

func tabColor(id string) ui.Color {
	h := 0
	for _, r := range id {
		h = (h*31 + int(r)) & 0x7fffffff
	}
	return tabPalette[h%len(tabPalette)]
}

func titleInitial(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "?"
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0]))
}

// ---- 工具栏 ----------------------------------------------------------------

func (a *App) toolbar(c *ui.Context, tk inkTokens) {
	active := a.mgr.Active()
	canBack, canForward, loading := false, false, false
	masked := false
	if active != nil {
		canBack = active.Page.CanGoBack()
		canForward = active.Page.CanGoForward()
		loading = active.Page.IsLoading()
		masked = a.mgr.Masked(active.ID)
	}
	ui.Row(c).Height(46).Padding(0, 12, 0, 12).Gap(8).AlignItems(ui.Center).
		Background(tk.paper2).Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			if a.iconButton(c, tk, iconBack, "后退", !canBack || masked, 28) {
				a.history("back")
			}
			if a.iconButton(c, tk, iconForward, "前进", !canForward || masked, 28) {
				a.history("forward")
			}
			if loading {
				if a.iconButton(c, tk, iconStop, "停止", masked, 28) {
					go func() {
						_, _ = a.mgr.Navigate(context.Background(), active.ID, "ui", "stop", "", browser.NavigateOptions{WaitUntil: "none", Human: true})
					}()
				}
			} else if a.iconButton(c, tk, iconReload, "刷新", masked, 28) {
				a.history("reload")
			}
		})
		a.addressBar(c, tk, active, masked, loading)
		a.agentChip(c, tk, a.mgr.State())
	})
}

// addressBar 是一段可编辑的地址：锁图标表示连接安全，地址本身是唯一的输入面，
// 右侧常驻前往键；加载中在右侧显示状态词。
func (a *App) addressBar(c *ui.Context, tk inkTokens, active *browser.Tab, masked, loading bool) {
	raw := a.address
	if a.addressAuto && active != nil {
		a.address = active.URL()
		raw = a.address
	}
	var input ui.Element
	bar := ui.Row(c).Grow(1).Height(32).Padding(0, 5, 0, 10).Gap(8).Radius(6).
		AlignItems(ui.Center).Background(tk.surface).Border(1, tk.line)
	bar.Children(func() {
		secure := strings.HasPrefix(strings.ToLower(raw), "https://")
		ic := iconGlobe
		icColor := tk.ink4
		if secure {
			ic = iconLock
			icColor = tk.info
		}
		ui.Icon(c, ic).Size(13, 13).TextColor(icColor)
		input = ui.TextInputBase(c, &a.address).Grow(1).FontSize(13).
			TextColor(tk.ink).Placeholder("输入网址或搜索…").Disabled(masked)
		if input.Changed() {
			a.addressAuto = false
		}
		if input.Submitted() {
			a.navigate(a.address)
		}
		if loading {
			ui.Text(c, "加载中").FontSize(10).TextColor(tk.bronze)
		}
		if a.iconButtonSized(c, tk, iconArrowRight, "前往", masked, 24, 13) {
			a.navigate(a.address)
		}
	})
	if input.Focused() {
		bar.Border(1, tk.bronze)
	}
}

// splitURL 返回「域名」与「路径」两段，空地址回退到提示文案。
func splitURL(raw string) (host, path string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "输入网址或搜索…", ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// agentChip 是「谁在操作这个浏览器」的常驻答案：空闲 / 工作中 / 你已接管 / 已暂停。
func (a *App) agentChip(c *ui.Context, tk inkTokens, cs browser.ControlState) {
	label, dot, active := "代理空闲", tk.ink4, false
	switch cs.State {
	case "agent":
		label, dot, active = "代理工作中", tk.bronze, true
	case "user":
		label, dot, active = "你已接管", tk.warn, true
	case "paused":
		label, dot, active = "已暂停", tk.warn, true
	}
	bg, border := tk.surface, tk.line
	if active {
		bg = tk.bronzeBg
		if cs.State == "user" || cs.State == "paused" {
			bg = tk.warnBg
		}
		border = tk.bronzeLine
	}
	ui.Row(c).Height(28).Padding(0, 10).Gap(7).Radius(6).
		Background(bg).Border(1, border).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, label).FontSize(11).TextColor(dot)
	})
}

// ---- 页面 ------------------------------------------------------------------

func (a *App) page(c *ui.Context, tk inkTokens, dark bool) {
	active := a.mgr.Active()
	if active == nil {
		a.emptyState(c, tk)
		return
	}
	ui.Row(c).Grow(1).Background(tk.paper).Children(func() {
		ui.Column(c).Fill().Padding(8, 8, 0, 8).Children(func() {
			a.viewportMeta(c, tk, active)
			ui.Column(c).Fill().Children(func() {
				ui.WebView(c, active.View).Fill().Radius(8).Border(1, tk.line)
			})
		})
	})
	masked := a.mgr.Masked(active.ID)
	if masked && !a.wasMasked {
		a.maskFocus = true
	}
	a.wasMasked = masked
	if masked {
		a.mask(c, tk, active)
	}
	if a.showActivity {
		a.activityPanel(c, tk)
	}
}

// viewportMeta 是网页上方 24px 的仪表条：标签、结构、站点策略与连接安全。
func (a *App) viewportMeta(c *ui.Context, tk inkTokens, active *browser.Tab) {
	info := active.Info(true)
	ui.Row(c).Height(24).Padding(0, 4, 0, 4).Gap(12).AlignItems(ui.Center).Children(func() {
		a.metaItem(c, tk, iconTab, strings.ToUpper(info.ID)+" · 主标签")
		a.metaItem(c, tk, iconLayers, "文档树")
		a.metaItem(c, tk, iconShield, "站点策略 · 允许")
		ui.Row(c).Grow(1)
		secure := strings.HasPrefix(strings.ToLower(info.URL), "https://")
		label, col := "HTTPS · 受信任", tk.info
		if !secure {
			label, col = "连接未加密", tk.ink4
		}
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconLock).Size(11, 11).TextColor(col)
			ui.Text(c, label).FontSize(10).TextColor(col)
		})
	})
}

func (a *App) metaItem(c *ui.Context, tk inkTokens, ic *ui.SVG, label string) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, ic).Size(11, 11).TextColor(tk.ink4)
		ui.Text(c, label).FontSize(10).TextColor(tk.ink4)
	})
}

// emptyState 是没有标签页时的空白页：一枚印章、两句解释、两个入口，下面是最近关闭。
func (a *App) emptyState(c *ui.Context, tk inkTokens) {
	ui.Column(c).Grow(1).Center().Gap(0).Children(func() {
		ui.Row(c).Size(64, 64).Radius(18).Background(tk.bronzeBg).Border(1, tk.bronzeLine).Center().Children(func() {
			ui.Icon(c, iconGlobe).Size(32, 32).TextColor(tk.bronze)
		})
		ui.Row(c).Height(22)
		ui.Text(c, "还没有打开任何页面").FontSize(26).Bold().TextColor(tk.ink)
		ui.Row(c).Height(8)
		ui.Text(c, "打开一个网址，或把任务交给代理；每个标签页都有独立的页面与审计记录。").
			FontSize(12).TextColor(tk.ink3)
		ui.Row(c).Height(20)
		ui.Row(c).Gap(10).Justify(ui.Center).Children(func() {
			btn := ui.Row(c).Height(32).Padding(0, 14).Gap(6).Radius(6).
				Background(tk.bronze).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip("新建标签页")
			btn.Children(func() {
				ui.Icon(c, iconPlus).Size(13, 13).TextColor(tk.onBronze)
				ui.Text(c, "新建标签页").FontSize(12).Bold().TextColor(tk.onBronze)
			})
			if btn.Clicked() {
				a.openTab("about:blank")
			}
			btn2 := ui.Row(c).Height(32).Padding(0, 14).Gap(6).Radius(6).
				Background(tk.surface).Border(1, tk.line).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Tooltip("让代理开始")
			btn2.Children(func() {
				ui.Icon(c, iconShield).Size(13, 13).TextColor(tk.ink2)
				ui.Text(c, "让代理开始").FontSize(12).TextColor(tk.ink2)
			})
		})
	})
}

// ---- 蒙版 ------------------------------------------------------------------

func (a *App) mask(c *ui.Context, tk inkTokens, tab *browser.Tab) {
	dim := float32(a.cfg.Control.Dim) / 255
	if dim <= 0 {
		dim = 0.08
	}
	overlay := ui.Column(c).Absolute().Top(chromeTop(c)).Left(0).Right(0).Bottom(26).
		Background(ui.RGBA(24, 20, 16, dim)).
		Border(1, tk.bronzeLine).Cursor(ui.CursorNotAllowed)
	if a.maskFocus {
		overlay.Focusable().Focus()
		a.maskFocus = false
	}
	overlay.Children(func() {
		// 顶部：代理操作卡
		ui.Row(c).Absolute().Top(14).Left(0).Right(0).Justify(ui.Center).Children(func() {
			card := ui.Row(c).Padding(10).Gap(10).Radius(12).
				Background(tk.surface).Border(1, tk.line).
				Shadow(0, 12, 32, 0, ui.RGBA(27, 25, 23, 40)).AlignItems(ui.Center)
			card.Children(func() {
				ui.Row(c).Size(32, 32).Radius(16).Background(tk.bronze).Center().Children(func() {
					ui.Icon(c, iconShield).Size(17, 17).TextColor(tk.onBronze)
				})
				ui.Column(c).Gap(2).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "代理正在操作").FontSize(13).Bold().TextColor(tk.ink)
						ui.Text(c, strings.ToUpper(tab.ID)).FontSize(9).TextColor(tk.bronze).
							Padding(1, 4).Radius(3).Background(tk.bronzeBg).Border(1, tk.bronzeLine)
					})
					act := tab.LastAction()
					if act == "" {
						act = "准备中"
					}
					ui.Text(c, "页面输入已锁定 · "+clip(act, 48)).FontSize(11).TextColor(tk.ink3)
				})
				ui.Row(c).Width(8)
				if ui.PrimaryButton(c, "接管").Clicked() {
					a.takeOver()
				}
				if ui.Button(c, "暂停").Clicked() {
					a.pause()
				}
			})
		})
		// 底部：接管提示
		ui.Row(c).Absolute().Bottom(16).Left(0).Right(0).Justify(ui.Center).Children(func() {
			ui.Row(c).Padding(0, 12).Height(26).Gap(6).Radius(13).Background(tk.paper2).
				Border(1, tk.line).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconLock).Size(11, 11).TextColor(tk.ink3)
				ui.Text(c, "点击「接管」后即可操作页面").FontSize(11).TextColor(tk.ink3)
			})
		})
		// 右下：最近动作流水（代理在场的证据）
		if a.srv != nil {
			entries := a.srv.Activity()
			if n := len(entries); n > 0 {
				start := n - 3
				if start < 0 {
					start = 0
				}
				ui.Column(c).Absolute().Bottom(16).Right(16).Width(232).Padding(8).Gap(4).Radius(8).
					Background(tk.paper2).Border(1, tk.line).
					Shadow(0, 8, 24, 0, ui.RGBA(27, 25, 23, 30)).Children(func() {
					ui.Text(c, "最近动作").FontSize(9).TextColor(tk.ink4)
					for i := n - 1; i >= start; i-- {
						e := entries[i]
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, e.Tool).Font(monoFamily).FontSize(10).TextColor(tk.ink2).Grow(1)
							ui.Text(c, fmt.Sprintf("%dms", e.MS)).Font(monoFamily).FontSize(9).TextColor(tk.ink4)
						})
					}
				})
			}
		}
	})
}

// ---- 状态条 ----------------------------------------------------------------

func (a *App) statusBar(c *ui.Context, tk inkTokens) {
	cs := a.mgr.State()
	state, col := "代理空闲", tk.ink4
	dot := tk.ink4
	switch cs.State {
	case "agent":
		state, col, dot = "代理工作中", tk.bronze, tk.bronze
	case "user":
		state, col, dot = "你已接管", tk.warn, tk.warn
	case "paused":
		state, col, dot = "已暂停", tk.warn, tk.warn
	}
	ui.Row(c).Height(26).Padding(0, 12, 0, 12).Gap(10).AlignItems(ui.Center).
		Background(tk.paper2).Border(1, tk.line).Children(func() {
		a.statusItem(c, tk, iconTab, fmt.Sprintf("%d 个标签页", len(a.mgr.Tabs())))
		ui.Row(c).Size(1, 11).Background(tk.line)
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(5, 5).Radius(3).Background(dot)
			ui.Text(c, state).FontSize(10).TextColor(col)
		})
		if a.srv != nil {
			ui.Text(c, fmt.Sprintf(":%d", a.srv.Port())).Font(monoFamily).FontSize(10).TextColor(tk.ink3)
		}
		if a.lastErr != "" {
			ui.Row(c).Size(1, 11).Background(tk.line)
			ui.Text(c, clip(a.lastErr, 72)).FontSize(10).TextColor(tk.danger)
		}
		ui.Row(c).Grow(1)
		if active := a.mgr.Active(); active != nil {
			ui.Text(c, clip(active.URL(), 90)).Font(monoFamily).FontSize(10).TextColor(tk.ink4).SingleLine()
		}
		if cs.State == "user" || cs.State == "paused" {
			if ui.Button(c, "恢复代理").Clicked() {
				a.resume()
			}
		}
		label := "活动"
		if a.showActivity {
			label = "收起"
		}
		act := ui.Row(c).Height(18).Padding(0, 7).Gap(5).Radius(4).
			Border(1, tk.line).AlignItems(ui.Center).Cursor(ui.CursorPointer).
			Background(tk.surface).Tooltip("活动审计")
		if a.showActivity {
			act.Background(tk.ink).Border(1, tk.ink)
		} else if act.Hovered() {
			act.Background(tk.paper3)
		}
		act.Children(func() {
			fg := tk.ink2
			if a.showActivity {
				fg = tk.paper2
			}
			ui.Icon(c, iconActivity).Size(10, 10).TextColor(fg)
			ui.Text(c, label).FontSize(10).TextColor(fg)
			if a.srv != nil {
				if n := len(a.srv.Activity()); n > 0 {
					ui.Text(c, fmt.Sprintf("%d", n)).Font(monoFamily).FontSize(9).TextColor(tk.ink4)
				}
			}
		})
		if act.Clicked() {
			a.showActivity = !a.showActivity
		}
	})
}

func (a *App) statusItem(c *ui.Context, tk inkTokens, ic *ui.SVG, label string) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, ic).Size(11, 11).TextColor(tk.ink3)
		ui.Text(c, label).FontSize(10).TextColor(tk.ink3)
	})
}

// ---- 活动抽屉 --------------------------------------------------------------

func (a *App) activityPanel(c *ui.Context, tk inkTokens) {
	if a.srv == nil {
		return
	}
	entries := a.srv.Activity()
	panel := ui.Column(c).Absolute().Top(chromeTop(c)).Right(0).Bottom(26).Width(320).
		Background(tk.surface).Border(1, tk.line).
		Shadow(-8, 0, 24, 0, ui.RGBA(27, 25, 23, 30))
	panel.Children(func() {
		ui.Row(c).Height(44).Padding(0, 12, 0, 12).Gap(8).AlignItems(ui.Center).
			Border(0, tk.line).Children(func() {
			ui.Icon(c, iconActivity).Size(15, 15).TextColor(tk.bronze)
			ui.Text(c, "活动").FontSize(12).Bold().TextColor(tk.ink)
			ui.Text(c, fmt.Sprintf("%d", len(entries))).Font(monoFamily).FontSize(9).TextColor(tk.ink3).
				Padding(2, 5).Radius(4).Background(tk.paper3).Border(1, tk.line)
			ui.Row(c).Grow(1)
			if a.iconButton(c, tk, iconClose, "关闭", false, 24) {
				a.showActivity = false
			}
		})
		ui.Row(c).Height(1).Background(tk.line)
		if len(entries) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Icon(c, iconActivity).Size(28, 28).TextColor(tk.ink4)
				ui.Text(c, "暂无调用").FontSize(12).TextColor(tk.ink3)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			for i := len(entries) - 1; i >= 0; i-- {
				a.activityRow(c, tk, entries[i])
				if i > 0 {
					ui.Row(c).Height(1).Background(tk.line)
				}
			}
		})
		ui.Row(c).Height(1).Background(tk.line)
		ui.Row(c).Height(34).Padding(0, 12, 0, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ok := 0
			for _, e := range entries {
				if e.OK {
					ok++
				}
			}
			ui.Text(c, fmt.Sprintf("%d 次调用 · 成功 %d · 失败 %d", len(entries), ok, len(entries)-ok)).
				Font(monoFamily).FontSize(9).TextColor(tk.ink4)
		})
	})
}

func (a *App) activityRow(c *ui.Context, tk inkTokens, e audit.Entry) {
	row := ui.Row(c).Padding(8, 12, 8, 12).Gap(8).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(tk.paper)
	}
	row.Children(func() {
		dot := tk.success
		if !e.OK {
			dot = tk.danger
		} else if e.MS >= 1000 {
			dot = tk.warn
		}
		ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Size(6, 6).Radius(3).Background(dot)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, e.Tool).Font(monoFamily).FontSize(11).Bold().TextColor(tk.ink)
					if e.Tab != "" {
						ui.Text(c, e.Tab).Font(monoFamily).FontSize(9).TextColor(tk.ink3).
							Padding(1, 4).Radius(3).Background(tk.paper3).Border(1, tk.line)
					}
					ui.Row(c).Grow(1)
					ui.Text(c, humanMS(e.MS)).Font(monoFamily).FontSize(9).TextColor(detailColor(tk, e))
				})
				detail := e.URL
				if !e.OK && e.Code != "" {
					detail = e.Code
				}
				if detail != "" {
					ui.Text(c, clip(detail, 52)).FontSize(10).TextColor(detailColor(tk, e)).SingleLine()
				}
			})
		})
	})
}

func detailColor(tk inkTokens, e audit.Entry) ui.Color {
	if !e.OK {
		return tk.danger
	}
	if e.MS >= 1000 {
		return tk.warn
	}
	return tk.ink3
}

func humanMS(ms int64) string {
	if ms >= 1000 {
		return time.Duration(ms * int64(time.Millisecond)).Round(10 * time.Millisecond).String()
	}
	return fmt.Sprintf("%dms", ms)
}

// ---- helpers ---------------------------------------------------------------

func (a *App) iconButton(c *ui.Context, tk inkTokens, icon *ui.SVG, tip string, disabled bool, size float32) bool {
	return a.iconButtonSized(c, tk, icon, tip, disabled, size, 15)
}

func (a *App) iconButtonSized(c *ui.Context, tk inkTokens, icon *ui.SVG, tip string, disabled bool, size, iconSize float32) bool {
	b := ui.ButtonBase(c).Label(tip).Size(size, size).Radius(6).Cursor(ui.CursorPointer).
		Tooltip(tip)
	if disabled {
		b.Cursor(ui.CursorDefault)
	}
	if !disabled && b.Hovered() {
		b.Background(tk.paper3)
	}
	color := tk.ink
	if disabled {
		color = tk.ink4
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(color)
	})
	if disabled {
		return false
	}
	return b.Clicked()
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
