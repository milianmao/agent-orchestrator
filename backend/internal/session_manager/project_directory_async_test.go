package sessionmanager

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	browsersvc "github.com/aoagents/agent-orchestrator/backend/internal/service/browser"
)

// TestProjectDirectoryAsyncRetry keeps a failed Chat start attached to its external checkout.
func TestProjectDirectoryAsyncRetry(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "missing_directory"}[missing], func(t *testing.T) {
			launcher := &recordingLauncher{startErr: errors.New("provider unavailable")}
			m, st, _ := newChatManager(launcher)
			m.dataDir = t.TempDir()
			m.browserCapabilities = browsersvc.NewAuthority()
			repo := newManagerGitRepo(t)
			runManagerGit(t, repo, "checkout", "--detach")
			project := st.projects[string(chatTestProject)]
			project.Path = repo
			st.projects[string(chatTestProject)] = project
			deferred := deferredBackground(m)
			cfg := asyncChatSpawnConfig("fix a bug")
			cfg.WorkspaceMode, cfg.NameFromPrompt = domain.WorkspaceModeProject, true
			rec, _, _, err := m.Spawn(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			(*deferred)[0]()
			if st.sessions[rec.ID].ProvisionState != domain.SessionProvisionFailed {
				t.Fatal("expected failed provider start")
			}
			if st.sessions[rec.ID].BranchNaming {
				t.Error("external checkout entered task branch naming")
			}
			launcher.startErr = nil
			if missing {
				// Move only the disposable repository owned by this test.
				if err := os.Rename(repo, repo+"-moved"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Rename(repo+"-moved", repo) })
			} else {
				runManagerGit(t, repo, "checkout", "-b", "user-branch")
				runManagerGit(t, repo, "commit", "--allow-empty", "-m", "later")
			}
			_, err = m.ResumeAgentWithMode(context.Background(), rec.ID)
			if err == nil && len(*deferred) > 1 {
				(*deferred)[1]()
			}
			stored := st.sessions[rec.ID]
			ws := m.workspace.(*fakeWorkspace)
			if ws.createCount != 0 || ws.destroyed != 0 || len(ws.fetches) != 0 {
				t.Fatalf("retry used worktree adapter: %+v", ws)
			}
			if missing {
				if err == nil && stored.ProvisionState != domain.SessionProvisionFailed {
					t.Fatal("missing project directory accepted")
				}
				if _, err := os.Stat(repo); !os.IsNotExist(err) {
					t.Fatal("missing directory recreated")
				}
			} else if err != nil || stored.ProvisionState != domain.SessionProvisionReady {
				t.Fatalf("retry failed: %v state=%s", err, stored.ProvisionState)
			}
			if stored.Metadata.DiffBaseSHA != rec.Metadata.DiffBaseSHA || stored.Metadata.Branch != "" || stored.Metadata.WorkspaceMode != domain.WorkspaceModeProject {
				t.Fatalf("retry rewrote external workspace identity: %+v", stored.Metadata)
			}
		})
	}
}
