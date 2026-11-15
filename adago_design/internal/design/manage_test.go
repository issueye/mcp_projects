package design

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// resetRuntime 把注册表恢复到「无停用、无覆盖」的内存态（测试之间隔离）。
func resetRuntime(t *testing.T) {
	t.Helper()
	if err := LoadSystemSettings(""); err != nil {
		t.Fatal(err)
	}
}

func TestDisableSystemPersistsAndFilters(t *testing.T) {
	resetRuntime(t)
	t.Cleanup(func() { _ = LoadSystemSettings("") })

	const id = "ant"
	if !IsSystemEnabled(id) {
		t.Fatalf("%s 初始应启用", id)
	}
	dir := t.TempDir()
	t.Cleanup(func() {
		_, _ = SetSystemDisabled(id, dir, false)
		_ = LoadSystemSettings(dir)
	})

	if _, err := SetSystemDisabled(id, dir, true); err != nil {
		t.Fatal(err)
	}
	if IsSystemEnabled(id) {
		t.Fatal("停用后 IsSystemEnabled 应为 false")
	}
	if _, ok := GetSystem(id); ok {
		t.Fatal("停用后 GetSystem 应视为不存在")
	}
	for _, s := range Systems() {
		if s.ID == id {
			t.Fatal("Systems() 不应返回停用体系")
		}
	}
	found := false
	for _, s := range AllSystems() {
		if s.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("AllSystems() 管理面应包含停用体系")
	}
	raw, err := os.ReadFile(SystemSettingsPath(dir))
	if err != nil {
		t.Fatalf("应写出 system-settings.json: %v", err)
	}
	var st systemSettings
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Disabled) != 1 || st.Disabled[0] != id {
		t.Fatalf("disabled 列表异常: %+v", st.Disabled)
	}

	// 模拟重启：先清空内存，再从磁盘加载 → 仍是停用
	resetRuntime(t)
	if !IsSystemEnabled(id) {
		t.Fatal("内存重置后应恢复启用")
	}
	if err := LoadSystemSettings(dir); err != nil {
		t.Fatal(err)
	}
	if IsSystemEnabled(id) {
		t.Fatal("重启加载后应保持停用")
	}
}

func TestBuiltinMetaOverrideAndReset(t *testing.T) {
	resetRuntime(t)
	const id = "element"
	orig, ok := GetSystem(id)
	if !ok {
		t.Fatalf("%s 不存在", id)
	}
	origName, origVendor := orig.Name, orig.Vendor
	origPrimary := orig.Light.Colors["primary"].Value
	dir := t.TempDir()
	t.Cleanup(func() {
		_, _ = ResetSystem(id, dir)
		_ = LoadSystemSettings("")
	})

	patchedName, patchedVendor := "Element Plus（自定义名）", "测试厂商"
	desc, summary := "改过的描述", "改过的一句话"
	if _, err := UpdateSystemMeta(id, dir, MetaPatch{
		Name: &patchedName, Vendor: &patchedVendor, Description: &desc, Summary: &summary,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := GetSystem(id)
	if !ok || got.Name != patchedName || got.Vendor != patchedVendor ||
		got.Description != desc || got.Style.Summary != summary {
		t.Fatalf("显示信息未生效: %+v", got)
	}
	if _, _, over, _ := SystemState(id); !over {
		t.Fatal("SystemState 应标记 overridden")
	}
	if got.Light.Colors["primary"].Value != origPrimary {
		t.Fatal("覆盖显示信息不应改 Token")
	}

	// 覆盖随 system-settings.json 持久化
	resetRuntime(t)
	if err := LoadSystemSettings(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetSystem(id); got == nil || got.Name != patchedName {
		t.Fatalf("重启后覆盖丢失: %+v", got)
	}

	// 恢复默认并重新启用
	if _, err := SetSystemDisabled(id, dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ResetSystem(id, dir); err != nil {
		t.Fatal(err)
	}
	got, ok = GetSystem(id)
	if !ok || got.Name != origName || got.Vendor != origVendor || !IsSystemEnabled(id) {
		t.Fatalf("恢复默认失败: %+v (enabled=%v)", got, IsSystemEnabled(id))
	}
	if _, _, over, _ := SystemState(id); over {
		t.Fatal("恢复默认后不应再标记 overridden")
	}
}

func TestBuiltinProtectedFromRemoveAndOverwrite(t *testing.T) {
	resetRuntime(t)
	if err := RemoveSystem("apple", t.TempDir()); !errors.Is(err, ErrSystemBuiltin) {
		t.Fatalf("内置体系应受保护, got %v", err)
	}
	// 自定义体系不能借用内置 ID
	err := SaveCustomSystem(&System{ID: "apple", Name: "冒名", Light: Tokens{Grid: 4}}, t.TempDir())
	if !errors.Is(err, ErrSystemBuiltin) {
		t.Fatalf("SaveCustomSystem 应拒绝内置 ID, got %v", err)
	}
}

func TestCustomSystemLifecycle(t *testing.T) {
	resetRuntime(t)
	const id = "manage-probe"
	dir := t.TempDir()
	t.Cleanup(func() {
		_ = RemoveSystem(id, dir)
		_ = LoadSystemSettings("")
	})

	sys, err := CreateSystem(CreateInput{
		ID: id, Name: "Manage Probe", Vendor: "测试厂商", Family: "web",
		Description: "管理测试体系", Base: "volcano", Primary: "#C8412B",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if sys.ID != id || sys.Builtin || !IsSystemEnabled(id) {
		t.Fatalf("新增结果异常: %+v", sys)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-systems", id+".json")); err != nil {
		t.Fatalf("自定义体系文件未落盘: %v", err)
	}
	if _, err := CreateSystem(CreateInput{ID: id, Name: "Dup", Base: "volcano", Primary: "#112233"}, dir); !errors.Is(err, ErrSystemExists) {
		t.Fatalf("重复 ID 应拒绝, got %v", err)
	}
	if _, err := CreateSystem(CreateInput{Name: "Bad Base", Base: "nope", Primary: "#112233"}, dir); !errors.Is(err, ErrSystemNotFound) {
		t.Fatalf("未知基底应拒绝, got %v", err)
	}
	if _, err := CreateSystem(CreateInput{Name: "Bad Color", Base: "volcano", Primary: "red"}, dir); !errors.Is(err, ErrSystemInvalid) {
		t.Fatalf("非法主色应拒绝, got %v", err)
	}

	// 停用/启用
	if _, err := SetSystemDisabled(id, dir, true); err != nil {
		t.Fatal(err)
	}
	if IsSystemEnabled(id) {
		t.Fatal("自定义体系停用失败")
	}
	if _, err := SetSystemDisabled(id, dir, false); err != nil {
		t.Fatal(err)
	}

	// 显示信息编辑直接改自定义 JSON
	name := "Manage Probe 2"
	if _, err := UpdateSystemMeta(id, dir, MetaPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetSystem(id); got == nil || got.Name != name {
		t.Fatalf("自定义体系改名失败: %+v", got)
	}

	// 整体 JSON 编辑
	raw, err := SystemJSON(id)
	if err != nil {
		t.Fatal(err)
	}
	var parsed System
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	parsed.Description = "JSON 整体编辑"
	updatedRaw, _ := json.Marshal(&parsed)
	upd, err := UpdateCustomSystemJSON(id, dir, string(updatedRaw))
	if err != nil {
		t.Fatal(err)
	}
	if upd.Description != "JSON 整体编辑" {
		t.Fatal("JSON 编辑未生效")
	}
	// ID 不可通过 JSON 偷改
	parsed.ID = "other-id"
	badRaw, _ := json.Marshal(&parsed)
	if _, err := UpdateCustomSystemJSON(id, dir, string(badRaw)); !errors.Is(err, ErrSystemInvalid) {
		t.Fatalf("JSON 改 ID 应拒绝, got %v", err)
	}

	// 移除（内置保护之外的自定义体系）
	if err := RemoveSystem(id, dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetSystem(id); ok {
		t.Fatal("移除后仍能查到")
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-systems", id+".json")); !os.IsNotExist(err) {
		t.Fatalf("移除后文件应删除: %v", err)
	}
	if err := RemoveSystem(id, dir); !errors.Is(err, ErrSystemNotFound) {
		t.Fatalf("重复移除应报不存在, got %v", err)
	}
}

func TestCustomJSONRejectsBadTokens(t *testing.T) {
	resetRuntime(t)
	dir := t.TempDir()
	const id = "manage-json-probe"
	if _, err := CreateSystem(CreateInput{ID: id, Name: "JSON Probe", Base: "element", Primary: "#3366CC"}, dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveSystem(id, dir) })
	if _, err := UpdateCustomSystemJSON(id, dir, `{"name":"x","light":{"colors":{}}}`); !errors.Is(err, ErrSystemInvalid) {
		t.Fatalf("非法 Token JSON 应拒绝, got %v", err)
	}
}
