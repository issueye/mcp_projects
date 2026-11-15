package native

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockNameScopedByDataDir(t *testing.T) {
	a := lockName(`C:\data\one`)
	b := lockName(`c:\data\ONE`) // Windows 路径大小写不敏感 → 同一把锁
	c := lockName(`C:\data\two`)
	if a != b {
		t.Fatalf("同一目录应得同一锁名：%q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("不同目录不应共用锁名：%q", a)
	}
	if !strings.HasPrefix(a, appTitle+"@") {
		t.Fatalf("锁名应带应用前缀：%q", a)
	}
	if lockName("") != appTitle {
		t.Fatalf("空数据目录应回落应用名")
	}
}

func TestWindowStateNormalize(t *testing.T) {
	got := windowState{Width: 10, Height: 10, X: 99999, Y: -99999, Theme: "blue"}.normalized()
	if got.Width != defaultWinW || got.Height != defaultWinH {
		t.Fatalf("过小的窗口应回落缺省：%+v", got)
	}
	if got.X != 0 || got.Y != 0 {
		t.Fatalf("离谱坐标应回到居中：%+v", got)
	}
	if got.Theme != "" {
		t.Fatalf("非法主题应被清空：%+v", got)
	}
	ok := windowState{Width: 1280, Height: 800, X: 40, Y: 30, Theme: "dark"}.normalized()
	if ok.Width != 1280 || ok.Height != 800 || ok.Theme != "dark" {
		t.Fatalf("合法值不应被改动：%+v", ok)
	}
}

func TestWindowStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := loadWindowState(dir); got.Width != defaultWinW || got.Height != defaultWinH {
		t.Fatalf("缺文件应回落缺省：%+v", got)
	}
	want := windowState{Width: 1280, Height: 800, X: 40, Y: 30, Maximized: true, Theme: "dark"}
	if err := saveWindowState(dir, want); err != nil {
		t.Fatal(err)
	}
	got := loadWindowState(dir)
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	// 坏 JSON 不应让窗口变成 0×0
	if err := os.WriteFile(filepath.Join(dir, "window.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadWindowState(dir); got.Width != defaultWinW || got.Height != defaultWinH {
		t.Fatalf("坏文件应回落缺省：%+v", got)
	}
}

func TestFetchHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "node_count": 41, "project_count": 4, "rev": 128,
			"persist_error": "disk full",
		})
	}))
	defer srv.Close()
	h, ok := fetchHealth(srv.Client(), srv.URL)
	if !ok {
		t.Fatal("健康检查应成功")
	}
	if !h.OK || h.NodeCount != 41 || h.ProjectCount != 4 || h.Rev != 128 || h.PersistError != "disk full" {
		t.Fatalf("解析异常：%+v", h)
	}
	if _, ok := fetchHealth(srv.Client(), ""); ok {
		t.Fatal("空地址应返回不可用")
	}
}

func TestPortLabel(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:8788": ":8788",
		"http://127.0.0.1":      "",
		"":                      "",
	}
	for in, want := range cases {
		if got := portLabel(in); got != want {
			t.Errorf("portLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanAge(t *testing.T) {
	if got := humanAge(time.Time{}); got != "—" {
		t.Errorf("零值 = %q", got)
	}
	if got := humanAge(time.Now().Add(-3 * time.Second)); !strings.HasSuffix(got, "s 前") {
		t.Errorf("3s = %q", got)
	}
	if got := humanAge(time.Now().Add(-2 * time.Minute)); !strings.HasSuffix(got, "m 前") {
		t.Errorf("2m = %q", got)
	}
	if got := humanAge(time.Now().Add(-2 * time.Hour)); !strings.HasSuffix(got, "h 前") {
		t.Errorf("2h = %q", got)
	}
}

func TestMCPConfig(t *testing.T) {
	s := &Shell{opt: Options{BaseURL: "http://127.0.0.1:8788/"}}
	got := s.mcpConfig()
	for _, want := range []string{"adago_design", "streamable-http", "127.0.0.1:8788/mcp"} {
		if !strings.Contains(got, want) {
			t.Errorf("MCP 配置缺 %q：%s", want, got)
		}
	}
}
