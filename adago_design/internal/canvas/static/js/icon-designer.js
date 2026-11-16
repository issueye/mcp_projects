/* adago-design 画布 · 图标设计器（ES module）
 *
 * 图标库原来的「新增」只能粘贴 SVG 源码；这里把同一套图标契约（24 网格、
 * 2px 视觉描边、currentColor）变成可绘制的画板：
 *   - 基础图形：直线 / 矩形 / 圆 / 椭圆 / 折线 / 多边形 / 路径（导入）
 *   - 0.5 网格吸附（Alt 0.1 细调）、Shift 约束、方向键微移
 *   - 图层顺序 / 显隐 / 复制 / 删除，40 步撤销重做
 *   - 12/16/20/24 尺寸预览 + 暗底检查
 *   - 保存时提交几何文档（design）给 POST/PUT /api/catalog/icons，
 *     服务端再渲染成 SVG 落库；手改 SVG 的旧图标也能打开（由 SVGo 反推）。
 *
 * 与 icons-admin.js 的分工：本模块只管设计器弹层；图标库弹层在打开前收起，
 * 返回/保存时通过 onClosed / onSaved 回调恢复。 */
"use strict";

import { $ } from "./state.js";
import { esc, toast, uiConfirm, isAskOpen } from "./ui.js";
import { api } from "./net.js";
import { icon } from "./icons-bridge.js";

const GROUPS = [
  ["arrow", "方向与层级"],
  ["action", "动作与工具"],
  ["status", "状态与反馈"],
  ["nav", "导航入口"],
  ["media", "内容互动"],
  ["data", "列表与数据"],
];
const TOOLS = [
  { id: "select", key: "V", label: "选择 / 移动", hint: "点击选中 · 拖动移动 · 方向键微移" },
  { id: "line", key: "L", label: "直线", hint: "拖拽画直线 · Shift 锁定 0°/45°/90° · Alt 细调" },
  { id: "rect", key: "R", label: "矩形", hint: "拖拽画矩形 · Shift 正方形 · Alt 细调" },
  { id: "circle", key: "O", label: "圆形", hint: "拖拽画圆 · Shift 正圆 · Alt 细调" },
  { id: "polyline", key: "P", label: "折线", hint: "逐点点击 · Enter / 双击结束 · Esc 取消" },
  { id: "polygon", key: "G", label: "多边形", hint: "逐点点击，点回起点闭合 · Enter / 双击结束" },
];
const NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const PATH_RE = /^[MmLlHhVvCcSsQqTtAaZz0-9eE.,+\-\s]+$/;
const MIN_SIZE = 0.2;
const DEFAULTS = { grid: 24, strokeWidth: 1.8, cap: "round", join: "round" };

const D = {
  open: false,
  mode: "create", // create | clone | edit
  original: "",
  name: "",
  group: "action",
  tags: [],
  doc: null,
  tool: "select",
  selected: -1,
  history: [],
  hi: -1,
  dirty: false,
  builtin: false,
  imported: false,
  designed: false,
  saving: false,
  drag: null,
  draw: null,
  poly: null,
  hover: null,
  onSaved: null,
  onClosed: null,
};

const clone = (v) => JSON.parse(JSON.stringify(v));
const svgEl = () => $("icd-svg");
const boardEl = () => $("icd-board");
const open = () => D.open;

function fnum(v) {
  const n = Number(v);
  if (!Number.isFinite(n)) return "0";
  const r = Math.round(n * 100) / 100;
  return String(r);
}

function round2(v) { return Math.round(Number(v) * 100) / 100; }

function normalizeDoc(raw) {
  const d = clone(raw || {});
  d.version = 1;
  d.grid = Number(d.grid) || DEFAULTS.grid;
  d.strokeWidth = Number(d.strokeWidth) || DEFAULTS.strokeWidth;
  d.cap = d.cap || DEFAULTS.cap;
  d.join = d.join || DEFAULTS.join;
  d.shapes = Array.isArray(d.shapes) ? d.shapes.map((s) => ({ ...s })) : [];
  return d;
}

function blankDoc() {
  return normalizeDoc({ grid: DEFAULTS.grid, strokeWidth: DEFAULTS.strokeWidth, shapes: [] });
}

function shapeLabel(s) {
  return ({
    line: "直线", rect: "矩形", circle: "圆形", ellipse: "椭圆",
    polyline: "折线", polygon: "多边形", path: "路径",
  })[s.type] || s.type;
}

function styleAttrs(s) {
  let a = "";
  if (s.strokeWidth > 0) a += ` stroke-width="${fnum(s.strokeWidth)}"`;
  if (s.stroke === false) a += ` stroke="none"`;
  if (s.fill) a += ` fill="currentColor"`;
  if (s.opacity > 0 && s.opacity < 1) a += ` opacity="${fnum(s.opacity)}"`;
  return a;
}

// 与 Go 侧 renderIconShape 对齐：同一份几何在画板、预览、落库三处长得一样。
function shapeMarkup(s, idx) {
  let tag;
  switch (s.type) {
    case "line":
      tag = `<line x1="${fnum(s.x1)}" y1="${fnum(s.y1)}" x2="${fnum(s.x2)}" y2="${fnum(s.y2)}"`;
      break;
    case "rect":
      tag = `<rect x="${fnum(s.x)}" y="${fnum(s.y)}" width="${fnum(s.width)}" height="${fnum(s.height)}"`;
      if (s.radius > 0) tag += ` rx="${fnum(s.radius)}"`;
      break;
    case "circle":
      tag = `<circle cx="${fnum(s.cx)}" cy="${fnum(s.cy)}" r="${fnum(s.r)}"`;
      break;
    case "ellipse":
      tag = `<ellipse cx="${fnum(s.cx)}" cy="${fnum(s.cy)}" rx="${fnum(s.rx)}" ry="${fnum(s.ry)}"`;
      break;
    case "polyline":
    case "polygon":
      tag = `<${s.type} points="${pointsAttr(s.points)}"`;
      break;
    case "path":
      tag = `<path d="${esc(String(s.d || ""))}"`;
      break;
    default:
      return "";
  }
  tag += styleAttrs(s) + "/>";
  if (idx == null || idx < 0) return tag;
  const attrs = ` data-idx="${idx}" class="icd-art"`;
  if (s.dx || s.dy) {
    return `<g transform="translate(${fnum(s.dx)} ${fnum(s.dy)})"${attrs}>${tag}</g>`;
  }
  return tag.replace(/^<([a-z]+)/, `<$1${attrs}`);
}

function pointsAttr(points) {
  return (points || []).map((p) => `${fnum(p[0])},${fnum(p[1])}`).join(" ");
}

function docSVG(doc) {
  const g = doc.grid || DEFAULTS.grid;
  const shapes = (doc.shapes || []).filter((s) => !s.hidden).map((s) => shapeMarkup(s, -1)).join("");
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${fnum(g)} ${fnum(g)}">` +
    `<g fill="none" stroke="currentColor" stroke-width="${fnum(doc.strokeWidth)}" ` +
    `stroke-linecap="${esc(doc.cap)}" stroke-linejoin="${esc(doc.join)}">${shapes}</g></svg>`;
}

// ---------------- 画板 ----------------

function gridMarkup(g) {
  const lines = [];
  for (let i = 0; i <= g; i++) {
    const cls = i % 4 === 0 ? "icd-grid-major" : "icd-grid-line";
    lines.push(`<line class="${cls}" x1="${i}" y1="0" x2="${i}" y2="${g}"/>`);
    lines.push(`<line class="${cls}" x1="0" y1="${i}" x2="${g}" y2="${i}"/>`);
  }
  // 安全区（2 单位内边距）与光学圆（视觉直径 20）—— 24 网格图标的通用关键线。
  const safe = g === 24 ? `<rect class="icd-safe" x="2" y="2" width="20" height="20"/>` +
    `<circle class="icd-keyline" cx="12" cy="12" r="10"/>` : "";
  return `<g class="icd-grid" aria-hidden="true">${lines.join("")}${safe}</g>`;
}

function renderBoard() {
  const svg = svgEl();
  if (!svg || !D.doc) return;
  const g = D.doc.grid || DEFAULTS.grid;
  svg.setAttribute("viewBox", `0 0 ${fnum(g)} ${fnum(g)}`);
  svg.dataset.tool = D.tool;
  const shapes = D.doc.shapes.map((s, i) => (s.hidden ? "" : shapeMarkup(s, i))).join("");
  const draft = D.draft && D.draft.type ? `<g class="icd-draft">${shapeMarkup(D.draft, -1)}</g>` : "";
  const hover = D.poly && D.hover
    ? `<g class="icd-draft">${shapeMarkup(
      { type: D.poly.type, points: [...D.poly.points, D.hover] }, -1)}</g>` : "";
  svg.innerHTML = gridMarkup(g) +
    `<g fill="none" stroke="currentColor" stroke-width="${fnum(D.doc.strokeWidth)}" ` +
    `stroke-linecap="${esc(D.doc.cap)}" stroke-linejoin="${esc(D.doc.join)}">${shapes}</g>` +
    draft + hover;
  decorateSelection();
}

function decorateSelection() {
  const svg = svgEl();
  const s = D.doc && D.selected >= 0 ? D.doc.shapes[D.selected] : null;
  if (!svg || !s) return;
  const el = svg.querySelector(`[data-idx="${D.selected}"]`);
  if (!el) return;
  let bbox;
  try {
    const target = el.tagName.toLowerCase() === "g" ? el.firstElementChild : el;
    bbox = target.getBBox();
  } catch (_) { return; }
  const ox = s.dx || 0, oy = s.dy || 0;
  const x = bbox.x + ox - 0.5, y = bbox.y + oy - 0.5;
  const w = bbox.width + 1, h = bbox.height + 1;
  let out = `<rect class="icd-sel" x="${fnum(x)}" y="${fnum(y)}" width="${fnum(w)}" height="${fnum(h)}"/>`;
  for (const [hx, hy] of [[x, y], [x + w, y], [x, y + h], [x + w, y + h]]) {
    out += `<circle class="icd-handle" cx="${fnum(hx)}" cy="${fnum(hy)}" r="0.55"/>`;
  }
  if (s.type === "line") {
    out += `<circle class="icd-handle" cx="${fnum(s.x1)}" cy="${fnum(s.y1)}" r="0.7"/>` +
      `<circle class="icd-handle" cx="${fnum(s.x2)}" cy="${fnum(s.y2)}" r="0.7"/>`;
  }
  svg.insertAdjacentHTML("beforeend", `<g class="icd-overlay" aria-hidden="true">${out}</g>`);
}

function pointOf(ev) {
  const rect = svgEl().getBoundingClientRect();
  if (!rect.width || !rect.height) return { x: 0, y: 0 };
  const g = D.doc.grid || DEFAULTS.grid;
  return {
    x: (ev.clientX - rect.left) / rect.width * g,
    y: (ev.clientY - rect.top) / rect.height * g,
  };
}

function snap(v, ev) {
  const step = ev && ev.altKey ? 0.1 : 0.5;
  return round2(Math.round(v / step) * step);
}

function snapPoint(ev, altKey) {
  const p = pointOf(ev);
  return { x: snap(p.x, { altKey }), y: snap(p.y, { altKey }) };
}

// ---------------- 图形几何 ----------------

function moveShape(s, dx, dy) {
  switch (s.type) {
    case "line": s.x1 += dx; s.y1 += dy; s.x2 += dx; s.y2 += dy; break;
    case "rect": s.x += dx; s.y += dy; break;
    case "circle": case "ellipse": s.cx += dx; s.cy += dy; break;
    case "polyline": case "polygon":
      s.points = (s.points || []).map((p) => [round2(p[0] + dx), round2(p[1] + dy)]);
      break;
    case "path": s.dx = round2((s.dx || 0) + dx); s.dy = round2((s.dy || 0) + dy); break;
  }
  return s;
}

function shapeIsBigEnough(s) {
  switch (s.type) {
    case "line": return Math.hypot(s.x2 - s.x1, s.y2 - s.y1) >= MIN_SIZE;
    case "rect": return s.width >= MIN_SIZE && s.height >= MIN_SIZE;
    case "circle": return s.r >= MIN_SIZE;
    case "ellipse": return s.rx >= MIN_SIZE && s.ry >= MIN_SIZE;
    case "polyline": return (s.points || []).length >= 2;
    case "polygon": return (s.points || []).length >= 3;
    case "path": return !!s.d;
  }
  return false;
}

function draftFrom(tool, start, cur, ev) {
  const x = Math.min(start.x, cur.x), y = Math.min(start.y, cur.y);
  let w = Math.abs(cur.x - start.x), h = Math.abs(cur.y - start.y);
  if (ev && ev.shiftKey) {
    const side = Math.max(w, h);
    w = side;
    h = side;
    if (cur.x < start.x) { /* 保持左上角锚点 */ }
  }
  switch (tool) {
    case "line": {
      let x2 = cur.x, y2 = cur.y;
      if (ev && ev.shiftKey) {
        const dx = cur.x - start.x, dy = cur.y - start.y;
        if (Math.abs(dx) > Math.abs(dy) * 2) y2 = start.y;
        else if (Math.abs(dy) > Math.abs(dx) * 2) x2 = start.x;
        else {
          const d = Math.min(Math.abs(dx), Math.abs(dy));
          x2 = start.x + Math.sign(dx || 1) * d;
          y2 = start.y + Math.sign(dy || 1) * d;
        }
      }
      return { type: "line", x1: start.x, y1: start.y, x2: round2(x2), y2: round2(y2) };
    }
    case "rect":
      return { type: "rect", x: round2(x), y: round2(y), width: round2(w), height: round2(h) };
    case "circle":
      return { type: "circle", cx: round2(x + w / 2), cy: round2(y + h / 2), r: round2(Math.min(w, h) / 2) };
    case "ellipse":
      return { type: "ellipse", cx: round2(x + w / 2), cy: round2(y + h / 2), rx: round2(w / 2), ry: round2(h / 2) };
  }
  return null;
}

// ---------------- 历史 ----------------

function resetHistory() {
  D.history = [JSON.stringify(D.doc)];
  D.hi = 0;
  D.dirty = false;
}

function pushHistory() {
  D.history = D.history.slice(0, D.hi + 1);
  D.history.push(JSON.stringify(D.doc));
  while (D.history.length > 40) D.history.shift();
  D.hi = D.history.length - 1;
  D.dirty = true;
}

function undo() {
  if (D.hi <= 0) return;
  D.hi--;
  D.doc = normalizeDoc(JSON.parse(D.history[D.hi]));
  D.selected = Math.min(D.selected, D.doc.shapes.length - 1);
  D.dirty = true;
  renderAll();
}

function redo() {
  if (D.hi >= D.history.length - 1) return;
  D.hi++;
  D.doc = normalizeDoc(JSON.parse(D.history[D.hi]));
  D.selected = Math.min(D.selected, D.doc.shapes.length - 1);
  D.dirty = true;
  renderAll();
}

// ---------------- 渲染 ----------------

function groupOptions(current) {
  return GROUPS.map(([id, label]) =>
    `<option value="${id}"${id === current ? " selected" : ""}>${esc(label)}</option>`).join("");
}

function renderPanelShell() {
  const panel = $("icd-panel");
  if (!panel) return;
  const strokeOpts = [1.2, 1.5, 1.8, 2, 2.4, 3].map((v) =>
    `<option value="${v}"${Math.abs(v - (D.doc.strokeWidth || 1.8)) < 0.01 ? " selected" : ""}>${v}</option>`).join("");
  panel.innerHTML = `<section class="icd-sec">
      <div class="icd-sec-head">图标信息</div>
      <label class="icd-field">名称
        <input id="icd-name" class="icd-input" maxlength="48" spellcheck="false" placeholder="kebab-case，如 brand-mark" value="${esc(D.name)}">
      </label>
      <label class="icd-field">分组
        <select id="icd-group" class="icd-input">${groupOptions(D.group)}</select>
      </label>
      <label class="icd-field">别名（逗号分隔，供检索）
        <input id="icd-tags" class="icd-input" maxlength="240" placeholder="logo, 品牌, 标记" value="${esc((D.tags || []).join(", "))}">
      </label>
    </section>
    <section class="icd-sec">
      <div class="icd-sec-head">光学尺寸预览 <span class="chip">12–24</span></div>
      <div class="icd-previews" id="icd-previews"></div>
      <div class="icd-dark-check"><span>暗底检查</span><span id="icd-dark-prev"></span><small>currentColor</small></div>
    </section>
    <section class="icd-sec">
      <div class="icd-sec-head">图形 <span class="chip" id="icd-shape-count">0</span></div>
      <div id="icd-layers"></div>
    </section>
    <section class="icd-sec">
      <div class="icd-sec-head">选中图形</div>
      <div id="icd-inspect"></div>
    </section>
    <section class="icd-sec">
      <div class="icd-sec-head">描边与网格</div>
      <div class="icd-props">
        <label class="icd-prop">描边宽度<select id="icd-stroke" class="icd-num">${strokeOpts}</select></label>
        <label class="icd-prop">端点<select id="icd-cap" class="icd-num">
          ${["round", "butt", "square"].map((v) => `<option value="${v}"${D.doc.cap === v ? " selected" : ""}>${v}</option>`).join("")}
        </select></label>
        <label class="icd-prop">连接<select id="icd-join" class="icd-num">
          ${["round", "miter", "bevel"].map((v) => `<option value="${v}"${D.doc.join === v ? " selected" : ""}>${v}</option>`).join("")}
        </select></label>
        <label class="icd-prop">网格<input class="icd-num" value="${fnum(D.doc.grid)}" readonly></label>
      </div>
    </section>
    <section class="icd-sec">
      <div class="icd-sec-head">SVG 交付物 <span class="chip">自动生成</span></div>
      <pre class="icd-code" id="icd-source" aria-label="生成的 SVG"></pre>
    </section>`;

  const name = $("icd-name");
  if (name && D.mode === "edit") {
    name.disabled = true;
    name.title = "编辑已有图标时名称不可改";
  }
  renderAll();
}

function renderChrome() {
  const mode = D.mode;
  const badge = $("icd-badge");
  if (badge) {
    badge.textContent = mode === "edit" ? (D.imported ? "改写现有图标" : "编辑设计稿")
      : (mode === "clone" ? "以内置为起点" : "新建");
  }
  const title = $("icd-title");
  if (title) title.textContent = mode === "edit" ? "设计图标 · " + D.original : "设计图标";
  const note = $("icd-note");
  if (note) {
    note.textContent = mode === "edit"
      ? (D.imported
        ? "现有图标由 SVG 反推为几何图形；保存会写回 custom-icons/" + D.original + ".json（含 design）"
        : "改动会同时写回设计稿与 SVG；下次打开仍是可编辑图层")
      : (mode === "clone"
        ? "内置图标只读；保存会新建一个自定义图标，原图标不变"
        : "保存为 design + SVG；design_icons 与图标精灵立即生效");
  }
  const save = $("icd-save");
  if (save) save.textContent = mode === "edit" ? "保存修改" : "保存并启用";
  const u = $("icd-undo"), r = $("icd-redo");
  if (u) u.disabled = D.hi <= 0;
  if (r) r.disabled = D.hi >= D.history.length - 1;
}

function renderPreviews() {
  const box = $("icd-previews");
  if (!box || !D.doc) return;
  const svg = docSVG(D.doc);
  box.innerHTML = [12, 16, 20, 24].map((size) =>
    `<div class="icd-prev" title="${size}px 光学尺寸">` +
    `<span style="width:${size}px;height:${size}px">${svg}</span><b>${size}</b></div>`).join("");
  // docSVG 未带尺寸，预览格用 CSS 控制显示大小（每个 svg 100% 填充自己的格子）。
  box.querySelectorAll(".icd-prev > span > svg").forEach((el) => {
    el.setAttribute("width", "100%");
    el.setAttribute("height", "100%");
  });
  const dark = $("icd-dark-prev");
  if (dark) dark.innerHTML = svg;
}

function renderLayers() {
  const box = $("icd-layers");
  if (!box || !D.doc) return;
  const count = $("icd-shape-count");
  if (count) count.textContent = String(D.doc.shapes.length);
  if (!D.doc.shapes.length) {
    box.innerHTML = `<div class="icd-empty">还没有图形。选左侧工具在画板上拖一笔，或用折线逐点描边。</div>`;
    return;
  }
  box.innerHTML = D.doc.shapes.map((s, i) => {
    const cls = ["icd-layer"];
    if (i === D.selected) cls.push("on");
    if (s.hidden) cls.push("off");
    return `<div class="${cls.join(" ")}" data-layer="${i}" role="button" tabindex="-1">` +
      `${icon(i === D.selected ? "edit" : "layers", 14)}` +
      `<span class="icd-layer-name">${i + 1}. ${esc(shapeLabel(s))}` +
      `${s.fill ? " · 实心" : ""}</span>` +
      `<button class="icd-eye${s.hidden ? "" : " on"}" data-vis="${i}" ` +
      `title="${s.hidden ? "显示" : "隐藏"}" aria-label="${s.hidden ? "显示" : "隐藏"}第 ${i + 1} 个图形">` +
      `${icon(s.hidden ? "eye-off" : "eye", 12)}</button></div>`;
  }).join("");
}

function prop(label, name, value, step) {
  return `<label class="icd-prop">${label}<input class="icd-num" type="number" step="${step || 0.5}" ` +
    `data-prop="${name}" value="${fnum(value || 0)}"></label>`;
}

function renderInspector() {
  const box = $("icd-inspect");
  if (!box || !D.doc) return;
  if (D.selected < 0 || !D.doc.shapes[D.selected]) {
    box.innerHTML = `<div class="icd-empty">点画板上的图形选中；也可在图层里点一行。选中后可拖动、改坐标或删除。</div>`;
    return;
  }
  const s = D.doc.shapes[D.selected];
  let geo = "";
  switch (s.type) {
    case "line":
      geo = `<div class="icd-props">${prop("X1", "x1", s.x1)}${prop("Y1", "y1", s.y1)}` +
        `${prop("X2", "x2", s.x2)}${prop("Y2", "y2", s.y2)}</div>`;
      break;
    case "rect":
      geo = `<div class="icd-props">${prop("X", "x", s.x)}${prop("Y", "y", s.y)}` +
        `${prop("宽", "width", s.width)}${prop("高", "height", s.height)}` +
        `${prop("圆角", "radius", s.radius)}</div>`;
      break;
    case "circle":
      geo = `<div class="icd-props">${prop("CX", "cx", s.cx)}${prop("CY", "cy", s.cy)}${prop("半径", "r", s.r)}</div>`;
      break;
    case "ellipse":
      geo = `<div class="icd-props">${prop("CX", "cx", s.cx)}${prop("CY", "cy", s.cy)}` +
        `${prop("RX", "rx", s.rx)}${prop("RY", "ry", s.ry)}</div>`;
      break;
    case "polyline":
    case "polygon":
      geo = `<label class="icd-field">点（x,y 空格分隔，至少 ${s.type === "polygon" ? 3 : 2} 个）` +
        `<input class="icd-input" data-prop="points" value="${esc(pointsAttr(s.points))}"></label>`;
      break;
    case "path":
      geo = `<label class="icd-field">路径 d` +
        `<input class="icd-input" data-prop="d" spellcheck="false" value="${esc(s.d || "")}"></label>` +
        `<div class="icd-props">${prop("平移 X", "dx", s.dx)}${prop("平移 Y", "dy", s.dy)}</div>`;
      break;
  }
  box.innerHTML = `<div class="icd-sec-head" style="margin-bottom:var(--ui-space-2)">` +
    `<span class="chip">${esc(shapeLabel(s))}</span><span class="mono">#${D.selected + 1}</span></div>` +
    geo +
    `<div class="icd-checks" style="margin-top:var(--ui-space-3)">` +
    `<label class="icd-check"><input type="checkbox" data-toggle="fill"${s.fill ? " checked" : ""}>实心 currentColor</label>` +
    `<label class="icd-check"><input type="checkbox" data-toggle="stroke"${s.stroke === false ? "" : " checked"}>描边</label>` +
    `<label class="icd-check">不透明度<input class="icd-num" style="max-width:var(--ui-space-16)" type="number" ` +
    `min="0" max="1" step="0.1" data-prop="opacity" value="${fnum(s.opacity || 1)}"></label>` +
    `</div>` +
    `<div class="icd-acts" style="margin-top:var(--ui-space-4)">` +
    `<button class="icd-mini" data-op="duplicate">${icon("copy", 12)}复制</button>` +
    `<button class="icd-mini" data-op="up">${icon("arrow-up", 12)}上移</button>` +
    `<button class="icd-mini" data-op="down">${icon("arrow-down", 12)}下移</button>` +
    `<button class="icd-mini icd-mini--danger" data-op="delete">${icon("trash", 12)}删除</button>` +
    `</div>`;
}

function renderCode() {
  const pre = $("icd-source");
  if (!pre || !D.doc) return;
  pre.textContent = docSVG(D.doc).replace(/></g, ">\n<");
}

function renderAll() {
  renderBoard();
  renderLayers();
  renderInspector();
  renderPreviews();
  renderCode();
  renderChrome();
}

// ---------------- 选择 / 绘制 ----------------

function selectShape(i) {
  if (D.selected === i) return;
  D.selected = i;
  renderBoard();
  renderLayers();
  renderInspector();
}

function hitIndex(ev) {
  const el = ev.target.closest("[data-idx]");
  if (!el || !svgEl().contains(el)) return -1;
  const n = Number(el.dataset.idx);
  return Number.isInteger(n) ? n : -1;
}

function setTool(id) {
  if (!TOOLS.some((t) => t.id === id)) return;
  if (D.poly) cancelPoly();
  D.tool = id;
  D.drag = null;
  D.draw = null;
  D.draft = null;
  for (const b of $("icd-tools").querySelectorAll("[data-tool]")) {
    const on = b.dataset.tool === id;
    b.classList.toggle("on", on);
    b.setAttribute("aria-pressed", on ? "true" : "false");
  }
  const hint = $("icd-hint");
  const tool = TOOLS.find((t) => t.id === id);
  if (hint) hint.textContent = tool ? tool.hint : "";
  renderBoard();
}

function beginPoly(pt) {
  D.poly = { type: D.tool === "polygon" ? "polygon" : "polyline", points: [pt] };
  D.hover = pt;
  renderBoard();
}

function addPolyPoint(pt) {
  if (!D.poly) { beginPoly(pt); return; }
  const pts = D.poly.points;
  const first = pts[0];
  if (D.poly.type === "polygon" && pts.length >= 3 && Math.hypot(pt.x - first[0], pt.y - first[1]) < 0.8) {
    finishPoly();
    return;
  }
  const last = pts[pts.length - 1];
  if (Math.hypot(pt.x - last[0], pt.y - last[1]) < 0.05) return;
  pts.push(pt);
  D.hover = pt;
  renderBoard();
}

function finishPoly() {
  if (!D.poly) return;
  const s = D.poly.type;
  const pts = D.poly.points;
  if ((s === "polygon" && pts.length < 3) || (s === "polyline" && pts.length < 2)) {
    D.poly = null;
    D.hover = null;
    renderBoard();
    return;
  }
  D.poly = null;
  D.hover = null;
  D.doc.shapes.push({ type: s, points: pts.map((p) => [round2(p[0]), round2(p[1])]), fill: false });
  D.selected = D.doc.shapes.length - 1;
  pushHistory();
  renderAll();
}

function cancelPoly() {
  D.poly = null;
  D.hover = null;
  D.draft = null;
  renderBoard();
}

function commitDraw() {
  const shape = D.draw && D.draw.shape;
  D.draw = null;
  D.draft = null;
  if (!shape || !shapeIsBigEnough(shape)) { renderBoard(); return; }
  if (shape.type === "rect" && shape.width >= 1 && shape.height >= 1) {
    shape.radius = Math.min(1, Math.min(shape.width, shape.height) / 2);
  }
  D.doc.shapes.push(shape);
  D.selected = D.doc.shapes.length - 1;
  pushHistory();
  renderAll();
}

function removeSelected() {
  if (D.selected < 0 || !D.doc.shapes[D.selected]) return;
  D.doc.shapes.splice(D.selected, 1);
  D.selected = Math.min(D.selected, D.doc.shapes.length - 1);
  pushHistory();
  renderAll();
}

function duplicateSelected() {
  if (D.selected < 0 || !D.doc.shapes[D.selected]) return;
  const copy = moveShape(clone(D.doc.shapes[D.selected]), 1, 1);
  D.doc.shapes.push(copy);
  D.selected = D.doc.shapes.length - 1;
  pushHistory();
  renderAll();
}

function moveLayer(dir) {
  const i = D.selected;
  const j = i + dir;
  if (i < 0 || j < 0 || j >= D.doc.shapes.length) return;
  const list = D.doc.shapes;
  [list[i], list[j]] = [list[j], list[i]];
  D.selected = j;
  pushHistory();
  renderAll();
}

// ---------------- 画板事件 ----------------

function onDown(ev) {
  if (!open() || ev.button !== 0) return;
  ev.preventDefault();
  const svg = svgEl();
  if (D.poly && ev.detail >= 2) { finishPoly(); return; }
  const pt = snapPoint(ev, ev.altKey);
  if (D.tool === "select") {
    const idx = hitIndex(ev);
    if (idx >= 0) {
      selectShape(idx);
      D.drag = { idx, start: pt, orig: clone(D.doc.shapes[idx]), moved: false };
      try { svg.setPointerCapture(ev.pointerId); } catch (_) { /* 忽略 */ }
    } else {
      selectShape(-1);
    }
    return;
  }
  if (D.tool === "polyline" || D.tool === "polygon") {
    addPolyPoint(pt);
    return;
  }
  D.draw = { tool: D.tool, start: pt, shape: null };
  D.draft = draftFrom(D.tool, pt, pt, ev);
  try { svg.setPointerCapture(ev.pointerId); } catch (_) { /* 忽略 */ }
  renderBoard();
}

function onMove(ev) {
  if (!open()) return;
  const pt = snapPoint(ev, ev.altKey);
  if (D.poly) {
    D.hover = pt;
    renderBoard();
    return;
  }
  if (D.drag) {
    const dx = round2(pt.x - D.drag.start.x);
    const dy = round2(pt.y - D.drag.start.y);
    if (dx === 0 && dy === 0) return;
    D.drag.moved = true;
    D.doc.shapes[D.drag.idx] = moveShape(clone(D.drag.orig), dx, dy);
    renderBoard();
    renderPreviews();
    renderCode();
    return;
  }
  if (D.draw) {
    D.draw.shape = draftFrom(D.draw.tool, D.draw.start, pt, ev);
    D.draft = D.draw.shape;
    renderBoard();
  }
}

function endPointer(ev) {
  if (!open()) return;
  try { svgEl().releasePointerCapture(ev.pointerId); } catch (_) { /* 忽略 */ }
  if (D.drag) {
    if (D.drag.moved) pushHistory();
    D.drag = null;
    renderAll();
    return;
  }
  if (D.draw) commitDraw();
}

function cancelPointer() {
  D.drag = null;
  D.draw = null;
  D.draft = null;
  renderBoard();
}

// ---------------- 面板事件 ----------------

function setError(msg) {
  const el = $("icd-error");
  if (el) el.textContent = msg || "";
  else if (msg) toast(msg, true);
}

function currentShape() {
  return D.selected >= 0 ? D.doc.shapes[D.selected] : null;
}

function parsePoints(raw) {
  const nums = String(raw || "").trim().split(/[\s,]+/).filter(Boolean).map(Number);
  if (nums.length < 4 || nums.length % 2 !== 0 || nums.some((n) => !Number.isFinite(n))) return null;
  const out = [];
  for (let i = 0; i < nums.length; i += 2) out.push([nums[i], nums[i + 1]]);
  return out;
}

function applyProp(name, input) {
  const s = currentShape();
  if (!s) return false;
  if (name === "points") {
    const pts = parsePoints(input.value);
    if (!pts) return false;
    s.points = pts;
    return true;
  }
  if (name === "d") {
    const v = String(input.value || "").trim();
    if (!PATH_RE.test(v)) { setError("路径只允许 SVG path 命令与数字（M/L/H/V/C/S/Q/T/A/Z）"); return false; }
    s.d = v;
    return true;
  }
  const v = Number(input.value);
  if (!Number.isFinite(v)) return false;
  if (name === "opacity") s.opacity = Math.min(1, Math.max(0, v));
  else if (name === "radius") s.radius = Math.max(0, v);
  else if (name === "width" || name === "height") s[name] = Math.max(0.05, v);
  else if (name === "r" || name === "rx" || name === "ry") s[name] = Math.max(0.05, v);
  else s[name] = v;
  return true;
}

// ---------------- 保存 / 关闭 ----------------

function closeModal() {
  $("icd-modal").classList.add("hidden");
  D.open = false;
}

async function requestClose(savedName) {
  if (D.saving && !savedName) return;
  if (D.asking) return;
  if (!savedName && D.dirty) {
    D.asking = true;
    let ok = false;
    try {
      ok = await uiConfirm({
        title: "放弃未保存的设计？",
        note: "当前设计稿还没有保存，关闭后这一版图形会丢失。",
        okText: "放弃修改",
        danger: true,
      });
    } finally {
      D.asking = false;
    }
    if (!ok) return;
  }
  const onSaved = D.onSaved, onClosed = D.onClosed;
  closeModal();
  D.doc = null;
  D.drag = D.draw = D.poly = null;
  if (savedName) { if (onSaved) onSaved(savedName); }
  else if (onClosed) onClosed();
}

async function save() {
  if (D.saving) return;
  const errEl = $("icd-error");
  if (errEl) errEl.textContent = "";
  const input = $("icd-name");
  let name = D.mode === "edit" ? D.original : (input ? input.value.trim().toLowerCase() : "");
  if (!name || name.length > 48 || !NAME_RE.test(name)) {
    setError("名称只能是小写字母/数字/连字符（1–48 位）");
    if (input && D.mode !== "edit") input.focus();
    return;
  }
  if (!D.doc.shapes.length) { setError("至少画一个图形再保存"); return; }
  for (const s of D.doc.shapes) {
    if (s.type === "path" && !PATH_RE.test(String(s.d || ""))) {
      setError("路径 d 含不允许的字符");
      return;
    }
  }
  const body = {
    group: D.group,
    tags: D.tags,
    design: D.doc,
  };
  D.saving = true;
  const btn = $("icd-save");
  if (btn) btn.disabled = true;
  try {
    if (D.mode === "edit") {
      await api("/api/catalog/icons/" + encodeURIComponent(D.original), {
        method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
      toast("已保存图标 " + D.original);
      D.dirty = false;
      await requestClose(D.original);
    } else {
      const data = await api("/api/catalog/icons", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, ...body }),
      });
      const saved = (data.icon && data.icon.name) || name;
      toast("已新增图标 " + saved);
      D.dirty = false;
      await requestClose(saved);
    }
  } catch (e) {
    setError((e.code ? e.code + "：" : "") + e.message);
  } finally {
    D.saving = false;
    if (btn) btn.disabled = false;
  }
}

// ---------------- 打开 ----------------

function suggestName(base) {
  let n = (base || "icon") + "-custom";
  if (n.length > 48) n = base.slice(0, 40) + "-custom";
  return n;
}

function splitTags(v) {
  return String(v || "").split(/[,，]/).map((s) => s.trim()).filter(Boolean).slice(0, 12);
}

export async function openIconDesigner(opt) {
  const o = opt || {};
  D.onSaved = o.onSaved || null;
  D.onClosed = o.onClosed || null;
  D.dirty = false;
  D.saving = false;
  D.asking = false;
  D.selected = -1;
  D.drag = D.draw = D.poly = D.hover = null;
  D.builtin = D.imported = D.designed = false;

  if (o.name) {
    let data;
    try {
      data = await api("/api/catalog/icons/" + encodeURIComponent(o.name) + "/design");
    } catch (e) {
      toast("打开设计器失败: " + e.message, true);
      if (D.onClosed) D.onClosed();
      return;
    }
    if (!data || !data.design) {
      toast("这个图标没有可用的设计稿", true);
      if (D.onClosed) D.onClosed();
      return;
    }
    D.doc = normalizeDoc(data.design);
    D.builtin = !!data.builtin;
    D.imported = !!data.imported;
    D.designed = !!data.designed;
    D.original = data.name || o.name;
    D.mode = D.builtin ? "clone" : "edit";
    D.name = D.builtin ? suggestName(D.original) : D.original;
    D.group = data.group || "action";
    D.tags = Array.isArray(data.tags) ? data.tags : [];
  } else {
    D.doc = blankDoc();
    D.mode = "create";
    D.original = "";
    D.name = "";
    D.group = "action";
    D.tags = [];
  }

  D.open = true;
  $("icd-modal").classList.remove("hidden");
  $("icd-tools").innerHTML = TOOLS.map((t) =>
    `<button class="icd-tool${t.id === "select" ? " on" : ""}" data-tool="${t.id}" ` +
    `data-tip="${esc(t.label)}（${t.key}）" aria-label="${esc(t.label)}" aria-pressed="${t.id === "select"}">` +
    `${t.key}</button>`).join("");
  D.tool = "select";
  const hint = $("icd-hint");
  if (hint) hint.textContent = TOOLS[0].hint;
  resetHistory();
  renderPanelShell();
  const board = boardEl();
  if (board) board.focus();
}

export function isIconDesignerOpen() { return D.open; }

// ---------------- 绑定 ----------------

$("icd-tools").addEventListener("click", (ev) => {
  const b = ev.target.closest("[data-tool]");
  if (b) setTool(b.dataset.tool);
});

$("icd-panel").addEventListener("input", (ev) => {
  const el = ev.target;
  if (el.id === "icd-name") { D.name = el.value; return; }
  if (el.id === "icd-tags") { D.tags = splitTags(el.value); return; }
  const prop = el.dataset.prop;
  if (prop && applyProp(prop, el)) {
    renderBoard();
    renderPreviews();
    renderCode();
  }
});

$("icd-panel").addEventListener("change", (ev) => {
  const el = ev.target;
  if (el.id === "icd-group") { D.group = el.value; return; }
  if (el.id === "icd-stroke") {
    D.doc.strokeWidth = Number(el.value) || DEFAULTS.strokeWidth;
    pushHistory(); renderAll(); return;
  }
  if (el.id === "icd-cap") { D.doc.cap = el.value; pushHistory(); renderAll(); return; }
  if (el.id === "icd-join") { D.doc.join = el.value; pushHistory(); renderAll(); return; }
  const toggle = el.dataset.toggle;
  const s = currentShape();
  if (toggle && s) {
    if (toggle === "fill") s.fill = el.checked;
    else if (toggle === "stroke") {
      if (el.checked) delete s.stroke;
      else s.stroke = false;
    }
    // 只刷新画板/预览/图层，不重建检查器：否则「改完半径再点实心」时，
    // 半径的 change 会把复选框节点换掉，点击落到已卸载的元素上。
    pushHistory();
    renderLayers(); renderBoard(); renderPreviews(); renderCode(); renderChrome();
    return;
  }
  const prop = el.dataset.prop;
  if (prop && applyProp(prop, el)) {
    pushHistory();
    renderLayers(); renderBoard(); renderPreviews(); renderCode(); renderChrome();
  }
});

$("icd-panel").addEventListener("click", (ev) => {
  const vis = ev.target.closest("[data-vis]");
  if (vis) {
    const i = Number(vis.dataset.vis);
    const s = D.doc.shapes[i];
    if (!s) return;
    s.hidden = !s.hidden;
    pushHistory();
    renderAll();
    return;
  }
  const layer = ev.target.closest("[data-layer]");
  if (layer && !ev.target.closest("[data-op]")) {
    selectShape(Number(layer.dataset.layer));
    return;
  }
  const op = ev.target.closest("[data-op]");
  if (!op) return;
  switch (op.dataset.op) {
    case "duplicate": duplicateSelected(); break;
    case "up": moveLayer(1); break;
    case "down": moveLayer(-1); break;
    case "delete": removeSelected(); break;
  }
});

$("icd-modal").addEventListener("click", (ev) => {
  if (ev.target.closest("[data-icd-close]")) { requestClose(null); return; }
  const act = ev.target.closest("[data-icd-act]");
  if (!act) return;
  switch (act.dataset.icdAct) {
    case "back":
    case "cancel":
      requestClose(null);
      break;
    case "undo": undo(); break;
    case "redo": redo(); break;
    case "save": save(); break;
  }
});

const svg = svgEl();
svg.addEventListener("pointerdown", onDown);
svg.addEventListener("pointermove", onMove);
svg.addEventListener("pointerup", endPointer);
svg.addEventListener("pointercancel", cancelPointer);
svg.addEventListener("dblclick", (ev) => {
  if (D.poly) { ev.preventDefault(); finishPoly(); }
});

$("icd-modal").addEventListener("keydown", (e) => {
  if (e.key !== "Tab") return;
  const list = [...$("icd-modal").querySelectorAll("button,input,textarea,select")]
    .filter((el) => !el.disabled && el.offsetParent !== null);
  if (!list.length) return;
  const first = list[0], last = list[list.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
});

// Esc 由本模块优先处理（图标库弹层此时已收起）；保存确认走 uiConfirm。
window.addEventListener("keydown", (e) => {
  if (!open()) return;
  if (e.key === "Escape") {
    // 确认弹层开着时，Esc 交给 ui.js/main.js 关闭它，不要叠第二层确认。
    if (isAskOpen()) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    if (D.poly) cancelPoly();
    else requestClose(null);
    return;
  }
  const tag = (e.target && e.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  if ((e.ctrlKey || e.metaKey) && (e.key === "z" || e.key === "Z")) {
    e.preventDefault();
    e.stopImmediatePropagation();
    if (e.shiftKey) redo(); else undo();
    return;
  }
  if ((e.ctrlKey || e.metaKey) && (e.key === "y" || e.key === "Y")) {
    e.preventDefault(); e.stopImmediatePropagation(); redo(); return;
  }
  if (e.key === "Delete" || e.key === "Backspace") {
    // 设计器打开时 Del/Backspace 只作用于图标图形，不能落到画布节点删除。
    e.preventDefault();
    e.stopImmediatePropagation();
    if (D.selected >= 0) removeSelected();
    return;
  }
  if (e.key === "Enter") {
    if (D.poly) { e.preventDefault(); finishPoly(); }
    return;
  }
  if (e.key.startsWith("Arrow") && D.selected >= 0) {
    e.preventDefault();
    e.stopImmediatePropagation();
    const step = e.shiftKey ? 1 : 0.5;
    const dx = e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0;
    const dy = e.key === "ArrowUp" ? -step : e.key === "ArrowDown" ? step : 0;
    D.doc.shapes[D.selected] = moveShape(D.doc.shapes[D.selected], dx, dy);
    pushHistory();
    renderBoard(); renderPreviews(); renderCode(); renderChrome();
    return;
  }
  const key = e.key.toLowerCase();
  const tool = TOOLS.find((t) => t.key.toLowerCase() === key);
  if (tool) { e.preventDefault(); e.stopImmediatePropagation(); setTool(tool.id); }
}, true);
