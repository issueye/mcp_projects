package design

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// 图标管理：停用 / 新增 / 编辑（分组/别名/SVG）/ 移除 / 恢复默认。
//
// 存放位置：
//   - <data>/custom-icons/<name>.json  自定义图标（名称 + 分组 + 别名 + SVG）
//   - <data>/icon-settings.json        内置图标的停用列表与分组/别名覆盖
//
// 内置图标（icon_paths.go，Builtin=true）可停用、可改分组/别名，但 SVG 只读、
// 不可删除；自定义图标可整体编辑与删除。停用只影响「选择面」（Icons/SearchIcons/
// IconCatalog/design_icons），精灵与渲染仍包含全部已注册图标，避免旧模板失效。
//
// 锁序：iconStoreMu（串行化写盘操作）→ iconMu（注册表 + 管理态）。
var (
	// ErrIconNotFound 目标图标不存在。
	ErrIconNotFound = errors.New("unknown icon")
	// ErrIconBuiltin 内置图标受保护（SVG 只读、不可删除）。
	ErrIconBuiltin = errors.New("builtin icon is protected")
	// ErrIconExists 图标名已被占用。
	ErrIconExists = errors.New("icon already exists")
	// ErrIconInvalid 参数或 SVG 不合法。
	ErrIconInvalid = errors.New("invalid icon")
)

const (
	iconSettingsFileName = "icon-settings.json"
	customIconsDirName   = "custom-icons"
	maxIconBytes         = 64 << 10
	maxIconNameLen       = 48
	maxIconTags          = 12
)

var (
	iconStoreMu sync.Mutex
	// 以下管理态与 iconRegistry 共用 iconMu 保护。
	iconDisabled   = map[string]bool{}
	iconOverridden = map[string]bool{}
	iconOriginals  = map[string]Icon{}

	iconNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	iconViewBoxRe   = regexp.MustCompile(`^-?[0-9.]+(?:[ ,]+-?[0-9.]+){3}$`)
	// 只允许基础图形、分组与渐变定义；禁用 script/foreignObject/text/image/style/use 等。
	iconElements = map[string]bool{
		"svg": true, "g": true, "defs": true,
		"path": true, "circle": true, "rect": true, "line": true,
		"polyline": true, "polygon": true, "ellipse": true,
		"linearGradient": true, "radialGradient": true, "stop": true,
		"clipPath": true,
	}
	iconGraphicElements = map[string]bool{
		"path": true, "circle": true, "rect": true, "line": true,
		"polyline": true, "polygon": true, "ellipse": true,
	}
	iconBannedSnippets = []string{
		"<!doctype", "<script", "<foreignobject", "<style", "<text",
		"<image", "<use", "<iframe", "<audio", "<video", "<embed",
		"<object", "javascript:", "data:",
	}
	iconURLRe       = regexp.MustCompile(`(?i)url\(\s*['"]?\s*(?:https?:|data:|javascript:)`)
	validIconGroups = map[string]bool{
		IconGroupArrow: true, IconGroupAction: true, IconGroupStatus: true,
		IconGroupNav: true, IconGroupMedia: true, IconGroupData: true,
	}
)

// iconOverride 是内置图标的分组/别名覆盖（不改 SVG）。
type iconOverride struct {
	Group string   `json:"group,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

func (o iconOverride) empty() bool { return o.Group == "" && len(o.Tags) == 0 }

// iconSettings 是 <data>/icon-settings.json 的结构。
type iconSettings struct {
	Disabled  []string                `json:"disabled,omitempty"`
	Overrides map[string]iconOverride `json:"overrides,omitempty"`
}

// iconFile 是 <data>/custom-icons/<name>.json 的结构。
// Design 是设计器的可再编辑源；SVG 是唯一交付物。两者不一致时以 SVG 为准，
// 手改 SVG（UpdateIcon 的 SVG patch）会清掉 Design（见 UpdateIcon）。
type iconFile struct {
	Name   string   `json:"name"`
	Group  string   `json:"group,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	SVG    string   `json:"svg"`
	Design *IconDoc `json:"design,omitempty"`
}

// IconInput 是「新增图标」的入参。SVG 与 Design 二选一（都给时 Design 优先）：
// Design 会先规范化再渲染成 SVG，保证设计稿与交付物同源。
type IconInput struct {
	Name   string
	Group  string
	Tags   []string
	SVG    string
	Design *IconDoc
}

// IconPatch 是「编辑图标」的部分更新：Group 为 nil 表示不改；TagsSet 表示整体替换别名。
// Design 非 nil 表示用设计稿整体替换 SVG；SVG 非空（且 Design 为 nil）表示手改源码。
type IconPatch struct {
	Group   *string
	Tags    []string
	TagsSet bool
	SVG     string
	Design  *IconDoc
}

// IconSettingsPath 返回图标设置文件路径。
func IconSettingsPath(dir string) string {
	return filepath.Join(dir, iconSettingsFileName)
}

// CustomIconsDir 返回自定义图标目录。
func CustomIconsDir(dir string) string {
	return filepath.Join(dir, customIconsDirName)
}

func readIconSettings(dir string) iconSettings {
	var st iconSettings
	if dir == "" {
		return st
	}
	raw, err := os.ReadFile(IconSettingsPath(dir))
	if err != nil {
		return st
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return iconSettings{}
	}
	return st
}

func writeIconSettings(dir string, st iconSettings) error {
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
	return writeFileAtomic(IconSettingsPath(dir), raw)
}

// writeFileAtomic 先写 .tmp 再 rename，避免半截 JSON 被下一次启动读到。
func writeFileAtomic(path string, raw []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ensureIconOriginalsLocked 首次管理操作前快照内置图标（含其注册时的分组/别名）。
func ensureIconOriginalsLocked() {
	if len(iconOriginals) > 0 {
		return
	}
	for name, ic := range iconRegistry {
		if ic.Builtin {
			iconOriginals[name] = ic
		}
	}
}

func setIconDisabledIn(st *iconSettings, name string, off bool) {
	kept := make([]string, 0, len(st.Disabled)+1)
	seen := map[string]bool{}
	for _, x := range st.Disabled {
		if x == name || seen[x] {
			continue
		}
		seen[x] = true
		kept = append(kept, x)
	}
	if off {
		kept = append(kept, name)
	}
	sort.Strings(kept)
	st.Disabled = kept
}

func applyIconOverride(ic Icon, ov iconOverride) Icon {
	if ov.Group != "" {
		ic.Group = ov.Group
	}
	if len(ov.Tags) > 0 {
		ic.Tags = ov.Tags
	}
	return ic
}

// LoadCustomIcons 加载 <data>/custom-icons/*.json；dir 为空时清空内存里的自定义图标。
// 返回成功加载的数量；单个文件失败不影响其他图标（返回第一个错误供日志）。
func LoadCustomIcons(dir string) (int, error) {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()

	iconMu.Lock()
	ensureIconOriginalsLocked()
	for name, ic := range iconRegistry {
		if !ic.Builtin {
			delete(iconRegistry, name)
		}
	}
	iconMu.Unlock()

	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(CustomIconsDir(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var firstErr error
	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(CustomIconsDir(dir), e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", e.Name(), err)
			}
			continue
		}
		var f iconFile
		if err := json.Unmarshal(raw, &f); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: JSON 解析失败: %w", e.Name(), err)
			}
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".json")
		if f.Name == "" {
			f.Name = stem
		}
		ic, err := iconFromFile(f)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", e.Name(), err)
			}
			continue
		}
		if ic.Name != stem {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: name %q 与文件名不一致", e.Name(), ic.Name)
			}
			continue
		}
		ic.Designed = f.Design != nil
		iconMu.Lock()
		if _, dup := iconRegistry[ic.Name]; dup {
			iconMu.Unlock()
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: 与已有图标重名", e.Name())
			}
			continue
		}
		registerIconLocked(ic, false)
		iconMu.Unlock()
		loaded++
	}
	return loaded, firstErr
}

// LoadIconSettings 套用 <data>/icon-settings.json：内置图标恢复原始副本后
// 应用覆盖，并重建停用集合。dir 为空时只重置内存态。
func LoadIconSettings(dir string) error {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	st := readIconSettings(dir)

	iconMu.Lock()
	defer iconMu.Unlock()
	ensureIconOriginalsLocked()
	for name, orig := range iconOriginals {
		iconRegistry[name] = orig
	}
	iconDisabled = map[string]bool{}
	iconOverridden = map[string]bool{}
	for _, name := range st.Disabled {
		if _, ok := iconRegistry[name]; ok {
			iconDisabled[name] = true
		}
	}
	for name, ov := range st.Overrides {
		orig, ok := iconOriginals[name]
		if !ok || ov.empty() {
			continue
		}
		iconRegistry[name] = applyIconOverride(orig, ov)
		iconOverridden[name] = true
	}
	return nil
}

// IconState 返回管理 UI 需要的标记。
func IconState(name string) (builtin, disabled, overridden, ok bool) {
	iconMu.RLock()
	defer iconMu.RUnlock()
	ic, ok := iconRegistry[name]
	if !ok {
		return
	}
	return ic.Builtin, iconDisabled[name], iconOverridden[name], true
}

// IsIconEnabled 判断图标当前是否在选择面可见（停用即不可见）。
func IsIconEnabled(name string) bool {
	iconMu.RLock()
	defer iconMu.RUnlock()
	_, ok := iconRegistry[name]
	return ok && !iconDisabled[name]
}

// SetIconDisabled 停用 / 启用一个图标并持久化，返回最新图标副本。
func SetIconDisabled(name, dir string, off bool) (Icon, error) {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	iconMu.Lock()
	defer iconMu.Unlock()
	ic, ok := iconRegistry[strings.TrimSpace(name)]
	if !ok {
		return Icon{}, fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	st := readIconSettings(dir)
	setIconDisabledIn(&st, ic.Name, off)
	if err := writeIconSettings(dir, st); err != nil {
		return Icon{}, err
	}
	if off {
		iconDisabled[ic.Name] = true
	} else {
		delete(iconDisabled, ic.Name)
	}
	return iconRegistry[ic.Name], nil
}

// CreateIcon 新增一个自定义图标并持久化（可来自 SVG 源码或设计文档）。
func CreateIcon(in IconInput, dir string) (Icon, error) {
	name, err := normalizeIconName(in.Name)
	if err != nil {
		return Icon{}, err
	}
	group, err := normalizeIconGroup(in.Group)
	if err != nil {
		return Icon{}, err
	}
	svg := strings.TrimSpace(in.SVG)
	var design *IconDoc
	if in.Design != nil {
		doc, docErr := NormalizeIconDoc(*in.Design)
		if docErr != nil {
			return Icon{}, docErr
		}
		rendered, renderErr := IconDocSVG(doc)
		if renderErr != nil {
			return Icon{}, renderErr
		}
		saved := doc
		design, svg = &saved, rendered
	}
	body, viewBox, err := parseIconSVG(svg)
	if err != nil {
		return Icon{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
	}
	tags := normalizeIconTags(in.Tags)

	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	iconMu.Lock()
	defer iconMu.Unlock()
	ensureIconOriginalsLocked()
	if _, dup := iconRegistry[name]; dup {
		return Icon{}, fmt.Errorf("%w: %s", ErrIconExists, name)
	}
	if err := writeIconFile(dir, iconFile{Name: name, Group: group, Tags: tags, SVG: svg, Design: design}); err != nil {
		return Icon{}, err
	}
	registerIconLocked(Icon{
		Name: name, Group: group, Tags: tags, Body: body, ViewBox: viewBox, Designed: design != nil,
	}, false)
	return iconRegistry[name], nil
}

// UpdateIcon 编辑图标：
//   - 内置：只接受分组/别名（写入 overrides），SVG 非空即 ErrIconBuiltin；
//   - 自定义：分组/别名/SVG 均可改，直接重写自己的 JSON 文件。
func UpdateIcon(name, dir string, patch IconPatch) (Icon, error) {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	iconMu.Lock()
	defer iconMu.Unlock()

	key := strings.TrimSpace(name)
	ic, ok := iconRegistry[key]
	if !ok {
		return Icon{}, fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	if ic.Builtin {
		if patch.Design != nil {
			return Icon{}, fmt.Errorf("%w: 内置图标设计稿只读；请以它为起点新建自定义图标", ErrIconBuiltin)
		}
		if strings.TrimSpace(patch.SVG) != "" {
			return Icon{}, fmt.Errorf("%w: 内置图标 SVG 只读", ErrIconBuiltin)
		}
		st := readIconSettings(dir)
		ov := st.Overrides[key]
		if patch.Group != nil {
			group, err := normalizeIconGroup(*patch.Group)
			if err != nil {
				return Icon{}, err
			}
			ov.Group = group
		}
		if patch.TagsSet {
			ov.Tags = normalizeIconTags(patch.Tags)
		}
		if st.Overrides == nil {
			st.Overrides = map[string]iconOverride{}
		}
		if ov.empty() {
			delete(st.Overrides, key)
		} else {
			st.Overrides[key] = ov
		}
		if err := writeIconSettings(dir, st); err != nil {
			return Icon{}, err
		}
		cp := applyIconOverride(iconOriginals[key], ov)
		iconRegistry[key] = cp
		iconOverridden[key] = !ov.empty()
		return cp, nil
	}

	f, hasFile := readIconFile(dir, key)
	if !hasFile {
		// 兼容只存在于内存的旧自定义图标（正常路径都有 JSON 文件）。
		f = iconFile{Name: key, SVG: formatIconSource(ic)}
	}
	f.Name = key
	f.Group, f.Tags = ic.Group, ic.Tags
	if patch.Group != nil {
		group, err := normalizeIconGroup(*patch.Group)
		if err != nil {
			return Icon{}, err
		}
		f.Group, ic.Group = group, group
	}
	if patch.TagsSet {
		ic.Tags = normalizeIconTags(patch.Tags)
		f.Tags = ic.Tags
	}
	switch {
	case patch.Design != nil:
		doc, err := NormalizeIconDoc(*patch.Design)
		if err != nil {
			return Icon{}, err
		}
		svg, err := IconDocSVG(doc)
		if err != nil {
			return Icon{}, err
		}
		body, viewBox, err := parseIconSVG(svg)
		if err != nil {
			return Icon{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
		}
		saved := doc
		f.SVG, f.Design = svg, &saved
		ic.Body, ic.ViewBox, ic.Filled, ic.Designed = body, viewBox, false, true
	case strings.TrimSpace(patch.SVG) != "":
		svg := strings.TrimSpace(patch.SVG)
		body, viewBox, err := parseIconSVG(svg)
		if err != nil {
			return Icon{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
		}
		// 手改源码后设计文档即失效：清掉，避免下次打开设计器看到旧形状。
		f.SVG, f.Design = svg, nil
		ic.Body, ic.ViewBox, ic.Designed = body, viewBox, false
	}
	if err := writeIconFile(dir, f); err != nil {
		return Icon{}, err
	}
	iconRegistry[key] = ic
	return ic, nil
}

// readIconFile 读取自定义图标 JSON；文件不存在或解析失败时返回 false。
func readIconFile(dir, name string) (iconFile, bool) {
	if dir == "" || name == "" {
		return iconFile{}, false
	}
	raw, err := os.ReadFile(filepath.Join(CustomIconsDir(dir), name+".json"))
	if err != nil {
		return iconFile{}, false
	}
	var f iconFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return iconFile{}, false
	}
	return f, true
}

// ResetIcon 恢复内置图标的分组/别名并重新启用（自定义图标不支持重置）。
func ResetIcon(name, dir string) (Icon, error) {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	iconMu.Lock()
	defer iconMu.Unlock()

	key := strings.TrimSpace(name)
	ic, ok := iconRegistry[key]
	if !ok {
		return Icon{}, fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	if !ic.Builtin {
		return Icon{}, fmt.Errorf("%w: 自定义图标请直接编辑或删除", ErrIconInvalid)
	}
	st := readIconSettings(dir)
	delete(st.Overrides, key)
	setIconDisabledIn(&st, key, false)
	if err := writeIconSettings(dir, st); err != nil {
		return Icon{}, err
	}
	cp := iconOriginals[key]
	iconRegistry[key] = cp
	delete(iconDisabled, key)
	delete(iconOverridden, key)
	return cp, nil
}

// RemoveIcon 删除一个自定义图标（内置图标受保护，返回 ErrIconBuiltin）。
func RemoveIcon(name, dir string) error {
	iconStoreMu.Lock()
	defer iconStoreMu.Unlock()
	iconMu.Lock()
	defer iconMu.Unlock()

	key := strings.TrimSpace(name)
	ic, ok := iconRegistry[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	if ic.Builtin {
		return fmt.Errorf("%w: %s（请改为停用）", ErrIconBuiltin, key)
	}
	st := readIconSettings(dir)
	delete(st.Overrides, key)
	setIconDisabledIn(&st, key, false)
	if err := writeIconSettings(dir, st); err != nil {
		return err
	}
	if dir != "" {
		path := filepath.Join(CustomIconsDir(dir), key+".json")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	delete(iconRegistry, key)
	delete(iconDisabled, key)
	delete(iconOverridden, key)
	return nil
}

// IconSource 返回图标的完整 SVG 源码（归一化换行，供详情与编辑）。
func IconSource(name string) (string, error) {
	ic, ok := GetIcon(name)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	return formatIconSource(ic), nil
}

// IconDesignInfo 是「打开设计器」需要的一份完整上下文。
type IconDesignInfo struct {
	Doc      IconDoc // 可编辑的几何文档
	Imported bool    // 由 SVG 反推（没有保存过 design），形状可能只是近似拆分
	Builtin  bool    // 内置图标：只读，可「以它为起点」另存为自定义
	Designed bool    // 自定义图标且已有 design 字段（可原地继续编辑）
}

// IconDesign 返回图标的设计文档。自定义图标优先读已保存的 design；
// 没有 design 的（包括全部内置图标）从当前 SVG 反推，Imported=true。
// 含渐变/裁剪/transform 的 SVG 无法反推，返回 ErrIconInvalid，让 UI 退回源码编辑。
func IconDesign(name, dir string) (IconDesignInfo, error) {
	ic, ok := GetIcon(name)
	if !ok {
		return IconDesignInfo{}, fmt.Errorf("%w: %s", ErrIconNotFound, name)
	}
	if !ic.Builtin && ic.Designed {
		if f, has := readIconFile(dir, ic.Name); has && f.Design != nil {
			doc, err := NormalizeIconDoc(*f.Design)
			if err != nil {
				return IconDesignInfo{}, err
			}
			return IconDesignInfo{Doc: doc, Designed: true}, nil
		}
	}
	src, err := IconSource(ic.Name)
	if err != nil {
		return IconDesignInfo{}, err
	}
	doc, err := ImportIconDoc(src)
	if err != nil {
		return IconDesignInfo{}, fmt.Errorf("%w: 该图标含设计器暂不支持的 SVG 结构（渐变/裁剪/transform），可继续用源码编辑", err)
	}
	return IconDesignInfo{Doc: doc, Imported: true, Builtin: ic.Builtin}, nil
}

// IconUsage 返回精灵引用写法。
func IconUsage(name string) string {
	ic, ok := GetIcon(name)
	if !ok {
		return ""
	}
	return `<svg class="ds-icon ds-icon--20"><use href="#ds-ic-` + ic.Name + `"/></svg>`
}

// ---------------- 校验与序列化 ----------------

func normalizeIconName(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if len(n) == 0 || len(n) > maxIconNameLen || !iconNamePattern.MatchString(n) {
		return "", fmt.Errorf("%w: 名称只能是小写字母/数字/连字符（1-%d 位）", ErrIconInvalid, maxIconNameLen)
	}
	return n, nil
}

func normalizeIconGroup(group string) (string, error) {
	g := strings.ToLower(strings.TrimSpace(group))
	if g == "" {
		return IconGroupAction, nil
	}
	if !validIconGroups[g] {
		return "", fmt.Errorf("%w: 分组只能是 arrow/action/status/nav/media/data", ErrIconInvalid)
	}
	return g, nil
}

func normalizeIconTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || len(t) > 32 {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) == maxIconTags {
			break
		}
	}
	return out
}

func formatIconSource(ic Icon) string {
	body := strings.ReplaceAll(ic.Body, "><", ">\n<")
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + ic.ViewBox + `">` + "\n" + body + "\n</svg>"
}

func iconFromFile(f iconFile) (Icon, error) {
	name, err := normalizeIconName(f.Name)
	if err != nil {
		return Icon{}, err
	}
	group, err := normalizeIconGroup(f.Group)
	if err != nil {
		return Icon{}, err
	}
	body, viewBox, err := parseIconSVG(f.SVG)
	if err != nil {
		return Icon{}, fmt.Errorf("%w: %v", ErrIconInvalid, err)
	}
	return Icon{
		Name: name, Group: group, Tags: normalizeIconTags(f.Tags),
		Body: body, ViewBox: viewBox,
	}, nil
}

func writeIconFile(dir string, f iconFile) error {
	if dir == "" {
		return errors.New("图标目录未配置（data dir 为空）")
	}
	dirPath := CustomIconsDir(dir)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dirPath, f.Name+".json"), raw)
}

// parseIconSVG 校验并拆分 SVG：返回 body（不含 <svg> 外壳）与 viewBox。
// 只允许基础图形与渐变定义；拦截脚本、外链、事件属性、文本与 style。
func parseIconSVG(raw string) (body, viewBox string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("SVG 不能为空")
	}
	if len(s) > maxIconBytes {
		return "", "", fmt.Errorf("SVG 超过 %d KB", maxIconBytes/1024)
	}
	low := strings.ToLower(s)
	for _, banned := range iconBannedSnippets {
		if strings.Contains(low, banned) {
			return "", "", fmt.Errorf("包含不允许的内容：%s", banned)
		}
	}

	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = true
	depth := 0
	rootIsSVG := false
	sawGraphic := false
	for {
		tok, tokErr := dec.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return "", "", fmt.Errorf("SVG 解析失败：%v", tokErr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if name == "svg" {
				if depth > 0 {
					return "", "", errors.New("不支持嵌套 <svg>")
				}
				rootIsSVG = true
			} else if !iconElements[name] {
				return "", "", fmt.Errorf("不支持的元素 <%s>", name)
			}
			if name == "svg" && depth == 0 {
				for _, a := range t.Attr {
					if a.Name.Local == "viewBox" {
						viewBox = strings.TrimSpace(a.Value)
					}
				}
			}
			for _, a := range t.Attr {
				if err := checkIconAttr(a); err != nil {
					return "", "", err
				}
			}
			if iconGraphicElements[name] {
				sawGraphic = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return "", "", errors.New("不支持文本节点")
			}
		}
	}
	if !sawGraphic {
		return "", "", errors.New("至少要有一个图形元素（path/circle/rect 等）")
	}
	if viewBox == "" {
		viewBox = IconDefaultViewBox
	} else if !iconViewBoxRe.MatchString(viewBox) {
		return "", "", fmt.Errorf("viewBox 格式不合法：%s（应为 0 0 24 24）", viewBox)
	}
	if rootIsSVG {
		start := strings.Index(s, ">")
		end := strings.LastIndex(s, "</svg>")
		if start < 0 || end <= start {
			return "", "", errors.New("SVG 结构不完整")
		}
		body = strings.TrimSpace(s[start+1 : end])
	} else {
		body = s
	}
	if body == "" {
		return "", "", errors.New("SVG 内容为空")
	}
	return body, viewBox, nil
}

func checkIconAttr(a xml.Attr) error {
	local := strings.ToLower(a.Name.Local)
	switch {
	case strings.HasPrefix(local, "on"):
		return fmt.Errorf("不允许事件属性 %s", a.Name.Local)
	case local == "style":
		return errors.New("不允许 style 属性（请用元素属性）")
	case local == "href":
		if v := strings.TrimSpace(a.Value); !strings.HasPrefix(v, "#") {
			return errors.New("href 只能引用文档内 #id")
		}
	case iconURLRe.MatchString(a.Value):
		return errors.New("不允许外链 url(...)")
	}
	return nil
}
