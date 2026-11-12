package domain

import "testing"

// TestCodexMCPValidation 验证引用与 transport 边界，避免凭据被误当作引用保存。
func TestCodexMCPValidation(t *testing.T) {
	for _, c := range []CodexMCPConfig{
		{Name: "tools", Transport: "stdio", Command: "mcp", EnvRefs: map[string]string{"TOKEN": "MCP_TOKEN"}},
		{Name: "remote", Transport: "http", URL: "https://example.test/mcp", HeaderRefs: map[string]string{"Authorization": "MCP_AUTH"}},
	} {
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []CodexMCPConfig{
		{Name: "ao", Transport: "stdio", Command: "mcp"},
		{Name: "tools", Transport: "stdio", Command: "mcp", EnvRefs: map[string]string{"TOKEN": "secret-value"}},
		{Name: "remote", Transport: "http", URL: "https://user:password@example.test/mcp"},
		{Name: "remote", Transport: "http", URL: "https://example.test/mcp?token=secret"},
		{Name: "remote", Transport: "http", URL: "https://example.test", Command: "mcp"},
	} {
		if err := c.Validate(); err == nil {
			t.Fatalf("accepted invalid MCP configuration")
		}
	}
}
