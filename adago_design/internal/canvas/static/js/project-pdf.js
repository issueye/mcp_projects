/* adago-design 画布 · 项目设计文稿导出为 PDF。
 *
 * 两种运行形态共用同一个只读文稿页 /projects/{id}/print：
 *   - 原生窗口（?embed=native）：POST /api/projects/{id}/pdf，服务端用
 *     隐藏窗口 + WebView2 PrintToPDF 渲染，直接下载 PDF 文件；
 *   - 浏览器（serve -browser）：没有内嵌渲染器，打开打印页 ?print=1，
 *     在打印对话框里「另存为 PDF」。
 *
 * 入口只挂在项目「更多」菜单与项目管理行（见 dock.js / panels.js）；
 * 文稿内容（封面 / 概览 / 索引 / 逐稿页）全部由服务端生成。
 */
"use strict";

import { S } from "./state.js";
import { toast, toastStay } from "./ui.js";
import { isEmbedded } from "./native.js";
import { safeName } from "./project-io.js";

function stamp() {
  const d = new Date();
  const p2 = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}${p2(d.getMonth() + 1)}${p2(d.getDate())}-${p2(d.getHours())}${p2(d.getMinutes())}`;
}

function downloadBlob(filename, blob) {
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

function projectLabel(pid) {
  const p = S.projects.find((x) => x.id === pid);
  return (p && p.name) || pid;
}

function openPrintPage(pid, name) {
  const url = "/projects/" + encodeURIComponent(pid) + "/print?print=1";
  // 不能给 window.open 传 noopener：那会让返回值恒为 null，把「成功打开」
  // 误判成「被拦截」。先拿句柄，再把 opener 断开（同源只读页，无信任问题）。
  const win = window.open(url, "_blank");
  if (!win) {
    toast("打印页被浏览器拦截：请允许弹窗后重试，或直接打开 " + url, true);
    return false;
  }
  try { win.opener = null; } catch (_) { /* 跨域/被策略禁止时忽略 */ }
  toast("已打开「" + name + "」的打印文稿页：在打印对话框选择「另存为 PDF」");
  return true;
}

// 导出项目设计文稿。id 省略时用当前画布作用域（「全部」作用域下提示先选项目）。
async function exportProjectPDF(id) {
  const pid = id || S.project;
  if (!pid) {
    toast("先在左侧项目树里选择一个项目，再导出 PDF；「全部」作用域没有唯一项目", true);
    return;
  }
  const name = projectLabel(pid);

  // 浏览器形态：没有 PrintToPDF，走打印页。原生形态也保留这条 501 回退。
  if (!isEmbedded()) {
    openPrintPage(pid, name);
    return;
  }

  toastStay("正在生成「" + name + "」的系统设计文稿…");
  try {
    const res = await fetch("/api/projects/" + encodeURIComponent(pid) + "/pdf", { method: "POST" });
    if (res.status === 501) {
      openPrintPage(pid, name);
      return;
    }
    if (!res.ok) {
      let payload = {};
      try { payload = await res.json(); } catch (_) { /* 非 JSON 错误体 */ }
      const err = (payload && payload.error) || {};
      const e = new Error(err.message || res.statusText || "导出失败");
      e.code = err.code;
      e.hint = err.hint;
      throw e;
    }
    const blob = await res.blob();
    if (!blob || blob.size === 0) throw new Error("服务端返回的 PDF 为空");
    downloadBlob(`${safeName(name)}-系统设计文稿-${stamp()}.pdf`, blob);
    toast("已导出「" + name + "」的系统设计文稿（" + Math.round(blob.size / 1024) +
      " KB，已存到系统下载目录；标题栏 ⋯ →「打开下载目录」可查看）");
  } catch (e) {
    toast("PDF 导出失败: " + e.message + (e.hint ? " · " + e.hint : ""), true);
  }
}

export { exportProjectPDF };
