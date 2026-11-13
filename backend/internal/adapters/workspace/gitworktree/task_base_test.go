package gitworktree

import (
	"context"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// TestTaskBaseFullRefSeedsActualHEAD 验证任务入口固定 SHA 后，完整 ref 不会被默认分支解析器重新解释。
func TestTaskBaseFullRefSeedsActualHEAD(t *testing.T) {
	git := requireGit(t)
	repo, head, previous := pinnedTestRepo(t, git)
	gitOutput(t, git, repo, "branch", "feature/local", previous)
	gitOutput(t, git, repo, "remote", "add", "origin", repo)
	gitOutput(t, git, repo, "fetch", "origin", "feature/local:refs/remotes/origin/feature/remote")
	for _, ref := range []string{"refs/heads/feature/local", "refs/remotes/origin/feature/remote"} {
		t.Run(ref, func(t *testing.T) {
			ws := newPinnedWorkspace(t, git, repo)
			sha := gitOutput(t, git, repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
			info, err := ws.Create(context.Background(), ports.WorkspaceConfig{
				ProjectID: "proj", SessionID: "task", Branch: "ao/task", FreshBranch: true,
				BaseBranch: ref, BaseRef: ref, BaseSHA: sha,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := gitOutput(t, git, info.Path, "rev-parse", "HEAD"); got != previous || got == head {
				t.Fatalf("HEAD = %s, want non-default seed %s", got, previous)
			}
			if info.BaseRef != ref || info.BaseSHA != previous {
				t.Fatalf("base = %s %s, want %s %s", info.BaseRef, info.BaseSHA, ref, previous)
			}
		})
	}
}
