/* SVG 图标库管理：搜索 + 过滤 + 列表 + 详情/表单（两栏 960，与体系管理同壳）。
 *
 * 与 MCP 的 design_icon_manage 共用同一套 HTTP 管理端点。
 * 内置图标：可停用、可改分组/别名、可恢复默认；SVG 只读、不可删除。
 * 自定义图标：可粘贴 SVG 新增、编辑分组/别名/SVG、移除。
 * 停用只影响选择面（design_icons / 检索 / 目录）；精灵仍保留，旧模板引用不会变空白。
 */
"use strict";

import { $ } from "./state.js";
import { esc, toast, uiConfirm } from "./ui.js";
import { api } from "./net.js";
import { icon } from "./icons-bridge.js";
import { openIconDesigner } from "./icon-designer.js";

const RIGHT = () => $("icons-right");
const GROUPS = [
  ["arrow", "方向与层级"],
  ["action", "动作与工具"],
  ["status", "状态与反馈"],
  ["nav", "导航入口"],
  ["media", "内容互动"],
  ["data", "列表与数据"],
];
// 与服务端 parseIconSVG 的允许清单保持一致（前端只做即时预览拦截，最终以服务端为准）。
const ICON_TAGS = new Set([
  "path", "circle", "rect", "line", "polyline", "polygon", "ellipse",
  "g", "defs", "lineargradient", "radialgradient", "stop", "clippath",
]);
const NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

let all = [];
let filter = "all";
let query = "";
let selected = "";
let mode = "detail"; // detail | create | edit
let detail = null;
let detailSeq = 0;
let previewTimer = 0;

function isIconManagerOpen() {
  const box = $("icons-modal");
  return !!box && !box.classList.contains("hidden");
}

// focusName 可选：设计器保存/返回后用它把列表定位回刚编辑的图标。
function openIconManager(focusName) {
  selected = focusName || "";
  detail = null;
  mode = "detail";
  $("icons-modal").classList.remove("hidden");
  $("icons-search").value = "";
  query = "";
  setFilter("all");
  loadAll().then(() => {
    scrollSelectedIntoView();
    $("icons-search").focus();
  });
}

// 列表容器只换 innerHTML，scrollTop 会保留：打开/键盘换行后把选中行带回视野。
function scrollSelectedIntoView() {
  const el = $("icons-rows").querySelector(".icm-row.on");
  if (el) el.scrollIntoView({ block: "nearest" });
}

function closeIconManager() {
  $("icons-modal").classList.add("hidden");
  const btn = $("icons-open");
  if (btn) btn.focus();
}

// ---------------- 数据 ----------------

async function loadAll() {
  try {
    const data = await api("/api/catalog/icons");
    all = data.icons || [];
  } catch (e) {
    toast("图标列表加载失败: " + e.message, true);
    return;
  }
  $("icons-count").textContent = String(all.length);
  updateCounts();
  const rows = visibleRows();
  if (mode === "detail" && !rows.some((ic) => ic.name === selected)) {
    selected = rows.length ? rows[0].name : "";
    detail = null;
  }
  renderList();
  if (mode === "detail") {
    if (selected) await loadDetail(selected);
    else renderRight();
  } else {
    renderRight();
  }
}

function visibleRows() {
  let list = all;
  if (filter === "builtin") list = list.filter((ic) => ic.builtin);
  else if (filter === "custom") list = list.filter((ic) => !ic.builtin);
  else if (filter === "disabled") list = list.filter((ic) => ic.disabled);
  if (query) {
    const q = query.toLowerCase();
    list = list.filter((ic) =>
      [ic.name, ic.group, ...(ic.tags || [])].some((v) => String(v || "").toLowerCase().includes(q)));
  }
  return list;
}

function updateCounts() {
  $("icons-cnt-all").textContent = String(all.length);
  $("icons-cnt-builtin").textContent = String(all.filter((ic) => ic.builtin).length);
  $("icons-cnt-custom").textContent = String(all.filter((ic) => !ic.builtin).length);
  $("icons-cnt-disabled").textContent = String(all.filter((ic) => ic.disabled).length);
}

function setFilter(next) {
  filter = next;
  document.querySelectorAll("#icons-segs .sysm-seg").forEach((b) => {
    b.classList.toggle("on", b.dataset.filter === next);
  });
}

function refreshView() {
  updateCounts();
  const rows = visibleRows();
  if (mode === "detail" && !rows.some((ic) => ic.name === selected)) {
    selected = rows.length ? rows[0].name : "";
    detail = null;
  }
  renderList();
  if (mode === "detail") {
    if (selected) loadDetail(selected);
    else renderRight();
  }
}

async function loadDetail(name) {
  const seq = ++detailSeq;
  if (!detail || detail.name !== name) {
    RIGHT().innerHTML = `<div class="icm-empty"><span>加载中…</span></div>`;
  }
  try {
    const data = await api("/api/catalog/icons/" + encodeURIComponent(name));
    if (seq !== detailSeq || selected !== name || mode !== "detail") return;
    detail = data.icon;
    renderRight();
  } catch (e) {
    if (seq !== detailSeq) return;
    toast("图标详情加载失败: " + e.message, true);
    selected = "";
    detail = null;
    renderRight();
  }
}

// ---------------- 列表 ----------------

function chipsOf(ic) {
  const out = [`<span class="chip">${ic.builtin ? "内置" : "自定义"}</span>`];
  if (ic.disabled) out.push(`<span class="chip chip--warn">已停用</span>`);
  else if (ic.overridden) out.push(`<span class="chip">已改分组/别名</span>`);
  if (ic.designed) out.push(`<span class="chip">可再设计</span>`);
  return out.join("");
}

function renderList() {
  const rows = visibleRows();
  const box = $("icons-rows");
  if (!rows.length) {
    box.innerHTML = `<div class="icm-empty">${icon("search", 24)}` +
      `<b>没有匹配的图标</b>` +
      `<span>试试别名（close / 搜索 / 爱心）或分组名（arrow / action）。</span></div>`;
    return;
  }
  box.innerHTML = rows.map((ic) => {
    const cls = ["sysm-row", "icm-row"];
    if (ic.name === selected) cls.push("on");
    if (ic.disabled) cls.push("off");
    const tags = (ic.tags || []).slice(0, 3).join(" / ");
    return `<div class="${cls.join(" ")}" data-name="${esc(ic.name)}" role="button" tabindex="-1">` +
      `<span class="icm-glyph">${ic.svg}</span>` +
      `<div class="sysm-main"><div class="sysm-name icm-name">${esc(ic.name)} ${chipsOf(ic)}</div>` +
      `<div class="sysm-meta"><span>${esc(ic.group)}</span><span>·</span>` +
      `<span>${esc(tags || "—")}</span></div></div>` +
      `<button class="sysm-sw${ic.disabled ? "" : " on"}" data-switch="${esc(ic.name)}" ` +
      `title="${ic.disabled ? "启用" : "停用"}" aria-label="${ic.disabled ? "启用" : "停用"} ${esc(ic.name)}" ` +
      `aria-pressed="${ic.disabled ? "false" : "true"}"></button></div>`;
  }).join("");
}

$("icons-rows").addEventListener("click", (ev) => {
  const sw = ev.target.closest("[data-switch]");
  if (sw) {
    ev.stopPropagation();
    toggleIcon(sw.dataset.switch);
    return;
  }
  const row = ev.target.closest(".icm-row");
  if (!row) return;
  if (row.dataset.name === selected && mode === "detail") return;
  selected = row.dataset.name;
  mode = "detail";
  detail = null;
  renderList();
  loadDetail(selected);
});

// ---------------- 详情 ----------------

function renderRight() {
  if (mode === "create" || mode === "edit") return renderForm();
  if (!selected) return renderNoSelection();
  if (detail && detail.name === selected) return renderDetail(detail);
  RIGHT().innerHTML = `<div class="icm-empty"><span>加载中…</span></div>`;
}

function renderNoSelection() {
  RIGHT().innerHTML = `<div class="icm-empty">${icon("grid", 24)}` +
    `<b>图标是检索对象，也是设计对象</b>` +
    `<p>用名称、别名或分组搜到图标后，右侧显示 12–24 尺寸档、两种用法、元信息与 SVG 源码；` +
    `自定义图标可用设计器画图形，内置图标可「以它为起点」另存。</p>` +
    `<span class="icm-empty-acts">` +
    `<button class="btn" data-act="design-new">${icon("edit", 14)}设计图标</button>` +
    `<button class="btn icm-copy" data-act="new">${icon("plus", 14)}粘贴 SVG</button></span></div>`;
}

function renderDetail(d) {
  const designLabel = d.builtin ? "以它为起点设计" : (d.designed ? "设计器编辑" : "用设计器改写");
  const actions = `<button class="btn" data-act="design">${icon("edit", 14)}${designLabel}</button>` +
    `<button class="btn" data-act="edit">${icon("settings", 14)}` +
    `${d.builtin ? "编辑分组与别名" : "编辑 SVG"}</button>`;
  const sizes = [12, 14, 16, 20, 24].map((s) =>
    `<div class="icm-size"><span class="icm-size-glyph" style="width:${s}px;height:${s}px">${d.svg}</span>` +
    `<b>${s}</b></div>`).join("");
  const tags = (d.tags || []).join(" / ") || "—";
  const source = String(d.source || "");
  const inline = source.replace(/\s+/g, " ").trim();
  const foot = d.builtin
    ? `<button class="btn" data-act="reset"${(d.overridden || d.disabled) ? "" : " disabled"}>恢复默认</button>` +
      `<button class="btn" data-act="done">完成</button>`
    : `<button class="btn btn--danger" data-act="remove">${icon("trash", 14)}删除图标</button>` +
      `<button class="btn" data-act="edit">${icon("settings", 14)}编辑 SVG</button>` +
      `<button class="btn" data-act="done">完成</button>`;

  RIGHT().innerHTML = `<div class="sysm-det">` +
    `<header class="sysm-det-head">` +
    `<span class="icm-hero">${d.svg}</span>` +
    `<div class="sysm-det-title"><div class="sysm-det-name icm-name">${esc(d.name)} ` +
    `<span class="chip">${d.builtin ? "内置" : "自定义"}</span>` +
    (d.disabled ? `<span class="chip chip--warn">已停用</span>` : `<span class="chip chip--ok">已启用</span>`) +
    (d.overridden ? `<span class="chip">已改分组/别名</span>` : "") +
    (d.designed ? `<span class="chip">可再设计</span>` : "") +
    `</div><div class="sysm-det-sub"><span>${esc(d.group)}</span><span>·</span>` +
    `<span>别名 ${esc(tags)}</span><span>·</span><span class="mono">${esc(d.viewBox || "0 0 24 24")}</span>` +
    `</div></div><div class="sysm-acts">${actions}</div></header>` +
    `<div class="sysm-body-pad">` +
    (d.disabled ? `<div class="sysm-banner">${icon("warning", 14)}` +
      `<span class="grow">已停用 · design_icons 与检索不再返回它；预览精灵与已有模板仍保留。</span>` +
      `<button class="btn" data-act="enable">启用</button></div>` : "") +
    `<div class="sysm-sec">尺寸档 <span class="line"></span>` +
    `<span class="sysm-hint">12 / 14 / 16 / 20 / 24 光学尺寸</span></div>` +
    `<div class="icm-sizes">${sizes}</div>` +
    `<div class="sysm-sec">用法 <span class="line"></span></div>` +
    `<div class="icm-uses">` +
    `<div class="icm-use"><code title="${esc(d.usage || "")}">${esc(d.usage || "")}</code>` +
    `<button class="btn icm-copy" data-act="copy-usage">复制</button></div>` +
    `<div class="icm-use"><code title="${esc(inline)}">${esc(inline)}</code>` +
    `<button class="btn icm-copy" data-act="copy-source">复制独立 SVG</button></div>` +
    `</div>` +
    `<div class="sysm-sec">元信息 <span class="line"></span></div>` +
    `<div class="sysm-kv-grid">` +
    `<div class="sysm-kv"><span class="k">分组</span><span class="v">${esc(d.group)}</span></div>` +
    `<div class="sysm-kv"><span class="k">别名</span><span class="v">${esc(tags)}</span></div>` +
    `<div class="sysm-kv"><span class="k">来源</span><span class="v">` +
    `${d.builtin ? "内置 · icon_paths.go" : "custom-icons/" + esc(d.name) + ".json" +
      (d.designed ? "（含 design）" : "")}</span></div>` +
    `<div class="sysm-kv"><span class="k">viewBox</span><span class="v mono">${esc(d.viewBox || "0 0 24 24")}</span></div>` +
    `</div>` +
    `<div class="sysm-sec">SVG 源码 <span class="line"></span>` +
    `<span class="chip">${d.builtin ? "只读" : "可编辑"}</span></div>` +
    `<pre class="icm-src">${esc(source)}</pre>` +
    `</div>` +
    `<footer class="sysm-foot"><span class="sysm-hint">${d.builtin
      ? "内置图标 SVG 只读 · 可改分组/别名、停用"
      : "自定义图标可改 SVG / 分组 / 别名，可移除"}</span>` +
    `<span class="grow"></span>${foot}</footer></div>`;
}

// ---------------- 新增 / 编辑 ----------------

function groupOptions(current) {
  return GROUPS.map(([id, label]) =>
    `<option value="${id}"${id === current ? " selected" : ""}>${esc(label)}</option>`).join("");
}

function renderForm() {
  const editing = mode === "edit" && detail;
  const d = editing ? detail : null;
  const builtin = !!(d && d.builtin);
  const nameVal = d ? d.name : "";
  const groupVal = d ? d.group : "action";
  const tagsVal = d ? (d.tags || []).join(", ") : "";
  const svgVal = d ? String(d.source || "") : "";
  const head = editing
    ? `<button class="sysm-back" data-act="back">${icon("chevron-right", 14)}返回详情</button>` +
      `<span class="sysm-title">${builtin ? "编辑分组与别名" : "编辑图标"}</span>` +
      `<span class="chip">${builtin ? "内置" : "自定义"}</span>` +
      `<span class="chip mono">${esc(d.name)}</span>`
    : `<button class="sysm-back" data-act="back">${icon("chevron-right", 14)}返回列表</button>` +
      `<span class="sysm-title">新增图标</span><span class="chip">粘贴 SVG</span>`;

  const fields = `<div class="sysm-grid">` +
    `<div class="sysm-field"><span class="sysm-lbl">名称 <b style="color:var(--ui-danger-ink)">*</b></span>` +
    `<input id="icm-name" class="proj-input" maxlength="48" value="${esc(nameVal)}" ` +
    `${editing ? "disabled" : ""} placeholder="kebab-case，如 brand-mark"></div>` +
    `<div class="sysm-field"><span class="sysm-lbl">分组</span>` +
    `<select id="icm-group" class="proj-input">${groupOptions(groupVal)}</select></div>` +
    `<div class="sysm-field full"><span class="sysm-lbl">别名（逗号分隔，供检索与替换字符）</span>` +
    `<input id="icm-tags" class="proj-input" maxlength="240" value="${esc(tagsVal)}" ` +
    `placeholder="logo, 品牌, 标记"></div>` +
    `</div>`;

  const svgBlock = builtin
    ? `<div class="sysm-sec">SVG 源码 <span class="line"></span><span class="chip">只读</span></div>` +
      `<pre class="icm-src">${esc(svgVal)}</pre>` +
      `<p class="sysm-hint">内置图标 SVG 只读；需要不同形态请用「以它为起点设计」另存为自定义图标。</p>`
    : `<div class="sysm-sec">SVG 源码 <span class="line"></span>` +
      `<span class="chip">${editing ? "可编辑" : "必填"}</span></div>` +
      `<textarea id="icm-svg" class="icm-code" spellcheck="false" aria-label="SVG 源码" ` +
      `placeholder="&lt;svg viewBox=&quot;0 0 24 24&quot;&gt;&lt;path d=&quot;…&quot;/&gt;&lt;/svg&gt; 或直接粘贴路径片段">${esc(svgVal)}</textarea>` +
      `<div class="sysm-sec">实时预览 <span class="line"></span></div>` +
      `<div id="icm-prev" class="icm-prev">${previewHTML(svgVal)}</div>` +
      `<div class="icm-rules">` +
      `<span>允许：path / circle / rect / line / polyline / polygon / ellipse / g / defs 渐变</span>` +
      `<span>禁止：script / foreignObject / text / image / style / 外链 href / on* 事件 · 上限 64 KB</span>` +
      `</div>` +
      (editing && d && d.designed
        ? `<div class="icm-note warn">${icon("warning", 14)}<div>手改 SVG 会清掉设计稿（design 字段）；` +
          `之后仍可用设计器重画。只改分组/别名不会影响设计稿。</div></div>` : "");

  RIGHT().innerHTML = `<div class="sysm-det">` +
    `<div class="sysm-form-head">${head}</div>` +
    `<div class="sysm-form">${fields}${svgBlock}` +
    `<div id="icm-form-err" class="sysm-err" role="alert"></div>` +
    `</div>` +
    `<footer class="sysm-foot"><span class="sysm-hint">${editing
      ? "Esc 返回详情 · 校验失败不会写盘"
      : "保存后立即生效：design_icons 可检索、预览精灵自动包含"}</span>` +
    `<span class="grow"></span>` +
    (builtin ? "" : `<button class="btn" data-act="${editing ? "design" : "design-new"}">` +
      `${icon("edit", 14)}${editing ? "改用设计器编辑" : "改用可视化设计"}</button>`) +
    `<button class="btn" data-act="back">取消</button>` +
    `<button class="btn btn--primary" data-act="save">${editing ? "保存" : "保存并启用"}</button>` +
    `</footer></div>`;
}

// ---------------- 预览与校验 ----------------

function previewSVG(text) {
  const raw = String(text || "").trim();
  if (!raw) return null;
  const src = raw.toLowerCase().startsWith("<svg")
    ? raw
    : `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">${raw}</svg>`;
  let doc;
  try {
    doc = new DOMParser().parseFromString(src, "image/svg+xml");
  } catch (_) {
    return { error: "SVG 解析失败" };
  }
  if (doc.getElementsByTagName("parsererror").length) return { error: "SVG 解析失败" };
  const root = doc.documentElement;
  if (!root || root.tagName.toLowerCase() !== "svg") return { error: "根元素必须是 <svg>" };
  let bad = "";
  for (const el of [root, ...root.querySelectorAll("*")]) {
    const tag = el.tagName.toLowerCase();
    if (tag !== "svg" && !ICON_TAGS.has(tag)) { bad = "元素 <" + el.tagName + "> 不允许"; break; }
    for (const a of [...el.attributes]) {
      const n = a.name.toLowerCase();
      if (n === "xmlns" || n === "xmlns:xlink") continue;
      if (n.startsWith("on") || n === "style") { bad = "属性 " + a.name + " 不允许"; break; }
      if (n === "href" && !a.value.trim().startsWith("#")) { bad = "href 只能引用文档内 #id"; break; }
      if (/url\(\s*['"]?\s*(?:https?:|data:|javascript:)/i.test(a.value)) { bad = "不允许外链 url()"; break; }
    }
    if (bad) break;
  }
  if (bad) return { error: bad };
  const viewBox = root.getAttribute("viewBox") || "0 0 24 24";
  return { svg: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}">${root.innerHTML}</svg>`, viewBox };
}

function previewHTML(text) {
  if (!String(text || "").trim()) return `<small>粘贴 SVG 后在这里预览</small>`;
  const p = previewSVG(text);
  if (!p || p.error) return `<small>${esc((p && p.error) || "SVG 解析失败")}</small>`;
  return `<span class="icm-prev-glyph">${p.svg}</span>` +
    `<small>${esc(p.viewBox)} · 颜色继承 currentColor</small>`;
}

function updatePreview() {
  const el = $("icm-prev");
  const svg = $("icm-svg");
  if (!el || !svg) return;
  el.innerHTML = previewHTML(svg.value);
}

function splitTags(v) {
  return String(v || "").split(/[,，]/).map((s) => s.trim()).filter(Boolean).slice(0, 12);
}

function formValues() {
  const name = $("icm-name");
  const group = $("icm-group");
  const tags = $("icm-tags");
  const svg = $("icm-svg");
  return {
    name: name ? name.value.trim() : "",
    group: group ? group.value : "action",
    tags: tags ? splitTags(tags.value) : [],
    svg: svg ? svg.value : "",
  };
}

// ---------------- 设计器 ----------------

// 打开设计器前收起图标库（两个弹层互斥），返回/保存后由设计器回调恢复列表。
function openDesigner(name) {
  const backName = selected;
  closeIconManager();
  openIconDesigner({
    name: name || undefined,
    onSaved: (saved) => openIconManager(saved),
    onClosed: () => openIconManager(backName),
  });
}

// ---------------- 操作 ----------------

async function reload(name) {
  mode = "detail";
  detail = null;
  if (name) selected = name;
  await loadAll();
}

async function toggleIcon(name) {
  const ic = all.find((x) => x.name === name);
  if (!ic) return;
  const off = !ic.disabled;
  try {
    await api("/api/catalog/icons/" + encodeURIComponent(name) + "/state", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ disabled: off }),
    });
    toast((off ? "已停用 " : "已启用 ") + name);
    await reload(selected === name ? name : selected);
  } catch (e) {
    toast(e.message, true);
  }
}

async function saveForm() {
  const err = $("icm-form-err");
  if (err) err.textContent = "";
  const editing = mode === "edit" && detail;
  const builtin = !!(editing && detail.builtin);
  const v = formValues();
  const fail = (msg, field) => {
    if (err) err.textContent = msg;
    if (field) field.focus();
  };

  if (!editing) {
    if (!v.name) return fail("名称必填", $("icm-name"));
    if (v.name.length > 48 || !NAME_RE.test(v.name)) {
      return fail("名称只能是小写字母/数字/连字符（1-48 位）", $("icm-name"));
    }
    if (all.some((ic) => ic.name === v.name)) {
      return fail("名称已存在：" + v.name + "，请换一个", $("icm-name"));
    }
  }
  if (!builtin) {
    const svgText = v.svg.trim();
    if (!svgText) return fail("SVG 不能为空", $("icm-svg"));
    const p = previewSVG(svgText);
    if (!p || p.error) return fail(p && p.error ? p.error : "SVG 解析失败", $("icm-svg"));
  }

  const body = { group: v.group, tags: v.tags };
  if (!builtin) body.svg = v.svg.trim();
  try {
    if (editing) {
      await api("/api/catalog/icons/" + encodeURIComponent(detail.name), {
        method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
      toast("已保存 " + detail.name);
      selected = detail.name;
    } else {
      const data = await api("/api/catalog/icons", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: v.name, ...body }),
      });
      toast("已新增图标 " + (data.icon && data.icon.name || v.name));
      selected = data.icon && data.icon.name || v.name;
    }
    await reload(selected);
  } catch (e) {
    fail(e.code ? e.code + "：" + e.message : e.message);
  }
}

async function removeSelected() {
  if (!detail) return;
  const ok = await uiConfirm({
    title: "移除图标",
    note: "将删除自定义图标「" + detail.name + "」（" + detail.name + ".json）。" +
      "画布上已引用 #ds-ic-" + detail.name + " 的设计稿会变成空白；此操作不可撤销。",
    okText: "移除",
    danger: true,
  });
  if (!ok) return;
  try {
    await api("/api/catalog/icons/" + encodeURIComponent(detail.name), { method: "DELETE" });
    toast("已移除 " + detail.name);
    selected = "";
    await reload("");
  } catch (e) {
    toast(e.message, true);
  }
}

async function resetSelected() {
  if (!detail) return;
  try {
    await api("/api/catalog/icons/" + encodeURIComponent(detail.name) + "/reset", { method: "POST" });
    toast("已恢复 " + detail.name + " 的默认分组/别名并启用");
    await reload(detail.name);
  } catch (e) {
    toast(e.message, true);
  }
}

async function copyText(text, label) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    toast("已复制" + (label ? " · " + label : ""));
  } catch (_) {
    toast("复制失败：浏览器拒绝写剪贴板", true);
  }
}

// ---------------- 右侧统一事件 ----------------

RIGHT().addEventListener("input", (ev) => {
  if (ev.target.id !== "icm-svg") return;
  clearTimeout(previewTimer);
  previewTimer = setTimeout(updatePreview, 180);
});

RIGHT().addEventListener("click", (ev) => {
  const act = ev.target.closest("[data-act]");
  if (!act) return;
  switch (act.dataset.act) {
    case "edit":
      if (!detail) break;
      mode = "edit";
      renderRight();
      break;
    case "design":
      if (detail) openDesigner(detail.name);
      break;
    case "design-new":
      openDesigner();
      break;
    case "back":
      if (mode === "create") {
        mode = "detail";
        detail = null;
        if (selected) loadDetail(selected);
        else loadAll();
      } else {
        mode = "detail";
        renderRight();
      }
      break;
    case "done": closeIconManager(); break;
    case "new":
      mode = "create";
      detail = null;
      renderRight();
      break;
    case "enable": toggleIcon(detail.name); break;
    case "reset": resetSelected(); break;
    case "remove": removeSelected(); break;
    case "save": saveForm(); break;
    case "copy-usage": copyText(detail && detail.usage, "精灵引用"); break;
    case "copy-source": copyText(detail && detail.source, "独立 SVG"); break;
  }
});

// ---------------- 头部 / 过滤 / 搜索 / 键盘 ----------------

$("icons-segs").addEventListener("click", (ev) => {
  const b = ev.target.closest(".sysm-seg");
  if (!b) return;
  setFilter(b.dataset.filter);
  refreshView();
});

$("icons-search").addEventListener("input", (ev) => {
  query = ev.target.value.trim();
  refreshView();
});

$("icons-new").addEventListener("click", () => {
  mode = "create";
  detail = null;
  renderRight();
});

// Esc 分级：表单 → 详情 → 关闭。监听先于 main.js 的全局 Esc（import 求值顺序）。
window.addEventListener("keydown", (e) => {
  if (!isIconManagerOpen() || e.key !== "Escape") return;
  if (mode !== "detail") {
    e.preventDefault();
    e.stopImmediatePropagation();
    mode = "detail";
    renderRight();
  }
}, true);

$("icons-modal").addEventListener("keydown", (e) => {
  if (e.key === "Tab") {
    const list = [...$("icons-modal").querySelectorAll("button,input,textarea,select")]
      .filter((el) => !el.disabled && el.offsetParent !== null);
    if (!list.length) return;
    const first = list[0], last = list[list.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    return;
  }
  const tag = (e.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    const rows = visibleRows();
    if (!rows.length) return;
    e.preventDefault();
    let idx = rows.findIndex((ic) => ic.name === selected);
    idx = e.key === "ArrowDown" ? Math.min(rows.length - 1, idx + 1) : Math.max(0, idx - 1);
    selected = rows[idx].name;
    mode = "detail";
    detail = null;
    renderList();
    scrollSelectedIntoView();
    loadDetail(selected);
  } else if (e.key === "Enter" && selected && mode === "detail") {
    e.preventDefault();
    if (detail) { mode = "edit"; renderRight(); }
  } else if (e.key === " " && selected && mode === "detail") {
    e.preventDefault();
    toggleIcon(selected);
  } else if (e.key === "n" || e.key === "N") {
    e.preventDefault();
    mode = "create";
    detail = null;
    renderRight();
  } else if (e.key === "d" || e.key === "D") {
    e.preventDefault();
    openDesigner();
  } else if (e.key === "/") {
    e.preventDefault();
    $("icons-search").focus();
  }
});

document.querySelectorAll("[data-icons-close]").forEach((el) => el.addEventListener("click", () => closeIconManager()));
$("icons-open").addEventListener("click", () => openIconManager());
$("icons-design").addEventListener("click", () => openDesigner());

export { openIconManager, closeIconManager, isIconManagerOpen };
