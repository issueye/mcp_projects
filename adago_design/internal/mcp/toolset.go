package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Tool 是工具声明。Name/Description 给模型看；Schema 可选，留空时由 In 的
// json/jsonschema tag 推导，非空时按手写 JSON Schema 注册（与 HTTP 工具表共用）。
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Handler 是四个 MCP 服务统一的强类型工具签名：入参由框架按 In 的 JSON
// Schema 反序列化，返回值由 registerMCPTool 统一装配为 content + structuredContent。
type Handler[In, Out any] func(ctx context.Context, in In) (*mcpgo.CallToolResult, Out, error)

// CallToolResult 是 mcp-go 结果的包内别名，业务 handler 不必反复写长包名。
type CallToolResult = mcpgo.CallToolResult

// registerMCPTool 注册一个强类型工具。输入/输出 schema 默认由 In/Out 推导；
// Tool.Schema 非空时用调用方给的手写 schema。参数解析失败与 handler 返回的
// error 都转成 isError 结果，让模型能看到并自行纠正。
func registerMCPTool[In, Out any](s *server.MCPServer, t *Tool, h Handler[In, Out]) {
	opts := []mcpgo.ToolOption{
		mcpgo.WithDescription(t.Description),
		mcpgo.WithOutputSchema[Out](),
	}
	if len(t.Schema) == 0 {
		opts = append(opts, mcpgo.WithInputSchema[In]())
	}
	tool := mcpgo.NewTool(t.Name, opts...)
	if len(t.Schema) > 0 {
		tool.RawInputSchema = t.Schema
	}
	s.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var in In
		if err := req.BindArguments(&in); err != nil {
			return toolError("invalid_arguments", "工具参数无法解析: "+err.Error(), "按工具 schema 传 JSON 对象"), nil
		}
		res, out, err := h(ctx, in)
		if err != nil {
			return toolError("tool_error", err.Error(), ""), nil
		}
		return withStructured(res, out), nil
	})
}

// registerRawTool 注册一个完全手写 JSON Schema 的工具（browser-use /
// computer-use 的工具表与本地 HTTP API 共用同一份 schema）。
func registerRawTool(s *server.MCPServer, t *Tool, h server.ToolHandlerFunc) {
	s.AddTool(mcpgo.NewToolWithRawSchema(t.Name, t.Description, t.Schema), h)
}

// withStructured 把强类型输出序列化进 structuredContent，保留 handler 自己写的
// text/image content。isError 结果也带 structuredContent，便于客户端同时拿到
// 错误文本与结构化字段。
func withStructured[Out any](res *mcpgo.CallToolResult, out Out) *mcpgo.CallToolResult {
	if res == nil {
		res = &mcpgo.CallToolResult{}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return res
	}
	var structured any
	if err := json.Unmarshal(raw, &structured); err != nil {
		return res
	}
	res.StructuredContent = structured
	return res
}

// textResult 构造纯文本工具结果。
func textResult(text string) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultText(text)
}

// imageResult 构造「文字说明 + 图片」工具结果；data 是原始图片字节。
func imageResult(text string, data []byte, mimeType string) *mcpgo.CallToolResult {
	return imageResultBase64(text, base64.StdEncoding.EncodeToString(data), mimeType)
}

// imageResultBase64 用于图片已经是 base64 文本的场景（避免二次编码）。
func imageResultBase64(text, data, mimeType string) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultImage(text, data, mimeType)
}

// toolError 构造带稳定错误码的 isError 结果；不返回 error，避免协议层失败。
func toolError(code, msg, hint string) *mcpgo.CallToolResult {
	text := fmt.Sprintf("[ERROR %s] %s", code, msg)
	if hint != "" {
		text += "\n提示: " + hint
	}
	return mcpgo.NewToolResultError(text)
}

// toolErrorf 是无需错误码时的简化形式。
func toolErrorf(format string, args ...any) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError(fmt.Sprintf(format, args...))
}

// fail 构造带稳定错误码的 isError 结果（不返回 error，避免丢失结构化错误信息）。
// 注意：isError 结果同样会做输出 schema 校验，O 的切片/map 字段需非 nil，
// 此类场景请改用 failWith 传入可校验的零值结构。
func fail[O any](code, msg, hint string) (*CallToolResult, O, error) {
	var zero O
	return failWith(zero, code, msg, hint)
}

// failWith 构造只带错误文本的 isError 结果，并把零值/部分输出交给 registerMCPTool
// 装配 structuredContent。
func failWith[O any](out O, code, msg, hint string) (*CallToolResult, O, error) {
	return toolError(code, msg, hint), out, nil
}
