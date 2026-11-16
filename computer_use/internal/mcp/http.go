//go:build windows

package mcp

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// NewHTTPHandler returns the stateless Streamable HTTP handler: /mcp for MCP and
// /healthz for a liveness probe.
func NewHTTPHandler(s *Server) http.Handler {
	streamable := server.NewStreamableHTTPServer(NewServer(s),
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", streamable)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"app":"computer-use","transport":"mcp-http"}`))
	})
	return mux
}

// HTTPHandler 保留方法形式，内部复用 NewHTTPHandler。
func (s *Server) HTTPHandler() http.Handler { return NewHTTPHandler(s) }

// ServeHTTP serves Streamable HTTP on addr until the server stops.
func (s *Server) ServeHTTP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "computer-use mcp http: http://%s/mcp\n", ln.Addr())
	srv := &http.Server{Handler: s.HTTPHandler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
