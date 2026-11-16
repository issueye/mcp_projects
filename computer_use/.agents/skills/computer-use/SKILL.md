---
name: computer-use
description: 使用 computer-use MCP 操作 Windows 桌面的规范。当 screen_info / screenshot / click / type / key / batch / mask / run_script / list_windows / clipboard 等工具可用，且任务需要看屏幕、点击窗口、填表、批处理、录制宏或向用户展示操作位置时使用本技能。
---

# computer-use Windows 桌面 MCP

computer-use 通过 Win32 截屏与输入注入操作真实桌面，并在屏幕上画一层**蒙板**：
变暗无关区域、框出目标元素并标注坐标。蒙板鼠标穿透、永远置顶，且**不进入截图**。

## 核心心智模型

1. **坐标是物理像素**。原点在主显示器左上角，主显示器左侧为负；截图返回的
   `meta` 带 `region` 与 `scale`，换算后再点击。
2. **先看再动**。调用 `screen_info` 了解显示器/指针/焦点窗口，再 `screenshot`
   确认目标，然后才 `click` / `type`。
3. **一动作一验证**。点击、输入、快捷键后重新截图或 `screen_info`，确认页面变化；
   不要闭眼连续操作。
4. **批量优先 `batch`**。一串动作用 `batch` 一次调用完成，比逐个工具往返快。
5. **蒙板是给人的，不是给模型的**。截图里看不到蒙板；`mask_preview` 才能看到
   用户眼前的标注。

## 标准工作流

```text
screen_info                    # 显示器、指针、前台窗口、蒙板状态
  └─ screenshot                # 全屏或 region；必要时 grid 烧坐标
      └─ list_windows          # 找目标窗口的 handle/title/矩形
          └─ focus_window      # 置前并给键盘焦点
              ├─ click / type / key / scroll / drag / wait
              ├─ batch         # 多步动作
              └─ outline_window / mask  # 在蒙板里标出区域
                  └─ screenshot / screen_info  # 验证结果
```

破坏性操作（删除、提交、转账、关闭窗口）前先 `pause` 或向用户确认；用户随时可用
`Ctrl+Alt+Shift+P` 暂停。任务结束用 `resume` 解除暂停。

## 工具速查

### 观察

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `screen_info` | 显示器/指针/焦点窗口/蒙板 | — |
| `screenshot` | 桌面截图（不含蒙板） | `display`、`region="x,y,w,h"`、`max_width`、`grid`、`format=png/jpeg`、`cursor` |
| `list_windows` | 可见窗口（标题/进程/矩形/handle） | — |
| `mask_preview` | 渲染用户看到的蒙板 | — |

### 鼠标与键盘

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `click` | 移动并点击 | `x`、`y`、`button=left/right/middle`、`clicks=1/2` |
| `move` | 移动指针 | `x`、`y`、`steps` |
| `drag` | 按住拖动 | `x`、`y`、`x2`、`y2`、`button` |
| `scroll` | 滚轮 | `x`、`y`、`dx`、`dy` |
| `type` | 键盘输入 Unicode 文本 | `text`、`delay_ms` |
| `key` | 按键/组合键 | `keys`（如 `ctrl+s`、`alt+tab`、`win+r`）、`repeat` |
| `wait` | 等待 | `ms`（≤30000） |
| `batch` | 一次执行多动作 | `actions[]`，每项带 `type` |

### 窗口与蒙板

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `focus_window` | 置前并聚焦 | `handle` 或 `title` |
| `outline_window` | 在蒙板框出窗口 | `handle`/`title`、`append` |
| `mask` | 画区域/标记/文字/标尺 | `regions[]`、`markers[]`、`chips[]`、`dim`、`grid`、`coords`、`visible`、`append`、`clear`、`ttl_ms` |
| `clear_mask` | 清空蒙板 | — |
| `pause` / `resume` | 暂停/恢复代理输入 | — |
| `clipboard` | 读写剪贴板 | `op=get/set`、`text`（set） |

### 脚本与宏

| 工具 | 用途 | 关键参数 |
|---|---|---|
| `run_script` | 执行绑定 `cu` API 的 JS | `source`、`args`、`timeout_ms`（默认 60000）、`record` |
| `record_start` / `record_stop` | 录制所有调用方的动作为宏 | `name` |
| `list_macros` | 列出宏 | — |
| `run_macro` | 回放宏 | `name`、`speed` |
| `macro_script` | 把宏导出为 JS | `name` |
| `delete_macro` | 删除宏 | `name` |

`run_script` 的 `cu` 绑定：`cu.click/move/drag/scroll/type/key/wait/focus/outline/
screenshot/state/cursor/windows/mask/clearMask/log/recordStart/recordStop/macroRun/
macroList`。脚本最后表达式的值就是结果；`console.log` 与 `cu.log` 会作为输出返回。

## 关键约束与坑

- **不要用逻辑坐标**：所有输入都是物理像素；高 DPI 下逻辑坐标会点偏。
- `screenshot` 的 `region` 是 `"x,y,w,h"`，不是对象；`grid` 是在图上烧网格并写坐标，
  适合读位置，不适合每次都用（图会变大）。
- `batch` 里的 action `type` 用 `click` / `move` / `drag` / `scroll` / `type` / `key` /
  `focus` / `outline` / `wait`；字段与单独工具一致。
- `type` 走 Unicode 事件，不受键盘布局限制；输入前先 `focus_window` 或点击目标。
- 蒙板不进入截图；想确认用户看到的效果用 `mask_preview`。
- 剪贴板可能含密码/令牌，读写前确认必要性，不要把内容回显给无关方。
- 应用必须已在运行；stdio MCP 启动时用 `computer-use mcp --spawn`，否则会提示
  “the app is not running ... pass --spawn”。
- 应用本地 HTTP API 默认 `127.0.0.1:7331`；MCP HTTP 端点默认
  `http://127.0.0.1:8767/mcp`（`--port` 可改）。设置 `COMPUTER_USE_TOKEN` 后，
  MCP 代理会把 Bearer token 透传给 API。

## 错误与排障

工具失败以 `isError` 返回，文本里带原因：

| 现象 | 处理 |
|---|---|
| `the app is not running at ...` | 先 `computer-use run`，或用 `mcp --spawn` |
| 点不到/点偏 | 重新 `screen_info` + `screenshot grid`，确认物理像素坐标与显示器 |
| 窗口不在前台 | `list_windows` 找 handle/title，再 `focus_window` |
| 输入被拒绝 | 检查是否 `pause`、用户是否接管、是否以 `--read-only` 启动（`AllowInput=false`） |
| 截图失败 | 检查显示器编号、权限、远程桌面/锁屏状态 |

录制宏时，所有调用方（HTTP、MCP、脚本）的动作都会进同一个宏；回放走同一套
actions，因此蒙板、暂停开关与速度参数同样生效。
