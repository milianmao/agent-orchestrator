package domain

// WorkspaceMode distinguishes an isolated task worktree from the user's checkout.
type WorkspaceMode string

const (
	WorkspaceModeWorktree WorkspaceMode = "worktree"
	WorkspaceModeProject  WorkspaceMode = "project"
)

// WithDefault preserves the worktree behavior of older clients and sessions.
func (m WorkspaceMode) WithDefault() WorkspaceMode {
	if m == "" {
		return WorkspaceModeWorktree
	}
	return m
}

// Valid rejects unknown modes before any workspace or session is created.
func (m WorkspaceMode) Valid() bool {
	return m.WithDefault() == WorkspaceModeWorktree || m == WorkspaceModeProject
}

// WorkspaceOwnership controls whether AO may retire the workspace directory.
type WorkspaceOwnership string

const (
	WorkspaceOwned    WorkspaceOwnership = "owned"
	WorkspaceExternal WorkspaceOwnership = "external"
)

// Ownership maps legacy worktree sessions to AO-owned directories.
func (m WorkspaceMode) Ownership() WorkspaceOwnership {
	if m == WorkspaceModeProject {
		return WorkspaceExternal
	}
	return WorkspaceOwned
}

// ExternalWorkspace fails closed if either durable ownership fact forbids removal.
func (m SessionMetadata) ExternalWorkspace() bool {
	return m.WorkspaceMode == WorkspaceModeProject || m.WorkspaceOwnership == WorkspaceExternal
}
