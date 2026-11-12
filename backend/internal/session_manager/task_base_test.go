package sessionmanager

import (
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestRestoreKeepsRecordedDiffBase 验证恢复不会用当前项目配置重算已记录的比较基线。
func TestRestoreKeepsRecordedDiffBase(t *testing.T) {
	m, st, _, ws := newManager()
	project := st.projects["mer"]
	project.Config.DefaultBranch = "newly-selected-base"
	st.projects["mer"] = project
	seedTerminal(st, "mer-1", domain.SessionMetadata{WorkspacePath: t.TempDir(), Branch: "task", AgentSessionID: "agent-x", DiffBaseSHA: "recorded-sha", DiffBaseRef: "refs/heads/recorded-topic"})
	result, err := m.RestoreWithMode(ctx, "mer-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.Metadata.DiffBaseSHA != "recorded-sha" || result.Session.Metadata.DiffBaseRef != "refs/heads/recorded-topic" {
		t.Fatalf("restore changed recorded base: %+v", result.Session.Metadata)
	}
	if len(ws.restoreConfigs) != 1 || ws.restoreConfigs[0].BaseRef != "refs/heads/recorded-topic" {
		t.Fatalf("restore used a different base: %+v", ws.restoreConfigs)
	}
}

// TestTaskPreparationClaimRequiresReadyMatchingBase 验证不匹配或未就绪 token 不转移清理所有权。
func TestTaskPreparationClaimRequiresReadyMatchingBase(t *testing.T) {
	m, st, _, _ := newManager()
	done := make(chan struct{})
	prep := &taskPreparation{project: st.projects["mer"], baseBranch: "refs/heads/topic", done: done, timer: time.NewTimer(time.Hour)}
	defer prep.timer.Stop()
	token := domain.TaskPreparationToken("token")
	m.taskPreparations[token] = prep
	if got := m.claimTaskPreparation(token, "mer", prep.baseBranch); got != nil {
		t.Fatal("claimed a preparation before it was ready")
	}
	close(done)
	if got := m.claimTaskPreparation(token, "mer", "refs/heads/other"); got != nil {
		t.Fatal("claimed a different base")
	}
	if m.taskPreparations[token] != prep {
		t.Fatal("cache miss lost cleanup ownership")
	}
	if got := m.claimTaskPreparation(token, "mer", prep.baseBranch); got != prep {
		t.Fatal("matching ready preparation was not claimed")
	}
	if got := m.claimTaskPreparation(token, "mer", prep.baseBranch); got != nil {
		t.Fatal("claimed token twice")
	}
}
