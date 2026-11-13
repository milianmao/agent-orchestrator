package sessionmanager

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/lifecycle"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

// TestProjectDirectoryRestartRestoreKeepsLeaseAndBase proves creation, restart,
// branch movement, and restore against the shipped database schema.
func TestProjectDirectoryRestartRestoreKeepsLeaseAndBase(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	repo := newManagerGitRepo(t)
	start := strings.TrimSpace(runManagerGit(t, repo, "rev-parse", "HEAD"))
	store := sqlitetest.MustOpenAt(t, dataDir)
	project := domain.ProjectRecord{ID: "project", Path: repo, Kind: domain.ProjectKindSingleRepo, Config: testRoleAgents(), RegisteredAt: time.Now()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	manager := newAdmittedTestManager(t, Deps{
		Runtime: &fakeRuntime{}, Agents: fakeAgents{}, Workspace: &fakeWorkspace{path: t.TempDir()},
		Store: store, Messenger: &fakeMessenger{}, Lifecycle: lifecycle.New(store, nil),
		DataDir: dataDir, LookPath: func(string) (string, error) { return "/bin/true", nil },
		Logger: slog.New(slog.DiscardHandler),
	})
	rec, _, _, err := manager.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "project", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
		WorkspaceMode: domain.WorkspaceModeProject, RequestedMode: domain.SessionModeTUI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Metadata.DiffBaseSHA != start || rec.Metadata.DiffBaseRef != start || rec.Metadata.WorkspacePath != repo {
		t.Fatalf("recorded base/path = %+v, want %q/%q/%q", rec.Metadata, start, start, repo)
	}
	if _, err := manager.Kill(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	rec, found, err := store.GetSession(ctx, rec.ID)
	if err != nil || !found {
		t.Fatalf("load terminated session: found=%v err=%v", found, err)
	}
	rec.Metadata.AgentSessionID = "project-native"
	rec.Metadata.Prompt = "continue"
	if err := store.UpdateSession(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	runManagerGit(t, repo, "checkout", "-b", "user-branch")
	runManagerGit(t, repo, "commit", "--allow-empty", "-m", "later branch head")
	current := runManagerGit(t, repo, "rev-parse", "HEAD")
	manager = newAdmittedTestManager(t, Deps{
		Runtime: &fakeRuntime{}, Agents: fakeAgents{}, Workspace: &fakeWorkspace{path: t.TempDir()},
		Store: store, Messenger: &fakeMessenger{}, Lifecycle: lifecycle.New(store, nil),
		DataDir: dataDir, LookPath: func(string) (string, error) { return "/bin/true", nil },
		Logger: slog.New(slog.DiscardHandler),
	})
	restored, err := manager.RestoreWithMode(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Session.Metadata.DiffBaseSHA != start || restored.Session.Metadata.DiffBaseRef != start {
		t.Fatalf("restore changed comparison base: %+v", restored.Session.Metadata)
	}
	if got := runManagerGit(t, repo, "rev-parse", "HEAD"); got != current {
		t.Fatalf("restore changed HEAD: %q -> %q", current, got)
	}
	if got := runManagerGit(t, repo, "branch", "--show-current"); got != "user-branch\n" {
		t.Fatalf("restore changed current branch: %q", got)
	}
}
