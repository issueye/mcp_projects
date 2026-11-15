package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 窗口几何与壳尺寸（设计稿 09 视觉规范）：标题条 40 / 状态条 26。
const (
	titleBarH   = 40
	statusBarH  = 26
	appTitle    = "adago-design"
	embedQuery  = "?embed=native"
	trayID      = "adago-design"
	windowMinW  = 1040
	windowMinH  = 640
	toastLife   = 3200 * time.Millisecond
	pollEvery   = 2 * time.Second
	retryLimit  = 1
	mcpEndpoint = "/mcp"
)

// Options 是壳的装配参数：由 main 在画布服务就绪后填入。
type Options struct {
	BaseURL     string // 画布服务地址；ConflictURL 非空时可为空
	DataDir     string // 数据目录（窗口记忆写这里）
	Theme       string // light | dark（缺省）
	Version     string // 版本号（托盘/关于用）
	ConflictURL string // 非空 = 同数据目录已有无窗口实例，只显示故障卡
	OnQuit      func() // 退出前回调（停 HTTP 服务）
}

// phase 是窗口当前的形态：页面、启动卡、故障卡三者互斥。
type phase int

const (
	phaseBoot phase = iota
	phaseReady
	phasePageFault
	phaseConflict
)

// pageState 是页面回写到 <html> dataset 的作用域读数（契约见页面 native.js）。
type pageState struct {
	Scope        string `json:"scope"`
	ScopeCount   int    `json:"scopeCount"`
	ProjectCount int    `json:"projectCount"`
	NodeCount    int    `json:"nodeCount"`
	Theme        string `json:"theme"`
}

// healthState 是 /api/health 的壳侧快照。
type healthState struct {
	OK           bool
	NodeCount    int
	ProjectCount int
	Rev          int64
	PersistError string
	UpdatedAt    time.Time
}

// Shell 是一枚原生窗口：窗口 + 内嵌画布页 + 标题条/状态条 + 托盘。
// 除 poll 循环读 page 指针外，其余状态只在主线程（RunOnMain）读写。
type Shell struct {
	opt   Options
	dark  bool
	win   *mygo.Window
	mu    sync.Mutex
	page  *mygo.WebView
	ph    phase
	boot  int // 启动卡已完成的步骤数（0..3）
	fault struct {
		code   string
		detail string
	}
	pageSt    pageState
	health    healthState
	lastTitle string
	toast     string
	toastAt   time.Time
	retries   int
	tray      *mygo.Tray
	more      bool
	quit      sync.Once
	saveTimer *time.Timer
}

// Acquire 取单实例锁。名字按数据目录分区：不同 -data 的实例可以并存，
// 同一数据目录的第二次启动只会把命令行转交给已运行实例（随后返回 false）。
func Acquire(opt Options) (*Shell, bool) {
	mygo.App.SetName(lockName(opt.DataDir))
	mygo.App.SetVersion(opt.Version)
	newInstance := mygo.App.RequestSingleInstanceLock()
	if !newInstance {
		return nil, false
	}
	s := &Shell{opt: opt, dark: strings.EqualFold(opt.Theme, "dark"), ph: phaseBoot}
	if opt.ConflictURL != "" {
		s.ph = phaseConflict
	}
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		if !s.Focus() {
			return
		}
		s.showToast("已切换到正在运行的窗口")
	})
	return s, true
}

// lockName 把数据目录折进应用名：mygo 的单实例锁按应用名分区，而本程序
// 的「唯一实例」语义是「每个数据目录一个」，两者要对齐。
func lockName(dataDir string) string {
	d := strings.TrimSpace(dataDir)
	if d == "" {
		return appTitle
	}
	sum := sha256.Sum256([]byte(strings.ToLower(d)))
	return appTitle + "@" + hex.EncodeToString(sum[:4])
}

// Run 进入 mygo 主循环并阻塞，直到窗口关闭（关闭窗口 = 退出应用）。
func (s *Shell) Run(ctx context.Context) error {
	mygo.App.WhenReady(func() {
		s.buildWindow()
		go s.pollLoop(ctx)
	})
	// 窗口几何要在窗口关闭「之前」取：OnWillQuit 时窗口已销毁。
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		s.saveBounds()
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if s.tray != nil {
			s.tray.Destroy()
			s.tray = nil
		}
	})
	mygo.App.OnQuit(func() {
		if s.saveTimer != nil {
			s.saveTimer.Stop()
			s.saveTimer = nil
		}
		s.quit.Do(func() {
			if s.opt.OnQuit != nil {
				s.opt.OnQuit()
			}
		})
	})
	go func() {
		<-ctx.Done()
		mygo.RunOnMain(func() {
			if w := s.window(); w != nil {
				w.Close()
			}
		})
	}()
	return mygo.App.Run()
}

// Focus 把已有窗口前置（二次启动、截图超时自动拉起时使用）。
// 返回 false 表示窗口还没建好（调用方可以退回系统浏览器）。
func (s *Shell) Focus() bool {
	w := s.window()
	if w == nil || w.IsDestroyed() {
		return false
	}
	mygo.RunOnMain(func() {
		w.Show()
		if w.IsMinimized() {
			w.Restore()
		}
		w.Focus()
		w.Invalidate()
	})
	return true
}

// BaseURL 暴露画布地址（托盘复制 MCP 配置用）。
func (s *Shell) BaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opt.BaseURL
}

// SetBaseURL 在 HTTP 服务绑定端口后注入画布地址（Acquire 时还没有）。
func (s *Shell) SetBaseURL(u string) {
	s.mu.Lock()
	s.opt.BaseURL = u
	s.mu.Unlock()
}

// SetConflict 让窗口进入「数据目录被占用」故障态（不启动画布服务）。
func (s *Shell) SetConflict(u string) {
	s.mu.Lock()
	s.opt.ConflictURL = u
	s.mu.Unlock()
	s.ph = phaseConflict
}

// SetOnQuit 注册退出前回调（停止 HTTP 服务）。
func (s *Shell) SetOnQuit(fn func()) {
	s.mu.Lock()
	s.opt.OnQuit = fn
	s.mu.Unlock()
}

// Quit 退出应用（关闭窗口、停服务、释放单实例锁）。
func (s *Shell) Quit() {
	mygo.RunOnMain(func() { mygo.App.Quit() })
}

func (s *Shell) window() *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.win
}

func (s *Shell) currentPage() *mygo.WebView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.page
}

// buildWindow 在主线程创建窗口：先画标题条/状态条，再挂 WebView。
func (s *Shell) buildWindow() {
	st := loadWindowState(s.opt.DataDir)
	if st.Theme != "" {
		s.dark = strings.EqualFold(st.Theme, "dark")
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           appTitle,
		Width:           st.Width,
		Height:          st.Height,
		X:               st.X,
		Y:               st.Y,
		MinWidth:        windowMinW,
		MinHeight:       windowMinH,
		BackgroundColor: bgHex(s.dark),
		TitleBarStyle:   mygo.TitleBarHidden,
		TitleBarHeight:  titleBarH,
		AutoHideMenuBar: true,
		Content:         ui.View(s.view),
	})
	s.mu.Lock()
	s.win = w
	s.mu.Unlock()
	if len(windowIconPNG) > 0 {
		if err := w.SetIcon(windowIconPNG); err != nil {
			log.Printf("adago-design: window icon: %v", err)
		}
	}
	w.SetMenu(s.buildMenu())
	// 窗口几何在关闭前保存：Windows 关闭最后一个窗口时，OnBeforeQuit 里
	// 窗口已被销毁，只有 OnClose / 尺寸事件还能读到有效 bounds。
	w.OnClose(func(*mygo.CloseEvent) { s.saveBounds() })
	w.OnResize(func() { s.scheduleSave() })
	w.OnMove(func() { s.scheduleSave() })
	s.tray = startTray(s)
	if st.Maximized {
		w.Maximize()
	}
	if s.opt.ConflictURL != "" {
		s.ph = phaseConflict
	} else {
		s.startPage()
	}
	w.Invalidate()
}

// startPage 建 WebView 指向本进程的画布服务；页面就绪前显示启动卡。
func (s *Shell) startPage() {
	w := s.window()
	if w == nil {
		return
	}
	v, err := w.NewWebView(mygo.WebViewOptions{
		URL:             s.opt.BaseURL + "/" + embedQuery,
		BackgroundColor: bgHex(s.dark),
	})
	if err != nil {
		s.failPage("webview_unavailable", err.Error())
		return
	}
	s.mu.Lock()
	s.page = v
	s.mu.Unlock()
	p := v.Page()
	p.OnDOMReady(func() {
		mygo.RunOnMain(func() {
			s.ph = phaseReady
			s.boot = 3
			s.retries = 0
			s.applyPageTheme()
			if w := s.window(); w != nil {
				w.Invalidate()
			}
			v.Focus()
		})
	})
	p.OnDidFailLoad(func(e *mygo.LoadError) {
		mygo.RunOnMain(func() {
			code := "load_failed"
			detail := "页面加载失败"
			if e != nil {
				code = fmt.Sprintf("ERR_%d", e.Code)
				if e.Description != "" {
					detail = e.Description
				}
			}
			s.pageFailed(code, detail)
		})
	})
	p.OnRenderProcessGone(func(reason string) {
		mygo.RunOnMain(func() { s.failPage("renderer_gone", "页面渲染进程已退出: "+reason) })
	})
	if s.opt.BaseURL == "" {
		s.failPage("no_service", "画布服务未就绪")
	}
}

// pageFailed 处理一次加载失败：自动重试一次，仍失败进入故障卡。
func (s *Shell) pageFailed(code, detail string) {
	if s.retries < retryLimit {
		s.retries++
		s.ph = phaseBoot
		s.boot = 2
		if w := s.window(); w != nil {
			w.Invalidate()
		}
		if v := s.currentPage(); v != nil {
			v.Page().Reload()
		}
		return
	}
	s.failPage(code, detail)
}

func (s *Shell) failPage(code, detail string) {
	s.ph = phasePageFault
	s.fault.code = code
	s.fault.detail = detail
	if w := s.window(); w != nil {
		w.Invalidate()
	}
}

// Reload 重新加载内嵌页（标题条按钮 / 菜单 / 托盘共用）。
func (s *Shell) Reload() {
	mygo.RunOnMain(func() {
		if s.opt.ConflictURL != "" {
			return
		}
		v := s.currentPage()
		if v == nil {
			s.startPage()
			return
		}
		s.ph = phaseBoot
		s.boot = 2
		s.retries = 0
		s.fault = struct {
			code   string
			detail string
		}{}
		v.Page().Reload()
		if w := s.window(); w != nil {
			w.Invalidate()
		}
	})
}

// ToggleTheme 同时切换壳与页面：壳换 ui.Theme，页面写 localStorage + data-theme。
func (s *Shell) ToggleTheme() {
	mygo.RunOnMain(func() {
		s.dark = !s.dark
		s.applyPageTheme()
		if w := s.window(); w != nil {
			w.Invalidate()
		}
	})
}

// applyPageTheme 把当前外观写给页面（页面自身也有主题按钮，浏览器模式下用）。
func (s *Shell) applyPageTheme() {
	v := s.currentPage()
	if v == nil {
		return
	}
	theme := "light"
	if s.dark {
		theme = "dark"
	}
	js := fmt.Sprintf(
		`(function(){try{localStorage.setItem("adago.theme",%q)}catch(e){};`+
			`document.documentElement.dataset.theme=%q;return document.documentElement.dataset.theme}())`,
		theme, theme)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_, _ = v.Page().EvalContext(ctx, js)
	}()
}

// ToggleFullScreen 沉浸全屏：隐藏两条原生栏，页面铺满。
func (s *Shell) ToggleFullScreen() {
	mygo.RunOnMain(func() {
		if w := s.window(); w != nil {
			w.ToggleFullScreen()
			w.Invalidate()
		}
	})
}

// OpenInBrowser 把画布交给系统浏览器（排障与回退路径）。
func (s *Shell) OpenInBrowser() {
	u := s.opt.BaseURL
	if u == "" {
		u = s.opt.ConflictURL
	}
	if u == "" {
		return
	}
	if err := mygo.Shell.OpenExternal(u); err != nil {
		log.Printf("adago-design: open browser: %v", err)
	}
}

// OpenDataDir 用系统文件管理器打开数据目录。
func (s *Shell) OpenDataDir() {
	if err := mygo.Shell.OpenPath(s.opt.DataDir); err != nil {
		log.Printf("adago-design: open data dir: %v", err)
	}
}

// ToggleDevTools 打开/关闭页面开发者工具（调试用）。
func (s *Shell) ToggleDevTools() {
	mygo.RunOnMain(func() {
		if v := s.currentPage(); v != nil {
			v.Page().ToggleDevTools()
		}
	})
}

func (s *Shell) showToast(msg string) {
	mygo.RunOnMain(func() {
		s.toast = msg
		s.toastAt = time.Now()
		stamp := s.toastAt
		if w := s.window(); w != nil {
			w.Invalidate()
		}
		time.AfterFunc(toastLife, func() {
			mygo.RunOnMain(func() {
				if s.toastAt.Equal(stamp) {
					s.toast = ""
					if w := s.window(); w != nil {
						w.Invalidate()
					}
				}
			})
		})
	})
}

// syncWindowTitle 让任务栏/Alt+Tab 显示当前作用域（内嵌页的 <title> 不会
// 自动传给宿主窗口：WebView 是子页，窗口标题由壳负责）。
func (s *Shell) syncWindowTitle() {
	w := s.window()
	if w == nil || s.ph == phaseConflict || s.ph == phasePageFault {
		return
	}
	title := appTitle
	if s.pageSt.Scope != "" && s.ph == phaseReady {
		title = s.pageSt.Scope + " · " + appTitle
	}
	if title == s.lastTitle {
		return
	}
	s.lastTitle = title
	w.SetTitle(title)
}

// scheduleSave 把拖动/缩放时的写盘合并成一次（700ms 静默后落盘）。
func (s *Shell) scheduleSave() {
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(700*time.Millisecond, func() {
		mygo.RunOnMain(func() { s.saveBounds() })
	})
}

func (s *Shell) saveBounds() {
	w := s.window()
	if w == nil || w.IsDestroyed() {
		return
	}
	// 用 Size/Position（与 WindowOptions 同一坐标系）；Bounds 在部分平台返回
	// 设备像素，反复保存会让窗口逐次变高。
	width, height := w.Size()
	x, y := w.Position()
	st := windowState{
		Width:     width,
		Height:    height,
		X:         x,
		Y:         y,
		Maximized: w.IsMaximized(),
		Theme:     themeName(s.dark),
	}
	if err := saveWindowState(s.opt.DataDir, st); err != nil {
		log.Printf("adago-design: save window state: %v", err)
	} else {
		log.Printf("adago-design: window state saved: %dx%d @ %d,%d", st.Width, st.Height, st.X, st.Y)
	}
}

func themeName(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// ---------------- 轮询：页面作用域 + 服务健康 ----------------

func (s *Shell) pollLoop(ctx context.Context) {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if st, ok := s.readPageState(ctx); ok {
				mygo.RunOnMain(func() {
					s.pageSt = st
					if st.Theme != "" {
						s.dark = st.Theme == "dark"
					}
					s.syncWindowTitle()
					if w := s.window(); w != nil {
						w.Invalidate()
					}
				})
			}
			if h, ok := fetchHealth(client, s.opt.BaseURL); ok {
				mygo.RunOnMain(func() {
					s.health = h
					if w := s.window(); w != nil {
						w.Invalidate()
					}
				})
			}
		}
	}
}

// readPageState 读页面 dataset（契约：state.js 每次作用域/数量变化时写入）。
func (s *Shell) readPageState(ctx context.Context) (pageState, bool) {
	v := s.currentPage()
	if v == nil {
		return pageState{}, false
	}
	const js = `(function(){var r=document.documentElement;return JSON.stringify({` +
		`scope:r.dataset.scope||"",scopeCount:+(r.dataset.scopeCount||0),` +
		`projectCount:+(r.dataset.projectCount||0),nodeCount:+(r.dataset.nodeCount||0),` +
		`theme:r.dataset.theme||""})})()`
	evalCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	raw, err := v.Page().EvalContext(evalCtx, js)
	if err != nil {
		return pageState{}, false
	}
	var text string
	if s, ok := raw.(string); ok {
		text = s
	} else {
		b, err := json.Marshal(raw)
		if err != nil {
			return pageState{}, false
		}
		text = string(b)
	}
	var st pageState
	if err := json.Unmarshal([]byte(text), &st); err != nil {
		return pageState{}, false
	}
	return st, true
}

// fetchHealth 读 /api/health；服务未起或超时都算「不可用」，不打扰用户。
func fetchHealth(client *http.Client, baseURL string) (healthState, bool) {
	if baseURL == "" {
		return healthState{}, false
	}
	res, err := client.Get(baseURL + "/api/health")
	if err != nil {
		return healthState{}, false
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if err != nil {
		return healthState{}, false
	}
	var body struct {
		OK           bool   `json:"ok"`
		NodeCount    int    `json:"node_count"`
		ProjectCount int    `json:"project_count"`
		Rev          int64  `json:"rev"`
		PersistError string `json:"persist_error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return healthState{}, false
	}
	return healthState{
		OK:           body.OK,
		NodeCount:    body.NodeCount,
		ProjectCount: body.ProjectCount,
		Rev:          body.Rev,
		PersistError: body.PersistError,
		UpdatedAt:    time.Now(),
	}, true
}

// ---------------- 小工具 ----------------

// portLabel 从 BaseURL 提取 ":8788" 形式，用于标题条/MCP 配置。
func portLabel(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Port() == "" {
		return ""
	}
	if _, err := strconv.Atoi(u.Port()); err != nil {
		return ""
	}
	return ":" + u.Port()
}

// humanAge 把「多久以前」写成短读数（3s / 2m / 1h）。
func humanAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Second:
		return "刚刚"
	case d < time.Minute:
		return fmt.Sprintf("%ds 前", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm 前", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh 前", int(d.Hours()))
	}
}

// mcpConfig 返回可直接粘进 MCP 客户端的 HTTP 条目（托盘复制）。
func (s *Shell) mcpConfig() string {
	u := strings.TrimRight(s.opt.BaseURL, "/") + mcpEndpoint
	cfg := map[string]any{
		"id":        "adago_design",
		"transport": "streamable-http",
		"url":       u,
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return ""
	}
	return string(raw) + "\n"
}
