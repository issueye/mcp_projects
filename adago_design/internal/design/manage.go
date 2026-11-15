package design

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// 设计体系管理：停用 / 用户可见信息覆盖 / 新建 / 编辑 / 移除。
//
// 存放位置：
//   - <data>/custom-systems/<id>.json   自定义体系正文（新增/编辑 JSON/移除都改这里）
//   - <data>/system-settings.json       停用列表 + 内置体系的显示信息覆盖
//
// 内置体系（data/*.json，Builtin=true）可停用、可改显示信息（overrides），
// 但不可删除；自定义体系可删除、可整体编辑 JSON。停用只影响目录/搜索/预览
// 等「选择面」，画布上已生成的设计稿不回收。

var (
	// ErrSystemNotFound 目标体系不存在。
	ErrSystemNotFound = errors.New("unknown system")
	// ErrSystemBuiltin 内置体系受保护（删不掉；请改为停用）。
	ErrSystemBuiltin = errors.New("builtin system is protected")
	// ErrSystemExists 体系 ID 已被占用。
	ErrSystemExists = errors.New("system id already exists")
	// ErrSystemInvalid 参数或体系 JSON 不合法。
	ErrSystemInvalid = errors.New("invalid system")
)

const settingsFileName = "system-settings.json"

var (
	settingsMu sync.Mutex
	// systemIDPattern 约束自定义体系 ID（slug，短且可写进 URL / system 参数）。
	systemIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	validFamilies   = map[string]bool{"mobile": true, "web": true, "cross": true}
)

// systemOverride 是内置体系的显示信息覆盖（不改 Token）。
type systemOverride struct {
	Name        string `json:"name,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Description string `json:"description,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

func (o systemOverride) empty() bool {
	return o.Name == "" && o.Vendor == "" && o.Description == "" && o.Summary == ""
}

// systemSettings 是 <data>/system-settings.json 的结构。
type systemSettings struct {
	Disabled  []string                  `json:"disabled,omitempty"`
	Overrides map[string]systemOverride `json:"overrides,omitempty"`
}

// SystemSettingsPath 返回设置文件路径（README / 测试 / 排障用）。
func SystemSettingsPath(dir string) string {
	return filepath.Join(dir, settingsFileName)
}

func readSystemSettings(dir string) systemSettings {
	var st systemSettings
	if dir == "" {
		return st
	}
	raw, err := os.ReadFile(SystemSettingsPath(dir))
	if err != nil {
		return st
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return systemSettings{}
	}
	return st
}

func writeSystemSettings(dir string, st systemSettings) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := SystemSettingsPath(dir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func applyOverride(s *System, ov systemOverride) {
	if ov.Name != "" {
		s.Name = ov.Name
	}
	if ov.Vendor != "" {
		s.Vendor = ov.Vendor
	}
	if ov.Description != "" {
		s.Description = ov.Description
	}
	if ov.Summary != "" {
		s.Style.Summary = ov.Summary
	}
}

func setDisabledIn(st *systemSettings, id string, off bool) {
	kept := make([]string, 0, len(st.Disabled)+1)
	seen := map[string]bool{}
	for _, x := range st.Disabled {
		if x == id || seen[x] {
			continue
		}
		seen[x] = true
		kept = append(kept, x)
	}
	if off {
		kept = append(kept, id)
	}
	sort.Strings(kept)
	st.Disabled = kept
}

// LoadSystemSettings 在启动时套用 <data>/system-settings.json：
// 把内置体系恢复成原始副本后套用覆盖，并重建停用集合。dir 为空时只重置内存态。
func LoadSystemSettings(dir string) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	st := readSystemSettings(dir)

	registryMu.Lock()
	defer registryMu.Unlock()
	for id, orig := range builtinOriginals {
		registry[id] = orig
	}
	disabled = map[string]bool{}
	overridden = map[string]bool{}
	for _, id := range st.Disabled {
		if _, ok := registry[id]; ok {
			disabled[id] = true
		}
	}
	for id, ov := range st.Overrides {
		orig, ok := builtinOriginals[id]
		if !ok || ov.empty() {
			continue
		}
		cp := *orig
		applyOverride(&cp, ov)
		registry[id] = &cp
		overridden[id] = true
	}
	return nil
}

// SetSystemDisabled 停用 / 启用一个体系并持久化，返回最新体系副本。
func SetSystemDisabled(id, dir string, off bool) (*System, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if _, ok := getSystemAny(id); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	st := readSystemSettings(dir)
	setDisabledIn(&st, id, off)
	if err := writeSystemSettings(dir, st); err != nil {
		return nil, err
	}
	registryMu.Lock()
	if off {
		disabled[id] = true
	} else {
		delete(disabled, id)
	}
	sys := registry[id]
	registryMu.Unlock()
	return sys, nil
}

// MetaPatch 是显示信息的部分更新；nil 表示不改，指向空串表示「恢复默认」。
type MetaPatch struct {
	Name        *string
	Vendor      *string
	Description *string
	Summary     *string
}

func applyMetaPatch(dst *systemOverride, p MetaPatch) {
	set := func(field *string, target *string) {
		if field == nil {
			return
		}
		*target = strings.TrimSpace(*field)
	}
	set(p.Name, &dst.Name)
	set(p.Vendor, &dst.Vendor)
	set(p.Description, &dst.Description)
	set(p.Summary, &dst.Summary)
}

// UpdateSystemMeta 更新体系的显示信息：
//   - 内置体系：写入 overrides（不碰 Token），空字段回落到原始值；
//   - 自定义体系：直接改自己的 JSON 文件（名字不能为空）。
func UpdateSystemMeta(id, dir string, patch MetaPatch) (*System, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	sys, ok := getSystemAny(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	if sys.Builtin {
		st := readSystemSettings(dir)
		ov := st.Overrides[id]
		applyMetaPatch(&ov, patch)
		if st.Overrides == nil {
			st.Overrides = map[string]systemOverride{}
		}
		if ov.empty() {
			delete(st.Overrides, id)
		} else {
			st.Overrides[id] = ov
		}
		if err := writeSystemSettings(dir, st); err != nil {
			return nil, err
		}
		registryMu.Lock()
		orig := builtinOriginals[id]
		cp := *orig
		applyOverride(&cp, ov)
		registry[id] = &cp
		overridden[id] = !ov.empty()
		registryMu.Unlock()
		out, _ := getSystemAny(id)
		return out, nil
	}

	cp := *sys
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrSystemInvalid)
		}
		cp.Name = name
	}
	if patch.Vendor != nil {
		cp.Vendor = strings.TrimSpace(*patch.Vendor)
	}
	if patch.Description != nil {
		cp.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Summary != nil {
		cp.Style.Summary = strings.TrimSpace(*patch.Summary)
	}
	if err := SaveCustomSystem(&cp, dir); err != nil {
		return nil, err
	}
	return &cp, nil
}

// ResetSystem 恢复内置体系的默认显示信息并重新启用（自定义体系不支持重置）。
func ResetSystem(id, dir string) (*System, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	sys, ok := getSystemAny(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	if !sys.Builtin {
		return nil, fmt.Errorf("%w: 自定义体系请直接编辑或删除", ErrSystemInvalid)
	}
	st := readSystemSettings(dir)
	delete(st.Overrides, id)
	setDisabledIn(&st, id, false)
	if err := writeSystemSettings(dir, st); err != nil {
		return nil, err
	}
	registryMu.Lock()
	orig := builtinOriginals[id]
	cp := *orig
	registry[id] = &cp
	delete(disabled, id)
	delete(overridden, id)
	registryMu.Unlock()
	return &cp, nil
}

// CreateInput 是「新增设计体系」的入参：以某个启用体系为结构基底，
// 用品牌主色重着色，再覆盖名称/厂商/落地类型/描述。
type CreateInput struct {
	ID          string
	Name        string
	Vendor      string
	Family      string
	Description string
	Base        string
	Primary     string
}

// CreateSystem 派生并持久化一个自定义体系。
func CreateSystem(in CreateInput, dir string) (*System, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrSystemInvalid)
	}
	baseID := SanitizeID(in.Base)
	if baseID == "" {
		return nil, fmt.Errorf("%w: base system is required", ErrSystemInvalid)
	}
	if _, ok := GetSystem(baseID); !ok {
		return nil, fmt.Errorf("%w: base system %q", ErrSystemNotFound, baseID)
	}
	derived, err := Derive(DeriveInput{Primary: in.Primary, Name: name, BaseSystemID: baseID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemInvalid, err)
	}
	if id := SanitizeID(in.ID); id != "" {
		if !systemIDPattern.MatchString(id) {
			return nil, fmt.Errorf("%w: id 只能是小写字母/数字/连字符（1-48 位）", ErrSystemInvalid)
		}
		if _, exists := getSystemAny(id); exists {
			return nil, fmt.Errorf("%w: %s", ErrSystemExists, id)
		}
		derived.ID = id
	}
	if _, exists := getSystemAny(derived.ID); exists {
		return nil, fmt.Errorf("%w: %s", ErrSystemExists, derived.ID)
	}
	if v := strings.TrimSpace(in.Vendor); v != "" {
		derived.Vendor = v
	}
	if f := strings.ToLower(strings.TrimSpace(in.Family)); f != "" {
		if !validFamilies[f] {
			return nil, fmt.Errorf("%w: family 只能是 mobile/web/cross", ErrSystemInvalid)
		}
		derived.Family = f
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		derived.Description = d
	}
	derived.Builtin = false
	if err := ValidateCustom(derived); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemInvalid, err)
	}
	if err := SaveCustomSystem(derived, dir); err != nil {
		return nil, err
	}
	return derived, nil
}

// UpdateCustomSystemJSON 整体替换一个自定义体系的 JSON（高级编辑）。
func UpdateCustomSystemJSON(id, dir, raw string) (*System, error) {
	sys, ok := getSystemAny(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	if sys.Builtin {
		return nil, fmt.Errorf("%w: 内置体系只能编辑显示信息", ErrSystemBuiltin)
	}
	var parsed System
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("%w: JSON 解析失败: %v", ErrSystemInvalid, err)
	}
	if parsed.ID != "" && SanitizeID(parsed.ID) != id {
		return nil, fmt.Errorf("%w: id 不可修改（当前 %s）", ErrSystemInvalid, id)
	}
	parsed.ID = id
	parsed.Builtin = false
	if err := ValidateCustom(&parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemInvalid, err)
	}
	if err := SaveCustomSystem(&parsed, dir); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// RemoveSystem 删除一个自定义体系（内置体系受保护，返回 ErrSystemBuiltin）。
func RemoveSystem(id, dir string) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	sys, ok := getSystemAny(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	if sys.Builtin {
		return fmt.Errorf("%w: %s（请改为停用）", ErrSystemBuiltin, id)
	}
	st := readSystemSettings(dir)
	delete(st.Overrides, id)
	setDisabledIn(&st, id, false)
	if err := writeSystemSettings(dir, st); err != nil {
		return err
	}
	unregisterSystem(id)
	if dir != "" {
		if err := os.Remove(filepath.Join(dir, "custom-systems", id+".json")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// SystemState 返回管理 UI 需要的标记。
func SystemState(id string) (builtin, isDisabled, isOverridden, ok bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok = registry[id]
	if !ok {
		return
	}
	return registry[id].Builtin, disabled[id], overridden[id], true
}

// PrimaryColor 取体系的品牌主色（不同体系命名不同，按常见键回退）。
func PrimaryColor(s *System) string {
	for _, k := range []string{"primary", "system-blue", "accent-blue", "brand"} {
		if c, ok := s.Light.Colors[k]; ok {
			return c.Value
		}
	}
	return ""
}

// SystemJSON 返回体系完整 JSON（自定义体系高级编辑用）。
func SystemJSON(id string) (string, error) {
	sys, ok := getSystemAny(id)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSystemNotFound, id)
	}
	raw, err := json.MarshalIndent(sys, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
