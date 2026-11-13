package session

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// TestProjectDirectoryCompareUsesExactStart prevents branch/PR observations from rebasing the comparison.
func TestProjectDirectoryCompareUsesExactStart(t *testing.T) {
	metadata := domain.SessionMetadata{WorkspaceMode: domain.WorkspaceModeProject, DiffBaseSHA: "start", DiffBaseRef: "start"}
	got := resolveSessionWorkspaceCompare(context.Background(), metadata, "changed-default", []domain.PullRequest{{BaseSHA: "new-base"}})
	if got.BaseSHA != "start" || got.BaseRef != "start" || got.Mode != WorkspaceCompareBase {
		t.Fatalf("comparison drifted: %+v", got)
	}
	project := domain.ProjectRecord{ID: "p"}
	if err := validateTaskWorkspaceMode(project, "", "main", ""); err != nil {
		t.Fatalf("legacy worktree rejected: %v", err)
	}
	if err := validateTaskWorkspaceMode(project, domain.WorkspaceModeProject, "main", ""); err == nil {
		t.Fatal("project mode accepted baseBranch")
	}
}

// TestProjectDirectoryPrepareAndDelegate checks the public boundary without allocating a preparation.
func TestProjectDirectoryPrepareAndDelegate(t *testing.T) {
	st := newFakeStore()
	st.projects["ao"] = domain.ProjectRecord{ID: "ao", Path: newWorkspaceRepo(t)}
	cmd := &fakeCommander{}
	svc := &Service{store: st, manager: cmd}
	ctx := context.Background()
	if token, err := svc.PrepareTask(ctx, "ao", domain.WorkspaceModeProject, ""); err != nil || token != "" {
		t.Fatalf("project preparation = %q, %v", token, err)
	}
	if _, err := svc.DelegateTask(ctx, DelegateTaskInput{ProjectID: "ao", WorkspaceMode: domain.WorkspaceModeProject, Brief: "Task"}); err != nil {
		t.Fatal(err)
	}
	if cfg := cmd.spawnedCfg; cfg.WorkspaceMode != domain.WorkspaceModeProject || cfg.BaseBranch != "" || cfg.Branch != "" || cfg.TaskPreparation != "" {
		t.Fatalf("project spawn config = %+v", cfg)
	}
	st.sessions["busy"] = domain.SessionRecord{ID: "busy", ProjectID: "ao", Metadata: domain.SessionMetadata{WorkspaceMode: domain.WorkspaceModeProject}}
	_, err := svc.PrepareTask(ctx, "ao", domain.WorkspaceModeProject, "")
	var apiError *apierr.Error
	if !errors.As(err, &apiError) || apiError.Code != "PROJECT_DIRECTORY_BUSY" {
		t.Fatalf("busy preparation = %v", err)
	}
	if _, err := svc.PrepareTask(ctx, "ao", "", ""); err != nil {
		t.Fatalf("project lease blocked legacy worktree: %v", err)
	}
	for _, mode := range []domain.WorkspaceMode{domain.WorkspaceModeProject, "invalid"} {
		if _, err := svc.PrepareTask(ctx, "ao", mode, "main"); err == nil {
			t.Fatal("prepare accepted unsupported mode/base combination")
		}
		if _, err := svc.DelegateTask(ctx, DelegateTaskInput{ProjectID: "ao", WorkspaceMode: mode, BaseBranch: "main"}); err == nil {
			t.Fatal("delegate accepted unsupported mode/base combination")
		}
	}
}
