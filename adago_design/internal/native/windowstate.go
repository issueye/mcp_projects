package native

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 窗口记忆：退出前写 <data>/window.json，下次启动恢复。坏值一律回落缺省，
// 不让一份坏 JSON 把窗口变成 0×0 或屏幕外。
const (
	defaultWinW = 1440
	defaultWinH = 900
	minWinW     = 1040
	minWinH     = 640
	maxWinSide  = 8192
)

// windowState 是窗口几何与外观的可持久化形态。
type windowState struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Maximized bool   `json:"maximized"`
	Theme     string `json:"theme,omitempty"` // light|dark；空 = 用 -theme 缺省
}

func (s windowState) normalized() windowState {
	if s.Width < minWinW || s.Width > maxWinSide {
		s.Width = defaultWinW
	}
	if s.Height < minWinH || s.Height > maxWinSide {
		s.Height = defaultWinH
	}
	if s.Theme != "dark" && s.Theme != "light" {
		s.Theme = ""
	}
	// 位置全零 = 由系统居中；离谱坐标（拔掉显示器后留下的负值）也回到居中。
	if s.X > 20000 || s.X < -20000 {
		s.X = 0
	}
	if s.Y > 20000 || s.Y < -20000 {
		s.Y = 0
	}
	return s
}

func windowStatePath(dataDir string) string {
	return filepath.Join(dataDir, "window.json")
}

// loadWindowState 读窗口记忆；文件缺失或损坏都返回缺省值（不报错）。
func loadWindowState(dataDir string) windowState {
	def := windowState{Width: defaultWinW, Height: defaultWinH}
	if dataDir == "" {
		return def
	}
	raw, err := os.ReadFile(windowStatePath(dataDir))
	if err != nil {
		return def
	}
	var st windowState
	if err := json.Unmarshal(raw, &st); err != nil {
		return def
	}
	return st.normalized()
}

// saveWindowState 原子写窗口记忆（tmp → rename），失败只返回错误由调用方记日志。
func saveWindowState(dataDir string, st windowState) error {
	if dataDir == "" {
		return nil
	}
	st = st.normalized()
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	path := windowStatePath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
