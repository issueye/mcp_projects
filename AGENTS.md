# AGENTS.md

本文件是本目录（`mcp_projects`）的工作约定，适用于所有子模块。开始改动前先读
[README.md](README.md) 与目标模块的 `README.md`；它们描述运行形态与数据目录。

## 1. 仓库地图

| 目录 | Go module | 角色 | MCP 传输 | 工具数 |
|---|---|---|---|---|
| `adago_design` | `adagodesign` | 设计体系 + 无限画布 | HTTP `/mcp` + stdio | 35 |
| `browser_use` | `browseruse` | 桌面浏览器（agent 用） | stdio + HTTP `/mcp` | 25 |
| `computer_use` | `computeruse` | Windows 桌面操作与蒙板 | stdio + HTTP `/mcp` | 26 |
| `mem_go` | `mem_go` | 长期记忆（向量库） | stdio | 7 |
| `ada_link` | `github.com/learningdog1/ada_link` | MyGo 跨进程原生 UI 嵌入库 | 非 MCP | — |

四个 MCP 服务统一使用 `github.com/mark3labs/mcp-go v0.52.0`。不要重新引入
`github.com/modelcontextprotocol/go-sdk`。

## 2. 铁律

1. **模块自包含**：每个子目录有自己的 `go.mod`，本目录没有根 `go.mod` / `go.work`。
   构建时设置 `GOWORK=off`；不要为兄弟模块添加 `replace`。
2. **工具实现同构**：四个服务都有 `internal/mcp/toolset.go`，内容应保持一致。
   改 `toolset.go` 时同步复制到另外三个模块（可用文件哈希校验）。
3. **强类型优先**：能用 Go 结构体描述参数/输出时用 `registerMCPTool`，schema 由
   `json` / `jsonschema` tag 推导；只有 browser-use / computer-use 这种与本地 HTTP
   API 共用工具表的场景才用 `registerRawTool` 手写 schema。
4. **工具失败不抛协议错误**：返回 `toolError` / `toolErrorf` / 该模块已有的
   `isError` 结果（browser-use 的 `errorResult` 等），保留稳定错误码与 `hint`，
   让模型能自纠。不要把业务失败变成 JSON-RPC error。
5. **HTTP 无状态**：`/mcp` 用 `server.WithStateLess(true)`；adago 额外
   `WithDisableStreaming(true)` 保持 JSON 响应。工具状态只能放在注入的
   Store/Engine/Service 里，不依赖会话内存。
6. **不提交产物**：`bin/`、`*.exe`、`*.db`、日志、导出/运行数据不要提交；
   `rsrc_*.syso` 与 `assets/icon.ico` 是受版本控制的资源，只有明确更换图标时才更新。
7. **不要回滚无关改动**：工作区可能已有用户未提交的修改。提交前用 `git status`
   确认，只暂存本次相关文件；未经用户明确要求不要 `git checkout --`、`git reset`
   或 `git clean`。
8. **Windows 约束**：`computer_use` 的 MCP 包带 `//go:build windows`；`browser_use`
   依赖 WebView2/mygo；`adago_design` 的缺省形态是原生窗口。跨平台改动要保留构建
   tag 与回退路径。

## 3. 统一代码结构

```text
<module>/internal/mcp/
├── server.go      # NewServer(deps) *server.MCPServer，Instructions + registerTools
├── toolset.go     # Tool / Handler / registerMCPTool / registerRawTool / result helpers
├── mcp.go         # 可选：领域桥接（browser-use 的 /v1/call、computer-use 的 HTTP 代理）
├── http.go        # NewHTTPHandler：无状态 Streamable HTTP
└── run.go         # 可选：CLI 子命令、stdio 入口
```

约定命名：

- 构造函数统一叫 `NewServer`；HTTP 入口统一叫 `NewHTTPHandler`。
- 工具声明用 `Tool{Name, Description}`；需要手写 schema 时填 `Schema`。
- 结果统一用 `textResult` / `imageResult` / `imageResultBase64` / `toolError`。
- 输出结构体由 `registerMCPTool` 自动写入 `StructuredContent`；handler 只负责返回
  `(*CallToolResult, Out, error)`。

## 4. 新增或修改 MCP 工具

1. 在对应 `internal/mcp` 里定义 `xxxIn` / `xxxOut`，字段必须有 `json` tag；
   面向模型的说明放 `jsonschema:"..."`。
2. 在 `registerTools` 中加 `registerMCPTool(srv, &Tool{...}, handler)`；browser-use /
   computer-use 没有独立 `registerTools`，在 `NewServer` 的工具循环里加
   `registerRawTool`。handler 返回 `textResult(...), out, nil`。错误用
   `toolError(code, msg, hint)` 或 `failWith`，不要返回裸 error。
3. browser-use / computer-use 的工具声明在 `control.Tools` / `toolList()` 中，
   schema 与 HTTP API 共用；改参数时同步改 dispatch 与文档，避免 MCP 与 HTTP 漂移。
4. 至少补一个工具级测试：
   - adago/mem：走 mcp-go 进程内客户端（`client.NewInProcessClient`）；
   - browser/computer：用现有 fake app / httptest API 模式。
5. 若工具数变化，更新本文件、根 README 与模块 README 中的数量/清单。

adago 的 `TestEveryToolHasGeneratedSchema` 会校验 35 个工具都有 object 输入 schema、
output schema 和非空 description；新增工具时同步更新期望数量。

## 5. 构建与测试

统一使用 PowerShell（Windows）：

```powershell
# 单模块
cd adago_design
$env:GOWORK='off'; $env:CGO_ENABLED='0'
go build ./...
go test ./...
go vet ./internal/mcp

cd ..\browser_use
$env:GOWORK='off'; $env:CGO_ENABLED='0'
go build ./...
go test ./...
go vet ./internal/mcp

cd ..\computer_use
$env:GOWORK='off'; $env:CGO_ENABLED='0'
go build ./...
go test ./...
go vet ./internal/mcp

cd ..\mem_go
$env:GOWORK='off'
go build ./...
go test ./...
go vet ./internal/mcp
```

- 发布构建优先用各模块的 `build.bat`（带 `-trimpath`、`windowsgui` 等参数）。
- 只改动了 MCP 层时，至少运行 `go test ./internal/mcp`；改共享类型时跑全量
  `go test ./...`。
- `gofmt -w` 本次改动涉及的 `.go` 文件；不要顺手格式化无关文件。
- 耗时的 GUI/真机测试（adago native、browser WebView、computer mask）可能创建
  窗口或覆盖层；在无桌面/CI 环境优先跑聚焦单测，并说明未执行的冒烟项。

## 6. 运行与连接

| 服务 | stdio | HTTP |
|---|---|---|
| adago_design | `adago-design mcp` | `adago-design serve` → `http://127.0.0.1:8788/mcp` |
| browser_use | `browser-use mcp` | `browser-use mcp --http --port 8766` |
| computer_use | `computer-use mcp --spawn` | `computer-use mcp --http --port 8767` |
| mem_go | `memmcp` | 无 |

- browser-use HTTP 可用 `BROWSER_USE_MCP_TOKEN` / `--token` 加 Bearer 鉴权；
  `/healthz` 不鉴权。
- computer-use 的 HTTP API token 用 `COMPUTER_USE_TOKEN` / `--token`；MCP 代理会把
  token 透传给本地 API。
- adago 的 stdio 与 HTTP 共用同一实例的 `Store` / `TaskQueue`；同数据目录只允许
  一个实例，冲突时按 README 的提示处理（换 `-data` 或关闭旧实例）。

## 7. 数据与平台细节

- `adago_design`：体系数据的单一真源是 `internal/design/data/*.json`；
  改体系/Token 后跑 `go test ./internal/design`，不要手改导出产物。
- `browser_use`：`internal/script/prelude.js` 与 `internal/control` 的工具有契约关系；
  改脚本 API 时同步检查 `internal/script` 测试与工具说明。
- `computer_use`：所有坐标是物理像素；截图与输入都走同一坐标系，改动时不要引入
  DPI 缩放偏差。
- `mem_go`：`mem_go.db` 是本地向量库，替换嵌入模型/维度前先读 README 的维度隔离
  说明；测试用临时目录，不要污染用户数据库。
- `mem_go/.agents/skills/mem-go-memory/SKILL.md` 是给使用者的记忆操作规范；
  改 `memory_*` 工具语义时同步更新它。
- 每个 MCP 服务在模块内有一份对应 skill：`adago_design/.agents/skills/adago-design/`、
  `browser_use/.agents/skills/browser-use/`、`computer_use/.agents/skills/computer-use/`、
  `mem_go/.agents/skills/mem-go-memory/`。改工具参数、工作流或安全约束时同步更新
  对应 `SKILL.md`。

## 8. 文档同步

改以下内容时必须同步文档：

- MCP SDK / 包路径：根 `README.md`、模块 `README.md`、adago `DESIGN.md`；
- 工具数量、参数、错误码：模块 `README.md`、对应的工具表与 `SKILL.md`；
- 构建命令、产物路径、端口：根 `README.md` 与模块 `README.md`。

本目录不包含 README 中提到的 `scripts/build-mcp.cmd` / `.ps1`；它们位于 ada-go
主仓。单独检出 `mcp_projects` 时请直接用上面的 `go build` 命令。

## 9. 提交前检查

```powershell
git status --short
git diff -- <本次改动的文件>
$env:GOWORK='off'; go test ./...
```

确认：没有 go-sdk 残留、四个 `toolset.go` 仍一致、没有提交 `bin/`/`*.db`/日志、
文档中的工具数与代码一致。
