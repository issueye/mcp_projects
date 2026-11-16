---
name: adago-design
description: 使用 adago-design 设计体系 MCP 的操作规范。当 design_systems / design_tokens / design_preview / design_screen / design_audit / design_logo / design_icon_manage 或 project_* / canvas_* 工具可用，且任务涉及设计体系、Token、组件预览、整页设计稿、无限画布、Logo、图标或设计走查时使用本技能。
---

# adago-design 设计体系 MCP

一个「设计体系 + 无限画布」MCP：内置 Apple HIG、火山 Arco、抖音、小米 MIUI、
Element Plus、Ant Design、TDesign、Material 3 八套体系，用真实 HTML+CSS 渲染
组件与整页预览，并把预览摆到项目画布上。

## 核心心智模型

1. **设计稿必须属于项目**。任何新增设计稿的工具都要带 `project`；没有项目就先
   `project_create`。否则返回 `project_required`。
2. **先定规范，再出预览**。先用 `design_artboard` 定尺寸，再用
   `design_systems` / `design_tokens` / `design_components` / `design_search`
   查 Token 与组件。
3. **预览是真实 HTML，不是效果图**。`design_preview` 生成组件组合，
   `design_screen` 生成 login/settings/feed/profile 整页模板。
4. **画布是项目作用域**。`canvas_list`、`canvas_export`、`canvas_clear` 都按
   `project` 过滤；一个项目一块画布。
5. **图标只用 SVG 库**。禁止 emoji 与 `▾ › ✕ ◉` 等字符当图标；字体只用
   `design_fonts` 中 `Commercial=true` 的条目。

## 标准工作流

```text
project_list / project_create          # 先拿到 project
  └─ design_artboard                   # 定画板（iPhone 393×852 / Web 1440×900…）
      └─ design_systems / design_tokens / design_components / design_search
          ├─ design_preview            # 组件组合预览（默认摆上画布）
          ├─ design_screen             # 整页模板预览（默认摆上画布）
          ├─ design_compare            # 两套体系 Token 对比
          └─ design_audit              # 规范走查
              └─ canvas_list / canvas_arrange / canvas_screenshot
                  └─ canvas_export     # 导出独立 HTML
```

首次进入一个项目时，建议顺序：

1. `project_list`：看已有项目；没有就 `project_create`。
2. `design_artboard`：确认移动端/Web 画板尺寸；需要限制整块画布比例时用
   `project_canvas`（`16:9` / `9:16` / `4:3` 或自定义宽高）。
3. `design_systems` → `design_tokens`（可先用 `brief=true` 概览）→
   `design_components`。
4. `design_preview` 或 `design_screen`：带 `project`、`system`、`theme`、
   `artboard`；默认 `add_to_canvas=true`。
5. `canvas_screenshot` 回看，`canvas_arrange` 排版，最后 `canvas_export`。

## 工具速查

### 项目与画布版式

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `project_create` | 建项目 | `name`（必填、同名拒绝）、`note` |
| `project_list` | 列项目/节点数/时间 | — |
| `project_remove` | 删空项目 | `id`、`confirm=true` |
| `project_canvas` | 设置项目画布比例/尺寸 | `project`、`ratio` 或 `width`/`height`、`free` |
| `canvas_resync_artboards` | 按节点 artboard 重算宽高 | `project`（省略=全部） |

### 设计规范

| 工具 | 用途 |
|---|---|
| `design_systems` | 列出全部体系（id/名称/厂商/主题/关键词） |
| `design_tokens` | 取 Token；`category=all/color/type/spacing/radius/shadow/motion`，`brief` 精简 |
| `design_components` | 组件清单；传 `component` 取单组件详情与 HTML |
| `design_search` | 跨体系搜体系/组件/Token |
| `design_artboard` | 画板尺寸、安全区、栅格 |
| `design_icons` | SVG 图标库（内置 + 自定义，已停用不返回） |
| `design_fonts` | 字体授权与字族栈 |

### 预览、走查与体系管理

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `design_preview` | 组件组合预览 | `project`、`system`、`components[]`、`theme`、`artboard`、`add_to_canvas` |
| `design_screen` | 整页模板 | `template=login/settings/feed/profile`、`app_name`、`artboard` |
| `design_compare` | 两体系 Token 对比 | `system_a`、`system_b`、`add_to_canvas` |
| `design_audit` | 走查（对比度/字号/间距/状态色） | `project`、可选 `node_id` |
| `design_derive` | 从主色派生体系 | `primary`、`name`、`base_system` |
| `design_register` | 注册自定义 Token JSON | `name`、`tokens_json`、可选 `dark_json`、`base_system`、`family`、`project` + `add_to_canvas` |
| `design_system_manage` | 体系管理 | `action=list/create/update/disable/enable/remove/reset`、`id`、`base`/`primary`（create）、`json`（update） |
| `design_icon_manage` | 图标库管理/设计 | `action=list/design/create/update/disable/enable/remove/reset`、`name`、`design`、`svg` |
| `design_logo` | Logo 工作台 | `action=list/get/preview/create/update/remove/spec/export/place`、`id`、`brand`、`layout`、`markIcon` |

### 画布操作

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `canvas_add` | 加任意 HTML 卡片 | `project`、`title`、`html`、`kind`、`width/height`、`x/y` |
| `canvas_list` | 列节点 | `project`、`system`、`tag` |
| `canvas_move` | 移动节点 | `id`、`x`、`y` |
| `canvas_remove` | 删节点 | `id` |
| `canvas_clear` | 清空项目（可恢复） | `project`、`confirm=true` |
| `canvas_undo` / `canvas_redo` | 撤销/重做内容操作 | — |
| `canvas_arrange` | 网格排版（体系分组优先） | `project`、`columns` |
| `canvas_restore` | 恢复最近一次 clear | — |
| `canvas_export` | 导出项目画布 HTML | `project`、`title` |
| `canvas_connect` | 节点间连线 | `from`、`to`、`label` |
| `canvas_group` | 分组框 | `title`、`node_ids[]`、`padding` |
| `canvas_screenshot` | 画布/节点截图 | `node_id`、`scale` |
| `canvas_url` | 画布地址与节点数 | — |

## 关键约束与坑

- **`project` 是新增设计稿的硬门槛**：`design_preview`、`design_screen`、
  `canvas_add`、`design_audit`、`design_derive`、`design_compare` 都需要它。
- `design_preview` 的 `components` 是数组，顺序即展示顺序；无效组件会在
  `missing` 中返回并跳过。
- `design_tokens` 全量输出较大；先 `brief=true` 看关键色/字阶/间距，再按
  `category` 取全量。
- `canvas_clear` 只清一个项目，必须显式 `confirm=true`；清空前快照只在进程内存，
  重启后 `canvas_restore` 失效。
- `canvas_undo` / `canvas_redo` 只覆盖画布内容操作（节点、连线），最多 50 步；
  项目增删改不进历史。
- `canvas_screenshot` 需要画布页已打开（原生窗口或浏览器）。超时会报
  `capture_timeout`；服务在无人值守时最多自动拉起一次画布，仍失败就手动打开
  `canvas_url` 返回的地址。
- 同数据目录只允许一个 adago-design 实例；stdio 模式与其他实例冲突时按错误提示
  换 `-data` 或关闭旧实例。

## 错误与排障

常见稳定错误码：

| 错误码 | 含义 | 处理 |
|---|---|---|
| `project_required` | 新增设计稿未带 project | 先 `project_create` 或 `project_list` 选项目 |
| `unknown_project` | project 不存在 | `project_list` 查看有效 ID |
| `unknown_system` / `unknown_theme` | 体系或主题不存在 | `design_systems` 查可用值 |
| `confirm_required` | 清空未确认 | 显式传 `confirm=true` |
| `capture_timeout` | 画布页未响应截图 | 打开 `canvas_url` 后重试 |
| `artboard_unavailable` | 进程未注入画板解析器 | 用正常 `serve`/`mcp` 启动 |

画布打不开或截图一直超时：先 `canvas_url` 拿地址，用原生窗口/系统浏览器打开；
仍不行再看 adago-design 日志（`<data>/adago-design.log`）。
