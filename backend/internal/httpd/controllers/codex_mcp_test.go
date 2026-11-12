package controllers

import (
	settingssvc "github.com/aoagents/agent-orchestrator/backend/internal/service/settings"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCodexMCPRoutes 验证真实 HTTP/服务/存储装配的 CRUD 与局部启停。
func TestCodexMCPRoutes(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	router := chi.NewRouter()
	(&CodexMCPController{Svc: &settingssvc.CodexMCPService{Store: store}}).Register(router)
	request := func(method, path, body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	base := "/settings/codex/mcp"
	request(http.MethodPost, base, `{"name":"tools","transport":"stdio","command":"mcp","enabled":true,"envRefs":{"TOKEN":"AO_MCP_TEST_MISSING"}}`, 201)
	request(http.MethodPatch, base+"/tools", `{"command":"updated"}`, 200)
	body := request(http.MethodGet, base, "", 200)
	if !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, "AO_MCP_TEST_MISSING") {
		t.Fatal(body)
	}
	request(http.MethodPatch, base+"/tools", `{"enabled":false}`, 200)
	body = request(http.MethodGet, base, "", 200)
	if !strings.Contains(body, `"enabled":false`) {
		t.Fatal(body)
	}
	request(http.MethodPost, base, `{"name":"bad","transport":"stdio","command":"mcp","env":{"TOKEN":"raw-secret"}}`, 400)
	request(http.MethodDelete, base+"/tools", "", 200)
	request(http.MethodPatch, base+"/tools", `{"enabled":true}`, 404)
}
