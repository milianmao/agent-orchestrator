package session

import (
	"context"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// resolveTaskBase 只接受已有分支，显式选择失败时绝不回退项目默认值。
func resolveTaskBase(ctx context.Context, project domain.ProjectRecord, requested string) (string, error) {
	if requested == "" {
		return "", nil
	}
	if project.Kind.WithDefault() == domain.ProjectKindScratch || project.Kind.WithDefault() == domain.ProjectKindWorkspace {
		return "", apierr.Invalid("BASE_BRANCH_UNSUPPORTED", "Base branch selection requires a Git worktree project", nil)
	}
	name := strings.TrimSpace(requested)
	if name == "" || strings.HasPrefix(name, "refs/") || name == "HEAD" || name == "origin/HEAD" {
		return "", apierr.Invalid("INVALID_BASE_BRANCH", "Choose a local branch or origin branch", nil)
	}
	if _, err := gitWorkspaceOutput(ctx, project.Path, "check-ref-format", "--branch", name); err != nil {
		return "", apierr.Invalid("INVALID_BASE_BRANCH", "Choose a local branch or origin branch", nil)
	}
	refs, err := gitWorkspaceOutput(ctx, project.Path, "for-each-ref", "--format=%(refname)", "refs/heads/", "refs/remotes/origin/")
	if err != nil {
		return "", apierr.Invalid("PROJECT_BRANCHES_UNAVAILABLE", "The project's branches could not be read", nil)
	}
	candidates := []string{"refs/heads/" + name, "refs/remotes/origin/" + name}
	if strings.HasPrefix(name, "origin/") {
		candidates = []string{"refs/remotes/" + name}
	}
	for _, candidate := range candidates {
		for _, ref := range strings.Fields(refs) {
			if candidate == ref {
				return ref, nil
			}
		}
	}
	return "", apierr.Invalid("BASE_BRANCH_NOT_FOUND", "The selected base branch no longer exists", nil)
}
