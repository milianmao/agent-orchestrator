package codexappserver

import (
	"encoding/json"
	"errors"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"strings"
	"testing"
)

// TestMCPReferenceInjection 验证两种 transport 投递真实值，但原始快照只保留引用。
func TestMCPReferenceInjection(t *testing.T) {
	t.Setenv("MCP_REF_TEST", "private-test-value")
	servers := []ports.ChatMCPServerConfig{
		{Name: "stdio", Type: "stdio", Command: "mcp", EnvRefs: map[string]string{"TOKEN": "MCP_REF_TEST"}},
		{Name: "http", Type: "http", URL: "https://example.test/mcp", HeaderRefs: map[string]string{"Authorization": "MCP_REF_TEST"}},
	}
	resolved, err := resolveMCPReferences(servers)
	if err != nil {
		t.Fatal(err)
	}
	config := threadConfig("", resolved)["mcp_servers"].(map[string]any)
	if config["stdio"].(map[string]any)["env"].(map[string]string)["TOKEN"] != "private-test-value" {
		t.Fatal("stdio reference missing")
	}
	if config["http"].(map[string]any)["http_headers"].(map[string]string)["Authorization"] != "private-test-value" {
		t.Fatal("http reference missing")
	}
	data, _ := json.Marshal(servers)
	if strings.Contains(string(data), "private-test-value") {
		t.Fatal("input snapshot mutated")
	}
	c := &conversation{}
	c.registerMCPReferences(servers)
	if strings.Contains(c.mcpDiagnosticError(errors.New("provider echoed private-test-value")).Error(), "private-test-value") {
		t.Fatal("RPC error leaked secret")
	}
	servers[0].EnvRefs["TOKEN"] = "AO_MCP_REFERENCE_EXPECTED_MISSING_37"
	_, err = resolveMCPReferences(servers)
	if err == nil || !strings.Contains(err.Error(), "AO_MCP_REFERENCE_EXPECTED_MISSING_37") || strings.Contains(err.Error(), "private-test-value") {
		t.Fatalf("missing reference: %v", err)
	}
}
