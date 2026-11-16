# adago design · 品牌 Logo

由 adago-design 内置「品牌 Logo 工作台」生成（Logo 资产 `adago-design` / `adago-mark`）。

## 概念

**纸页 · 折角 · 墨点** —— 设计体系里的「一张纸」：右上折角是画板/文档，
左下墨点是落笔的 Inkwell。延续产品既有的 Black Swan 纸墨单色品牌，
不引入第二种彩色，让 Logo 在浅色纸面、深色墨面、单色印刷三种场景下都成立。

- 标记：24 网格、1.8 描边、圆头圆角；纸页折角 + 实心墨点。
- 字标：`adago`，Inter 700，字距 -0.4px；副标 `design system`，Inter 400 / 11px。
- 主组合：横排（标记 52 + 间距 16 + 字标 34），四周安全空间 26px。
- 最小宽度：72px（标记/字标 ≥16px 的换算值）；更小尺寸改用 mark-only。
- 字体：Inter，SIL OFL 1.1，可商用、可嵌入；生产交付前建议把字标转曲。

## 文件

| 文件 | 用途 |
|---|---|
| `adago-logo.svg` | 正形横排（浅底） |
| `adago-logo-inverse.svg` | 反形横排（品牌墨底 + 纸白标记/字标） |
| `adago-logo-mono.svg` | 单色横排（印刷 / 低色域） |
| `adago-logo@4x.png` | 正形 812×416 透明 PNG |
| `adago-mark.svg` | 纯标记（透明底） |
| `adago-mark@4x.png` | 纯标记 1024×1024 透明 PNG |
| `adago-mark-inverse.svg` | 纯标记反形（墨底） |
| `adago-mark-inverse@4x.png` | 纯标记反形 1024×1024 PNG（应用图标候选） |
| `index.html` | 一页品牌展示（变体/尺寸/规范） |
| `adago-design.json` / `adago-mark.json` | 可导入 Logo 工作台的资产 JSON |
| `adago-page.json` | 自定义标记图标的设计文档（可先导回图标库） |
| `preview.png` / `workbench.png` | 品牌展示页与工作台截图 |

## 使用

1. 横排主标用于页眉、启动页、官网；左侧标记与字标间距不要小于 16px。
2. 深底或品牌墨底使用反形；单色印刷使用 mono。
3. 纯标记用于应用图标（`adago-mark-inverse`）与 favicon；建议保留 0.5×标记高度的安全空间。
4. 不要拉伸、改色、加描边、加投影；标记与字标不可互换比例。
