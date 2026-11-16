package mcp

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
)

// NewHTTPHandler 把同一套工具挂成 Streamable HTTP 端点（供 serve 模式的
// <BaseURL>/mcp 使用，Agent 不必再 spawn stdio 子进程）。
//
// 无状态：每个请求开一个临时会话，工具状态全在 Deps（Store/TaskQueue）里，
// 所以不需要 Mcp-Session-Id，也避免常驻会话的回收问题。关闭流式响应后
// 统一走 JSON，客户端两种格式都认。
func NewHTTPHandler(d Deps) http.Handler {
	srv := NewServer(d)
	return server.NewStreamableHTTPServer(srv,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
		server.WithDisableStreaming(true),
	)
}
