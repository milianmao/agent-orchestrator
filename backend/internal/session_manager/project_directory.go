package sessionmanager

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// validateWorkspaceMode rejects unsupported workspace shapes before admission.
func validateWorkspaceMode(project domain.ProjectRecord, cfg ports.SpawnConfig) error {
	if !cfg.WorkspaceMode.Valid() {
		return ports.ErrWorkspaceModeInvalid
	}
	if cfg.WorkspaceMode == domain.WorkspaceModeProject && (cfg.ProjectID == "" ||
		project.Kind.WithDefault() != domain.ProjectKindSingleRepo || cfg.Kind != domain.KindWorker ||
		cfg.Branch != "" || cfg.BaseBranch != "" || cfg.TaskPreparation != "" || len(cfg.Repos) > 0) {
		return ports.ErrWorkspaceModeInvalid
	}
	return nil
}

// projectDirectoryMetadata reads the checkout once; no branch or worktree is changed.
func projectDirectoryMetadata(ctx context.Context, path string, metadata domain.SessionMetadata) (domain.SessionMetadata, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return metadata, fmt.Errorf("resolve project directory: %w", err)
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return metadata, err
	}
	head, ok := spawnGitSingleLine(ctx, physical, "rev-parse", "--verify", "HEAD^{commit}")
	if !ok {
		return metadata, fmt.Errorf("project directory requires a committed HEAD")
	}
	metadata.WorkspacePath, metadata.WorkspaceRepoPath = path, path
	metadata.WorkspaceLeaseKey = filepath.Clean(physical)
	if runtime.GOOS == "windows" {
		metadata.WorkspaceLeaseKey = strings.ToLower(metadata.WorkspaceLeaseKey)
	}
	metadata.Branch, _ = spawnGitSingleLine(ctx, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	metadata.DiffBaseSHA, metadata.DiffBaseRef = head, head
	return metadata, nil
}

// verifyProjectDirectory accepts dirty files and branch changes, but never recreates a missing checkout.
func verifyProjectDirectory(ctx context.Context, metadata domain.SessionMetadata) error {
	current, err := projectDirectoryMetadata(ctx, metadata.WorkspacePath, domain.SessionMetadata{})
	if err != nil {
		return err
	}
	if current.WorkspaceLeaseKey != metadata.WorkspaceLeaseKey {
		return fmt.Errorf("project directory identity changed")
	}
	if _, ok := spawnGitSingleLine(ctx, metadata.WorkspacePath, "rev-parse", "--verify", "--end-of-options", metadata.DiffBaseSHA+"^{commit}"); !ok {
		return fmt.Errorf("recorded project starting commit is unavailable")
	}
	return nil
}

// provisionOwnedWorkspace limits worktree setup scripts and copied files to AO-owned directories.
func (m *Manager) provisionOwnedWorkspace(ctx context.Context, project domain.ProjectRecord, ws ports.WorkspaceInfo) error {
	if ws.Ownership == domain.WorkspaceExternal {
		return nil
	}
	return m.provisionWorkspace(ctx, project, ws.Path)
}

// saveExternalSession saves only a restart marker and tears down controllers; it never stashes or removes files.
func (m *Manager) saveExternalSession(ctx context.Context, rec domain.SessionRecord) error {
	release, err := m.beginShellTerminalTeardown(ctx, rec.ID)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}
	if err := m.terminateNativeSession(ctx, rec); err != nil {
		return err
	}
	if err := m.teardownReviewerTerminal(ctx, rec.ID); err != nil {
		return err
	}
	if domain.NormalizeSessionMode(rec.Mode) == domain.SessionModeChat {
		m.stopChatBestEffort(ctx, rec.ID)
	} else if handle := runtimeHandle(rec.Metadata); handle.ID != "" {
		if err := m.runtime.Destroy(ctx, handle); err != nil {
			return err
		}
	}
	if err := m.store.UpsertSessionWorktree(ctx, domain.SessionWorktreeRecord{
		SessionID: rec.ID, RepoName: domain.RootWorkspaceRepoName,
		Branch: rec.Metadata.Branch, WorktreePath: rec.Metadata.WorkspacePath, State: "removed",
	}); err != nil {
		return err
	}
	return m.lcm.MarkTerminated(ctx, rec.ID)
}
