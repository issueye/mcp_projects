/* 设计体系管理弹层：停用 / 新增 / 编辑 / 移除。
 *
 * 与 MCP 的 design_system_manage 共用同一套 HTTP 管理端点（/api/catalog/systems）。
 * 内置体系：可停用、可改显示信息、可恢复默认；不可删除。
 * 自定义体系：可编辑显示信息、整体编辑 JSON、移除。
 * 停用只影响「选择面」（侧栏目录 / 搜索 / 预览）；画布上已生成的稿不回收。
 */
"use strict";

import { $ } from "./state.js";
import { esc, toast, uiConfirm } from "./ui.js";
import { api } from "./net.js";
import { icon } from "./icons-bridge.js";
import { loadCatalog, backToList } from "./catalog.js";

let rows = [];
let editing = "";
let editingJSON = "";

function isSystemManagerOpen() {
  const box = $("sys-modal");
  return !!box && !box.classList.contains("hidden");
}

function openSystemManager() {
  $("sys-edit").classList.add("hidden");
  editing = "";
  editingJSON = "";
  $("sys-modal").classList.remove("hidden");
  refreshAll().then(() => {
    const first = $("sys-new-name");
    if (first) first.focus();
  });
}

function closeSystemManager() {
  $("sys-modal").classList.add("hidden");
  $("sys-edit").classList.add("hidden");
  editing = "";
  editingJSON = "";
  const btn = $("sys-manage");
  if (btn) btn.focus();
}

async function refreshAll() {
  try {
    const data = await api("/api/catalog/systems?all=1");
    rows = data.systems || [];
  } catch (e) {
    toast("体系列表加载失败: " + e.message, true);
    return;
  }
  $("sys-count").textContent = String(rows.length);
  renderCreate();
  renderRows();
  await loadCatalog();
  backToList();
}

// ---------------- 新增 ----------------

function renderCreate() {
  const bases = rows.filter((s) => !s.disabled);
  const box = $("sys-create");
  if (!bases.length) {
    box.innerHTML = `<div class="sys-form__title">新增体系</div>` +
      `<div class="ask-note">没有可用作结构基底的体系（全部被停用）。先启用一个再新增。</div>`;
    return;
  }
  box.innerHTML =
    `<div class="sys-form__title">新增体系 <span class="chip">品牌派生</span></div>` +
    `<div class="sys-form__grid">` +
    `<label class="sys-field"><span>名称（必填）</span><input id="sys-new-name" class="proj-input" maxlength="40" placeholder="如「山茶茶饮」"></label>` +
    `<label class="sys-field"><span>厂商 / 来源</span><input id="sys-new-vendor" class="proj-input" maxlength="40" placeholder="缺省「自定义 · 品牌派生」"></label>` +
    `<label class="sys-field"><span>结构基底</span><select id="sys-new-base" class="proj-input">` +
    bases.map((s) => `<option value="${esc(s.id)}">${esc(s.name)}（${esc(s.id)}）</option>`).join("") +
    `</select></label>` +
    `<label class="sys-field"><span>落地类型</span><select id="sys-new-family" class="proj-input">` +
    `<option value="">继承基底</option><option value="mobile">mobile</option>` +
    `<option value="web">web</option><option value="cross">cross</option></select></label>` +
    `<label class="sys-field"><span>品牌主色（必填）</span><input id="sys-new-primary" class="proj-input" maxlength="7" placeholder="RRGGBB"></label>` +
    `<label class="sys-field"><span>体系 ID（可选）</span><input id="sys-new-id" class="proj-input" maxlength="48" placeholder="小写字母/数字/连字符"></label>` +
    `<label class="sys-field sys-field--wide"><span>描述（可选）</span><input id="sys-new-desc" class="proj-input" maxlength="120" placeholder="一句话说明这个品牌体系"></label>` +
    `</div>` +
    `<div class="sys-form__acts"><button id="sys-create-btn" class="btn btn--primary">${icon("plus", 14)} 新增体系</button>` +
    `<span class="ask-note">颜色从主色派生 10 阶色板；字型/间距/圆角/阴影/动效继承基底。</span></div>`;
  $("sys-create-btn").addEventListener("click", createSystem);
}

function normPrimary(v) {
  const s = String(v || "").trim();
  if (!s) return "";
  return s.startsWith("#") ? s : "#" + s;
}

async function createSystem() {
  const name = $("sys-new-name").value.trim();
  const primary = normPrimary($("sys-new-primary").value);
  if (!name) { toast("名称必填", true); $("sys-new-name").focus(); return; }
  if (!/^#[0-9a-f]{6}$/i.test(primary)) { toast("品牌主色要写成 RRGGBB（如 C8412B）", true); $("sys-new-primary").focus(); return; }
  const payload = {
    name,
    vendor: $("sys-new-vendor").value.trim(),
    base: $("sys-new-base").value,
    family: $("sys-new-family").value,
    primary,
    id: $("sys-new-id").value.trim(),
    description: $("sys-new-desc").value.trim(),
  };
  try {
    const data = await api("/api/catalog/systems", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    toast("已新增体系「" + (data.system && data.system.name || name) + "」");
    await refreshAll();
  } catch (e) { toast("新增失败: " + e.message, true); }
}

// ---------------- 列表与行操作 ----------------

function flagsOf(sys) {
  const out = [`<span class="chip">${sys.builtin ? "内置" : "自定义"}</span>`];
  if (sys.disabled) out.push(`<span class="chip">已停用</span>`);
  if (sys.overridden) out.push(`<span class="chip">已改显示信息</span>`);
  return out.join("");
}

function renderRows() {
  $("sys-rows").innerHTML = rows.map((sys) => {
    const acts =
      `<button class="btn" data-act="toggle">${sys.disabled ? "启用" : "停用"}</button>` +
      `<button class="btn" data-act="edit">编辑</button>` +
      (sys.builtin
        ? `<button class="btn" data-act="reset" ${(sys.disabled || sys.overridden) ? "" : "disabled"}>恢复默认</button>`
        : `<button class="btn btn--danger" data-act="del">移除</button>`);
    return `<div class="sys-row${sys.disabled ? " is-off" : ""}" data-id="${esc(sys.id)}">` +
      `<div class="sys-row__main">` +
      `<div class="sys-row__name">${esc(sys.name)} ${flagsOf(sys)}</div>` +
      `<div class="sys-row__meta">${esc(sys.vendor)} · ${esc(sys.family)} · <span class="mono">${esc(sys.id)}</span>` +
      (sys.primary ? ` · <i class="sys-dot" style="background:${esc(sys.primary)}"></i><span class="mono">${esc(sys.primary)}</span>` : "") +
      `</div></div>` +
      `<div class="sys-row__acts">${acts}</div></div>`;
  }).join("");
}

$("sys-rows").addEventListener("click", async (ev) => {
  const btn = ev.target.closest("[data-act]");
  if (!btn) return;
  const rowEl = btn.closest(".sys-row");
  const sys = rows.find((s) => s.id === rowEl.dataset.id);
  if (!sys) return;
  switch (btn.dataset.act) {
    case "toggle": return setDisabled(sys, !sys.disabled);
    case "edit": return showEdit(sys);
    case "reset": return resetSystem(sys);
    case "del": return removeSystem(sys);
  }
});

async function setDisabled(sys, off) {
  try {
    await api("/api/catalog/systems/" + encodeURIComponent(sys.id) + "/state", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ disabled: off }),
    });
    toast((off ? "已停用「" : "已启用「") + sys.name + "」");
    await refreshAll();
  } catch (e) { toast((off ? "停用" : "启用") + "失败: " + e.message, true); }
}

async function resetSystem(sys) {
  try {
    await api("/api/catalog/systems/" + encodeURIComponent(sys.id) + "/reset", { method: "POST" });
    toast("已恢复「" + sys.name + "」默认显示信息并启用");
    await refreshAll();
  } catch (e) { toast("恢复失败: " + e.message, true); }
}

async function removeSystem(sys) {
  const ok = await uiConfirm({
    title: "移除设计体系",
    note: "将从自定义体系中删除「" + sys.name + "」（" + sys.id + "）。" +
      "画布上已生成的设计稿不受影响；此操作不可撤销。",
    okText: "移除",
    danger: true,
  });
  if (!ok) return;
  try {
    await api("/api/catalog/systems/" + encodeURIComponent(sys.id), { method: "DELETE" });
    toast("已移除「" + sys.name + "」");
    await refreshAll();
  } catch (e) { toast("移除失败: " + e.message, true); }
}

// ---------------- 编辑 ----------------

async function showEdit(sys) {
  editing = sys.id;
  editingJSON = "";
  if (!sys.builtin) {
    try {
      const data = await api("/api/catalog/systems/" + encodeURIComponent(sys.id) + "/json");
      editingJSON = data.json || "";
    } catch (e) {
      toast("读取体系 JSON 失败: " + e.message, true);
      return;
    }
  }
  const box = $("sys-edit");
  box.classList.remove("hidden");
  box.innerHTML =
    `<div class="sys-form__title">编辑 · ${esc(sys.name)} <span class="chip">${sys.builtin ? "内置" : "自定义"}</span></div>` +
    `<div class="sys-form__grid">` +
    `<label class="sys-field"><span>名称</span><input id="sys-edit-name" class="proj-input" maxlength="40" value="${esc(sys.name)}"></label>` +
    `<label class="sys-field"><span>厂商 / 来源</span><input id="sys-edit-vendor" class="proj-input" maxlength="40" value="${esc(sys.vendor)}"></label>` +
    `<label class="sys-field sys-field--wide"><span>描述</span><input id="sys-edit-desc" class="proj-input" maxlength="160" value="${esc(sys.description || "")}"></label>` +
    `<label class="sys-field sys-field--wide"><span>一句话风格（Style.Summary）</span><input id="sys-edit-summary" class="proj-input" maxlength="120" value="${esc(sys.summary || "")}"></label>` +
    `<div class="sys-field sys-field--wide"><span>ID · 落地类型</span>` +
    `<div class="sys-row__meta"><span class="mono">${esc(sys.id)}</span><span class="chip">${esc(sys.family)}</span>` +
    `<span class="chip">${esc((sys.themes || []).join(" / "))}</span>` +
    (sys.primary ? `<i class="sys-dot" style="background:${esc(sys.primary)}"></i><span class="mono">${esc(sys.primary)}</span>` : "") +
    `</div></div>` +
    (sys.builtin ? "" :
      `<label class="sys-field sys-field--wide"><span>JSON（整体替换；改动后以上面的名称等字段会被忽略）</span>` +
      `<textarea id="sys-edit-json" class="sys-json" spellcheck="false"></textarea></label>`) +
    `</div>` +
    `<div class="sys-form__acts"><button id="sys-edit-save" class="btn btn--primary">保存</button>` +
    `<button id="sys-edit-cancel" class="btn">取消</button>` +
    (sys.builtin ? `<span class="ask-note">内置体系只改显示信息，Token 不变；用「恢复默认」清除覆盖。</span>`
      : `<span class="ask-note">JSON 支持改 Token（colors/type/spacing/radius/shadows/motion）；空字段保留现值。</span>`) +
    `</div>`;
  if (!sys.builtin) $("sys-edit-json").value = editingJSON;
  $("sys-edit-save").addEventListener("click", saveEdit);
  $("sys-edit-cancel").addEventListener("click", () => {
    box.classList.add("hidden");
    editing = "";
    editingJSON = "";
  });
  $("sys-edit-name").focus();
}

async function saveEdit() {
  const sys = rows.find((s) => s.id === editing);
  if (!sys) return;
  const name = $("sys-edit-name").value.trim();
  if (!name) { toast("名称不能为空", true); return; }
  const meta = {
    name,
    vendor: $("sys-edit-vendor").value.trim(),
    description: $("sys-edit-desc").value.trim(),
    summary: $("sys-edit-summary").value.trim(),
  };
  let body = meta;
  if (!sys.builtin) {
    const now = $("sys-edit-json").value;
    if (now !== editingJSON) body = { json: now };
  }
  try {
    await api("/api/catalog/systems/" + encodeURIComponent(sys.id), {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    toast("已保存「" + name + "」");
    $("sys-edit").classList.add("hidden");
    editing = "";
    editingJSON = "";
    await refreshAll();
  } catch (e) { toast("保存失败: " + e.message, true); }
}

// ---------------- 初始化 ----------------

document.querySelectorAll("[data-sys-close]").forEach((el) => el.addEventListener("click", closeSystemManager));
$("sys-manage").addEventListener("click", openSystemManager);
// Tab 陷阱：弹层打开时焦点不出弹层（Esc 由 main.js 统一关闭）
$("sys-modal").addEventListener("keydown", (e) => {
  if (e.key !== "Tab") return;
  const list = [...$("sys-modal").querySelectorAll("button,input,select,textarea")]
    .filter((el) => !el.disabled && el.offsetParent !== null);
  if (!list.length) return;
  const first = list[0], last = list[list.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
});

export { openSystemManager, closeSystemManager, isSystemManagerOpen };
