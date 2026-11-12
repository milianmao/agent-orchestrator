package controllers

import (
	"encoding/json"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apispec"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	settingssvc "github.com/aoagents/agent-orchestrator/backend/internal/service/settings"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
)

// decodeCodexMCPBody 拒绝真实值字段与尾随 JSON，错误不回显请求内容。
func decodeCodexMCPBody(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(into)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}
	if err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "validation", "MCP_CONFIG_INVALID", "MCP configuration must contain only supported reference fields", nil)
		return false
	}
	return true
}

// CodexMCPController 只暴露 Codex 的机器级引用配置，不操作 provider 文件。
type CodexMCPController struct{ Svc *settingssvc.CodexMCPService }

// Register 沿现有 API 权限边界挂载配置 CRUD；PATCH 同时承担显式启停。
func (c *CodexMCPController) Register(r chi.Router) {
	r.Get("/settings/codex/mcp", c.list)
	r.Post("/settings/codex/mcp", c.create)
	r.Patch("/settings/codex/mcp/{name}", c.patch)
	r.Delete("/settings/codex/mcp/{name}", c.delete)
}

// available 保持未装配服务的 501 行为。
func (c *CodexMCPController) available(w http.ResponseWriter, r *http.Request) bool {
	if c.Svc != nil {
		return true
	}
	apispec.NotImplemented(w, r, r.Method, "/api/v1/settings/codex/mcp")
	return false
}

func (c *CodexMCPController) list(w http.ResponseWriter, r *http.Request) {
	if !c.available(w, r) {
		return
	}
	configs, err := c.Svc.List(r.Context())
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, CodexMCPListResponse{Servers: configs})
}

func (c *CodexMCPController) create(w http.ResponseWriter, r *http.Request) {
	if !c.available(w, r) {
		return
	}
	var req CreateCodexMCPRequest
	if !decodeCodexMCPBody(w, r, &req) {
		return
	}
	if err := c.Svc.Save(r.Context(), req.CodexMCPConfig, true); err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, CodexMCPMutationResponse{NewSessionsOnly: true})
}

func (c *CodexMCPController) patch(w http.ResponseWriter, r *http.Request) {
	if !c.available(w, r) {
		return
	}
	var req PatchCodexMCPRequest
	if !decodeCodexMCPBody(w, r, &req) {
		return
	}
	if err := c.Svc.Patch(r.Context(), chi.URLParam(r, "name"), req.CodexMCPPatch); err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, CodexMCPMutationResponse{NewSessionsOnly: true})
}

func (c *CodexMCPController) delete(w http.ResponseWriter, r *http.Request) {
	if !c.available(w, r) {
		return
	}
	if err := c.Svc.Delete(r.Context(), chi.URLParam(r, "name")); err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, CodexMCPMutationResponse{NewSessionsOnly: true})
}
