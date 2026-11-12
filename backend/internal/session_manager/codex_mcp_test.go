package sessionmanager

import (
	"context"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"testing"
)

type mcpSnapshotTestStore struct {
	Store
	configs  []domain.CodexMCPConfig
	captures int
}

func (s *mcpSnapshotTestStore) CaptureCodexMCPConfigs(context.Context, domain.SessionID) ([]domain.CodexMCPConfig, error) {
	s.captures++
	return s.configs, nil
}
func (s *mcpSnapshotTestStore) SessionCodexMCPConfigs(context.Context, domain.SessionID) ([]domain.CodexMCPConfig, error) {
	return s.configs, nil
}

// TestCodexMCPEnabledSelection 验证禁用不注入、恢复不重读全局，以及其他 harness 隔离。
func TestCodexMCPEnabledSelection(t *testing.T) {
	store := &mcpSnapshotTestStore{configs: []domain.CodexMCPConfig{
		{Name: "enabled", Transport: "stdio", Command: "mcp", Enabled: true, EnvRefs: map[string]string{"TOKEN": "TOKEN_REF"}},
		{Name: "disabled", Transport: "http", URL: "https://example.test", Enabled: false},
	}}
	m := Manager{store: store}
	servers, err := m.codexMCPServers(context.Background(), "session", domain.HarnessCodex, true)
	if err != nil || len(servers) != 1 || servers[0].Name != "enabled" || servers[0].EnvRefs["TOKEN"] != "TOKEN_REF" || len(servers[0].Env) != 0 {
		t.Fatalf("selection: %+v %v", servers, err)
	}
	if _, err := m.codexMCPServers(context.Background(), "session", domain.HarnessCodex, false); err != nil || store.captures != 1 {
		t.Fatalf("restore recaptured: %d %v", store.captures, err)
	}
	servers, err = m.codexMCPServers(context.Background(), "session", domain.HarnessClaudeCode, true)
	if err != nil || len(servers) != 0 || store.captures != 1 {
		t.Fatalf("other harness injected: %+v %v", servers, err)
	}
}
