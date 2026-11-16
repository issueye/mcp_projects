package design

// 品牌 Logo 资产：<data>/logos/<id>.json。
//
// 与图标库一样，Logo 是「运行期可写」的一等资产：保存设计稿（LogoDoc）与
// 资产元信息（名称/备注/体系/时间），列表/详情/导出/移除都走同一份注册表。
// 与体系管理的区别：Logo 不做停用（它本身就是交付物），删除即从资产库移除；
// 画布上已经摆上的节点不回收（HTML 已自包含）。

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
	"time"
)

var (
	// ErrLogoNotFound 目标 Logo 不存在。
	ErrLogoNotFound = errors.New("unknown logo")
	// ErrLogoExists Logo ID 已存在。
	ErrLogoExists = errors.New("logo id already exists")
	// ErrLogoInvalid Logo 字段不合法。
	ErrLogoInvalid = errors.New("invalid logo")
)

const (
	logoDirName        = "logos"
	logoExportsDirName = "logo-exports"
	maxLogoNameLen     = 80
	maxLogoNoteLen     = 240
)

var (
	logoStoreMu sync.Mutex
	logoMu      sync.RWMutex
	logos       = map[string]Logo{}

	logoIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
)

// Logo 是一份持久化的品牌 Logo 资产。
type Logo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Note      string    `json:"note,omitempty"`
	System    string    `json:"system,omitempty"`
	Doc       LogoDoc   `json:"doc"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// LogoInput 是新增 / 编辑 Logo 的入参。Update 时 Name/Note/System 为空表示沿用旧值。
type LogoInput struct {
	ID     string  `json:"id,omitempty"`
	Name   string  `json:"name,omitempty"`
	Note   string  `json:"note,omitempty"`
	System string  `json:"system,omitempty"`
	Doc    LogoDoc `json:"doc"`
}

// LogoDir 返回 Logo 资产目录。
func LogoDir(dir string) string { return filepath.Join(dir, logoDirName) }

// LogoExportDir 返回 Logo SVG 导出目录。
func LogoExportDir(dir string) string { return filepath.Join(dir, logoExportsDirName) }

// LoadLogos 加载 <data>/logos/*.json；dir 为空时清空内存注册表。
func LoadLogos(dir string) (int, error) {
	logoStoreMu.Lock()
	defer logoStoreMu.Unlock()

	logoMu.Lock()
	logos = map[string]Logo{}
	logoMu.Unlock()
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(LogoDir(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	loaded := 0
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(LogoDir(dir), e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", e.Name(), err)
			}
			continue
		}
		var l Logo
		if err := json.Unmarshal(raw, &l); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: JSON 解析失败: %w", e.Name(), err)
			}
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".json")
		if l.ID == "" {
			l.ID = stem
		}
		if l.ID != stem || !logoIDPattern.MatchString(l.ID) {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: id 与文件名不一致或格式不合法", e.Name())
			}
			continue
		}
		if _, err := NormalizeLogoDoc(l.Doc); err != nil {
			// 自定义标记图标被删除后，Logo 不应整个丢失：降级为纯字标资产，
			// 用户仍可打开并换一个新标记。没有品牌名可降级时才跳过。
			degraded := false
			if l.Doc.Brand != "" {
				fallback := l.Doc
				fallback.Mark = LogoMark{Kind: "none"}
				fallback.Layout = LogoLayoutWordmark
				if _, ferr := NormalizeLogoDoc(fallback); ferr == nil {
					l.Doc = fallback
					degraded = true
				}
			}
			if !degraded {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", e.Name(), err)
				}
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: 标记图标缺失，已降级为纯字标", e.Name())
			}
		}
		if l.Name == "" {
			l.Name = l.ID
		}
		logoMu.Lock()
		logos[l.ID] = l
		logoMu.Unlock()
		loaded++
	}
	return loaded, firstErr
}

// AllLogos 返回全部 Logo（按更新时间倒序）。
func AllLogos() []Logo {
	logoMu.RLock()
	defer logoMu.RUnlock()
	out := make([]Logo, 0, len(logos))
	for _, l := range logos {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// GetLogo 按 ID 取 Logo。
func GetLogo(id string) (Logo, bool) {
	logoMu.RLock()
	defer logoMu.RUnlock()
	l, ok := logos[strings.ToLower(strings.TrimSpace(id))]
	return l, ok
}

// CreateLogo 新建 Logo 资产并持久化。
func CreateLogo(in LogoInput, dir string) (Logo, error) {
	doc, err := NormalizeLogoDoc(in.Doc)
	if err != nil {
		return Logo{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.TrimSpace(doc.Brand)
	}
	if name == "" {
		name = "未命名 Logo"
	}
	if runeLen(name) > maxLogoNameLen {
		return Logo{}, fmt.Errorf("%w: 名称超过 %d 个字符", ErrLogoInvalid, maxLogoNameLen)
	}
	note := strings.TrimSpace(in.Note)
	if runeLen(note) > maxLogoNoteLen {
		return Logo{}, fmt.Errorf("%w: 备注超过 %d 个字符", ErrLogoInvalid, maxLogoNoteLen)
	}
	system := strings.TrimSpace(in.System)
	requested := strings.ToLower(strings.TrimSpace(in.ID))
	if requested != "" && !logoIDPattern.MatchString(requested) {
		return Logo{}, fmt.Errorf("%w: id 只能是小写字母/数字/连字符（1-48 位）", ErrLogoInvalid)
	}

	logoStoreMu.Lock()
	defer logoStoreMu.Unlock()
	logoMu.Lock()
	defer logoMu.Unlock()
	id := requested
	if id == "" {
		base := logoSlug(name)
		if base == "" {
			base = "logo"
		}
		id = base
		for i := 2; ; i++ {
			if _, dup := logos[id]; !dup {
				break
			}
			id = fmt.Sprintf("%s-%d", base, i)
		}
	} else if _, dup := logos[id]; dup {
		return Logo{}, fmt.Errorf("%w: %s", ErrLogoExists, id)
	}

	now := time.Now().UTC()
	l := Logo{ID: id, Name: name, Note: note, System: system, Doc: doc, CreatedAt: now, UpdatedAt: now}
	if err := writeLogoFile(dir, l); err != nil {
		return Logo{}, err
	}
	logos[id] = l
	return l, nil
}

// UpdateLogo 全量更新一个 Logo（ID 不可改；Name 为空沿用旧名，Note/System 全量替换）。
func UpdateLogo(id string, in LogoInput, dir string) (Logo, error) {
	key := strings.ToLower(strings.TrimSpace(id))
	logoStoreMu.Lock()
	defer logoStoreMu.Unlock()
	logoMu.Lock()
	defer logoMu.Unlock()

	old, ok := logos[key]
	if !ok {
		return Logo{}, fmt.Errorf("%w: %s", ErrLogoNotFound, id)
	}
	doc, err := NormalizeLogoDoc(in.Doc)
	if err != nil {
		return Logo{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = old.Name
	}
	if runeLen(name) > maxLogoNameLen {
		return Logo{}, fmt.Errorf("%w: 名称超过 %d 个字符", ErrLogoInvalid, maxLogoNameLen)
	}
	note := strings.TrimSpace(in.Note)
	if runeLen(note) > maxLogoNoteLen {
		return Logo{}, fmt.Errorf("%w: 备注超过 %d 个字符", ErrLogoInvalid, maxLogoNoteLen)
	}
	// Note/System 是「全量替换」：显式清空要能落库；Name 留空才沿用旧名。
	system := strings.TrimSpace(in.System)
	l := Logo{
		ID: key, Name: name, Note: note, System: system, Doc: doc,
		CreatedAt: old.CreatedAt, UpdatedAt: time.Now().UTC(),
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = l.UpdatedAt
	}
	if err := writeLogoFile(dir, l); err != nil {
		return Logo{}, err
	}
	logos[key] = l
	return l, nil
}

// RemoveLogo 删除一个 Logo 资产。
func RemoveLogo(id, dir string) error {
	key := strings.ToLower(strings.TrimSpace(id))
	logoStoreMu.Lock()
	defer logoStoreMu.Unlock()
	logoMu.Lock()
	defer logoMu.Unlock()
	if _, ok := logos[key]; !ok {
		return fmt.Errorf("%w: %s", ErrLogoNotFound, id)
	}
	if dir != "" {
		path := filepath.Join(LogoDir(dir), key+".json")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	delete(logos, key)
	return nil
}

// LogoVariantSVG 返回指定变体的 SVG（用于详情/导出/画布）。
func LogoVariantSVG(id, variant string) (svg, filename string, err error) {
	l, ok := GetLogo(id)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrLogoNotFound, id)
	}
	variant = strings.ToLower(strings.TrimSpace(variant))
	if variant == "" {
		variant = LogoVariantPositive
	}
	svg, err = LogoSVG(l.Doc, variant)
	if err != nil {
		return "", "", err
	}
	filename = fmt.Sprintf("%s-%s.svg", l.ID, variant)
	return svg, filename, nil
}

func writeLogoFile(dir string, l Logo) error {
	if dir == "" {
		return errors.New("Logo 目录未配置（data dir 为空）")
	}
	if err := os.MkdirAll(LogoDir(dir), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(LogoDir(dir), l.ID+".json"), raw)
}

// logoSlug 从名称提取可写进 URL 的 ID；中文等非 ASCII 名称返回空，由调用方兜底。
func logoSlug(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '.' || r == '·':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// LogoSpecFor 返回 Logo 资产规范。
func LogoSpecFor(l Logo) (LogoSpec, error) {
	return LogoDocSpec(l.Doc)
}

// LogoCanvasHTML 返回可摆放画布的 HTML 与尺寸。
func LogoCanvasHTML(id, variant string) (html string, width, height int, err error) {
	l, ok := GetLogo(id)
	if !ok {
		return "", 0, 0, fmt.Errorf("%w: %s", ErrLogoNotFound, id)
	}
	return LogoHTML(l.Doc, variant)
}
