package scm

import (
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestProjectDirectoryDoesNotClaimPR leaves automatic branch ownership to owned worktrees.
func TestProjectDirectoryDoesNotClaimPR(t *testing.T) {
	external := sessionRepo{branch: "user-branch", session: domain.SessionRecord{ID: "external", Metadata: domain.SessionMetadata{WorkspaceMode: domain.WorkspaceModeProject}}}
	if _, ok := matchSession([]sessionRepo{external}, "user-branch"); ok {
		t.Fatal("external checkout automatically claimed PR")
	}
	owned := sessionRepo{branch: "user-branch", session: domain.SessionRecord{ID: "owned"}}
	if got, ok := matchSession([]sessionRepo{external, owned}, "user-branch"); !ok || got.session.ID != "owned" {
		t.Fatal("external checkout blocked owned worktree PR matching")
	}
}
