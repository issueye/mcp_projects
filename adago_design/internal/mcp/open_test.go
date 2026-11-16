package mcp

import (
	"strings"
	"testing"
)

// AutoOpen 的注入点：原生窗口模式聚焦既有窗口，不再弹系统浏览器；
// 且「每进程至多一次」的既有约定不变。
func TestOpenCanvasUsesInjectedOpenerOnce(t *testing.T) {
	autoOpenMu.Lock()
	autoOpenedURL = ""
	autoOpenMu.Unlock()
	t.Cleanup(func() {
		autoOpenMu.Lock()
		autoOpenedURL = ""
		autoOpenMu.Unlock()
	})

	var calls []string
	open := func(url string) error {
		calls = append(calls, url)
		return nil
	}
	first := openCanvasPageOnce("http://127.0.0.1:8788", open)
	second := openCanvasPageOnce("http://127.0.0.1:8788", open)
	if len(calls) != 1 || calls[0] != "http://127.0.0.1:8788" {
		t.Fatalf("opener 应只被调用一次，实际 %v", calls)
	}
	if !strings.Contains(first, "http://127.0.0.1:8788") || first != second {
		t.Fatalf("提示文本异常：%q / %q", first, second)
	}
}
