/* adago-design 画布 · 视图变换：平移/缩放/适配/居中、画布控制菜单、比例预设、可视区计算。 */
"use strict";

import { S, $, world, grid, paperEl, UI, tokenPx } from "./state.js";
import { icon } from "./icons-bridge.js";
import { esc, toast, setTip } from "./ui.js";
import { api } from "./net.js";
import { isAllScope, totalNodes, projectName, renderProjectTree, switchProject } from "./projects.js";
import { renderProjectRows, isProjectModalOpen, scheduleLabels, updateLabelScale, closeTreeMenu } from "./panels.js";
import { scheduleEdges } from "./edges.js";
// ---------------- 视图变换 ----------------
function applyTransform() {
  world.style.transform = `translate(${S.tx}px,${S.ty}px) scale(${S.scale})`;
  grid.style.transform = `translate(${S.tx}px,${S.ty}px) scale(${S.scale})`;
  $("zoom-label").textContent = Math.round(S.scale * 100) + "%";
  updateLabelScale();
}

function zoomAt(cx, cy, factor) {
  const ns = Math.min(4, Math.max(0.08, S.scale * factor));
  if (ns === S.scale) return;
  const k = ns / S.scale;
  S.tx = cx - (cx - S.tx) * k;
  S.ty = cy - (cy - S.ty) * k;
  S.scale = ns;
  applyTransform();
  scheduleEdges("*");
}

// 适配几何的统一实现：**以「内容全部可见」为第一原则**（zoom-to-fit）。
// 只有在 rect 本身比可视区还大（几乎不会发生，除非单块内容巨大）时才不缩到底，
// 此时对齐内容左上角，让人知道「后面还有，拖动/滚轮继续看」。
// opt.pad / opt.cap 可覆盖外边距与最大放大倍数。
function fitRect(rect, opt) {
  const o = opt || {};
  const v = visibleRect();
  const pad = o.pad != null ? o.pad : 64;
  const availW = Math.max(60, v.w - pad * 2);
  const availH = Math.max(60, v.h - pad * 2);
  const cap = o.cap || 1.2;
  let scale = Math.min(availW / rect.w, availH / rect.h, cap);
  scale = Math.min(4, Math.max(0.04, scale));
  S.scale = scale;
  const overflowX = rect.w * scale > availW;
  const overflowY = rect.h * scale > availH;
  S.tx = overflowX ? v.x + pad - rect.x * scale : v.x + v.w / 2 - (rect.x + rect.w / 2) * scale;
  S.ty = overflowY ? v.y + pad - rect.y * scale : v.y + v.h / 2 - (rect.y + rect.h / 2) * scale;
  applyTransform();
  scheduleEdges("*");
  scheduleLabels();
}

function fitView(opt) {
  const soloNode = S.solo ? S.nodes.get(S.solo) : null;
  const pool = soloNode ? [soloNode] : [...S.nodes.values()];
  if (!pool.length) { S.tx = innerWidth / 2 - 200; S.ty = innerHeight / 2 - 150; S.scale = 1; applyTransform(); return; }
  let minX = 1e9, minY = 1e9, maxX = -1e9, maxY = -1e9;
  for (const n of pool) {
    minX = Math.min(minX, n.x); minY = Math.min(minY, n.y);
    maxX = Math.max(maxX, n.x + n.width); maxY = Math.max(maxY, n.y + n.height);
  }
  const pad = soloNode ? 48 : 80;
  // 全部视图：行首标签（约 152px 宽）在内容左侧，把它算进边界（聚焦时无标签）
  const labelPad = !soloNode && isAllScope() && S.nodes.size > 0 ? 170 : 0;
  minX -= labelPad;
  fitRect(
    { x: minX, y: minY, w: maxX - minX, h: maxY - minY },
    Object.assign({ pad: pad, cap: soloNode ? 1.6 : 1.2 }, opt || {}),
  );
}

// fitNode 把单张设计稿整体放进视野：双击单独展示、树里定位、选中后按 F 都用它。
// 语义是「这张稿必须完整可见」，所以缩放不设可读下限。
function fitNode(id) {
  const n = S.nodes.get(id);
  if (!n) return false;
  fitRect({ x: n.x, y: n.y, w: n.width, h: n.height }, { pad: 40, cap: 2 });
  return true;
}

// focusContent 是首屏 / 切项目 / 导入后的默认视角：把当前作用域的全部内容
// 完整放进视野（不裁切任何设计稿）。要看清单张稿用双击或选中后按 F。
function focusContent() {
  const b = canvasBounds();
  if (!b) return;
  fitRect(b, { pad: b.kind === "canvas" ? 56 : 48, cap: 1.6 });
}

// ---------------- 画布版式（选择 / 比例 / 居中） ----------------
// 画布 = 项目作用域 + 一份版式（宽高/比例）。版式是项目的属性，随项目走：
//   选择画布 = 切项目作用域；修改比例 = PUT /api/projects/{id}/canvas；
//   居中展示 = 把画布矩形（无版式时用全部节点包围盒）对齐到可视区中心。
// 纸张只作版式参考，不裁剪、不拦截事件：无限画布仍可把草稿摆到边界外。

function currentProject() {
  return S.project ? S.projects.find((p) => p.id === S.project) || null : null;
}

function canvasCaption(c) {
  if (!c || !c.width) return "随内容";
  return c.width + "×" + c.height + " · " + (c.ratio || "custom");
}

// 内容包围盒：纸面「随内容」模式的唯一几何来源。
// 分组框（frame）也计入 —— 它是成员的背板，漏掉会让纸面比视觉内容短一截。
function contentBounds() {
  let minX = 1e9, minY = 1e9, maxX = -1e9, maxY = -1e9;
  let any = false;
  for (const n of S.nodes.values()) {
    any = true;
    minX = Math.min(minX, n.x);
    minY = Math.min(minY, n.y);
    maxX = Math.max(maxX, n.x + n.width);
    maxY = Math.max(maxY, n.y + n.height);
  }
  if (!any) return null;
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

// 从项目列表同步当前画布版式，并刷新纸张与工具条。
function syncCanvasFromScope() {
  const p = currentProject();
  S.canvas = p && p.canvas ? p.canvas : null;
  renderCanvas();
}

// 纸面渲染：固定比例用配置尺寸；没有配置（自由画布）时随内容包围盒伸缩，
// 让画布的长度始终跟上设计稿 —— 不需要为了看全内容把视口缩到看不清。
// 纸面只是视觉背板，不裁剪、不拦截事件，所以「自由」的拖拽语义不变。
function renderCanvas() {
  const pad = tokenPx("--ui-canvas-pad", 48);
  // 「全部项目」视图左侧有行首标签，额外留出一段避免标签骑在纸面边上
  const labelPad = isAllScope() ? tokenPx("--ui-canvas-label-pad", 176) : pad;
  let rect = null;
  let ratioText = "";
  if (S.canvas && S.canvas.width > 0) {
    rect = { x: 0, y: 0, w: S.canvas.width, h: S.canvas.height };
    ratioText = S.canvas.ratio || "custom";
    // 固定比例画布同样要「随内容延长」：设计稿超出边界时纸面跟着长，
    // 而不是把内容留在纸外（看起来像「画布太短、内容被切掉」）。
    const b = contentBounds();
    if (b) {
      const x0 = Math.min(0, b.x - pad), y0 = Math.min(0, b.y - pad);
      const x1 = Math.max(S.canvas.width, b.x + b.w + pad);
      const y1 = Math.max(S.canvas.height, b.y + b.h + pad);
      if (x0 < -1 || y0 < -1 || x1 > S.canvas.width + 1 || y1 > S.canvas.height + 1) {
        rect = { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
        ratioText = (S.canvas.ratio || "custom") + " · 随内容延长";
      }
    }
  } else {
    const b = contentBounds();
    if (b) {
      rect = {
        x: b.x - labelPad, y: b.y - pad,
        w: b.w + labelPad + pad, h: b.h + pad * 2,
      };
      ratioText = "随内容";
    }
  }
  if (rect) {
    paperEl.classList.remove("hidden");
    paperEl.style.left = Math.round(rect.x) + "px";
    paperEl.style.top = Math.round(rect.y) + "px";
    paperEl.style.width = Math.round(rect.w) + "px";
    paperEl.style.height = Math.round(rect.h) + "px";
    paperEl.querySelector(".cv-size").textContent = Math.round(rect.w) + " × " + Math.round(rect.h);
    paperEl.querySelector(".cv-ratio").textContent = ratioText;
  } else {
    paperEl.classList.add("hidden");
  }
  // 工具条只显示图标：当前画布 / 比例都写进 tooltip（data-tip）
  const p = currentProject();
  const scopeTip = isAllScope()
    ? "画布：全部项目 · 点击选择画布"
    : "画布：" + (p ? p.name : "未选择") + " · " +
      (S.canvas ? S.canvas.width + "×" + S.canvas.height + "（" + (S.canvas.ratio || "custom") + "）" : "随内容自适应") +
      " · 点击选择";
  setTip("cv-select", scopeTip);

  const ratioTip = !S.project
    ? "先选择一个画布（项目），再设置比例"
    : S.canvas
      ? "画布比例：" + (S.canvas.ratio || "自定义") + "（" + S.canvas.width + "×" + S.canvas.height + "） · 点击修改"
      : "画布比例：随内容自适应（自由画布） · 点击修改";
  setTip("cv-ratio", ratioTip);
  const ratioBtn = $("cv-ratio");
  if (ratioBtn) {
    ratioBtn.classList.toggle("is-disabled", !S.project);
    ratioBtn.setAttribute("aria-disabled", S.project ? "false" : "true");
  }
}

// ---- 下拉菜单 ----
function renderCanvasMenu() {
  const box = $("cv-menu");
  if (!box) return;
  const rows = S.projects
    .map((p) => {
      const active = p.id === S.project;
      const meta = canvasCaption(p.canvas) + " · " + (p.node_count || 0) + " 稿";
      return `<button data-act="scope" data-id="${esc(p.id)}" class="${active ? "is-active" : ""}">
        ${icon("folder", 14)}<span class="tb-menu__name">${esc(p.name)}</span><span class="tb-menu__meta">${esc(meta)}</span></button>`;
    })
    .join("");
  box.innerHTML =
    `<div class="tb-menu__head">画布（项目作用域）</div>` +
    `<button data-act="scope" data-id="" class="${isAllScope() ? "is-active" : ""}">` +
    `${icon("grid", 14)}<span class="tb-menu__name">全部项目</span><span class="tb-menu__meta">${totalNodes()} 个稿</span></button>` +
    (rows || '<div class="tb-menu__empty">还没有项目。设计稿必须归属项目，先新建一个。</div>') +
    `<div class="tb-menu__sep"></div>` +
    `<button data-act="new">${icon("plus", 14)}<span>新建项目…</span></button>` +
    `<button data-act="manage">${icon("settings", 14)}<span>项目管理…</span></button>`;
}

function renderRatioMenu() {
  const box = $("ratio-menu");
  if (!box) return;
  const cur = S.canvas ? (S.canvas.ratio || "custom") : "free";
  const rows = S.presets
    .map((p) => {
      const active = cur === p.id;
      return `<button data-ratio="${esc(p.id)}" class="${active ? "is-active" : ""}">
        ${icon("fit", 14)}<span>${esc(p.name)}</span><span class="tb-menu__meta">${p.width}×${p.height}</span></button>`;
    })
    .join("");
  box.innerHTML =
    `<div class="tb-menu__head">画布比例</div>` +
    `<button data-ratio="free" class="${cur === "free" ? "is-active" : ""}">` +
    `${icon("grid", 14)}<span>随内容自适应</span><span class="tb-menu__meta">无边界 · 跟随内容</span></button>` +
    rows +
    `<div class="tb-menu__sep"></div>` +
    `<button data-ratio="custom" class="${cur === "custom" ? "is-active" : ""}">${icon("settings", 14)}<span>自定义宽高…</span></button>`;
}

function closeCanvasMenus() {
  for (const id of ["cv-menu", "ratio-menu"]) {
    const el = $(id);
    if (el) el.classList.add("hidden");
  }
  for (const id of ["cv-select", "cv-ratio"]) {
    const trigger = $(id);
    if (trigger) trigger.setAttribute("aria-expanded", "false");
  }
}

function toggleCanvasMenu(id) {
  const menu = $(id);
  if (!menu) return;
  const willOpen = menu.classList.contains("hidden");
  closeCanvasMenus();
  closeTreeMenu();
  if (!willOpen) return;
  if (id === "cv-menu") renderCanvasMenu();
  else renderRatioMenu();
  menu.classList.remove("hidden");
  const trigger = $(id === "cv-menu" ? "cv-select" : "cv-ratio");
  if (trigger) trigger.setAttribute("aria-expanded", "true");
}

async function loadCanvasPresets() {
  try {
    const data = await api("/api/canvas/presets");
    if (Array.isArray(data.presets) && data.presets.length) S.presets = data.presets;
    if (data.min_side) S.minSide = data.min_side;
    if (data.max_side) S.maxSide = data.max_side;
  } catch (_) {
    // 接口不可用时退回内置最小集，保证菜单仍可用（服务端仍会做最终校验）
    S.presets = [
      { id: "16:9", name: "宽屏 16:9", width: 1920, height: 1080 },
      { id: "4:3", name: "标准 4:3", width: 1440, height: 1080 },
      { id: "1:1", name: "方形 1:1", width: 1440, height: 1440 },
      { id: "9:16", name: "竖屏 9:16", width: 1080, height: 1920 },
    ];
  }
  renderRatioMenu();
}

function applyProjectPatch(p) {
  if (!p || !p.id) return;
  const i = S.projects.findIndex((x) => x.id === p.id);
  if (i >= 0) S.projects[i] = Object.assign({}, S.projects[i], p);
  else S.projects.push(p);
}

async function setCanvasConfig(payload, okText) {
  if (!S.project) {
    toast("画布是项目的属性：先在「全部项目」下拉里选一个项目", true);
    return false;
  }
  try {
    const data = await api("/api/projects/" + encodeURIComponent(S.project) + "/canvas", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    applyProjectPatch(data.project);
    syncCanvasFromScope();
    renderProjectTree();
    if (isProjectModalOpen()) renderProjectRows();
    centerView(true);
    toast(okText || "画布已更新");
    return true;
  } catch (e) {
    toast("画布设置失败: " + e.message, true);
    return false;
  }
}

// ---- 居中 ----
// 目标矩形：单画布聚焦时是该节点；否则有版式用画布矩形，没有则用全部节点包围盒。
function canvasBounds() {
  const solo = S.solo && S.nodes.get(S.solo);
  if (solo) return { x: solo.x, y: solo.y, w: solo.width, h: solo.height, kind: "node" };
  const content = contentBounds();
  if (S.canvas && S.canvas.width > 0) {
    if (!content) return { x: 0, y: 0, w: S.canvas.width, h: S.canvas.height, kind: "canvas" };
    // 固定比例画布也把溢出内容算进适配范围：居中/适配后能看到全部稿子，
    // 而不是只看到纸面、把边界外的设计稿留在视野外。
    const x = Math.min(0, content.x);
    const y = Math.min(0, content.y);
    const right = Math.max(S.canvas.width, content.x + content.w);
    const bottom = Math.max(S.canvas.height, content.y + content.h);
    return { x, y, w: right - x, h: bottom - y, kind: "canvas" };
  }
  if (!content) return null;
  return { x: content.x, y: content.y, w: content.w, h: content.h, kind: "nodes" };
}

// 可视区：扣掉左侧浮窗、右侧检查器、顶部工具条（聚焦时还有一条返回条）。
// 数值全部来自 tokens.css 的布局令牌，这里不再复制 292/288/64。
function leftDockWidth() {
  const dock = $("left-dock");
  if (!dock) return 0;
  const open = [...dock.querySelectorAll(".dock-panel")].some((p) => !p.classList.contains("is-collapsed"));
  return open ? UI.dockW + UI.gutter : 0;
}

function visibleRect() {
  const left = leftDockWidth();
  const inspEl = $("inspector");
  const right = inspEl && !inspEl.classList.contains("hidden") ? UI.inspectorW : 0;
  const top = UI.topSafe + UI.gutter + (S.solo ? UI.soloBarH + UI.gutter : 0);
  return {
    x: left, y: top,
    w: Math.max(160, innerWidth - left - right),
    h: Math.max(160, innerHeight - top - 12),
  };
}

// centerView(fit)：fit=true 时缩放到刚好放下，否则保持当前缩放（放不下才缩）。
function centerView(fit) {
  const b = canvasBounds();
  if (!b) {
    toast("画布上还没有内容");
    return;
  }
  const v = visibleRect();
  const pad = b.kind === "canvas" ? 72 : 48;
  const availW = Math.max(40, v.w - pad * 2);
  const availH = Math.max(40, v.h - pad * 2);
  let scale = S.scale;
  if (fit || b.w * scale > availW || b.h * scale > availH) {
    scale = Math.min(1, availW / b.w, availH / b.h);
  }
  scale = Math.min(4, Math.max(0.06, scale));
  S.scale = scale;
  S.tx = v.x + v.w / 2 - (b.x + b.w / 2) * scale;
  S.ty = v.y + v.h / 2 - (b.y + b.h / 2) * scale;
  applyTransform();
  scheduleEdges("*");
  scheduleLabels();
}

export {
  applyTransform, zoomAt, fitView, fitRect, fitNode, focusContent, currentProject, canvasCaption,
  syncCanvasFromScope, contentBounds,
  renderCanvas, renderCanvasMenu, renderRatioMenu, closeCanvasMenus, toggleCanvasMenu,
  loadCanvasPresets, applyProjectPatch, setCanvasConfig, canvasBounds, leftDockWidth,
  visibleRect, centerView,
};
