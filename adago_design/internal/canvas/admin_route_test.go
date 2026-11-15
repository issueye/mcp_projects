package canvas

import (
	"net/http"
	"testing"
)

// fakeAdmin 只验证路由/状态码/错误码映射；真实行为在 internal/app 与 internal/design 测。
type fakeAdmin struct{}

func (fakeAdmin) SystemSummariesAll() any {
	return []map[string]any{{"id": "fake", "name": "Fake", "disabled": true}}
}
func (fakeAdmin) SystemJSON(id string) (string, error) {
	if id != "fake" {
		return "", &AdminError{Code: "unknown_system", Message: "不存在"}
	}
	return `{"id":"fake"}`, nil
}
func (fakeAdmin) CreateSystem(in SystemInput) (any, error) {
	if in.Name == "" {
		return nil, &AdminError{Code: "invalid_system", Message: "name required"}
	}
	return map[string]any{"id": "created", "name": in.Name}, nil
}
func (fakeAdmin) UpdateSystem(id string, in SystemInput) (any, error) {
	if id == "missing" {
		return nil, &AdminError{Code: "unknown_system", Message: "不存在"}
	}
	return map[string]any{"id": id, "name": in.Name}, nil
}
func (fakeAdmin) SetSystemDisabled(id string, off bool) (any, error) {
	if id == "missing" {
		return nil, &AdminError{Code: "unknown_system", Message: "不存在"}
	}
	return map[string]any{"id": id, "disabled": off}, nil
}
func (fakeAdmin) ResetSystem(id string) (any, error) {
	return map[string]any{"id": id}, nil
}
func (fakeAdmin) RemoveSystem(id string) error {
	if id == "builtin" {
		return &AdminError{Code: "system_builtin_protected", Message: "受保护"}
	}
	return nil
}

func TestSystemAdminRoutes(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.Catalog = fakeCatalog{}
	srv.Admin = fakeAdmin{}

	// 管理面列表
	res, data := doJSON(t, "GET", ts.URL+"/api/catalog/systems?all=1", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("all list = %d", res.StatusCode)
	}
	list, _ := data["systems"].([]any)
	if len(list) != 1 || data["all"] != true {
		t.Fatalf("all list body = %v", data)
	}

	// 新增参数校验 → 400 + invalid_system
	res, data = doJSON(t, "POST", ts.URL+"/api/catalog/systems", map[string]any{})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create missing name = %d", res.StatusCode)
	}
	if e, _ := data["error"].(map[string]any); e["code"] != "invalid_system" {
		t.Fatalf("错误码 = %v", data)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/catalog/systems", map[string]any{"name": "X"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", res.StatusCode)
	}

	// 停用 / 启用
	res, _ = doJSON(t, "POST", ts.URL+"/api/catalog/systems/fake/state", map[string]any{"disabled": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("state = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/catalog/systems/fake/state", map[string]any{})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("state 缺 disabled = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/catalog/systems/missing/state", map[string]any{"disabled": true})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("未知体系 state = %d", res.StatusCode)
	}

	// JSON 读取
	res, _ = doJSON(t, "GET", ts.URL+"/api/catalog/systems/fake/json", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("json = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/catalog/systems/nope/json", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("未知体系 json = %d", res.StatusCode)
	}

	// 编辑 / 重置 / 删除
	res, _ = doJSON(t, "PUT", ts.URL+"/api/catalog/systems/fake", map[string]any{"name": "Fake 2"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/catalog/systems/fake/reset", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reset = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/catalog/systems/builtin", nil)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("内置删除 = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/catalog/systems/custom", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("自定义删除 = %d", res.StatusCode)
	}
}

func TestSystemAdminNotMounted(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.Catalog = fakeCatalog{}
	res, _ := doJSON(t, "GET", ts.URL+"/api/catalog/systems?all=1", nil)
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("未装配管理面 all=1 = %d", res.StatusCode)
	}
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/catalog/systems/fake", nil)
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("未装配管理面 delete = %d", res.StatusCode)
	}
}
