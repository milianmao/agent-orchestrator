package store_test

import (
	"context"
	"encoding/json"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	settingssvc "github.com/aoagents/agent-orchestrator/backend/internal/service/settings"
	"strings"
	"testing"
)

// TestCodexMCPCRUDAndSnapshot 验证真实存储、局部更新、启停与新旧会话隔离。
func TestCodexMCPCRUDAndSnapshot(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	svc := settingssvc.CodexMCPService{Store: s}
	t.Setenv("MCP_TEST_SECRET", "value-must-not-persist")
	c := domain.CodexMCPConfig{Name: "tools", Transport: "stdio", Enabled: true, Command: "mcp", EnvRefs: map[string]string{"TOKEN": "MCP_TEST_SECRET"}}
	if err := svc.Save(ctx, c, true); err != nil {
		t.Fatal(err)
	}
	command := "updated-mcp"
	if err := svc.Patch(ctx, c.Name, settingssvc.CodexMCPPatch{Command: &command}); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.List(ctx)
	if err != nil || len(listed) != 1 || !listed[0].Enabled || listed[0].Command != command {
		t.Fatalf("partial update: %+v, %v", listed, err)
	}
	seedProject(t, s, "mcp-project")
	rec, err := s.CreateSession(ctx, sampleRecord("mcp-project"))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := s.CaptureCodexMCPConfigs(ctx, rec.ID)
	if err != nil || len(frozen) != 1 || !frozen[0].Enabled {
		t.Fatalf("capture: %+v %v", frozen, err)
	}
	enabled := false
	if err := svc.Patch(ctx, c.Name, settingssvc.CodexMCPPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.List(ctx)
	if err != nil || listed[0].Enabled {
		t.Fatalf("disable: %+v %v", listed, err)
	}
	old, err := s.SessionCodexMCPConfigs(ctx, rec.ID)
	if err != nil || !old[0].Enabled {
		t.Fatalf("old session changed: %+v %v", old, err)
	}
	enabled = true
	if err := svc.Patch(ctx, c.Name, settingssvc.CodexMCPPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{listed, old} {
		data, _ := json.Marshal(value)
		if strings.Contains(string(data), "value-must-not-persist") {
			t.Fatal("secret returned or persisted")
		}
	}
	if err := svc.Delete(ctx, c.Name); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.List(ctx)
	if err != nil || len(listed) != 0 {
		t.Fatalf("delete: %+v %v", listed, err)
	}
}
