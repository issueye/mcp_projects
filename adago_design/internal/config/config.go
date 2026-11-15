// Package config 装配 adago-design 的运行配置。
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Version 是 adago-design 程序版本。
const Version = "0.1.0"

// UIKind 是界面形态：原生窗口 / 系统浏览器 / 无界面。
type UIKind string

const (
	// UINative 是 mygo 原生窗口内嵌画布页（serve 的缺省形态）。
	UINative UIKind = "native"
	// UIBrowser 是系统默认浏览器打开画布（旧行为，留作回退）。
	UIBrowser UIKind = "browser"
	// UINone 只跑服务，不打开任何界面（CI / 服务器 / -no-open）。
	UINone UIKind = "none"
)

// NormalizeUI 收敛界面形态写法；无法识别返回空（表示未指定）。
func NormalizeUI(s string) UIKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "native", "window", "app":
		return UINative
	case "browser", "web", "external":
		return UIBrowser
	case "none", "headless", "off", "false", "0":
		return UINone
	}
	return ""
}

// Config 是进程级配置。
type Config struct {
	Port    int    // 画布 HTTP 端口；8788 默认，被占用自动 +1 探测
	DataDir string // 数据目录（canvas.json 与导出 HTML）
	Open    bool   // 是否打开界面（-open / ADAGO_DESIGN_OPEN）
	Theme   string // 画布界面默认主题 dark|light

	Browser bool   // -browser：用系统浏览器代替原生窗口
	Window  bool   // -window：mcp 模式也创建原生窗口
	NoOpen  bool   // -no-open：只跑服务，不打开界面
	UIEnv   UIKind // ADAGO_DESIGN_UI 环境变量，优先于标志
}

// ResolveUI 按子命令与标志决定界面形态。
//
//	serve 缺省 → 原生窗口；mcp 缺省 → 无界面。
//	-no-open 最高优先；-browser / -window 次之；-open 再次。
//	ADAGO_DESIGN_UI 环境变量压过一切标志（便于 CI / 脚本固定形态）。
func (c *Config) ResolveUI(mode string) UIKind {
	if c.UIEnv != "" {
		return c.UIEnv
	}
	switch {
	case c.NoOpen:
		return UINone
	case c.Browser:
		return UIBrowser
	case c.Window:
		return UINative
	case c.Open:
		return UINative
	case mode == "mcp":
		return UINone
	default:
		return UINative
	}
}

// DefaultDataDir 返回默认数据目录：%APPDATA%\adago-design（可用环境变量覆盖）。
func DefaultDataDir() string {
	if v := os.Getenv("ADAGO_DESIGN_DATA"); v != "" {
		return v
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		if runtime.GOOS == "windows" {
			return `C:\ProgramData\adago-design`
		}
		return filepath.Join(os.TempDir(), "adago-design")
	}
	return filepath.Join(base, "adago-design")
}

// HasOpenFlag 判断参数里是否显式出现 -open。
func HasOpenFlag(args []string) bool {
	for _, a := range args {
		if a == "-open" || a == "--open" || strings.HasPrefix(a, "-open=") || strings.HasPrefix(a, "--open=") {
			return true
		}
	}
	return false
}

// FromFlags 从参数解析配置（子命令后缀的参数）。
func FromFlags(name string, args []string) (*Config, error) {
	cfg := &Config{}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.IntVar(&cfg.Port, "port", 8788, "画布 HTTP 端口")
	dataDir := fs.String("data", "", "数据目录（默认 %APPDATA%\\adago-design，可被 ADAGO_DESIGN_DATA 覆盖）")
	fs.BoolVar(&cfg.Open, "open", os.Getenv("ADAGO_DESIGN_OPEN") == "1", "打开界面（serve 缺省原生窗口）")
	fs.BoolVar(&cfg.NoOpen, "no-open", false, "只跑服务，不打开任何界面")
	fs.BoolVar(&cfg.Browser, "browser", false, "用系统默认浏览器代替原生窗口（排障/回退）")
	fs.BoolVar(&cfg.Window, "window", false, "mcp 模式下也创建原生窗口")
	theme := fs.String("theme", "light", "画布界面主题: light（纸面，缺省）|dark（墨纸）")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	} else {
		cfg.DataDir = DefaultDataDir()
	}
	// 主题白名单收敛：只接受 light/dark，其余按缺省（纸面）处理。
	// 注意：用户手动切换会写到 localStorage 并在页面 bootstrap 时覆盖此缺省。
	if strings.EqualFold(strings.TrimSpace(*theme), "dark") {
		cfg.Theme = "dark"
	} else {
		cfg.Theme = "light"
	}
	cfg.UIEnv = NormalizeUI(os.Getenv("ADAGO_DESIGN_UI"))
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("端口 %d 超出范围", cfg.Port)
	}
	return cfg, nil
}
