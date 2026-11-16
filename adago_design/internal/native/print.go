// Package native 的原生窗口负责提供「项目设计文稿 → PDF」的最后一段：
// 开一个隐藏窗口加载 /projects/{id}/print，等页面与全部预览 iframe 就绪，
// 调用 WebView2 的 PrintToPDF 得到 PDF 字节，然后关窗返回。
//
// 为什么开隐藏窗口而不是在主窗口上打印：
//   - 主窗口是画布编辑器，直接打印会把工具条、浮窗一起印进去；在页面里
//     临时铺一张打印文稿再还原，会把用户正在看的画布状态搅乱（缩放/滚动/焦点）。
//   - 独立隐藏窗口的 URL 是只读文稿页，打印期间主窗口仍可继续编辑。
//   - PrintToPDF 走 DevTools 的 Page.printToPDF（见 mygo Windows 实现），
//     WebView2 即使窗口不可见也能排版，不需要抢焦点。
package native

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

const (
	// 文稿页在 1122×794（A4 横向 @96dpi）的视口里排版；纵向互换。
	// 真正决定 PDF 纸张的是 PrintToPDF 选项与页面 @page，视口只影响屏幕布局。
	printViewportW = 1122
	printViewportH = 794
	// printReadyTimeout 覆盖「页面加载 + 预览 iframe 加载 + 字体就绪」。
	// 文稿可能有几十张稿，给足 20 秒；超时返回错误，不打印半截文档。
	printReadyTimeout = 20 * time.Second
	// printSettle 是就绪标记置位后再等一拍，让 WebView2 完成首帧绘制 / 字体栅格化。
	printSettle = 220 * time.Millisecond
)

// ErrPrintTimeout 表示文稿页没有在限定时间内进入可打印状态。
var ErrPrintTimeout = errors.New("打印文稿页在限定时间内未就绪")

// PrintPDF 把文稿页渲染成 PDF。landscape 与页面 @page 的方向一致。
//
// 该函数会阻塞到 PDF 返回（WebView2 渲染整本文稿），调用方（HTTP handler）
// 用 context 控制整体超时；这里同时轮询 context，取消时尽快关窗返回。
func (s *Shell) PrintPDF(ctx context.Context, pageURL string, landscape bool) ([]byte, error) {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		return nil, errors.New("打印文稿页地址为空")
	}
	s.pdfMu.Lock()
	defer s.pdfMu.Unlock()

	width, height := printViewportW, printViewportH
	if !landscape {
		width, height = printViewportH, printViewportW
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           appTitle + " · 文稿",
		URL:             pageURL,
		Hidden:          true,
		SkipTaskbar:     true,
		Width:           width,
		Height:          height,
		UseContentSize:  true,
		BackgroundColor: "#FFFFFF",
		Page:            mygo.PageOptions{DevTools: mygo.DevToolsDisabled},
	})
	if w == nil || w.IsDestroyed() {
		return nil, errors.New("无法创建文稿打印窗口")
	}
	defer mygo.RunOnMain(func() { w.Close() })

	page := w.Page()
	loadErr := make(chan error, 1)
	page.OnDidFailLoad(func(e *mygo.LoadError) {
		select {
		case loadErr <- fmt.Errorf("文稿页加载失败: %s", loadErrorText(e)):
		default:
		}
	})

	if err := waitPrintReady(ctx, page, loadErr); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(printSettle):
	}

	pdf, err := page.PrintToPDF(mygo.PDFOptions{
		PageSize:   mygo.PageA4,
		Landscape:  landscape,
		Background: true,      // 文稿的纸面 / 封面油墨都是背景色，必须保留
		Margins:    &mygo.Margins{}, // 页边距由文稿自己的页内 padding 负责
	})
	if err != nil {
		return nil, fmt.Errorf("PrintToPDF: %w", err)
	}
	if len(pdf) < 5 || !strings.HasPrefix(string(pdf[:5]), "%PDF-") {
		return nil, errors.New("PrintToPDF 返回的不是有效 PDF")
	}
	return pdf, nil
}

// waitPrintReady 轮询页面上的就绪标记（由文稿页脚本在全部 iframe + 字体
// 就绪后置位）。不用 DOM ready：那时预览还是空白，会印出半本文稿。
func waitPrintReady(ctx context.Context, page *mygo.Page, loadErr <-chan error) error {
	deadline := time.Now().Add(printReadyTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-loadErr:
			return err
		default:
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		v, err := page.EvalContext(probeCtx, "document.documentElement.getAttribute('data-print-ready') === '1'")
		cancel()
		if err == nil {
			if ready, ok := v.(bool); ok && ready {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return ErrPrintTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-loadErr:
			return err
		case <-time.After(60 * time.Millisecond):
		}
	}
}

func loadErrorText(e *mygo.LoadError) string {
	if e == nil {
		return "未知错误"
	}
	if e.Description != "" {
		return e.Description
	}
	return fmt.Sprintf("ERR_%d", e.Code)
}
