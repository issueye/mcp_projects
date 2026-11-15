package native

import (
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// buildMenu 是窗口菜单（AutoHideMenuBar：按 Alt 才出现）。它同时承担
// 键盘快捷键：Ctrl+R 重载、F11 全屏、Ctrl+Shift+I 开发者工具。
func (s *Shell) buildMenu() *mygo.Menu {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "重新加载页面", Accelerator: "CmdOrCtrl+R", Click: func(*mygo.MenuItem, *mygo.Window) { s.Reload() }},
			{Label: "开发者工具", Accelerator: "CmdOrCtrl+Shift+I", Click: func(*mygo.MenuItem, *mygo.Window) { s.ToggleDevTools() }},
			{Label: "沉浸全屏", Accelerator: "F11", Click: func(*mygo.MenuItem, *mygo.Window) { s.ToggleFullScreen() }},
			{Label: "切换界面主题", Accelerator: "CmdOrCtrl+Shift+L", Click: func(*mygo.MenuItem, *mygo.Window) { s.ToggleTheme() }},
			mygo.Separator(),
			{Label: "在浏览器中打开", Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { s.OpenInBrowser() }},
			{Label: "打开数据目录", Click: func(*mygo.MenuItem, *mygo.Window) { s.OpenDataDir() }},
		}},
		{Label: "窗口", Submenu: []*mygo.MenuItem{
			{Label: "隐藏窗口", Click: func(*mygo.MenuItem, *mygo.Window) { s.hide() }},
			mygo.Separator(),
			{Label: "退出 adago-design", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		}},
	})
	return menu
}

// startTray 在应用就绪后创建托盘图标；失败只记日志（没有托盘也能用窗口）。
func startTray(s *Shell) *mygo.Tray {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示窗口", Click: func(*mygo.MenuItem, *mygo.Window) { s.Focus() }},
		{Label: "隐藏窗口", Click: func(*mygo.MenuItem, *mygo.Window) { s.hide() }},
		mygo.Separator(),
		{Label: "在浏览器中打开", Click: func(*mygo.MenuItem, *mygo.Window) { s.OpenInBrowser() }},
		{Label: "复制 MCP 配置（HTTP）", Click: func(*mygo.MenuItem, *mygo.Window) { s.copyMCPConfig() }},
		{Label: "打开数据目录", Click: func(*mygo.MenuItem, *mygo.Window) { s.OpenDataDir() }},
		mygo.Separator(),
		{Label: "重新加载画布页", Click: func(*mygo.MenuItem, *mygo.Window) { s.Reload() }},
		mygo.Separator(),
		{Label: "退出 adago-design", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      trayID,
		Icon:    trayIconPNG,
		ToolTip: appTitle,
		Menu:    menu,
	})
	if err != nil {
		log.Printf("adago-design: tray: %v", err)
		return nil
	}
	return tray
}

// hide 把窗口收进托盘（服务与 MCP 继续运行）。
func (s *Shell) hide() {
	mygo.RunOnMain(func() {
		if w := s.window(); w != nil {
			w.Hide()
		}
	})
}

// copyMCPConfig 把可直接使用的 HTTP MCP 条目写进剪贴板。
func (s *Shell) copyMCPConfig() {
	cfg := s.mcpConfig()
	if cfg == "" {
		return
	}
	mygo.Clipboard.WriteText(cfg)
	s.showToast("已复制 MCP 配置（HTTP）")
}

// ---------------- 视图 ----------------

func (s *Shell) view(c *ui.Context) {
	c.SetTheme(themeFor(s.dark))
	tk := tokensFor(s.dark)
	w := s.window()
	full := w != nil && w.IsFullScreen()

	ui.Column(c).Fill().Background(tk.canvas).AlignItems(ui.Stretch).Children(func() {
		if !full {
			s.titleBar(c, tk)
			ui.Row(c).Height(1).Background(tk.line)
		}
		s.body(c, tk)
		if !full {
			ui.Row(c).Height(1).Background(tk.line)
			s.statusBar(c, tk)
		}
		if s.toast != "" && time.Since(s.toastAt) < toastLife {
			s.toastView(c, tk, full)
		}
		if full {
			s.fullHint(c, tk)
		}
	})
}

func (s *Shell) titleBar(c *ui.Context, tk tokens) {
	bar := c.TitleBar()
	h := bar.Height
	if h < titleBarH {
		h = titleBarH
	}
	ui.Row(c).Height(h).Padding(0, bar.Right+8, 0, 12).Gap(10).
		AlignItems(ui.Center).Background(tk.surface).DragWindow().Children(func() {
		// 身份：墨底纸字的印章 + 名称 + 机器化 kicker
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(22, 22).Radius(6).Background(tk.brand).Center().Children(func() {
				ui.Text(c, "D").FontSize(12).Bold().TextColor(tk.onBrand)
			})
			ui.Text(c, appTitle).FontSize(13).FontWeight(600).TextColor(tk.ink)
			ui.Text(c, "CANVAS").Font(monoFamily).FontSize(9).TextColor(tk.ink3)
		})
		ui.Row(c).Width(1).Height(16).Background(tk.line)

		// 作用域只读回显（页面 dataset 提供，切换仍在页面里）
		if s.ph == phaseReady || s.ph == phaseBoot {
			scope := s.pageSt.Scope
			if scope == "" {
				scope = "全部项目"
			}
			ui.Row(c).Height(26).Padding(0, 8).Gap(6).Radius(6).
				AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconFolder).Size(14, 14).TextColor(tk.ink3)
				ui.Text(c, scope).FontSize(12).FontWeight(500).TextColor(tk.ink).SingleLine()
				if s.pageSt.ProjectCount > 0 && s.pageSt.Scope == "" {
					ui.Textf(c, "%d", s.pageSt.ProjectCount).Font(monoFamily).FontSize(10).
						TextColor(tk.ink3)
				}
			})
		}

		ui.Row(c).Grow(1)
		s.mcpPill(c, tk)
		s.actions(c, tk)
	})
}

func (s *Shell) mcpPill(c *ui.Context, tk tokens) {
	label, fg, bg, dot := "等待就绪", tk.ink3, tk.surface, tk.ink3
	switch {
	case s.health.PersistError != "":
		label, fg, bg, dot = "写盘失败", tk.dangerInk, tk.dangerBg, tk.dangerInk
	case s.health.OK:
		label, fg, bg, dot = portLabel(s.opt.BaseURL)+"/mcp", tk.successInk, tk.successBg, tk.successInk
	}
	ui.Row(c).Height(24).Padding(0, 8).Gap(6).Radius(6).
		Background(bg).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, "MCP").Font(monoFamily).FontSize(10).FontWeight(600).TextColor(fg)
		if s.health.OK && s.health.PersistError == "" {
			ui.Row(c).Width(1).Height(10).Background(tk.line)
		}
		ui.Text(c, label).Font(monoFamily).FontSize(10).TextColor(fg)
	})
}

func (s *Shell) actions(c *ui.Context, tk tokens) {
	if s.iconButton(c, tk, iconTheme, "切换界面主题（Ctrl+Shift+L）") {
		s.ToggleTheme()
	}
	if s.iconButton(c, tk, iconRefresh, "重新加载页面（Ctrl+R）") {
		s.Reload()
	}
	if s.iconButton(c, tk, iconExternal, "在浏览器中打开") {
		s.OpenInBrowser()
	}
	more := ui.ButtonBase(c).Label("更多").Size(28, 28).Radius(6).Cursor(ui.CursorPointer).
		Tooltip("更多")
	if more.Hovered() {
		more.Background(tk.subtle)
	}
	more.Children(func() {
		ui.Icon(c, iconMore).Size(16, 16).TextColor(tk.ink2)
	})
	if more.Clicked() {
		s.more = !s.more
	}
	ui.Popover(c, more, &s.more, func() {
		s.menuRow(c, tk, "重新加载页面", "Ctrl+R", func() { s.more = false; s.Reload() })
		s.menuRow(c, tk, "开发者工具", "Ctrl+Shift+I", func() { s.more = false; s.ToggleDevTools() })
		s.menuRow(c, tk, "沉浸全屏", "F11", func() { s.more = false; s.ToggleFullScreen() })
		ui.Row(c).Height(1).Margin(4, 6).Background(tk.line)
		s.menuRow(c, tk, "在浏览器中打开", "", func() { s.more = false; s.OpenInBrowser() })
		s.menuRow(c, tk, "打开数据目录", "", func() { s.more = false; s.OpenDataDir() })
		s.menuRow(c, tk, "复制 MCP 配置", "", func() { s.more = false; s.copyMCPConfig() })
	})
}

func (s *Shell) menuRow(c *ui.Context, tk tokens, label, accel string, click func()) {
	row := ui.Row(c).Height(30).Padding(0, 8).Gap(10).Radius(6).Width(232).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(tk.subtle)
	}
	row.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(tk.ink)
		ui.Row(c).Grow(1)
		if accel != "" {
			ui.Text(c, accel).Font(monoFamily).FontSize(10).TextColor(tk.ink3)
		}
	})
	if row.Clicked() {
		click()
	}
}

// iconButton 画一枚 28×28 图标按钮，返回是否被点击。
func (s *Shell) iconButton(c *ui.Context, tk tokens, icon *ui.SVG, tip string) bool {
	b := ui.ButtonBase(c).Label(tip).Size(28, 28).Radius(6).
		Cursor(ui.CursorPointer).Tooltip(tip)
	if b.Hovered() {
		b.Background(tk.subtle)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(tk.ink2)
	})
	return b.Clicked()
}

func (s *Shell) body(c *ui.Context, tk tokens) {
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		v := s.currentPage()
		showPage := v != nil && (s.ph == phaseReady || s.ph == phaseBoot)
		if showPage {
			ui.WebView(c, v).Fill()
		}
		switch s.ph {
		case phaseBoot:
			s.bootView(c, tk)
		case phasePageFault:
			s.faultView(c, tk, faultPage)
		case phaseConflict:
			s.faultView(c, tk, faultConflict)
		}
	})
}

// bootView 是冷启动/重载时的启动卡：三步状态 + 一条进度线。
func (s *Shell) bootView(c *ui.Context, tk tokens) {
	ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(tk.canvas).Center().Children(func() {
		ui.Column(c).Width(460).Padding(28, 28, 22, 28).Radius(12).
			Background(tk.elevated).Border(1, tk.line).Children(func() {
			ui.Row(c).Size(56, 56).Radius(16).Background(tk.brand).Center().Children(func() {
				ui.Text(c, "D").FontSize(26).Bold().TextColor(tk.onBrand)
			})
			ui.Row(c).Height(16)
			ui.Text(c, "正在启动 adago-design").FontSize(17).FontWeight(600).TextColor(tk.ink)
			ui.Row(c).Height(6)
			ui.Text(c, "窗口先出现，画布页在服务绑定端口后加载。首次启动需要初始化 WebView2 运行时，通常不超过 3 秒。").
				FontSize(12).TextColor(tk.ink3)
			ui.Row(c).Height(16)

			stepDone(c, tk, "数据目录", shortDir(s.opt.DataDir))
			stepDone(c, tk, "画布服务", portLabel(s.opt.BaseURL))
			stepBusy(c, tk, "加载画布页面", "/?embed=native")

			ui.Row(c).Height(18)
			ui.Row(c).Height(2).Radius(1).Background(tk.line).Children(func() {
				ui.Row(c).WidthPercent(62).Height(2).Background(tk.ink)
			})
			ui.Row(c).Height(6)
			ui.Text(c, "加载完成即切换；失败进入「页面故障」，可重试或改用浏览器。").
				FontSize(11).TextColor(tk.ink3)
		})
	})
}

func stepDone(c *ui.Context, tk tokens, label, value string) {
	ui.Row(c).Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(16, 16).Radius(8).Background(tk.successBg).Center().Children(func() {
			ui.Icon(c, iconCheck).Size(10, 10).TextColor(tk.successInk)
		})
		ui.Text(c, label).FontSize(12).TextColor(tk.ink2)
		ui.Row(c).Grow(1)
		ui.Text(c, value).Font(monoFamily).FontSize(10).TextColor(tk.ink3).SingleLine()
	})
}

func stepBusy(c *ui.Context, tk tokens, label, value string) {
	ui.Row(c).Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(14, 14)
		ui.Text(c, label).FontSize(12).TextColor(tk.ink2)
		ui.Row(c).Grow(1)
		ui.Text(c, value).Font(monoFamily).FontSize(10).TextColor(tk.ink3)
	})
}

// faultKind 决定故障卡上的文案与按钮。
type faultKind int

const (
	faultPage faultKind = iota
	faultConflict
)

// faultView 是页面级的故障卡（加载失败 / 渲染进程退出 / 实例占用）。
func (s *Shell) faultView(c *ui.Context, tk tokens, kind faultKind) {
	title, body, code, url := "", "", "", ""
	conflict := kind == faultConflict
	if conflict {
		title = "此数据目录已有实例在运行"
		body = "另一个 adago-design 进程正使用同一份数据目录，且它没有窗口（通常以 serve -no-open 或 mcp 启动）。两个进程同时写 canvas.json 会互相覆盖，所以本窗口不启动服务。"
		code = "instance_exists · 已有实例无窗口"
		url = s.opt.ConflictURL
	} else {
		title = "画布页面无法加载"
		body = "本地服务仍在运行（MCP 端点可用），只是内嵌页面没有渲染成功。修好前 canvas_screenshot 会超时。"
		code = s.fault.code
		if s.fault.detail != "" {
			code = strings.TrimSpace(code + " · " + s.fault.detail)
		}
		url = strings.TrimRight(s.opt.BaseURL, "/") + "/?embed=native"
	}

	ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(tk.canvas).Center().Children(func() {
		ui.Column(c).Width(460).Padding(28, 28, 22, 28).Radius(12).
			Background(tk.elevated).Border(1, tk.line).Children(func() {
			ui.Row(c).Size(56, 56).Radius(16).Background(tk.dangerBg).Center().Children(func() {
				ui.Icon(c, iconWarning).Size(28, 28).TextColor(tk.dangerInk)
			})
			ui.Row(c).Height(16)
			ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(tk.ink)
			ui.Row(c).Height(6)
			ui.Text(c, body).FontSize(12).TextColor(tk.ink3)
			ui.Row(c).Height(14)

			ui.Row(c).Padding(8, 10).Radius(6).Background(tk.dangerBg).Children(func() {
				ui.Text(c, code).Font(monoFamily).FontSize(10).TextColor(tk.dangerInk)
			})
			ui.Row(c).Height(8)
			ui.Text(c, url).Font(monoFamily).FontSize(10).TextColor(tk.ink3).SingleLine()
			ui.Row(c).Height(16)

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if conflict {
					if ui.PrimaryButton(c, "在浏览器中打开").Clicked() {
						s.OpenInBrowser()
					}
					if ui.Button(c, "打开数据目录").Clicked() {
						s.OpenDataDir()
					}
					if ui.Button(c, "关闭窗口").Clicked() {
						mygo.RunOnMain(func() {
							if w := s.window(); w != nil {
								w.Close()
							}
						})
					}
					return
				}
				if ui.PrimaryButton(c, "重新加载页面").Clicked() {
					s.Reload()
				}
				if ui.Button(c, "开发者工具").Clicked() {
					s.ToggleDevTools()
				}
				if ui.Button(c, "在浏览器打开").Clicked() {
					s.OpenInBrowser()
				}
			})
			ui.Row(c).Height(14)
			hint := "自动重试已执行 1 次 · 页面进程崩溃与加载失败共用此态；重载不影响 canvas.json。"
			if conflict {
				hint = "原生窗口需要独占数据目录。关闭旧实例后重开即回到窗口形态；想并存请用 -data 指定另一目录。"
			}
			ui.Text(c, hint).FontSize(11).TextColor(tk.ink3)
		})
	})
}

func (s *Shell) statusBar(c *ui.Context, tk tokens) {
	dot, tone := tk.ink3, "连接中"
	switch {
	case s.ph == phaseConflict:
		dot, tone = tk.warnInk, "未启动画布服务"
	case s.ph == phasePageFault:
		dot, tone = tk.dangerInk, "页面故障"
	case s.health.PersistError != "":
		dot, tone = tk.dangerInk, "写盘失败"
	case s.health.OK:
		dot, tone = tk.successInk, "画布服务"
	}
	ui.Row(c).Height(statusBarH).Padding(0, 12).Gap(10).AlignItems(ui.Center).
		Background(tk.surface).Children(func() {
		ui.Row(c).Size(5, 5).Radius(3).Background(dot)
		ui.Text(c, tone).FontSize(11).TextColor(tk.ink3)
		if s.opt.BaseURL != "" {
			ui.Text(c, strings.TrimPrefix(s.opt.BaseURL, "http://")).Font(monoFamily).
				FontSize(10).TextColor(tk.ink2)
		}
		ui.Row(c).Width(1).Height(11).Background(tk.line)
		ui.Text(c, "数据").FontSize(11).TextColor(tk.ink3)
		ui.Text(c, shortDir(s.opt.DataDir)).Font(monoFamily).FontSize(10).
			TextColor(tk.ink2).SingleLine()
		ui.Row(c).Grow(1)
		if s.health.OK {
			ui.Textf(c, "%d 个项目 · %d 张稿", s.health.ProjectCount, s.health.NodeCount).
				FontSize(11).TextColor(tk.ink3)
			ui.Row(c).Width(1).Height(11).Background(tk.line)
			ui.Textf(c, "rev %d", s.health.Rev).Font(monoFamily).FontSize(10).TextColor(tk.ink3)
			ui.Row(c).Width(1).Height(11).Background(tk.line)
			if s.health.PersistError != "" {
				ui.Text(c, clip("持久化失败: "+s.health.PersistError, 60)).
					FontSize(10).TextColor(tk.dangerInk).SingleLine()
			} else {
				ui.Text(c, "持久化正常").FontSize(11).TextColor(tk.successInk)
			}
		}
	})
}

func (s *Shell) toastView(c *ui.Context, tk tokens, full bool) {
	bottom := float32(statusBarH + 12)
	if full {
		bottom = 16
	}
	ui.Row(c).Absolute().Bottom(bottom).Left(0).Right(0).Justify(ui.Center).Children(func() {
		ui.Row(c).Height(34).Padding(0, 12).Gap(8).Radius(6).
			Background(tk.elevated).Border(1, tk.line).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconCheck).Size(14, 14).TextColor(tk.successInk)
			ui.Text(c, s.toast).FontSize(12).TextColor(tk.ink)
		})
	})
}

func (s *Shell) fullHint(c *ui.Context, tk tokens) {
	ui.Row(c).Absolute().Top(14).Right(14).Children(func() {
		b := ui.ButtonBase(c).Label("退出全屏").Height(26).Padding(0, 10).Gap(6).Radius(13).
			Background(tk.surface).Border(1, tk.line).AlignItems(ui.Center).
			Cursor(ui.CursorPointer).Tooltip("F11 退出全屏")
		b.Children(func() {
			ui.Icon(c, iconFullscreen).Size(12, 12).TextColor(tk.ink2)
			ui.Text(c, "全屏 · F11 退出").FontSize(11).TextColor(tk.ink2)
		})
		if b.Clicked() {
			s.ToggleFullScreen()
		}
	})
}

// shortDir 把用户主目录缩成 ~，状态条与启动卡才放得下。
func shortDir(dir string) string {
	d := strings.TrimSpace(dir)
	if d == "" {
		return "—"
	}
	if home, err := mygo.App.Path(mygo.PathHome); err == nil && home != "" {
		if strings.HasPrefix(strings.ToLower(d), strings.ToLower(home)) {
			return "~" + d[len(home):]
		}
	}
	return d
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
