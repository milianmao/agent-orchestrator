package session

import (
	"context"
	"os/exec"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestTaskBaseUsesOnlyExistingBranches exercises real refs rather than accepting Git revision expressions.
func TestTaskBaseUsesOnlyExistingBranches(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	git("branch", "feature/local")
	git("update-ref", "refs/remotes/origin/feature/remote", "HEAD")
	project := domain.ProjectRecord{ID: "test", Path: repo}
	for input, want := range map[string]string{"feature/local": "refs/heads/feature/local", "origin/feature/remote": "refs/remotes/origin/feature/remote", "feature/remote": "refs/remotes/origin/feature/remote"} {
		got, err := resolveTaskBase(context.Background(), project, input)
		if err != nil || got != want {
			t.Fatalf("resolve %q = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"missing", "HEAD~0", "refs/heads/main", "origin/HEAD"} {
		if got, err := resolveTaskBase(context.Background(), project, input); err == nil {
			t.Fatalf("accepted %q as %q", input, got)
		}
	}
	project.Kind = domain.ProjectKindScratch
	if _, err := resolveTaskBase(context.Background(), project, "main"); err == nil {
		t.Fatal("scratch accepted explicit base")
	}
	project.Kind = domain.ProjectKindWorkspace
	if _, err := resolveTaskBase(context.Background(), project, "main"); err == nil {
		t.Fatal("workspace accepted explicit base")
	}
}
