package canvas

import (
	"io/fs"
	"strings"
	"testing"
)

// 原生壳契约：页面只新增一条 embed 分支，浏览器模式零变化。
// 这组断言锁住三件事：① 原生 URL 参数在样式前生效；② 品牌与主题按钮让给原生层；
// ③ state.js 侧把作用域/计数回写到 <html data-*>，原生标题条只读这些值。
func TestNativeEmbedContract(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := fs.ReadFile(staticFS, "static/"+name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(raw)
	}

	index := read("index.html")
	for _, want := range []string{
		`indexOf("embed=native")`,
		`document.documentElement.dataset.embed = "native"`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html 缺少原生内嵌 bootstrap：%q", want)
		}
	}
	// bootstrap 必须早于样式表：否则内嵌首帧会闪出品牌区
	if i, j := strings.Index(index, "embed=native"), strings.Index(index, "tokens.css"); i < 0 || j < 0 || i > j {
		t.Errorf("embed bootstrap 应先于 tokens.css（embed=%d, css=%d）", i, j)
	}

	css := read("canvas.css")
	for _, want := range []string{
		`html[data-embed="native"] .tb-brand`,
		`html[data-embed="native"] #btn-theme{display:none}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("canvas.css 缺少内嵌隐藏规则：%q", want)
		}
	}

	nativeJS := read("js/native.js")
	for _, want := range []string{
		"export function publishNativeState",
		`dataset.scope`,
		`dataset.scopeCount`,
		`dataset.projectCount`,
		`dataset.nodeCount`,
		`document.title`,
	} {
		if !strings.Contains(nativeJS, want) {
			t.Errorf("js/native.js 缺少契约字段：%q", want)
		}
	}

	mainJS := read("js/main.js")
	for _, want := range []string{
		`import { publishNativeState } from "./native.js"`,
		"setInterval(publishNativeState",
	} {
		if !strings.Contains(mainJS, want) {
			t.Errorf("main.js 缺少原生契约接线：%q", want)
		}
	}
}
