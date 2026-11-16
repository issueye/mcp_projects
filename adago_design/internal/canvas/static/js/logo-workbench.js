/* adago-design 画布 · 品牌 Logo 工作台（ES module）
 *
 * 一个弹层内完成 Logo 的「资产库 + 设计 + 规范 + 导出」：
 *   - 列表：<data>/logos/*.json 的已保存资产
 *   - 详情：正形/反形/单色三变体、光学尺寸档、安全空间/最小宽度/字体授权
 *   - 编辑：标记（图标库，或去图标设计器自绘）+ 字标/副标 + 布局/字体/颜色/底色
 *   - 导出：SVG（服务端确定性渲染）、PNG（浏览器 4x 光栅化）、摆上画布
 *
 * 设计稿与 SVG 的单一真源在服务端（design/logodoc.go）；编辑期用 HTML/CSS
 * 锁排 + canvas.measureText 实测字宽，保存时回填 WordmarkWidth/TaglineWidth，
 * 保证「所见」和落库 SVG 的横向对位一致。 */

"use strict";

import { $, S, tokenColor, tokenPx } from "./state.js";
import { esc, toast, uiConfirm } from "./ui.js";
import { api } from "./net.js";
import { icon } from "./icons-bridge.js";
import { openIconDesigner } from "./icon-designer.js";

const LAYOUTS = [
  ["horizontal", "横排：标记 + 字标"],
  ["stacked", "竖排：标记在上"],
  ["mark", "纯标记"],
  ["wordmark", "纯字标"],
];
const VARIANT_LABELS = { positive: "正形", inverse: "反形", mono: "单色" };
const VARIANT_ORDER = ["positive", "inverse", "mono"];

const L = {
  open: false,
  mode: "empty", // empty | detail | edit
  logos: [],
  selected: "",
  detail: null,
  doc: null,
  editing: null,
  name: "",
  note: "",
  system: "",
  dirty: false,
  saving: false,
  asking: false,
  variant: "positive",
  guide: false,
  query: "",
  fonts: [],
  systems: [],
  icons: [],
  refsLoaded: false,
  markPicker: false,
  markQuery: "",
  error: "",
};

const clone = (v) => JSON.parse(JSON.stringify(v));
const pane = () => $("logo-panel");
const stage = () => $("logo-stage");

function hexFromCss(value) {
  const v = String(value || "").trim();
  if (/^#[0-9a-f]{6}$/i.test(v)) return v;
  const m = v.match(/rgba?\(([^)]+)\)/);
  if (!m) return "";
  const parts = m[1].split(/[,\s/]+/).map(Number);
  if (parts.length < 3 || parts.some((n) => !Number.isFinite(n))) return "";
  const h = (n) => Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, "0");
  return "#" + h(parts[0]) + h(parts[1]) + h(parts[2]);
}

function tokenHex(name) {
  return hexFromCss(tokenColor(name, ""));
}

const INK_HEX = () => tokenHex("--ui-ink");
const ON_BRAND_HEX = () => tokenHex("--ui-on-brand");
const SURFACE_HEX = () => tokenHex("--ui-surface");

function isHex(v) { return /^#[0-9a-f]{6}$/i.test(String(v || "")); }

function luminance(hex) {
  if (!isHex(hex)) return 0.5;
  const h = hex.replace("#", "");
  const vals = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const lin = (v) => (v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4));
  return 0.2126 * lin(vals[0]) + 0.7152 * lin(vals[1]) + 0.0722 * lin(vals[2]);
}

function contrast(fg, bg) {
  const a = luminance(fg), b = luminance(bg);
  const hi = Math.max(a, b), lo = Math.min(a, b);
  return (hi + 0.05) / (lo + 0.05);
}

// ---------------- 数据 ----------------

function fontById(id) {
  return L.fonts.find((f) => f.id === id) || L.fonts[0] || null;
}

function fontStack(font) {
  const family = (font && font.family) || "Inter";
  const quoted = `'${family.replace(/'/g, "\\'")}'`;
  // 选中的就是中文字体时不要再重复一次 Noto，避免 canvas.measureText / CSS 里出现两份同族。
  if (family.toLowerCase().includes("noto sans sc") || family.toLowerCase().includes("pingfang")) {
    return `${quoted}, sans-serif`;
  }
  return `${quoted}, 'Noto Sans SC', sans-serif`;
}

function weightOptions(font) {
  const weights = (font && font.weights) || [];
  return weights.length ? weights.slice() : [400, 500, 600, 700, 800];
}

function currentSystem() {
  return L.systems.find((s) => s.id === L.system) || L.systems[0] || null;
}

function defaultDoc() {
  const sys = currentSystem();
  const color = (sys && isHex(sys.primary) && sys.primary) || INK_HEX() || ON_BRAND_HEX();
  const font = fontById("inter") || L.fonts.find((f) => f.commercial) || L.fonts[0] || { id: "inter", family: "Inter", weights: [400, 700] };
  const weight = (font.weights || []).includes(700) ? 700 : (font.weights || [700])[font.weights ? font.weights.length - 1 : 0] || 700;
  const markIcon = (L.icons.find((ic) => !ic.disabled && ic.name === "star") ||
    L.icons.find((ic) => !ic.disabled) || {}).name || "star";
  return {
    version: 1, brand: "", tagline: "",
    layout: "horizontal",
    mark: { kind: "icon", icon: markIcon },
    markSize: 40, gap: 16,
    font: font.id, fontSize: 36, weight, taglineSize: 14,
    letterSpacing: 0, casing: "none",
    color, background: "transparent", clearSpace: 0.5,
  };
}

function measureText(text, size, weight, family, spacing) {
  if (!text) return 0;
  const ctx = measureText._ctx || (measureText._ctx = document.createElement("canvas").getContext("2d"));
  ctx.font = `${weight} ${size}px ${family}`;
  let w = ctx.measureText(text).width;
  if (spacing) w += spacing * (Array.from(text).length - 1);
  return Math.round(w * 100) / 100;
}

function applyMeasurements(doc) {
  if (!doc) return;
  const font = fontById(doc.font);
  const family = fontStack(font);
  const brand = doc.casing === "upper" ? String(doc.brand || "").toUpperCase() : String(doc.brand || "");
  doc.wordmarkWidth = measureText(brand, doc.fontSize, doc.weight, family, doc.letterSpacing);
  doc.taglineWidth = doc.tagline
    ? measureText(doc.tagline, doc.taglineSize, 400, family, doc.letterSpacing * 0.6) : 0;
}

function logoMetrics(doc) {
  if (!doc) return null;
  const hasMark = doc.layout !== "wordmark" && doc.mark && doc.mark.kind !== "none" && doc.mark.icon;
  const markSize = hasMark ? doc.markSize : 0;
  const hasTagline = !!doc.tagline && doc.layout !== "mark";
  const wordH = Math.round(doc.fontSize * 1.15 * 100) / 100;
  const tagH = hasTagline ? Math.round(doc.taglineSize * 1.2 * 100) / 100 : 0;
  const textH = wordH + tagH;
  const textW = Math.max(doc.wordmarkWidth || 0, doc.taglineWidth || 0);
  const base = hasMark ? doc.markSize : doc.fontSize;
  const pad = Math.round(doc.clearSpace * base * 100) / 100;
  let contentW = 0, contentH = 0;
  switch (doc.layout) {
    case "horizontal": contentW = markSize + doc.gap + textW; contentH = Math.max(markSize, textH); break;
    case "stacked": contentW = Math.max(markSize, textW); contentH = markSize + doc.gap + textH; break;
    case "mark": contentW = contentH = markSize; break;
    case "wordmark": contentW = textW; contentH = textH; break;
    default: contentW = markSize + doc.gap + textW; contentH = Math.max(markSize, textH);
  }
  return { width: contentW + pad * 2, height: contentH + pad * 2, contentW, contentH, markSize, textW, textH, wordH, tagH, pad, hasMark, hasTagline };
}

function variantColors(doc, variant) {
  const ink = isHex(doc.color) ? doc.color : INK_HEX();
  const bg = doc.background || "transparent";
  if (variant === "inverse") {
    const light = luminance(ink) > 0.55 ? INK_HEX() : ON_BRAND_HEX();
    return { ink: light || ON_BRAND_HEX(), bg: ink };
  }
  if (variant === "mono") {
    if (bg === "transparent") return { ink: INK_HEX(), bg: "transparent" };
    return { ink: luminance(bg) < 0.45 ? ON_BRAND_HEX() : INK_HEX(), bg };
  }
  return { ink, bg };
}

function iconByName(name) {
  return L.icons.find((ic) => ic.name === name) || null;
}

// ---------------- 预览（HTML/CSS 锁排） ----------------

function markHTML(doc, ink, size) {
  if (doc.layout === "wordmark" || !doc.mark || doc.mark.kind === "none" || !doc.mark.icon) return "";
  const ic = iconByName(doc.mark.icon);
  if (!ic) {
    return `<span class="lgm-mark-hint" style="color:${ink};font-size:${Math.max(9, size * 0.28)}px">标记缺失</span>`;
  }
  // 用服务端返回的独立 <svg>（ic.svg）而不是页面精灵：新建的自定义图标
  // 不会进入当前页面的 icons.js 精灵，只有内联 SVG 才能立刻预览。
  return `<span class="lgm-markicon" style="width:${size}px;height:${size}px;font-size:${size}px;color:${ink}">${ic.svg}</span>`;
}

function textHTML(doc, ink, scale) {
  if (doc.layout === "mark" || !doc.brand) return "";
  const font = fontById(doc.font);
  const family = fontStack(font).replace(/"/g, "'");
  const brand = doc.casing === "upper" ? doc.brand.toUpperCase() : doc.brand;
  const fsize = doc.fontSize * scale;
  const tsize = doc.taglineSize * scale;
  const ls = doc.letterSpacing * scale;
  const gap = doc.tagline ? Math.max(2, doc.fontSize * 0.18) * scale : 0;
  const word = `<span class="lgm-word" style="font-family:${family};font-size:${fsize}px;` +
    `font-weight:${doc.weight};letter-spacing:${ls}px;color:${ink}">${esc(brand)}</span>`;
  const tag = doc.tagline
    ? `<span class="lgm-tag" style="font-family:${family};font-size:${tsize}px;font-weight:400;` +
      `letter-spacing:${ls * 0.6}px;color:${ink}">${esc(doc.tagline)}</span>` : "";
  return `<div class="lgm-wordmark" style="gap:${gap}px">${word}${tag}</div>`;
}

function logoPreviewHTML(doc, variant, scale, guide) {
  const m = logoMetrics(doc);
  if (!m) return "";
  const colors = variantColors(doc, variant);
  const size = m.markSize * scale;
  const gap = doc.layout === "horizontal" ? doc.gap * scale : (doc.layout === "stacked" ? doc.gap * scale : 0);
  const pad = m.pad * scale;
  const mark = markHTML(doc, colors.ink, size);
  const text = textHTML(doc, colors.ink, scale);
  const bg = colors.bg === "transparent" ? "transparent" : colors.bg;
  const content = doc.layout === "mark" ? mark : (doc.layout === "wordmark" ? text : mark + text);
  return `<div class="lgm-lockup lgm-lockup--${doc.layout}${guide ? " lgm-guide" : ""}" ` +
    `style="padding:${pad}px;background:${bg};color:${colors.ink}">` +
    `<div class="lgm-lockup__content" style="gap:${gap}px">${content}</div></div>`;
}

// ---------------- 弹层 ----------------

function isOpen() { return L.open; }

export async function openLogoManager(focusId) {
  if (L.open) return;
  L.open = true;
  L.mode = "empty";
  L.logos = [];
  L.selected = focusId || "";
  L.detail = null;
  L.doc = null;
  L.editing = null;
  L.name = "";
  L.note = "";
  L.system = "";
  L.dirty = false;
  L.asking = false;
  L.error = "";
  L.variant = "positive";
  L.guide = false;
  L.query = "";
  L.markPicker = false;
  $("logos-modal").classList.remove("hidden");
  const search = $("logos-search");
  if (search) search.value = "";
  await ensureRefs();
  await loadLogos(focusId);
  if (search) search.focus();
}

function closeLogoManager() {
  $("logos-modal").classList.add("hidden");
  L.markPicker = false;
  L.open = false;
  const btn = $("logos-open");
  if (btn) btn.focus();
}

async function ensureRefs() {
  if (L.refsLoaded) return;
  try {
    const [fonts, systems, icons] = await Promise.all([
      api("/api/catalog/fonts").catch(() => ({ fonts: [] })),
      api("/api/catalog/systems").catch(() => ({ systems: [] })),
      api("/api/catalog/icons").catch(() => ({ icons: [] })),
    ]);
    L.fonts = (fonts.fonts || []).filter((f) => f.commercial !== false);
    L.systems = systems.systems || [];
    L.icons = icons.icons || [];
    if (!L.fonts.length) {
      L.fonts = [{ id: "inter", family: "Inter", license: "OFL-1.1", commercial: true, weights: [400, 500, 600, 700] }];
    }
    if (!L.systems.length) L.systems = [];
    L.refsLoaded = true;
  } catch (e) {
    toast("Logo 参考数据加载失败: " + e.message, true);
    L.refsLoaded = true;
  }
}

async function refreshIcons() {
  try {
    const data = await api("/api/catalog/icons");
    L.icons = data.icons || [];
  } catch (_) { /* 保持旧列表 */ }
}

async function loadLogos(focusId) {
  try {
    const data = await api("/api/catalog/logos");
    L.logos = data.logos || [];
  } catch (e) {
    toast("Logo 列表加载失败: " + e.message, true);
    L.logos = [];
  }
  const id = focusId || L.selected;
  L.selected = L.logos.some((l) => l.id === id) ? id : (L.logos[0] ? L.logos[0].id : "");
  renderList();
  renderHead();
  if (!L.selected) { L.mode = "empty"; L.detail = null; renderStage(); renderPanel(); return; }
  await loadDetail(L.selected);
}

async function loadDetail(id) {
  try {
    const data = await api("/api/catalog/logos/" + encodeURIComponent(id));
    L.detail = data;
    L.doc = clone(data.doc);
    L.name = data.logo.name || "";
    L.note = data.logo.note || "";
    L.system = data.logo.system || "";
    L.mode = "detail";
    L.editing = null;
    L.dirty = false;
    applyMeasurements(L.doc);
    renderStage();
    renderPanel();
  } catch (e) {
    toast("Logo 详情加载失败: " + e.message, true);
    L.detail = null;
    L.doc = null;
    L.mode = "empty";
    renderStage();
    renderPanel();
  }
}

function renderHead() {
  const count = $("logos-count");
  if (count) count.textContent = String(L.logos.length);
}

function setError(msg) {
  L.error = msg || "";
  const el = $("logo-error");
  if (el) el.textContent = L.error;
  if (msg) toast(msg, true);
}

// ---------------- 列表 ----------------

function filteredLogos() {
  const q = L.query.toLowerCase();
  if (!q) return L.logos;
  return L.logos.filter((l) =>
    [l.name, l.brand, l.system, l.layout, l.note].some((v) => String(v || "").toLowerCase().includes(q)));
}

function renderList() {
  const box = $("logo-rows");
  if (!box) return;
  const rows = filteredLogos();
  if (!rows.length) {
    const hasAny = L.logos.length > 0;
    box.innerHTML = `<div class="lgm-empty">${icon("star", 24)}` +
      `<b>${hasAny ? "没有匹配的 Logo" : "还没有 Logo"}</b>` +
      `<p>${hasAny ? "换一个名称/品牌/体系关键词，或清空搜索。" : "新建一个品牌标识：选标记图标、写字标，自动生成正形/反形/单色三变体与安全空间规范。"}</p>` +
      `<button class="btn" data-lg-act="new">${icon("plus", 14)}新建 Logo</button></div>`;
    return;
  }
  box.innerHTML = rows.map((l) => {
    const cls = `lgm-row${l.id === L.selected ? " on" : ""}`;
    const when = String(l.updatedAt || "").slice(0, 10);
    return `<div class="${cls}" data-logo="${esc(l.id)}" role="button" tabindex="-1">` +
      `<span class="lgm-thumb">${l.svg || ""}</span>` +
      `<div class="lgm-row-main"><span class="lgm-row-name">${esc(l.name || l.id)}</span>` +
      `<span class="lgm-row-meta">${esc(l.layout || "")}${l.system ? " · " + esc(l.system) : ""}` +
      `${when ? " · " + esc(when) : ""}</span></div></div>`;
  }).join("");
}

// ---------------- 预览区 ----------------

function tabsHTML() {
  return `<div class="lgm-tabs" role="tablist" aria-label="Logo 变体">` +
    VARIANT_ORDER.map((v) =>
      `<button class="lgm-tab${L.variant === v ? " on" : ""}" data-lg-variant="${v}" ` +
      `role="tab" aria-selected="${L.variant === v}">${VARIANT_LABELS[v]}</button>`).join("") +
    `</div>`;
}

function detailSizesHTML() {
  if (!L.detail) return "";
  const spec = L.detail.spec || {};
  const v = L.detail.variants[L.variant] || "";
  const ratio = (L.detail.width || 1) / (L.detail.height || 1);
  return [64, 40, 24].map((h) => {
    const w = Math.round(h * ratio);
    const warn = spec.minWidth && w < spec.minWidth;
    return `<div class="lgm-size${warn ? " lgm-size--warn" : ""}" ` +
      `title="${h}px 高 · 宽约 ${w}px${warn ? "（低于最小宽度 " + spec.minWidth + "px）" : ""}">` +
      `<span style="height:${h}px">${v}</span><small>${h}px</small></div>`;
  }).join("");
}

function editSizesHTML(doc) {
  const m = logoMetrics(doc);
  if (!m) return "";
  return [64, 40, 24].map((h) => {
    const k = Math.min(h / Math.max(1, m.height), 150 / Math.max(1, m.width));
    return `<div class="lgm-size" title="${h}px 高 · 宽约 ${Math.round(m.width * k)}px">` +
      `${logoPreviewHTML(doc, L.variant, k, false)}<small>${h}px</small></div>`;
  }).join("");
}

function fitScale(m) {
  const el = stage();
  const width = Math.max(240, (el ? el.clientWidth : 620) - 72);
  const height = Math.max(140, (el ? el.clientHeight : 420) - 150);
  if (!m.width || !m.height) return 1;
  return Math.max(0.2, Math.min(1.5, width / m.width, height / m.height));
}

function renderStage() {
  const box = stage();
  if (!box) return;
  if (L.mode === "empty") {
    box.innerHTML = `<div class="lgm-empty">${icon("star", 28)}` +
      `<b>设计第一个品牌 Logo</b>` +
      `<p>从图标库选标记（或去图标设计器画一个），写上品牌名，选横排/竖排；保存后自动生成三变体、安全空间与最小尺寸规范。</p>` +
      `<button class="btn btn--primary" data-lg-act="new">${icon("plus", 14)}新建 Logo</button></div>`;
    return;
  }
  if (L.mode === "detail" && L.detail) {
    const v = L.detail.variants[L.variant] || L.detail.variants.positive || "";
    const spec = L.detail.spec || {};
    box.innerHTML = `<div class="lgm-stage-head">${tabsHTML()}<span class="grow"></span>` +
      `<span class="lgm-stage-hint">${L.detail.width}x${L.detail.height}px · 最小宽 ${spec.minWidth || "—"}px</span></div>` +
      `<div class="lgm-canvas">${v}</div>` +
      `<div class="lgm-sizes">${detailSizesHTML()}</div>` +
      `<div class="lgm-stage-hint">切换变体查看反形/单色；上方预览为服务端确定性 SVG。</div>`;
    return;
  }
  if (L.mode === "edit" && L.doc) {
    applyMeasurements(L.doc);
    const m = logoMetrics(L.doc);
    const k = fitScale(m);
    box.innerHTML = `<div class="lgm-stage-head">${tabsHTML()}<span class="grow"></span>` +
      `<button class="lgm-mini" data-lg-act="guide">${L.guide ? "隐藏安全空间" : "显示安全空间"}</button></div>` +
      `<div class="lgm-canvas">${logoPreviewHTML(L.doc, L.variant, k, L.guide)}</div>` +
      `<div class="lgm-sizes">${editSizesHTML(L.doc)}</div>` +
      `<div class="lgm-stage-hint">编辑预览按实际字宽锁排；保存后服务端按同一字宽生成 SVG。` +
      `${L.guide ? " 蓝色虚线为安全空间。" : ""}</div>`;
  }
}

// ---------------- 检查器 / 表单 ----------------

function layoutOptions(current) {
  return LAYOUTS.map(([id, label]) =>
    `<option value="${id}"${id === current ? " selected" : ""}>${esc(label)}</option>`).join("");
}

function systemOptions(current) {
  if (!L.systems.length) return `<option value="">（无可用体系）</option>`;
  return `<option value=""${current ? "" : " selected"}>不绑定体系</option>` +
    L.systems.map((s) => `<option value="${esc(s.id)}"${s.id === current ? " selected" : ""}>` +
      `${esc(s.name)}${s.primary ? " · " + esc(s.primary) : ""}</option>`).join("");
}

function fontOptions(current) {
  return L.fonts.map((f) => `<option value="${esc(f.id)}"${f.id === current ? " selected" : ""}>` +
    `${esc(f.family)} · ${esc(f.license)}${f.embeddable ? "" : "（不可嵌入）"}</option>`).join("");
}

function weightSelect(doc) {
  const opts = weightOptions(fontById(doc.font));
  return `<select class="lgm-input" data-lg="weight" aria-label="字重">` +
    opts.map((w) => `<option value="${w}"${w === doc.weight ? " selected" : ""}>${w}</option>`).join("") + `</select>`;
}

function numField(label, field, value, step) {
  return `<label class="lgm-field">${label}<input class="lgm-num" type="number" step="${step || 0.5}" ` +
    `data-lg="${field}" value="${Number(value || 0)}"></label>`;
}

function markSection(doc) {
  if (doc.layout === "wordmark") {
    return `<div class="lgm-note">${icon("info", 14)}<span>纯字标布局不使用标记；切回横排/竖排即可选择标记。</span></div>`;
  }
  const ic = iconByName(doc.mark && doc.mark.icon);
  return `<div class="lgm-mark"><span class="lgm-mark-glyph">${ic ? ic.svg : icon("question", 20)}</span>` +
    `<div class="lgm-mark-main"><span class="lgm-mark-name">${esc((doc.mark && doc.mark.icon) || "未选择")}</span>` +
    `<span class="lgm-mark-hint">${ic ? esc(ic.group) + " · " + esc((ic.tags || []).slice(0, 2).join(" / ") || "—") : "标记缺失"}</span></div>` +
    `<button class="lgm-mini" data-lg-act="pick-mark">选择</button></div>` +
    `<div class="lgm-acts"><button class="lgm-mini" data-lg-act="pick-mark">${icon("grid", 12)}从图标库选择</button>` +
    `<button class="lgm-mini" data-lg-act="draw-mark">${icon("edit", 12)}画新标记</button></div>`;
}

function renderPanel() {
  const box = pane();
  if (!box) return;
  if (L.mode === "empty") { box.innerHTML = ""; return; }
  if (L.mode === "detail" && L.detail) return renderDetailPanel();
  if (L.mode === "edit" && L.doc) return renderEditPanel();
}

function renderDetailPanel() {
  const l = L.detail.logo;
  const spec = L.detail.spec || {};
  const doc = L.detail.doc;
  const font = fontById(doc.font);
  const variantSvg = L.detail.variants[L.variant] || "";
  const warnings = (spec.warnings || []).map((w) => `<div class="lgm-warn">${icon("warning", 14)}<span>${esc(w)}</span></div>`).join("");
  pane().innerHTML =
    `<section class="lgm-sec"><div class="lgm-sec-head">资产</div>
      <div class="lgm-kv"><span class="k">名称</span><span class="v">${esc(l.name || l.id)}</span></div>
      <div class="lgm-kv"><span class="k">品牌 / 副标</span><span class="v">${esc(doc.brand || "—")}${doc.tagline ? " · " + esc(doc.tagline) : ""}</span></div>
      <div class="lgm-kv"><span class="k">体系 / 布局</span><span class="v">${esc(l.system || "—")} · ${esc(doc.layout)}</span></div>
      ${l.note ? `<div class="lgm-kv"><span class="k">备注</span><span class="v">${esc(l.note)}</span></div>` : ""}
      <div class="lgm-acts"><button class="lgm-mini" data-lg-act="edit">${icon("edit", 12)}编辑设计</button>
      <button class="lgm-mini lgm-mini--danger" data-lg-act="remove">${icon("trash", 12)}删除</button></div>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">交付规范 <span class="chip">${L.detail.width}x${L.detail.height}</span></div>
      <div class="lgm-spec">
        <div class="lgm-kv"><span class="k">最小宽度</span><span class="v">${spec.minWidth}px</span></div>
        <div class="lgm-kv"><span class="k">最小高度</span><span class="v">${spec.minHeight}px</span></div>
        <div class="lgm-kv"><span class="k">安全空间</span><span class="v">${spec.clearSpace}px</span></div>
        <div class="lgm-kv"><span class="k">变体</span><span class="v">${(spec.variants || []).join(" / ")}</span></div>
      </div>
      <div class="lgm-note">${icon("info", 14)}<span>${esc(spec.clearSpaceRule || "")}</span></div>
      <div class="lgm-kv"><span class="k">字体授权</span><span class="v">${esc(spec.fontLicense || (font ? font.license : "—"))}</span></div>
      ${warnings}
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">导出与摆放</div>
      <div class="lgm-acts">
        <button class="lgm-mini" data-lg-act="export-svg">${icon("download", 12)}SVG</button>
        <button class="lgm-mini" data-lg-act="export-png">${icon("image", 12)}PNG 4x</button>
        <button class="lgm-mini" data-lg-act="copy-svg">${icon("copy", 12)}复制 SVG</button>
        <button class="lgm-mini" data-lg-act="place">${icon("plus", 12)}摆上画布</button>
      </div>
      <div class="lgm-note">${icon("warning", 14)}<span>SVG 的字标仍是文本；生产交付前转曲，或确认字体可嵌入/已授权。</span></div>
      <div class="lgm-stage-hint">${(spec.usage || []).map((u) => "· " + esc(u)).join("<br>")}</div>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">SVG 源码 <span class="chip">${VARIANT_LABELS[L.variant]}</span></div>
      <pre class="lgm-src">${esc(variantSvg)}</pre>
    </section>`;
}

function renderEditPanel() {
  const doc = L.doc;
  const font = fontById(doc.font);
  const sys = currentSystem();
  const bgTransparent = doc.background === "transparent";
  const colorValue = isHex(doc.color) ? doc.color : INK_HEX();
  const bgValue = bgTransparent ? (SURFACE_HEX() || colorValue) : doc.background;
  const fontLicense = font ? `${font.license} · 可商用=${font.commercial ? "是" : "否"} · 可嵌入=${font.embeddable ? "是" : "否"}` : "—";
  const localWarn = [];
  const posContrast = contrast(doc.color, bgTransparent ? (SURFACE_HEX() || doc.color) : doc.background);
  if (posContrast < 3) localWarn.push(`正形对比度 ${posContrast.toFixed(2)} < 3.0，考虑换品牌色/底色`);
  if (!doc.brand && doc.layout !== "mark") localWarn.push("字标为空，组合布局只剩标记");
  if (!isHex(doc.color)) localWarn.push("品牌色不是 #RRGGBB");

  pane().innerHTML =
    `<section class="lgm-sec"><div class="lgm-sec-head">资产</div>
      <label class="lgm-field">资产名称<input class="lgm-input" data-lg="name" maxlength="80" value="${esc(L.name)}" placeholder="默认取品牌名"></label>
      <label class="lgm-field">备注<input class="lgm-input" data-lg="note" maxlength="240" value="${esc(L.note)}" placeholder="用途 / 版本说明"></label>
      <label class="lgm-field">关联设计体系<select class="lgm-input" data-lg="system">${systemOptions(L.system)}</select></label>
      <button class="lgm-mini" data-lg-act="apply-system">${icon("theme", 12)}应用体系主色${sys && sys.primary ? "（" + esc(sys.primary) + "）" : ""}</button>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">品牌文字</div>
      <label class="lgm-field">品牌名<input class="lgm-input" data-lg="brand" maxlength="64" value="${esc(doc.brand)}" placeholder="Acme Cloud"></label>
      <label class="lgm-field">副标（可选）<input class="lgm-input" data-lg="tagline" maxlength="120" value="${esc(doc.tagline)}" placeholder="design system"></label>
      <label class="lgm-field">布局<select class="lgm-input" data-lg="layout">${layoutOptions(doc.layout)}</select></label>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">标记</div>${markSection(doc)}</section>
    <section class="lgm-sec"><div class="lgm-sec-head">字体</div>
      <label class="lgm-field">字体<select class="lgm-input" data-lg="font">${fontOptions(doc.font)}</select></label>
      <div class="lgm-note">${icon("info", 14)}<span>${esc(fontLicense)}</span></div>
      <div class="lgm-grid2">
        <label class="lgm-field">字重${weightSelect(doc)}</label>
        <label class="lgm-field">大小写<select class="lgm-input" data-lg="casing">
          <option value="none"${doc.casing !== "upper" ? " selected" : ""}>保持原样</option>
          <option value="upper"${doc.casing === "upper" ? " selected" : ""}>全大写</option>
        </select></label>
        ${numField("字号", "fontSize", doc.fontSize, 1)}
        ${numField("字距", "letterSpacing", doc.letterSpacing, 0.5)}
        ${numField("副标字号", "taglineSize", doc.taglineSize, 1)}
      </div>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">组合与留白</div>
      <div class="lgm-grid2">
        ${numField("标记尺寸", "markSize", doc.markSize, 1)}
        ${numField("标记间距", "gap", doc.gap, 1)}
        ${numField("安全空间倍数", "clearSpace", doc.clearSpace, 0.1)}
      </div>
    </section>
    <section class="lgm-sec"><div class="lgm-sec-head">颜色与底色</div>
      <div class="lgm-row-inline"><label class="lgm-field grow">品牌色<input type="color" class="lgm-color" data-lg="color" value="${colorValue}"></label>
        <span class="lgm-mark-hint">${esc(doc.color)}</span></div>
      <label class="lgm-check"><input type="checkbox" data-lg="transparent"${bgTransparent ? " checked" : ""}>透明底色</label>
      ${bgTransparent ? "" : `<div class="lgm-row-inline"><label class="lgm-field grow">底色<input type="color" class="lgm-color" data-lg="background" value="${bgValue}"></label>
        <span class="lgm-mark-hint">${esc(doc.background)}</span></div>`}
      ${localWarn.length ? `<div class="lgm-warn">${icon("warning", 14)}<span>${esc(localWarn.join("；"))}</span></div>` : ""}
    </section>
    <div class="lgm-err" id="logo-error" role="alert">${esc(L.error)}</div>
    <div class="lgm-foot"><span class="lgm-stage-hint">${L.dirty ? "未保存" : "已保存"}</span><span class="grow"></span>
      <button class="btn" data-lg-act="cancel">取消</button>
      <button class="btn btn--primary" data-lg-act="save" id="logo-save">${L.editing ? "保存修改" : "保存 Logo"}</button>
    </div>`;
}

// ---------------- 干活 ----------------

async function startCreate() {
  L.mode = "edit";
  L.editing = null;
  L.detail = null;
  L.name = "";
  L.note = "";
  L.system = currentSystem() ? currentSystem().id : "";
  L.doc = defaultDoc();
  L.dirty = false;
  L.error = "";
  applyMeasurements(L.doc);
  renderStage();
  renderPanel();
}

function startEdit() {
  if (!L.detail) return;
  L.mode = "edit";
  L.editing = L.detail.logo.id;
  L.doc = clone(L.detail.doc);
  L.name = L.detail.logo.name || "";
  L.note = L.detail.logo.note || "";
  L.system = L.detail.logo.system || "";
  L.dirty = false;
  L.error = "";
  applyMeasurements(L.doc);
  renderStage();
  renderPanel();
}

async function confirmLeaveEdit() {
  if (!(L.mode === "edit" && L.dirty)) return true;
  if (L.asking) return false;
  L.asking = true;
  let ok = false;
  try {
    ok = await uiConfirm({
      title: "放弃未保存的 Logo 改动？",
      note: "当前设计稿还没有保存，离开编辑后这一版会丢失。",
      okText: "放弃修改",
      danger: true,
    });
  } finally {
    L.asking = false;
  }
  return ok;
}

async function cancelEdit() {
  if (!(await confirmLeaveEdit())) return;
  L.dirty = false;
  if (L.editing) {
    await loadDetail(L.editing);
  } else if (L.selected) {
    await loadDetail(L.selected);
  } else {
    L.mode = "empty"; L.doc = null; L.detail = null;
    renderStage(); renderPanel();
  }
}

async function save() {
  if (L.saving || !L.doc) return;
  L.error = "";
  applyMeasurements(L.doc);
  const name = (L.name || L.doc.brand || "").trim();
  if (!name) { setError("资产名称或品牌名至少填一个"); return; }
  if (L.doc.layout !== "mark" && !L.doc.brand) { setError("组合布局需要品牌名；纯标记布局可留空"); return; }
  L.saving = true;
  const btn = $("logo-save");
  if (btn) btn.disabled = true;
  const body = {
    name, note: L.note, system: L.system,
    doc: L.doc,
  };
  try {
    let data;
    if (L.editing) {
      data = await api("/api/catalog/logos/" + encodeURIComponent(L.editing), {
        method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
    } else {
      data = await api("/api/catalog/logos", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
    }
    const id = (data.logo && data.logo.id) || L.editing || "";
    L.dirty = false;
    L.selected = id;
    toast("已保存 Logo「" + name + "」");
    await loadLogos(id);
  } catch (e) {
    setError((e.code ? e.code + "：" : "") + e.message);
  } finally {
    L.saving = false;
    if (btn) btn.disabled = false;
  }
}

async function removeSelected() {
  if (!L.detail) return;
  const id = L.detail.logo.id;
  const ok = await uiConfirm({
    title: "删除 Logo 资产",
    note: `将删除「${L.detail.logo.name}」的 logos/${id}.json；已摆上画布的节点不会回收。`,
    okText: "删除",
    danger: true,
  });
  if (!ok) return;
  try {
    await api("/api/catalog/logos/" + encodeURIComponent(id), { method: "DELETE" });
    toast("已删除 Logo " + id);
    L.selected = "";
    L.detail = null;
    await loadLogos("");
  } catch (e) { toast(e.message, true); }
}

function downloadBlob(name, blob) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url; a.download = name;
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}

async function exportSVG() {
  if (!L.detail) return;
  const id = L.detail.logo.id;
  try {
    const res = await fetch(`/api/catalog/logos/${encodeURIComponent(id)}/export?variant=${L.variant}`);
    if (!res.ok) throw new Error("导出失败 (" + res.status + ")");
    const blob = await res.blob();
    downloadBlob(`${id}-${L.variant}.svg`, blob);
    toast("已导出 SVG");
  } catch (e) { toast(e.message, true); }
}

async function exportPNG() {
  if (!L.detail) return;
  const svg = (L.detail.variants || {})[L.variant];
  if (!svg) { toast("当前变体没有可导出的 SVG", true); return; }
  try {
    await (document.fonts ? document.fonts.ready : Promise.resolve());
    const blob = new Blob([svg], { type: "image/svg+xml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const img = new Image();
    await new Promise((resolve, reject) => {
      img.onload = resolve;
      img.onerror = () => reject(new Error("SVG 光栅化失败"));
      img.src = url;
    });
    const scale = 4;
    const w = Math.max(1, Math.round((L.detail.width || img.width || 1) * scale));
    const h = Math.max(1, Math.round((L.detail.height || img.height || 1) * scale));
    const canvas = document.createElement("canvas");
    canvas.width = w; canvas.height = h;
    const ctx = canvas.getContext("2d");
    ctx.drawImage(img, 0, 0, w, h);
    URL.revokeObjectURL(url);
    canvas.toBlob((png) => {
      if (!png) { toast("PNG 生成失败", true); return; }
      downloadBlob(`${L.detail.logo.id}-${L.variant}@4x.png`, png);
      toast("已导出 PNG (4x)");
    }, "image/png");
  } catch (e) { toast(e.message, true); }
}

async function placeOnCanvas() {
  if (!L.detail) return;
  const project = S.project || (S.projects.length === 1 ? S.projects[0].id : "");
  if (!project) { toast("先在左侧项目树里选一个项目，再摆上画布", true); return; }
  try {
    const data = await api(`/api/catalog/logos/${encodeURIComponent(L.detail.logo.id)}/canvas?variant=${L.variant}`);
    await api("/api/nodes", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        project, title: "Logo · " + (L.detail.logo.name || L.detail.logo.id),
        kind: "component", html: data.html, width: data.width, height: data.height,
        tags: ["logo", "logo:" + L.detail.logo.id, "variant:" + L.variant],
      }),
    });
    toast("已把 Logo 摆上画布");
  } catch (e) { toast("摆放失败: " + e.message, true); }
}

async function copySVG() {
  if (!L.detail) return;
  const svg = (L.detail.variants || {})[L.variant] || "";
  try {
    await navigator.clipboard.writeText(svg);
    toast("已复制 " + VARIANT_LABELS[L.variant] + " SVG");
  } catch (_) { toast("复制失败：浏览器拒绝写剪贴板", true); }
}

// ---------------- 标记选择 ----------------

function openMarkPicker() {
  L.markPicker = true;
  L.markQuery = "";
  renderMarkPicker();
  const search = $("lgm-mark-search");
  if (search) search.focus();
}

function closeMarkPicker() {
  L.markPicker = false;
  const box = $("logo-markpick");
  if (box) box.classList.add("hidden");
}

function markGridHTML() {
  const q = L.markQuery.toLowerCase();
  const list = L.icons.filter((ic) => !ic.disabled &&
    (!q || ic.name.toLowerCase().includes(q) || String(ic.group || "").toLowerCase().includes(q) ||
      (ic.tags || []).some((t) => String(t).toLowerCase().includes(q)))).slice(0, 120);
  if (!list.length) {
    return `<div class="lgm-empty"><b>没有匹配的图标</b><p>换个名称/分组，或去图标设计器画一个新标记。</p></div>`;
  }
  return list.map((ic) =>
    `<button class="lgm-markcell" data-lg-mark="${esc(ic.name)}" title="${esc(ic.name)}" aria-label="${esc(ic.name)}">${ic.svg}</button>`).join("");
}

function renderMarkGrid() {
  const grid = $("lgm-mark-grid");
  if (grid) grid.innerHTML = markGridHTML();
  const count = $("lgm-mark-count");
  if (count) {
    const q = L.markQuery.toLowerCase();
    const n = L.icons.filter((ic) => !ic.disabled &&
      (!q || ic.name.toLowerCase().includes(q) || String(ic.group || "").toLowerCase().includes(q) ||
        (ic.tags || []).some((t) => String(t).toLowerCase().includes(q)))).length;
    count.textContent = `共 ${n} 个可用图标`;
  }
}

function renderMarkPicker() {
  const box = $("logo-markpick");
  if (!box) return;
  if (!L.markPicker) { box.classList.add("hidden"); return; }
  box.classList.remove("hidden");
  const list = L.icons.filter((ic) => !ic.disabled);
  box.innerHTML = `<div class="lgm-markpick-head">
      <label class="sysm-search grow">${icon("search", 14)}<input id="lgm-mark-search" type="search" placeholder="搜索图标名 / 分组 / 别名" value="${esc(L.markQuery)}"></label>
      <button class="modal__x" data-lg-act="close-markpick" aria-label="关闭标记选择">${icon("close", 14)}</button>
    </div>
    <div class="lgm-markpick-grid" id="lgm-mark-grid">${markGridHTML()}</div>
    <div class="lgm-markpick-foot"><span class="lgm-stage-hint" id="lgm-mark-count">共 ${list.length} 个可用图标</span><span class="grow"></span>
      <button class="lgm-mini" data-lg-act="draw-mark">${icon("edit", 12)}画新标记</button></div>`;
  renderMarkGrid();
}

function chooseMark(name) {
  if (!L.doc) return;
  if (!iconByName(name)) return;
  L.doc.mark = { kind: "icon", icon: name };
  if (L.doc.layout === "wordmark") L.doc.layout = "horizontal";
  L.dirty = true;
  closeMarkPicker();
  applyMeasurements(L.doc);
  renderStage(); renderPanel();
}

async function drawMark() {
  closeMarkPicker();
  await openIconDesigner({
    onSaved: async (name) => {
      if (!L.doc) return;
      await refreshIcons();
      L.doc.mark = { kind: "icon", icon: name };
      if (L.doc.layout === "wordmark") L.doc.layout = "horizontal";
      L.dirty = true;
      applyMeasurements(L.doc);
      renderStage(); renderPanel();
      toast("已把新标记接入 Logo：" + name);
    },
    onClosed: () => { /* 取消绘图：回到 Logo 工作台 */ },
  });
}

// ---------------- 事件 ----------------

function updateField(field, el) {
  if (!L.doc && field !== "name" && field !== "note" && field !== "system") return;
  if (field === "name") { L.name = el.value; L.dirty = true; return; }
  if (field === "note") { L.note = el.value; L.dirty = true; return; }
  if (field === "system") { L.system = el.value; L.dirty = true; return; }
  if (field === "brand") { L.doc.brand = el.value; }
  else if (field === "tagline") { L.doc.tagline = el.value; }
  else if (field === "layout") { L.doc.layout = el.value; }
  else if (field === "font") { L.doc.font = el.value; }
  else if (field === "weight") { L.doc.weight = Number(el.value); }
  else if (field === "casing") { L.doc.casing = el.value; }
  else if (field === "color") { L.doc.color = String(el.value || "").toLowerCase(); }
  else if (field === "background") { L.doc.background = String(el.value || "").toLowerCase(); }
  else if (field === "fontSize" || field === "markSize" || field === "gap" || field === "taglineSize" ||
    field === "letterSpacing" || field === "clearSpace") {
    const v = Number(el.value);
    if (Number.isFinite(v)) L.doc[field] = v;
  }
  L.dirty = true;
  applyMeasurements(L.doc);
}

$("logo-rows").addEventListener("click", async (ev) => {
  const newBtn = ev.target.closest('[data-lg-act="new"]');
  if (newBtn) {
    if (await confirmLeaveEdit()) startCreate();
    return;
  }
  const row = ev.target.closest("[data-logo]");
  if (!row) return;
  if (row.dataset.logo === L.selected && L.mode === "detail") return;
  if (!(await confirmLeaveEdit())) return;
  L.selected = row.dataset.logo;
  L.mode = "detail";
  L.detail = null;
  renderList();
  loadDetail(L.selected);
});

$("logos-search").addEventListener("input", (ev) => {
  L.query = ev.target.value.trim();
  renderList();
});

$("logo-new").addEventListener("click", async () => {
  if (await confirmLeaveEdit()) startCreate();
});

$("logo-stage").addEventListener("click", (ev) => {
  const tab = ev.target.closest("[data-lg-variant]");
  if (tab) { L.variant = tab.dataset.lgVariant; renderStage(); renderPanel(); return; }
  const act = ev.target.closest("[data-lg-act]");
  if (!act) return;
  if (act.dataset.lgAct === "guide") { L.guide = !L.guide; renderStage(); }
  if (act.dataset.lgAct === "new") {
    confirmLeaveEdit().then((ok) => { if (ok) startCreate(); });
  }
});

pane().addEventListener("input", (ev) => {
  const el = ev.target;
  if (el.dataset.lg === "transparent") return; // change 处理
  if (!el.dataset.lg) return;
  updateField(el.dataset.lg, el);
  renderStage();
});

pane().addEventListener("change", (ev) => {
  const el = ev.target;
  if (el.dataset.lg === "transparent") {
    if (el.checked) L.doc.background = "transparent";
    else L.doc.background = SURFACE_HEX() || L.doc.color;
    L.dirty = true;
    renderStage(); renderPanel();
    return;
  }
  const field = el.dataset.lg;
  if (!field) return;
  updateField(field, el);
  if (field === "font" || field === "layout" || field === "system") {
    if (field === "font") {
      const opts = weightOptions(fontById(L.doc.font));
      if (!opts.includes(L.doc.weight)) L.doc.weight = opts[opts.length - 1];
    }
    renderStage(); renderPanel();
    return;
  }
  renderStage();
});

pane().addEventListener("click", (ev) => {
  const act = ev.target.closest("[data-lg-act]");
  if (act) {
    switch (act.dataset.lgAct) {
      case "edit": startEdit(); break;
      case "remove": removeSelected(); break;
      case "cancel": cancelEdit(); break;
      case "save": save(); break;
      case "guide": L.guide = !L.guide; renderStage(); break;
      case "export-svg": exportSVG(); break;
      case "export-png": exportPNG(); break;
      case "copy-svg": copySVG(); break;
      case "place": placeOnCanvas(); break;
      case "pick-mark": openMarkPicker(); break;
      case "draw-mark": drawMark(); break;
      case "apply-system": {
        const sys = currentSystem();
        if (!sys) { toast("没有可用的设计体系", true); break; }
        if (isHex(sys.primary)) { L.doc.color = sys.primary; L.dirty = true; }
        renderStage(); renderPanel();
        break;
      }
      case "new": confirmLeaveEdit().then((ok) => { if (ok) startCreate(); }); break;
    }
    return;
  }
  const mark = ev.target.closest("[data-lg-mark]");
  if (mark) { chooseMark(mark.dataset.lgMark); return; }
});

$("logo-markpick").addEventListener("click", (ev) => {
  const mark = ev.target.closest("[data-lg-mark]");
  if (mark) { chooseMark(mark.dataset.lgMark); return; }
  const act = ev.target.closest("[data-lg-act]");
  if (!act) return;
  if (act.dataset.lgAct === "close-markpick") closeMarkPicker();
  if (act.dataset.lgAct === "draw-mark") drawMark();
});

$("logo-markpick").addEventListener("input", (ev) => {
  if (ev.target.id !== "lgm-mark-search") return;
  L.markQuery = ev.target.value.trim();
  renderMarkGrid();
});

$("logos-modal").addEventListener("click", (ev) => {
  if (ev.target.closest("[data-logos-close]")) { requestClose(); return; }
});

async function requestClose() {
  if (L.asking || L.saving) return;
  if (L.mode === "edit" && L.dirty) {
    if (L.markPicker) { closeMarkPicker(); return; }
    L.asking = true;
    let ok = false;
    try {
      ok = await uiConfirm({
        title: "放弃未保存的 Logo 改动？",
        note: "当前设计稿还没有保存，关闭工作台后这一版会丢失。",
        okText: "放弃修改",
        danger: true,
      });
    } finally { L.asking = false; }
    if (!ok) return;
  }
  closeLogoManager();
}

$("logos-modal").addEventListener("keydown", (e) => {
  if (e.key !== "Tab") return;
  const list = [...$("logos-modal").querySelectorAll("button,input,select,textarea")]
    .filter((el) => !el.disabled && el.offsetParent !== null);
  if (!list.length) return;
  const first = list[0], last = list[list.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
});

window.addEventListener("keydown", (e) => {
  if (!isOpen()) return;
  if (e.key === "Escape") {
    if (L.markPicker) {
      e.preventDefault(); e.stopImmediatePropagation();
      closeMarkPicker();
      return;
    }
    if (L.asking) return;
    e.preventDefault(); e.stopImmediatePropagation();
    requestClose();
    return;
  }
  const tag = (e.target && e.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  if (e.key === "Delete" || e.key === "Backspace" || ((e.ctrlKey || e.metaKey) && (e.key === "z" || e.key === "Z"))) {
    // Logo 工作台里这些键不作用于画布节点 / 画布历史。
    e.stopImmediatePropagation();
  }
}, true);

$("logos-open").addEventListener("click", () => openLogoManager());

export function isLogoManagerOpen() { return L.open; }

export { closeLogoManager };
