/* 设计体系管理 v2：搜索 + 过滤 + 列表 + 详情/编辑（两栏）。
 *
 * 与 MCP 的 design_system_manage 共用同一套 HTTP 管理端点。
 * 内置体系：可停用、可改显示信息、可恢复默认；不可删除。
 * 自定义体系：可编辑显示信息 / 完整 JSON、可移除。
 * 停用只影响选择面（目录/搜索/预览/MCP 工具）；画布上已生成的稿不回收。
 */
"use strict";

import { $ } from "./state.js";
import { esc, toast, uiConfirm } from "./ui.js";
import { api } from "./net.js";
import { icon } from "./icons-bridge.js";
import { loadCatalog, backToList } from "./catalog.js";

const RIGHT = () => $("sys-right");
let all = [];
let filter = "all";
let query = "";
let selected = "";
let mode = "detail"; // detail | create | edit
let editTab = "meta"; // meta | json
let detail = null;
let detailSeq = 0;
let jsonRaw = "";
let paletteTimer = 0;

function isSystemManagerOpen() {
  const box = $("sys-modal");
  return !!box && !box.classList.contains("hidden");
}

function openSystemManager() {
  selected = "";
  detail = null;
  mode = "detail";
  editTab = "meta";
  $("sys-modal").classList.remove("hidden");
  $("sys-search").value = "";
  query = "";
  setFilter("all");
  loadAll().then(() => $("sys-search").focus());
}

function closeSystemManager() {
  $("sys-modal").classList.add("hidden");
  const btn = $("sys-manage");
  if (btn) btn.focus();
}

// ---------------- 数据 ----------------

async function loadAll() {
  try {
    const data = await api("/api/catalog/systems?all=1");
    all = data.systems || [];
  } catch (e) {
    toast("体系列表加载失败: " + e.message, true);
    return;
  }
  $("sys-count").textContent = String(all.length);
  updateCounts();
  const rows = visibleRows();
  if (mode !== "create" && !rows.some((s) => s.id === selected)) {
    selected = rows.length ? rows[0].id : "";
    mode = "detail";
  }
  renderList();
  if (mode === "detail") {
    if (selected) await loadDetail(selected);
    else renderRight();
  }
}

function visibleRows() {
  let list = all;
  if (filter === "builtin") list = list.filter((s) => s.builtin);
  else if (filter === "custom") list = list.filter((s) => !s.builtin);
  else if (filter === "disabled") list = list.filter((s) => s.disabled);
  if (query) {
    const q = query.toLowerCase();
    list = list.filter((s) => [s.name, s.vendor, s.id].some((v) => String(v || "").toLowerCase().includes(q)));
  }
  return list;
}

function updateCounts() {
  $("sys-cnt-all").textContent = String(all.length);
  $("sys-cnt-builtin").textContent = String(all.filter((s) => s.builtin).length);
  $("sys-cnt-custom").textContent = String(all.filter((s) => !s.builtin).length);
  $("sys-cnt-disabled").textContent = String(all.filter((s) => s.disabled).length);
}

function setFilter(next) {
  filter = next;
  document.querySelectorAll("#sys-segs .sysm-seg").forEach((b) => {
    b.classList.toggle("on", b.dataset.filter === next);
  });
}

function refreshView() {
  updateCounts();
  const rows = visibleRows();
  if (mode !== "create" && !rows.some((s) => s.id === selected)) {
    selected = rows.length ? rows[0].id : "";
    mode = "detail";
  }
  renderList();
  if (mode === "create") { renderRight(); return; }
  if (selected) loadDetail(selected); else renderRight();
}

async function loadDetail(id) {
  const seq = ++detailSeq;
  if (!detail || detail.id !== id) {
    RIGHT().innerHTML = `<div class="sysm-empty"><span>加载中…</span></div>`;
  }
  try {
    const d = await api("/api/catalog/systems/" + encodeURIComponent(id));
    if (seq !== detailSeq || selected !== id || mode !== "detail") return;
    detail = d;
    renderRight();
  } catch (e) {
    if (seq !== detailSeq) return;
    toast("体系详情加载失败: " + e.message, true);
    selected = "";
    renderRight();
  }
}

// ---------------- 列表 ----------------

function chipsOf(sys) {
  const out = [`<span class="chip">${sys.builtin ? "内置" : "自定义"}</span>`];
  if (sys.disabled) out.push(`<span class="chip chip--warn">已停用</span>`);
  else if (sys.overridden) out.push(`<span class="chip">已改显示信息</span>`);
  return out.join("");
}

function renderList() {
  const rows = visibleRows();
  const box = $("sys-rows");
  if (!rows.length) {
    box.innerHTML = `<div class="sysm-empty">${icon("search", 24)}` +
      `<b style="font-size:var(--ui-text-sm);color:var(--ui-ink-2);font-weight:var(--ui-weight-medium)">没有匹配的体系</b>` +
      `<span>换个关键词，或清空搜索</span></div>`;
    return;
  }
  box.innerHTML = rows.map((sys) => {
    const cls = ["sysm-row"];
    if (sys.id === selected) cls.push("on");
    if (sys.disabled) cls.push("off");
    return `<div class="${cls.join(" ")}" data-id="${esc(sys.id)}" role="button" tabindex="-1">` +
      `<span class="sysm-swatch" style="background:${esc(sys.primary || "transparent")}"></span>` +
      `<div class="sysm-main"><div class="sysm-name">${esc(sys.name)} ${chipsOf(sys)}</div>` +
      `<div class="sysm-meta">${esc(sys.vendor)} · ${esc(sys.family)} · <span class="mono">${esc(sys.id)}</span></div></div>` +
      `<button class="sysm-sw${sys.disabled ? "" : " on"}" data-switch="${esc(sys.id)}" title="${sys.disabled ? "启用" : "停用"}" aria-label="${sys.disabled ? "启用" : "停用"} ${esc(sys.name)}" aria-pressed="${sys.disabled ? "false" : "true"}"></button>` +
      `</div>`;
  }).join("");
}

$("sys-rows").addEventListener("click", (ev) => {
  const sw = ev.target.closest("[data-switch]");
  if (sw) {
    ev.stopPropagation();
    toggleSystem(sw.dataset.switch);
    return;
  }
  const row = ev.target.closest(".sysm-row");
  if (!row) return;
  if (row.dataset.id === selected && mode === "detail") return;
  selected = row.dataset.id;
  mode = "detail";
  detail = null;
  renderList();
  loadDetail(selected);
});

// ---------------- 详情 ----------------

function countOf(obj) { return Object.keys(obj || {}).length; }

function renderRight() {
  if (mode === "create") return renderCreate();
  if (mode === "edit") return renderEdit();
  if (!selected) return renderNoSelection();
  if (detail && detail.id === selected) return renderDetail(detail);
  RIGHT().innerHTML = `<div class="sysm-empty"><span>加载中…</span></div>`;
}

function renderNoSelection() {
  RIGHT().innerHTML = `<div class="sysm-empty">` +
    `<span class="ring">${icon("grid", 20)}</span>` +
    `<b style="font-size:var(--ui-text-md);color:var(--ui-ink-2);font-weight:var(--ui-weight-medium)">没有匹配的体系</b>` +
    `<span>换个关键词，或清空搜索后从左侧选择一个体系。</span>` +
    `<span class="sysm-hint">也可以点右上「新增体系」，以品牌主色派生一套新的设计体系。</span>` +
    `</div>`;
}

function renderDetail(d) {
  const toks = d.light || {};
  const prim = d.primary || "";
  const themes = (d.themes || []).join(" / ");
  const actions = [];
  actions.push(`<button class="btn" data-act="edit">${icon("settings", 14)}编辑显示信息</button>`);
  if (!d.builtin) {
    actions.push(`<button class="btn btn--danger" data-act="remove">${icon("trash", 14)}移除</button>`);
  }
  const footBtns = [];
  if (d.builtin) {
    footBtns.push(`<button class="btn" data-act="reset"${(d.overridden || d.disabled) ? "" : " disabled"}>恢复默认</button>`);
  }
  footBtns.push(`<button class="btn" data-act="done">完成</button>`);

  RIGHT().innerHTML = `<div class="sysm-det">` +
    `<header class="sysm-det-head">` +
    `<span class="sysm-det-swatch" style="background:${esc(prim || "transparent")}"></span>` +
    `<div class="sysm-det-title"><div class="sysm-det-name">${esc(d.name)} ` +
    `<span class="chip">${d.builtin ? "内置" : "自定义"}</span>` +
    (d.disabled ? `<span class="chip chip--warn">已停用</span>` : `<span class="chip chip--ok">已启用</span>`) +
    (d.overridden ? `<span class="chip">已改显示信息</span>` : "") +
    `</div><div class="sysm-det-sub"><span>${esc(d.vendor || "—")}</span><span>·</span><span>${esc(d.family)}</span>` +
    `<span>·</span><span class="mono">${esc(d.id)}</span></div></div>` +
    `<div class="sysm-acts">${actions.join("")}</div></header>` +
    `<div class="sysm-body-pad">` +
    (d.disabled ? `<div class="sysm-banner">${icon("warning", 14)}` +
      `<span class="grow">已停用 · 目录、搜索、预览与 MCP 工具不再出现它；画布上已有的稿保留。</span>` +
      `<button class="btn" data-act="enable">启用</button></div>` : "") +
    `<div class="sysm-kv-grid">` +
    `<div class="sysm-kv"><span class="k">厂商</span><span class="v">${esc(d.vendor || "—")}</span></div>` +
    `<div class="sysm-kv"><span class="k">落地类型</span><span class="v">${esc(d.family)}</span></div>` +
    `<div class="sysm-kv"><span class="k">默认画板</span><span class="v mono">${esc(d.default_artboard || "—")}</span></div>` +
    `<div class="sysm-kv"><span class="k">组件 / 主题</span><span class="v"><span class="mono">${(d.components || []).length || "—"}</span> 个 · ${esc(themes || "light")}</span></div>` +
    `<div class="sysm-kv"><span class="k">主色</span><span class="v"><i class="sysm-swatch" style="width:var(--ui-icon-12);height:var(--ui-icon-12);border-radius:var(--ui-radius-xs);display:inline-block;vertical-align:-2px;background:${esc(prim || "transparent")}"></i> <span class="mono">${esc(prim || "—")}</span></span></div>` +
    `<div class="sysm-kv"><span class="k">来源</span><span class="v">${d.builtin ? "内置 data/" + esc(d.id) + ".json" : "自定义 custom-systems/" + esc(d.id) + ".json"}</span></div>` +
    `</div>` +
    `<div class="sysm-sec">Token 概览 <span class="line"></span></div>` +
    `<div class="sysm-tiles">` +
    `<div class="sysm-tile"><span class="k">颜色</span><span class="v">${countOf(toks.colors)}</span></div>` +
    `<div class="sysm-tile"><span class="k">字阶</span><span class="v">${countOf(toks.type && toks.type.levels)}</span></div>` +
    `<div class="sysm-tile"><span class="k">间距</span><span class="v">${countOf(toks.spacing)}</span></div>` +
    `<div class="sysm-tile"><span class="k">圆角</span><span class="v">${countOf(toks.radius)}</span></div>` +
    `</div>` +
    `<div class="sysm-sec">显示信息 <span class="line"></span></div>` +
    `<div class="sysm-kv-grid">` +
    `<div class="sysm-kv"><span class="k">名称</span><span class="v">${esc(d.name)}</span></div>` +
    `<div class="sysm-kv"><span class="k">一句话风格</span><span class="v">${esc(d.style && d.style.summary || "—")}</span></div>` +
    `</div>` +
    `<p class="sysm-desc">${esc(d.description || "")}` +
    (d.builtin ? `<br><b>内置体系只读 Token</b>，可改名称 / 厂商 / 描述 / 一句话风格；改动用「恢复默认」清除。` : "") +
    `</p></div>` +
    `<footer class="sysm-foot"><span class="sysm-hint">${d.builtin ? "内置体系不可删除 · Token 只读" : "自定义体系可整体编辑 JSON"}</span>` +
    `<span class="grow"></span>${footBtns.join("")}</footer></div>`;
}

// ---------------- 新增 / 编辑 ----------------

function normPrimary(v) {
  const s = String(v || "").trim();
  if (!s) return "";
  return s.startsWith("#") ? s : "#" + s;
}

function primaryValid(p) { return /^#[0-9a-f]{6}$/i.test(p); }

function renderCreate() {
  const bases = all.filter((s) => !s.disabled);
  if (!bases.length) {
    RIGHT().innerHTML = `<div class="sysm-empty"><b style="color:var(--ui-ink-2)">没有可用作结构基底的体系</b>` +
      `<span>先启用一个体系，再新增。</span></div>`;
    return;
  }
  RIGHT().innerHTML = `<div class="sysm-form-head">` +
    `<button class="sysm-back" data-act="back">${icon("chevron-right", 14)}返回详情</button>` +
    `<span class="sysm-title">新增体系</span><span class="chip">品牌派生</span>` +
    `<span class="grow"></span><span class="sysm-hint">从结构基底继承字型 / 间距 / 圆角 / 阴影 / 动效</span></div>` +
    `<div class="sysm-form">` +
    `<div class="sysm-sec">基本信息 <span class="line"></span></div>` +
    `<div class="sysm-grid">` +
    field("名称", `<input id="sys-name" class="proj-input" maxlength="40" placeholder="如「山茶茶饮」">`, true) +
    field("厂商 / 来源", `<input id="sys-vendor" class="proj-input" maxlength="40" placeholder="缺省「自定义 · 品牌派生」">`) +
    field("体系 ID", `<input id="sys-id" class="proj-input" maxlength="48" placeholder="小写字母 / 数字 / 连字符">`) +
    field("落地类型", `<select id="sys-family" class="proj-input"><option value="">继承基底</option>` +
      `<option value="mobile">mobile</option><option value="web">web</option><option value="cross">cross</option></select>`) +
    `<div class="sysm-field full"><span class="sysm-lbl">描述</span><input id="sys-desc" class="proj-input" maxlength="160" placeholder="一句话说明这个品牌体系"></div>` +
    `</div>` +
    `<div class="sysm-sec">结构与主色 <span class="line"></span></div>` +
    `<div class="sysm-grid">` +
    field("结构基底", `<select id="sys-base" class="proj-input">` +
      bases.map((s) => `<option value="${esc(s.id)}">${esc(s.name)}（${esc(s.id)}）</option>`).join("") + `</select>`) +
    field("品牌主色", `<div class="sysm-primary"><i id="sys-primary-dot" class="sysm-swatch" style="background:transparent"></i>` +
      `<input id="sys-primary" class="proj-input" maxlength="7" placeholder="RRGGBB"></div>`, true) +
    `<div class="sysm-field full"><span class="sysm-lbl">派生色板（预览）</span><div id="sysm-palette" class="sysm-palette"></div></div>` +
    `</div>` +
    `<div id="sysm-form-err" class="sysm-err" role="alert"></div>` +
    `</div>` +
    `<footer class="sysm-foot"><span class="sysm-hint">Esc 取消 · 新增后自动切到该体系详情</span>` +
    `<span class="grow"></span><button class="btn" data-act="back">取消</button>` +
    `<button class="btn btn--primary" data-act="create">新增体系</button></footer>`;
  $("sys-base").addEventListener("change", schedulePalette);
}

function field(label, control, required) {
  return `<div class="sysm-field"><span class="sysm-lbl">${esc(label)}${required ? ' <b style="color:var(--ui-danger-ink)">*</b>' : ""}</span>` +
    control + `</div>`;
}

function renderEdit() {
  if (!detail || detail.id !== selected) {
    RIGHT().innerHTML = `<div class="sysm-empty"><span>加载中…</span></div>`;
    return;
  }
  const d = detail;
  const custom = !d.builtin;
  const tabs = custom
    ? `<div class="sysm-tabs">` +
      `<button class="sysm-tab${editTab === "meta" ? " on" : ""}" data-act="tab-meta">显示信息</button>` +
      `<button class="sysm-tab${editTab === "json" ? " on" : ""}" data-act="tab-json">完整 JSON</button></div>`
    : "";
  const meta = editTab === "json" && custom
    ? jsonEditor()
    : `<div class="sysm-form">` +
      `<div class="sysm-sec">显示信息 <span class="line"></span></div>` +
      `<div class="sysm-grid">` +
      field("名称", `<input id="sys-name" class="proj-input" maxlength="40" value="${esc(d.name)}">`, true) +
      field("厂商 / 来源", `<input id="sys-vendor" class="proj-input" maxlength="40" value="${esc(d.vendor || "")}">`) +
      `<div class="sysm-field full"><span class="sysm-lbl">描述</span><input id="sys-desc" class="proj-input" maxlength="160" value="${esc(d.description || "")}"></div>` +
      `<div class="sysm-field full"><span class="sysm-lbl">一句话风格（Style.Summary）</span><input id="sys-summary" class="proj-input" maxlength="120" value="${esc(d.style && d.style.summary || "")}"></div>` +
      `</div>` +
      `<div class="sysm-sec">只读 <span class="line"></span></div>` +
      `<div class="sysm-kv-grid">` +
      `<div class="sysm-kv"><span class="k">ID</span><span class="v mono">${esc(d.id)}</span></div>` +
      `<div class="sysm-kv"><span class="k">落地类型</span><span class="v">${esc(d.family)}</span></div>` +
      `<div class="sysm-kv"><span class="k">主色</span><span class="v"><i class="sysm-swatch" style="width:var(--ui-icon-12);height:var(--ui-icon-12);border-radius:var(--ui-radius-xs);display:inline-block;vertical-align:-2px;background:${esc(d.primary || "transparent")}"></i> <span class="mono">${esc(d.primary || "—")}</span></span></div>` +
      `<div class="sysm-kv"><span class="k">主题</span><span class="v">${esc((d.themes || []).join(" / ") || "light")}</span></div>` +
      `</div>` +
      `<div id="sysm-form-err" class="sysm-err" role="alert"></div>` +
      (d.builtin
        ? `<p class="sysm-hint">内置体系只改显示信息，Token 不变；用「恢复默认」清除覆盖。</p>`
        : `<p class="sysm-hint">空字段保留现值；要改 Token 请切到「完整 JSON」。</p>`) +
      `</div>`;
  RIGHT().innerHTML = `<div class="sysm-form-head">` +
    `<button class="sysm-back" data-act="back">${icon("chevron-right", 14)}返回详情</button>` +
    `<span class="sysm-title">编辑 · ${esc(d.name)}</span>` +
    `<span class="chip">${d.builtin ? "内置" : "自定义"}</span></div>` +
    tabs + meta +
    `<footer class="sysm-foot"><span class="sysm-hint">Esc 返回详情 · 校验失败不会写盘</span>` +
    `<span class="grow"></span><button class="btn" data-act="back">取消</button>` +
    `<button class="btn btn--primary" data-act="save">保存</button></footer>`;
}

function jsonEditor() {
  return `<div class="sysm-form">` +
    `<textarea id="sys-json" class="sysm-code" spellcheck="false" aria-label="体系完整 JSON"></textarea>` +
    `<div id="sysm-form-err" class="sysm-json-err hidden" role="alert"></div>` +
    `<p class="sysm-hint">整体替换自定义体系 JSON；严格校验 colors / 字号 / 圆角；id 不可修改。<br>改动 JSON 时以 JSON 为准，「显示信息」表单字段会被忽略。</p>` +
    `</div>`;
}

function fillJSON() {
  const el = $("sys-json");
  if (el) el.value = jsonRaw;
}

async function openEdit() {
  if (!detail) return;
  mode = "edit";
  editTab = "meta";
  jsonRaw = "";
  if (!detail.builtin) {
    try {
      const data = await api("/api/catalog/systems/" + encodeURIComponent(detail.id) + "/json");
      jsonRaw = data.json || "";
    } catch (e) {
      toast("读取体系 JSON 失败: " + e.message, true);
      mode = "detail";
      renderRight();
      return;
    }
  }
  renderRight();
}

// ---------------- 操作 ----------------

async function mutate(fn, okMsg) {
  try {
    await fn();
    if (okMsg) toast(okMsg);
    await loadAll();
    loadCatalog();
    backToList();
  } catch (e) {
    toast(e.message, true);
  }
}

async function toggleSystem(id) {
  const sys = all.find((s) => s.id === id);
  if (!sys) return;
  const off = !sys.disabled;
  await mutate(
    () => api("/api/catalog/systems/" + encodeURIComponent(id) + "/state", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ disabled: off }),
    }),
    (off ? "已停用「" : "已启用「") + sys.name + "」",
  );
}

async function createSystem() {
  const name = $("sys-name").value.trim();
  const primary = normPrimary($("sys-primary").value);
  const base = $("sys-base").value;
  const err = $("sysm-form-err");
  err.textContent = "";
  if (!name) { err.textContent = "名称必填"; $("sys-name").focus(); return; }
  if (!primaryValid(primary)) { err.textContent = "品牌主色要写成 RRGGBB（如 C8412B）"; $("sys-primary").focus(); return; }
  try {
    const data = await api("/api/catalog/systems", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name, primary, base,
        vendor: $("sys-vendor").value.trim(),
        id: $("sys-id").value.trim(),
        family: $("sys-family").value,
        description: $("sys-desc").value.trim(),
      }),
    });
    toast("已新增体系「" + (data.system && data.system.name || name) + "」");
    selected = data.system && data.system.id || selected;
    mode = "detail";
    detail = null;
    await loadAll();
    loadCatalog();
    backToList();
  } catch (e) {
    err.textContent = e.code ? e.code + "：" + e.message : e.message;
  }
}

async function saveEdit() {
  if (!detail) return;
  const d = detail;
  const custom = !d.builtin;
  const err = $("sysm-form-err");
  if (err) err.textContent = "";
  try {
    if (custom && editTab === "json") {
      const raw = $("sys-json").value;
      await api("/api/catalog/systems/" + encodeURIComponent(d.id), {
        method: "PUT", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ json: raw }),
      });
    } else {
      const name = $("sys-name").value.trim();
      if (!name) { err.textContent = "名称不能为空"; $("sys-name").focus(); return; }
      await api("/api/catalog/systems/" + encodeURIComponent(d.id), {
        method: "PUT", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          vendor: $("sys-vendor").value.trim(),
          description: $("sys-desc").value.trim(),
          summary: $("sys-summary").value.trim(),
        }),
      });
    }
    toast("已保存「" + d.name + "」");
    mode = "detail";
    detail = null;
    await loadAll();
    loadCatalog();
    backToList();
  } catch (e) {
    err.textContent = e.code ? e.code + "：" + e.message : e.message;
    err.classList.remove("hidden");
  }
}

async function removeSelected() {
  if (!detail) return;
  const ok = await uiConfirm({
    title: "移除设计体系",
    note: "将从自定义体系中删除「" + detail.name + "」（" + detail.id + "）。画布上已生成的设计稿不受影响；此操作不可撤销。",
    okText: "移除",
    danger: true,
  });
  if (!ok) return;
  await mutate(
    () => api("/api/catalog/systems/" + encodeURIComponent(detail.id), { method: "DELETE" }),
    "已移除「" + detail.name + "」",
  );
}

async function resetSelected() {
  if (!detail) return;
  await mutate(
    () => api("/api/catalog/systems/" + encodeURIComponent(detail.id) + "/reset", { method: "POST" }),
    "已恢复「" + detail.name + "」默认显示信息并启用",
  );
}

// ---------------- 调色板预览 ----------------

function schedulePalette() {
  clearTimeout(paletteTimer);
  paletteTimer = setTimeout(updatePalette, 220);
}

async function updatePalette() {
  const input = $("sys-primary");
  const dot = $("sys-primary-dot");
  const box = $("sysm-palette");
  if (!input || !dot || !box) return;
  const primary = normPrimary(input.value);
  dot.style.background = primaryValid(primary) ? primary : "transparent";
  if (!primaryValid(primary)) { box.innerHTML = ""; return; }
  try {
    const data = await api("/api/catalog/palette?primary=" + encodeURIComponent(primary));
    box.innerHTML = (data.palette || []).map((c, i) =>
      `<span class="sysm-pal"><i style="background:${esc(c)}"></i><span>${i + 1}</span></span>`).join("");
  } catch (_) { box.innerHTML = ""; }
}

// ---------------- 右侧统一事件 ----------------

RIGHT().addEventListener("input", (ev) => {
  if (ev.target.id === "sys-primary") schedulePalette();
  if (ev.target.id === "sys-json") {
    const err = $("sysm-form-err");
    if (err) err.classList.add("hidden");
  }
});

RIGHT().addEventListener("click", (ev) => {
  const act = ev.target.closest("[data-act]");
  if (!act) return;
  switch (act.dataset.act) {
    case "edit": if (detail) openEdit(); break;
    case "back": mode = "detail"; renderRight(); break;
    case "done": closeSystemManager(); break;
    case "enable": if (detail) mutate(() => api("/api/catalog/systems/" + encodeURIComponent(detail.id) + "/state", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ disabled: false }) }), "已启用「" + detail.name + "」"); break;
    case "reset": resetSelected(); break;
    case "remove": removeSelected(); break;
    case "tab-meta": editTab = "meta"; renderRight(); break;
    case "tab-json": editTab = "json"; renderRight(); fillJSON(); break;
    case "create": createSystem(); break;
    case "save": saveEdit(); break;
  }
});

// ---------------- 头部 / 过滤 / 搜索 / 键盘 ----------------

$("sys-segs").addEventListener("click", (ev) => {
  const b = ev.target.closest(".sysm-seg");
  if (!b) return;
  setFilter(b.dataset.filter);
  refreshView();
});

$("sys-search").addEventListener("input", (ev) => {
  query = ev.target.value.trim();
  refreshView();
});

$("sys-new").addEventListener("click", () => {
  mode = "create";
  detail = null;
  renderRight();
});

// Esc 分级：编辑 → 详情 → 关闭。监听先于 main.js 的全局 Esc（import 求值顺序）。
window.addEventListener("keydown", (e) => {
  if (!isSystemManagerOpen() || e.key !== "Escape") return;
  if (mode !== "detail") {
    e.preventDefault();
    e.stopImmediatePropagation();
    mode = "detail";
    renderRight();
  }
}, true);

$("sys-modal").addEventListener("keydown", (e) => {
  if (e.key === "Tab") {
    const list = [...$("sys-modal").querySelectorAll("button,input,textarea,select")]
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
    let idx = rows.findIndex((s) => s.id === selected);
    idx = e.key === "ArrowDown" ? Math.min(rows.length - 1, idx + 1) : Math.max(0, idx - 1);
    selected = rows[idx].id;
    mode = "detail";
    detail = null;
    renderList();
    loadDetail(selected);
  } else if (e.key === "Enter" && selected && mode === "detail") {
    e.preventDefault();
    if (detail) openEdit();
  } else if (e.key === " " && selected && mode === "detail") {
    e.preventDefault();
    toggleSystem(selected);
  } else if (e.key === "n" || e.key === "N") {
    e.preventDefault();
    mode = "create";
    detail = null;
    renderRight();
  } else if (e.key === "/") {
    e.preventDefault();
    $("sys-search").focus();
  }
});

document.querySelectorAll("[data-sys-close]").forEach((el) => el.addEventListener("click", closeSystemManager));
$("sys-manage").addEventListener("click", openSystemManager);

export { openSystemManager, closeSystemManager, isSystemManagerOpen };
