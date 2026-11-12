package specgen

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/controllers"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"net/http"
)

// codexMCPOperations 注册引用配置契约，保持运行路由与生成客户端一致。
func codexMCPOperations() []operation {
	common := []respUnit{{http.StatusBadRequest, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}}
	name := []any{controllers.CodexMCPNameParam{}}
	return []operation{
		{method: http.MethodGet, path: "/api/v1/settings/codex/mcp", id: "listCodexMCPConfigs", tag: "settings", summary: "List machine-shared Codex MCP reference configurations", resps: append([]respUnit{{http.StatusOK, controllers.CodexMCPListResponse{}}}, common...)},
		{method: http.MethodPost, path: "/api/v1/settings/codex/mcp", id: "createCodexMCPConfig", tag: "settings", summary: "Create a Codex MCP configuration for new sessions", reqBody: controllers.CreateCodexMCPRequest{}, resps: append([]respUnit{{http.StatusCreated, controllers.CodexMCPMutationResponse{}}}, common...)},
		{method: http.MethodPatch, path: "/api/v1/settings/codex/mcp/{name}", id: "patchCodexMCPConfig", tag: "settings", summary: "Update or enable a Codex MCP configuration for new sessions", pathParams: name, reqBody: controllers.PatchCodexMCPRequest{}, resps: append([]respUnit{{http.StatusOK, controllers.CodexMCPMutationResponse{}}}, common...)},
		{method: http.MethodDelete, path: "/api/v1/settings/codex/mcp/{name}", id: "deleteCodexMCPConfig", tag: "settings", summary: "Delete a Codex MCP configuration for new sessions", pathParams: name, resps: append([]respUnit{{http.StatusOK, controllers.CodexMCPMutationResponse{}}}, common...)},
	}
}
