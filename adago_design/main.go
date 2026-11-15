// adago-design 是设计体系 MCP：内置多套设计体系 Token 与组件库，
// 提供原生窗口内的无限画布（内嵌 WebView，摆放/对比组件预览、截图回传）。
//
// 形态：
//
//	adago-design.exe            # 原生窗口（mygo + 内嵌画布页），带 HTTP MCP
//	adago-design.exe serve      # 同上（显式写法）
//	adago-design.exe serve -browser   # 回退：系统浏览器打开画布
//	adago-design.exe mcp        # stdio MCP + 无窗口画布服务（Agent 接入）
//	adago-design.exe version    # 打印版本
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"adagodesign/internal/app"
	"adagodesign/internal/applog"
	"adagodesign/internal/config"
	"adagodesign/internal/mcpserver"
	"adagodesign/internal/native"
	"adagodesign/internal/openbrowser"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[adago-design] ")

	cmd := "serve"
	rest := os.Args[1:]
	if len(rest) > 0 {
		switch rest[0] {
		case "mcp", "serve", "version", "help":
			cmd = rest[0]
			rest = rest[1:]
		}
	}
	var err error
	switch cmd {
	case "version":
		fmt.Println("adago-design", config.Version)
	case "help":
		printHelp()
	case "mcp":
		err = runMCP(rest)
	default:
		err = runServe(rest)
	}
	if err != nil {
		log.Fatalf("退出: %v", err)
	}
}

func printHelp() {
	fmt.Print(`adago-design ` + config.Version + ` — 设计体系 MCP（原生窗口 · 内嵌无限画布）

用法:
  adago-design [serve] [flags]   原生窗口：内嵌画布页 + HTTP MCP（缺省）
  adago-design mcp [flags]       stdio MCP（Agent 接入；缺省无窗口）
  adago-design version           打印版本号

Flags:
  -port int      画布 HTTP 端口，默认 8788（被占用自动 +1 探测）
  -data string   数据目录（默认 %APPDATA%\adago-design，可被 ADAGO_DESIGN_DATA 覆盖）
  -open          打开界面（serve 缺省原生窗口；mcp 加此项等于 -window）
  -browser       用系统默认浏览器代替原生窗口（回退/排障）
  -window        mcp 模式下也创建原生窗口
  -no-open       只跑服务，不打开任何界面
  -theme string  画布界面主题 light（纸面，默认）|dark（墨纸）

环境变量:
  ADAGO_DESIGN_DATA   数据目录
  ADAGO_DESIGN_OPEN=1 打开界面（等价 -open）
  ADAGO_DESIGN_UI     界面形态 native|browser|none，压过以上标志

说明:
  本程序以 windowsgui 链接（无黑色控制台）；日志写在 <数据目录>\adago-design.log（4MB 轮换）。
`)
}

// runServe 是人工查看模式：缺省原生窗口，-browser 回退系统浏览器。
func runServe(args []string) error {
	cfg, err := config.FromFlags("adago-design serve", args)
	if err != nil {
		return err
	}
	applog.Setup(cfg.DataDir)
	// 兼容旧语义：serve 缺省要有界面；-open=false 显式关掉。
	if !config.HasOpenFlag(args) && os.Getenv("ADAGO_DESIGN_OPEN") != "1" {
		cfg.Open = !cfg.NoOpen
	}
	ui := cfg.ResolveUI("serve")
	if config.HasOpenFlag(args) && !cfg.Open {
		ui = config.UINone
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch ui {
	case config.UINative:
		return runNativeServe(ctx, cfg)
	case config.UIBrowser:
		return runHeadlessServe(ctx, cfg, true)
	default:
		return runHeadlessServe(ctx, cfg, false)
	}
}

// runNativeServe 起服务后把画布装进原生窗口；窗口关闭即退出。
func runNativeServe(ctx context.Context, cfg *config.Config) error {
	sh, first := native.Acquire(native.Options{
		DataDir: cfg.DataDir,
		Theme:   cfg.Theme,
		Version: config.Version,
	})
	if !first {
		// 同数据目录的窗口已在前台：命令行已转交给它，本次直接退出。
		log.Printf("已有窗口在运行：已切换到该窗口")
		return nil
	}
	srv, err := app.Start(ctx, cfg, withHTTPMCP(cfg, sh.Focus))
	if err != nil {
		var ie *app.InstanceExistsError
		if errors.As(err, &ie) {
			// 数据目录被「无窗口实例」（serve -no-open / mcp）占着：
			// 不抢服务、不写数据，窗口里给故障卡与出口。
			log.Printf("数据目录已被无窗口实例占用：%s", ie.URL)
			sh.SetConflict(ie.URL)
			return sh.Run(ctx)
		}
		return err
	}
	sh.SetBaseURL(srv.BaseURL)
	sh.SetOnQuit(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Stop(stopCtx)
	})
	log.Printf("原生窗口已就绪（画布 %s）", srv.BaseURL)
	return sh.Run(ctx)
}

// runHeadlessServe 只跑 HTTP 服务（浏览器或完全无界面）。
func runHeadlessServe(ctx context.Context, cfg *config.Config, openBrowser bool) error {
	srv, err := app.Start(ctx, cfg, withHTTPMCP(cfg, nil))
	if err != nil {
		// 已有同数据目录实例：为避免双写覆盖，本进程不启动。
		var ie *app.InstanceExistsError
		if errors.As(err, &ie) {
			log.Printf("实例已在运行：%s —— 本次不启动新实例", ie.URL)
			if openBrowser {
				_ = openbrowser.Open(ie.URL)
			}
			return nil
		}
		return err
	}
	if openBrowser {
		_ = openbrowser.Open(srv.BaseURL)
	}
	log.Printf("按 Ctrl+C 退出。")
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Stop(shutdownCtx)
}

// withHTTPMCP 让 serve / mcp 都把工具挂到 <BaseURL>/mcp（Streamable HTTP）。
//
// Agent 接入首选这条 HTTP 端点：不需要 spawn 子进程，也不会因为「同数据目录
// 只能有一个实例」和已运行的画布服务互斥。stdio 模式（mcp）仍保留，两者共用
// 同一实例内的 Store/TaskQueue。focus 非 nil 时，截图超时的自动拉起会先尝试
// 聚焦原生窗口，失败才退回浏览器。
func withHTTPMCP(cfg *config.Config, focus func() bool) app.Option {
	return app.WithMCP(func(s *app.Server) http.Handler {
		log.Printf("MCP(HTTP) 已就绪: %s/mcp", s.BaseURL)
		return mcpserver.NewHTTPHandler(mcpserver.Deps{
			Store:    s.Store(),
			Tasks:    s.Tasks(),
			BaseURL:  func() string { return s.BaseURL },
			DataDir:  cfg.DataDir,
			AutoOpen: true,
			OpenCanvas: func(url string) error {
				if focus != nil && focus() {
					return nil
				}
				return openbrowser.Open(url)
			},
		})
	})
}

// runMCP Agent 接入模式：stdio MCP（-window 时同时开原生窗口）。
func runMCP(args []string) error {
	cfg, err := config.FromFlags("adago-design mcp", args)
	if err != nil {
		return err
	}
	applog.Setup(cfg.DataDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.ResolveUI("mcp") == config.UINative {
		return runMCPWindow(ctx, cfg)
	}

	srv, err := app.Start(ctx, cfg, withHTTPMCP(cfg, nil))
	if err != nil {
		// mcp 模式没有「委派给已有实例」的退路（工具要写数据），只能快速失败：
		// 消息给出三条出路 —— 关旧实例 / 用已有实例的画布 / 换数据目录
		var ie *app.InstanceExistsError
		if errors.As(err, &ie) {
			return fmt.Errorf("mcp 模式无法启动: %v\n  处理：关闭旧实例（serve/另一个 mcp）后重试，或用 -data 指定其它数据目录", ie)
		}
		return err
	}
	// 截图任务 GC，防长驻泄漏
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.Tasks().GC()
			}
		}
	}()

	deps := mcpserver.Deps{
		Store:    srv.Store(),
		Tasks:    srv.Tasks(),
		BaseURL:  func() string { return srv.BaseURL },
		DataDir:  cfg.DataDir,
		AutoOpen: true, // 截图超时自动拉起画布并重试一次（无窗口时用浏览器）
	}
	server := mcpserver.NewServer(deps)
	log.Printf("MCP(stdio) 已就绪，画布: %s", srv.BaseURL)
	runErr := server.Run(ctx, &mcp.StdioTransport{})
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Stop(shutdownCtx)
	return runErr
}

// runMCPWindow 是 mcp -window：stdio 与原生窗口同进程。
// 客户端断开（stdin EOF）只结束 MCP 会话，窗口留着给人工继续看画布；
// 关闭窗口即退出应用。
func runMCPWindow(ctx context.Context, cfg *config.Config) error {
	sh, first := native.Acquire(native.Options{
		DataDir: cfg.DataDir,
		Theme:   cfg.Theme,
		Version: config.Version,
	})
	if !first {
		return nil
	}
	srv, err := app.Start(ctx, cfg, withHTTPMCP(cfg, sh.Focus))
	if err != nil {
		return err
	}
	sh.SetBaseURL(srv.BaseURL)
	sh.SetOnQuit(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Stop(stopCtx)
	})

	deps := mcpserver.Deps{
		Store:      srv.Store(),
		Tasks:      srv.Tasks(),
		BaseURL:    func() string { return srv.BaseURL },
		DataDir:    cfg.DataDir,
		AutoOpen:   true,
		OpenCanvas: func(url string) error { sh.Focus(); return nil },
	}
	server := mcpserver.NewServer(deps)
	mcpCtx, cancelMCP := context.WithCancel(ctx)
	go func() {
		runErr := server.Run(mcpCtx, &mcp.StdioTransport{})
		if runErr != nil {
			log.Printf("MCP(stdio) 结束: %v", runErr)
		}
		cancelMCP()
	}()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.Tasks().GC()
			}
		}
	}()
	log.Printf("MCP(stdio) + 原生窗口已就绪，画布: %s", srv.BaseURL)
	return sh.Run(ctx)
}
