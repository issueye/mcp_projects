package config

import (
	"path/filepath"
	"testing"
)

func TestFromFlagsExplicit(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	t.Setenv("ADAGO_DESIGN_OPEN", "")

	dir := filepath.Join("tmp", "design")
	cfg, err := FromFlags("test", []string{
		"-port", "9123", "-data", dir, "-theme", "DARK", "-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9123 || cfg.DataDir != dir || cfg.Theme != "dark" || !cfg.Open {
		t.Fatalf("配置解析异常: %+v", cfg)
	}
}

func TestFromFlagsPortValidation(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	for _, port := range []string{"-1", "65536"} {
		if _, err := FromFlags("test", []string{"-port", port}); err == nil {
			t.Fatalf("端口 %s 应被拒绝", port)
		}
	}
}

func TestFromFlagsThemeWhitelist(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	cases := map[string]string{
		"":      "light", // 缺省纸面
		"blue":  "light", // 非法值收敛为缺省
		"dark":  "dark",
		"DARK":  "dark",
		" dark": "dark",
	}
	for in, want := range cases {
		args := []string{}
		if in != "" {
			args = []string{"-theme", in}
		}
		cfg, err := FromFlags("test", args)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Theme != want {
			t.Errorf("theme %q = %q, 期望 %q", in, cfg.Theme, want)
		}
	}
}

func TestFromFlagsOpenEnv(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	t.Setenv("ADAGO_DESIGN_OPEN", "1")
	cfg, err := FromFlags("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Open {
		t.Fatal("ADAGO_DESIGN_OPEN=1 时 -open 缺省应为 true")
	}

	t.Setenv("ADAGO_DESIGN_OPEN", "0")
	cfg, err = FromFlags("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Open {
		t.Fatal("ADAGO_DESIGN_OPEN=0 时 -open 缺省应为 false")
	}
}

func TestFromFlagsNoOpen(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	t.Setenv("ADAGO_DESIGN_OPEN", "1")
	cfg, err := FromFlags("test", []string{"-no-open"})
	if err != nil {
		t.Fatal(err)
	}
	// -no-open 只影响 serve 模式的默认打开行为（main 里处理），
	// 不改写 -open 自身的环境默认值。
	if !cfg.Open {
		t.Fatal("-no-open 不应改写 -open 的环境默认值")
	}
	if HasOpenFlag([]string{"-no-open"}) {
		t.Fatal("-no-open 不应被识别为显式 -open")
	}
}

func TestHasOpenFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"-open"}, true},
		{[]string{"--open"}, true},
		{[]string{"-open=true"}, true},
		{[]string{"--open=false"}, true},
		{[]string{"-no-open"}, false},
	}
	for _, c := range cases {
		if got := HasOpenFlag(c.args); got != c.want {
			t.Errorf("HasOpenFlag(%v) = %v, 期望 %v", c.args, got, c.want)
		}
	}
}

func TestDefaultDataDirEnvOverride(t *testing.T) {
	want := filepath.Join("custom", "design-data")
	t.Setenv("ADAGO_DESIGN_DATA", want)
	if got := DefaultDataDir(); got != want {
		t.Fatalf("DefaultDataDir() = %q, 期望 %q", got, want)
	}
}

func TestResolveUI(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_UI", "")
	cases := []struct {
		name string
		cfg  Config
		mode string
		want UIKind
	}{
		{"serve 缺省原生窗口", Config{}, "serve", UINative},
		{"mcp 缺省无界面", Config{}, "mcp", UINone},
		{"no-open 最高优先", Config{NoOpen: true, Browser: true, Open: true, Window: true}, "serve", UINone},
		{"browser 回退", Config{Browser: true}, "serve", UIBrowser},
		{"mcp -window", Config{Window: true}, "mcp", UINative},
		{"mcp -open", Config{Open: true}, "mcp", UINative},
		{"mcp -browser", Config{Browser: true}, "mcp", UIBrowser},
	}
	for _, c := range cases {
		if got := c.cfg.ResolveUI(c.mode); got != c.want {
			t.Errorf("%s: ResolveUI(%q) = %q, 期望 %q", c.name, c.mode, got, c.want)
		}
	}

	t.Setenv("ADAGO_DESIGN_UI", "none")
	if got := (&Config{UIEnv: UINone, Window: true}).ResolveUI("serve"); got != UINone {
		t.Errorf("ADAGO_DESIGN_UI 应压过标志，得 %q", got)
	}
}

func TestFromFlagsUIFlags(t *testing.T) {
	t.Setenv("ADAGO_DESIGN_DATA", "")
	t.Setenv("ADAGO_DESIGN_OPEN", "")
	t.Setenv("ADAGO_DESIGN_UI", "")

	cfg, err := FromFlags("test", []string{"-browser"})
	if err != nil || !cfg.Browser || cfg.Window || cfg.NoOpen {
		t.Fatalf("-browser 解析异常: %+v, %v", cfg, err)
	}
	cfg, err = FromFlags("test", []string{"-window"})
	if err != nil || !cfg.Window {
		t.Fatalf("-window 解析异常: %+v, %v", cfg, err)
	}
	cfg, err = FromFlags("test", []string{"-no-open"})
	if err != nil || !cfg.NoOpen {
		t.Fatalf("-no-open 解析异常: %+v, %v", cfg, err)
	}

	for raw, want := range map[string]UIKind{
		"native": UINative, "window": UINative,
		"browser": UIBrowser, "web": UIBrowser,
		"none": UINone, "headless": UINone,
		"": UIKind(""), "nonsense": UIKind(""),
	} {
		t.Setenv("ADAGO_DESIGN_UI", raw)
		cfg, err := FromFlags("test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.UIEnv != want {
			t.Errorf("ADAGO_DESIGN_UI=%q → %q, 期望 %q", raw, cfg.UIEnv, want)
		}
	}
}