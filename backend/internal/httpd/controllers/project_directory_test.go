package controllers_test

import (
	"net/http"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestProjectDirectoryRequests carries both workspace fields across the HTTP boundary.
func TestProjectDirectoryRequests(t *testing.T) {
	svc := newFakeSessionService()
	srv := newSessionTestServer(t, svc)
	for _, mode := range []domain.WorkspaceMode{"", domain.WorkspaceModeWorktree, domain.WorkspaceModeProject} {
		base := ""
		if mode != domain.WorkspaceModeProject {
			base = "feature/base"
		}
		payload := `{"projectId":"ao","workspaceMode":"` + string(mode) + `","baseBranch":"` + base + `"}`
		body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/ao/tasks/prepare", payload)
		if status != http.StatusAccepted || svc.preparedWorkspaceMode != mode || svc.preparedBaseBranch != base {
			t.Fatalf("prepare = %d %s, mode=%s base=%s", status, body, svc.preparedWorkspaceMode, svc.preparedBaseBranch)
		}
		body, status, _ = doRequest(t, srv, http.MethodPost, "/api/v1/orchestrators/delegate", payload)
		if status != http.StatusAccepted || svc.delegationInput.WorkspaceMode != mode || svc.delegationInput.BaseBranch != base {
			t.Fatalf("delegate = %d %s, input=%+v", status, body, svc.delegationInput)
		}
	}
}
