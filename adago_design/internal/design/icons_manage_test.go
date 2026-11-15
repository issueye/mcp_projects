package design

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetIconStateForTest 清掉测试写入的全局图标与管理态（测试顺序无关）。
func resetIconStateForTest(t *testing.T) {
	t.Helper()
	_, _ = LoadCustomIcons("")
	_ = LoadIconSettings("")
	t.Cleanup(func() {
		_, _ = LoadCustomIcons("")
		_ = LoadIconSettings("")
	})
}

const brandMarkSVG = `<svg viewBox="0 0 48 48"><path d="M24 6l15 9v18l-15 9-15-9V15z" fill="none" stroke="currentColor" stroke-width="3"/><path d="M24 16l8 8-8 8-8-8z" fill="currentColor"/></svg>`

func TestCustomIconLifecycle(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()

	ic, err := CreateIcon(IconInput{
		Name: "probe-mark", Group: "media", Tags: []string{"probe-logo", "标记"}, SVG: brandMarkSVG,
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if ic.Builtin || ic.ViewBox != "0 0 48 48" || ic.Group != "media" {
		t.Fatalf("创建结果异常: %+v", ic)
	}
	if !IsIconEnabled(ic.Name) || !HasIcon(ic.Name) {
		t.Fatal("新图标应可检索")
	}
	if got := SearchIcons("probe-logo"); len(got) != 1 || got[0].Name != ic.Name {
		t.Fatalf("别名检索失败: %+v", got)
	}
	if raw, err := os.ReadFile(filepath.Join(CustomIconsDir(dir), ic.Name+".json")); err != nil ||
		!strings.Contains(string(raw), "probe-mark") {
		t.Fatalf("图标文件未写入: %v", err)
	}
	if _, err := CreateIcon(IconInput{Name: ic.Name, SVG: brandMarkSVG}, dir); !errors.Is(err, ErrIconExists) {
		t.Fatalf("重名应返回 ErrIconExists，得到 %v", err)
	}
	src, err := IconSource(ic.Name)
	if err != nil || !strings.Contains(src, `viewBox="0 0 48 48"`) {
		t.Fatalf("IconSource = %q, %v", src, err)
	}

	// 停用后从选择面消失，但精灵与渲染仍保留（旧模板不受影响）
	if _, err := SetIconDisabled(ic.Name, dir, true); err != nil {
		t.Fatal(err)
	}
	if IsIconEnabled(ic.Name) {
		t.Fatal("停用后 IsIconEnabled 应为 false")
	}
	for _, got := range Icons() {
		if got.Name == ic.Name {
			t.Fatal("停用后 Icons() 不应包含它")
		}
	}
	if !strings.Contains(IconSprite(), `id="ds-ic-`+ic.Name+`"`) {
		t.Fatal("精灵应保留停用图标")
	}
	if !strings.Contains(IconRef(ic.Name, 20), `#ds-ic-`+ic.Name) {
		t.Fatal("停用后仍应可渲染引用")
	}
	if _, err := SetIconDisabled(ic.Name, dir, false); err != nil {
		t.Fatal(err)
	}

	// 编辑：分组、别名、SVG 正文
	group := "action"
	if _, err := UpdateIcon(ic.Name, dir, IconPatch{
		Group: &group, Tags: []string{"mark-only"}, TagsSet: true,
		SVG: `<svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor"/></svg>`,
	}); err != nil {
		t.Fatal(err)
	}
	updated, _ := GetIcon(ic.Name)
	if updated.Group != "action" || updated.ViewBox != "0 0 16 16" || len(updated.Tags) != 1 {
		t.Fatalf("编辑未生效: %+v", updated)
	}
	if got := SearchIcons("probe-logo"); len(got) != 0 {
		t.Fatalf("别名替换后不应再命中旧词: %+v", got)
	}

	if _, err := ResetIcon(ic.Name, dir); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("自定义图标重置应被拒绝，得到 %v", err)
	}
	if err := RemoveIcon(ic.Name, dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetIcon(ic.Name); ok {
		t.Fatal("移除后不应还能取到")
	}
	if _, err := os.Stat(filepath.Join(CustomIconsDir(dir), ic.Name+".json")); !os.IsNotExist(err) {
		t.Fatalf("移除后文件应删除: %v", err)
	}
}

func TestBuiltinIconProtection(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	const name = "chevron-right"
	orig, ok := GetIcon(name)
	if !ok || !orig.Builtin {
		t.Fatal("chevron-right 应为内置图标")
	}

	if _, err := UpdateIcon(name, dir, IconPatch{SVG: `<path d="M0 0"/>`}); !errors.Is(err, ErrIconBuiltin) {
		t.Fatalf("内置 SVG 编辑应被拒绝，得到 %v", err)
	}
	if err := RemoveIcon(name, dir); !errors.Is(err, ErrIconBuiltin) {
		t.Fatalf("内置删除应被拒绝，得到 %v", err)
	}

	// 改分组/别名 → 覆盖生效并持久化
	group := "data"
	tags := []string{"right-chevron", "向右箭头"}
	ic, err := UpdateIcon(name, dir, IconPatch{Group: &group, Tags: tags, TagsSet: true})
	if err != nil {
		t.Fatal(err)
	}
	builtin, off, over, _ := IconState(name)
	if !builtin || off || !over || ic.Group != "data" {
		t.Fatalf("覆盖状态异常: builtin=%v off=%v over=%v group=%s", builtin, off, over, ic.Group)
	}
	if err := LoadIconSettings(dir); err != nil {
		t.Fatal(err)
	}
	if again, _ := GetIcon(name); again.Group != "data" || len(again.Tags) != 2 {
		t.Fatalf("重新加载后覆盖丢失: %+v", again)
	}

	// 停用持久化
	if _, err := SetIconDisabled(name, dir, true); err != nil {
		t.Fatal(err)
	}
	if err := LoadIconSettings(dir); err != nil {
		t.Fatal(err)
	}
	if IsIconEnabled(name) {
		t.Fatal("重新加载后停用状态丢失")
	}

	// 恢复默认：清覆盖 + 重新启用
	restored, err := ResetIcon(name, dir)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Group != orig.Group || !IsIconEnabled(name) {
		t.Fatalf("恢复默认失败: %+v", restored)
	}
	if _, _, over, _ := IconState(name); over {
		t.Fatal("恢复后不应再标记覆盖")
	}
}

func TestParseIconSVGValidation(t *testing.T) {
	bad := map[string]string{
		"脚本":        `<svg><script>alert(1)</script><path d="M0 0"/></svg>`,
		"文本":        `<svg><text x="0">Hi</text><path d="M0 0"/></svg>`,
		"事件属性":      `<svg><path d="M0 0" onclick="x()"/></svg>`,
		"外链 href":   `<svg><use href="https://example.com/x.svg"/></svg>`,
		"图片":        `<svg><image href="data:image/png;base64,AA"/></svg>`,
		"style":     `<svg><path d="M0 0" style="fill:red"/></svg>`,
		"空 SVG":     `<svg></svg>`,
		"坏 viewBox": `<svg viewBox="0 0 24"><path d="M0 0"/></svg>`,
		"空串":        ``,
	}
	for label, raw := range bad {
		if _, _, err := parseIconSVG(raw); err == nil {
			t.Errorf("%s 应被拒绝: %s", label, raw)
		}
	}
	cases := map[string]string{
		`<path d="M0 0"/>`: IconDefaultViewBox,
		`<svg viewBox="0 0 16 16"><circle cx="8" cy="8" r="6"/></svg>`: "0 0 16 16",
	}
	for raw, wantViewBox := range cases {
		body, viewBox, err := parseIconSVG(raw)
		if err != nil {
			t.Errorf("合法 SVG 被拒: %v", err)
			continue
		}
		if strings.TrimSpace(body) == "" || viewBox != wantViewBox {
			t.Errorf("parse(%q) = body=%q viewBox=%q", raw, body, viewBox)
		}
	}
	// 外链 url(...) 也要拦
	if _, _, err := parseIconSVG(`<svg><path d="M0 0" fill="url(https://x/y)"/></svg>`); err == nil {
		t.Error("外链 url() 应被拒绝")
	}
}

func TestLoadCustomIconsAndSettings(t *testing.T) {
	resetIconStateForTest(t)
	dir := t.TempDir()
	if err := os.MkdirAll(CustomIconsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	file := `{"name":"disk-probe","group":"status","tags":["probe-disk"],"svg":"` +
		strings.ReplaceAll(brandMarkSVG, `"`, `\"`) + `"}`
	if err := os.WriteFile(filepath.Join(CustomIconsDir(dir), "disk-probe.json"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	st := `{"disabled":["disk-probe"],"overrides":{"chevron-up":{"group":"nav"}}}`
	if err := os.WriteFile(IconSettingsPath(dir), []byte(st), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := LoadCustomIcons(dir)
	if err != nil || n != 1 {
		t.Fatalf("LoadCustomIcons = %d, %v", n, err)
	}
	if _, ok := GetIcon("disk-probe"); !ok {
		t.Fatal("磁盘上的自定义图标未注册")
	}
	if err := LoadIconSettings(dir); err != nil {
		t.Fatal(err)
	}
	if IsIconEnabled("disk-probe") {
		t.Fatal("停用列表应作用于自定义图标")
	}
	if up, _ := GetIcon("chevron-up"); up.Group != "nav" {
		t.Fatalf("内置覆盖未生效: %s", up.Group)
	}
}
