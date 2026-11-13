package sessionmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// TestProjectDirectoryLifecycle protects the checkout across each public teardown path.
func TestProjectDirectoryLifecycle(t *testing.T) {
	for _, operation := range []string{"kill", "archive", "replacement", "shutdown", "cleanup", "rollback", "delete"} {
		t.Run(operation, func(t *testing.T) {
			m, st, rt, ws := newManager()
			repo := newManagerGitRepo(t)
			project := st.projects["mer"]
			project.Path = repo
			project.Config.PostCreate = []string{"must-not-run"}
			project.Config.PreRemove = []string{"must-not-run"}
			st.projects["mer"] = project
			if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("user dirty work\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := runManagerGit(t, repo, "status", "--porcelain")
			head := strings.TrimSpace(runManagerGit(t, repo, "rev-parse", "HEAD"))
			branches := runManagerGit(t, repo, "branch", "--format=%(refname)")
			rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker,
				WorkspaceMode: domain.WorkspaceModeProject, RequestedMode: domain.SessionModeTUI, Prompt: "task", NameFromPrompt: true})
			if err != nil {
				t.Fatal(err)
			}
			if ws.createCount != 0 || len(ws.fetches) != 0 || rt.lastCfg.WorkspacePath != repo {
				t.Fatalf("project create used workspace adapter: create=%d fetch=%v cwd=%s", ws.createCount, ws.fetches, rt.lastCfg.WorkspacePath)
			}
			if rec.Metadata.WorkspaceMode != domain.WorkspaceModeProject || rec.Metadata.WorkspaceOwnership != domain.WorkspaceExternal || rec.Metadata.DiffBaseSHA != head || rec.Metadata.DiffBaseRef != head {
				t.Fatalf("project metadata = %+v", rec.Metadata)
			}
			switch operation {
			case "delete":
				_, err = m.Kill(ctx, rec.ID)
				m.store = &projectDirectoryDeleteStore{st}
				if err == nil {
					err = m.DeleteSession(ctx, rec.ID, true)
				}
				if _, found := st.sessions[rec.ID]; found {
					t.Fatal("session record was not deleted")
				}
			case "kill":
				_, err = m.Kill(ctx, rec.ID)
			case "archive":
				_, err = m.RequestKill(ctx, rec.ID)
			case "replacement":
				err = m.RetireForReplacement(ctx, rec.ID)
			case "shutdown":
				err = m.SaveAndTeardownAll(ctx)
			case "cleanup":
				err = m.lcm.MarkTerminated(ctx, rec.ID)
				if err == nil {
					_, err = m.Cleanup(ctx, rec.ProjectID)
				}
			case "rollback":
				if !m.destroySpawnWorkspace(ctx, workspaceInfo(rec), nil) {
					t.Fatal("external rollback failed")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if ws.destroyed != 0 || ws.stashCalls != 0 || len(ws.calls) != 0 {
				t.Fatalf("external teardown called adapter: %v", ws.calls)
			}
			if got := runManagerGit(t, repo, "status", "--porcelain"); got != before {
				t.Fatalf("dirty state changed: %q -> %q", before, got)
			}
			if got := strings.TrimSpace(runManagerGit(t, repo, "rev-parse", "HEAD")); got != head {
				t.Fatalf("HEAD changed: %s", got)
			}
			if got := runManagerGit(t, repo, "branch", "--format=%(refname)"); got != branches {
				t.Fatalf("branches changed: %s", got)
			}
			if got, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(got) != "user dirty work\n" {
				t.Fatalf("dirty file changed: %q", got)
			}
		})
	}
}

// TestProjectDirectoryRestoreKeepsStartCommit accepts a user's new branch without moving it back.
func TestProjectDirectoryRestoreKeepsStartCommit(t *testing.T) {
	m, st, _, ws := newManager()
	repo := newManagerGitRepo(t)
	project := st.projects["mer"]
	project.Path = repo
	st.projects["mer"] = project
	metadata, err := projectDirectoryMetadata(ctx, repo, domain.SessionMetadata{WorkspaceMode: domain.WorkspaceModeProject, WorkspaceOwnership: domain.WorkspaceExternal})
	if err != nil {
		t.Fatal(err)
	}
	metadata.AgentSessionID = "agent-x"
	seedTerminal(st, "mer-1", metadata)
	runManagerGit(t, repo, "checkout", "-b", "user-change")
	runManagerGit(t, repo, "commit", "--allow-empty", "-m", "later")
	head := runManagerGit(t, repo, "rev-parse", "HEAD")
	result, err := m.RestoreWithMode(ctx, "mer-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.restoreConfigs) != 0 || result.Session.Metadata.DiffBaseSHA != metadata.DiffBaseSHA || result.Session.Metadata.DiffBaseRef != metadata.DiffBaseRef {
		t.Fatalf("restore replaced workspace/base: %+v %+v", ws.restoreConfigs, result.Session.Metadata)
	}
	if got := runManagerGit(t, repo, "rev-parse", "HEAD"); got != head {
		t.Fatal("restore moved HEAD")
	}
	if got := strings.TrimSpace(runManagerGit(t, repo, "branch", "--show-current")); got != "user-change" {
		t.Fatalf("restore moved branch: %s", got)
	}
}

// TestProjectDirectoryValidation verifies invalid requests fail before a session/worktree exists.
func TestProjectDirectoryValidation(t *testing.T) {
	for _, invalid := range []string{"unknown", "base", "branch", "workspace", "scratch", "standalone", "repos", "preparation"} {
		t.Run(invalid, func(t *testing.T) {
			m, st, _, ws := newManager()
			cfg := ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, WorkspaceMode: domain.WorkspaceModeProject}
			project := st.projects["mer"]
			switch invalid {
			case "unknown":
				cfg.WorkspaceMode = "unknown"
			case "base":
				cfg.BaseBranch = "main"
			case "branch":
				cfg.Branch = "task"
			case "workspace":
				project.Kind = domain.ProjectKindWorkspace
			case "scratch":
				project.Kind = domain.ProjectKindScratch
			case "standalone":
				cfg.ProjectID = ""
			case "repos":
				cfg.Repos = []ports.SpawnRepo{{ProjectID: "mer"}}
			case "preparation":
				cfg.TaskPreparation = "old-worktree"
			}
			st.projects["mer"] = project
			_, _, _, err := m.Spawn(ctx, cfg)
			if !errors.Is(err, ports.ErrWorkspaceModeInvalid) || len(st.sessions) != 0 || ws.createCount != 0 {
				t.Fatalf("invalid create: %v sessions=%d worktrees=%d", err, len(st.sessions), ws.createCount)
			}
		})
	}
}

// projectDirectoryDeleteStore supplies the optional record-delete port for the lifecycle test.
type projectDirectoryDeleteStore struct{ *fakeStore }

func (s *projectDirectoryDeleteStore) DeleteTerminatedSession(_ context.Context, id domain.SessionID) (bool, error) {
	rec, ok := s.sessions[id]
	if !ok || !rec.IsTerminated {
		return false, nil
	}
	delete(s.sessions, id)
	return true, nil
}
