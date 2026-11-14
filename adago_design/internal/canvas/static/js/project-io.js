/* adago-design 画布 · 项目导出 / 导入。
 *
 * 导出走服务端 bundle（项目 + 节点正文 + 连线 + 画布版式），浏览器侧存成 .json；
 * 导入是纯增量操作：服务端永远新建项目，同名自动加「（导入）」后缀，不会覆盖已有项目。
 * 文件选择入口挂在「项目 · 更多」菜单与项目管理行上（见 dock.js / panels.js）。
 */
"use strict";

import { S } from "./state.js";
import { toast, uiConfirm } from "./ui.js";
import { api } from "./net.js";
import { reloadProjects, switchProject } from "./projects.js";
import { pollNodes } from "./sync.js";

// 文件名安全化：只保留可读字符，去掉路径分隔符与首尾空白。
function safeName(name) {
  const clean = String(name || "adago-design")
    .replace(/[\\/:*?"<>|\u0000-\u001f]/g, "-")
    .replace(/\s+/g, " ")
    .trim();
  return (clean || "adago-design").slice(0, 48);
}

function stamp() {
  const d = new Date();
  const p2 = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}${p2(d.getMonth() + 1)}${p2(d.getDate())}-${p2(d.getHours())}${p2(d.getMinutes())}`;
}

function downloadJSON(filename, data) {
  const blob = new Blob([JSON.stringify(data)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.rel = "noopener";
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1500);
}

// 导出单个项目：id 省略时用当前画布作用域（「全部」作用域下提示先选项目）。
async function exportProject(id) {
  const pid = id || S.project;
  if (!pid) {
    toast("先在左侧项目树里选择一个项目，再导出；或使用「导出全部项目」", true);
    return;
  }
  const name = (S.projects.find((p) => p.id === pid) || {}).name;
  try {
    const bundle = await api("/api/projects/" + encodeURIComponent(pid) + "/export");
    downloadJSON(`${safeName(name || pid)}-${stamp()}.json`, bundle);
    toast("已导出项目「" + (name || pid) + "」");
  } catch (e) {
    toast("导出失败: " + e.message, true);
  }
}

async function exportAllProjects() {
  if (!S.projects.length) { toast("还没有项目可导出", true); return; }
  try {
    const bundle = await api("/api/export");
    downloadJSON(`adago-workspace-${stamp()}.json`, bundle);
    toast("已导出 " + S.projects.length + " 个项目");
  } catch (e) {
    toast("导出失败: " + e.message, true);
  }
}

function bundleList(data) {
  if (data && Array.isArray(data.projects)) return data.projects;
  if (data && data.project) return [data];
  return [];
}

// 导入文件：先本地解析做一次友好校验与确认，再交给服务端建项目。
async function importProjectFile(file) {
  if (!file) return;
  let bundle;
  try {
    bundle = JSON.parse(await file.text());
  } catch (e) {
    toast("文件不是合法的 JSON: " + e.message, true);
    return;
  }
  const bundles = bundleList(bundle);
  if (!bundles.length) {
    toast("没有找到项目数据：请选择由 adago-design 导出的 .json 文件", true);
    return;
  }
  const names = bundles.map((b) => (b.project && b.project.name) || "未命名项目");
  const nodeTotal = bundles.reduce((sum, b) => sum + ((b.nodes && b.nodes.length) || 0), 0);
  const ok = await uiConfirm({
    title: "导入项目",
    note:
      "将从「" + file.name + "」导入 " + bundles.length + " 个项目" +
      (nodeTotal ? "（约 " + nodeTotal + " 张设计稿）" : "") + "。\n" +
      "项目名：" + names.slice(0, 6).join("、") + (names.length > 6 ? " 等" : "") + "。\n" +
      "导入会新建项目，同名项目自动加「（导入）」后缀，不会覆盖现有内容。",
    okText: "导入",
  });
  if (!ok) return;
  try {
    const data = await api("/api/import", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(bundle),
    });
    await reloadProjects();
    const first = (data.projects || [])[0];
    if (first && first.id) {
      await switchProject(first.id);
    } else {
      await pollNodes({ full: true });
    }
    toast("已导入 " + (data.imported || bundles.length) + " 个项目、共 " +
      (data.node_count || 0) + " 个节点");
  } catch (e) {
    toast("导入失败: " + e.message + (e.hint ? " · " + e.hint : ""), true);
  }
}

export { exportProject, exportAllProjects, importProjectFile, safeName };
