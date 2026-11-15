/* 原生壳契约（WebView 内嵌时启用）
 *
 * 原生窗口的标题条不读 DOM，只读 <html data-*>：
 *   data-scope        当前作用域名（"全部项目" 或项目名）
 *   data-scope-count  当前作用域里的设计稿数
 *   data-project-count / data-node-count  全局计数（状态条用）
 *   data-theme        当前主题（原生层切换时也会写这里）
 * 同时把 document.title 写成「作用域 · adago-design」：浏览器模式下标签页也读得懂。
 *
 * 只在 embed=native 时可调度；浏览器模式仍会更新标题（对用户有好处）。
 */
"use strict";

import { S } from "./state.js";
import { projectName, totalNodes } from "./projects.js";

/** 当前页面是否跑在原生窗口里（index.html 的 bootstrap 写 data-embed）。 */
export function isEmbedded() {
  return document.documentElement.dataset.embed === "native";
}

/** 把作用域/计数/标题回写到 <html> 与 document.title（幂等、廉价）。 */
export function publishNativeState() {
  const html = document.documentElement;
  const scope = S.project ? projectName(S.project) : "全部项目";
  const scopeCount = S.project ? S.nodes.size : totalNodes();
  html.dataset.scope = scope;
  html.dataset.scopeCount = String(scopeCount);
  html.dataset.projectCount = String(S.projects.length);
  html.dataset.nodeCount = String(S.nodes.size);
  const title = scope + " · adago-design";
  if (document.title !== title) document.title = title;
}
