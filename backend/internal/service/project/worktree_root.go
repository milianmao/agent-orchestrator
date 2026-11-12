package project

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// WorktreeRoot resolution and safety rules.
//
// A project's worktree root decides where every NEW session worktree of that
// project lands (<root>/<project-id>/<session-id>). The rules live here, in the
// project service, because they depend on the project registry: a root is only
// safe relative to the project's own repository and every other project's root.
// The workspace adapter still owns per-session materialisation and re-checks the
// snapshot it is handed (see gitworktree.Workspace).

// EffectiveWorktreeRoot is the root a project actually uses: its configured
// WorktreeRoot, or the installation default when the config leaves it empty
// (projects registered before the field existed).
func EffectiveWorktreeRoot(cfg domain.ProjectConfig, dataDir string) string {
	if root := strings.TrimSpace(cfg.WorktreeRoot); root != "" {
		return filepath.Clean(root)
	}
	return domain.DefaultWorktreeRoot(dataDir)
}

// applyDefaultWorktreeRoot fills a registration config's WorktreeRoot with the
// installation default. New projects persist the resolved value so a later
// AO_DATA_DIR change does not silently move their worktrees, while a project
// that already names a root keeps it untouched.
func applyDefaultWorktreeRoot(cfg *domain.ProjectConfig, dataDir string) {
	if cfg == nil || strings.TrimSpace(cfg.WorktreeRoot) != "" {
		return
	}
	cfg.WorktreeRoot = domain.DefaultWorktreeRoot(dataDir)
}

// validateWorktreeRootSelection applies the filesystem-facing half of the
// worktree-root contract to an EXPLICIT root being saved: the directory is
// created when missing, must be writable, and must not sit inside the project's
// own repository, equal another project's root, or nest with one. It is a no-op
// for an empty root, which means the unchanged installation default.
//
// Roots are recorded and compared in physical (symlink-resolved) form so two
// spellings of one directory cannot both pass the equality check.
func (m *Service) validateWorktreeRootSelection(ctx context.Context, projectID domain.ProjectID, projectPath, root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", nil
	}
	if err := domain.ValidateWorktreeRoot(trimmed); err != nil {
		return "", apierr.Invalid("INVALID_WORKTREE_ROOT", err.Error(), map[string]any{"worktreeRoot": trimmed})
	}
	clean := filepath.Clean(trimmed)
	physical, err := m.prepareWorktreeRootDirectory(clean)
	if err != nil {
		return "", err
	}
	if err := rejectWorktreeRootInsideRepo(projectID, projectPath, physical); err != nil {
		return "", err
	}
	if err := m.rejectWorktreeRootConflicts(ctx, projectID, physical); err != nil {
		return "", err
	}
	return physical, nil
}

// prepareWorktreeRootDirectory makes sure root exists as a writable directory,
// creating it ONLY beneath an existing parent. It returns root's physical path.
func (m *Service) prepareWorktreeRootDirectory(root string) (string, error) {
	info, err := os.Stat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(root))
		if parentErr != nil {
			return "", apierr.Invalid("WORKTREE_ROOT_PARENT_MISSING",
				"The worktree root's parent folder does not exist. Create it first, or choose another folder.",
				map[string]any{"worktreeRoot": root, "parent": filepath.Dir(root)})
		}
		if err := os.Mkdir(root, 0o750); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return "", apierr.Invalid("WORKTREE_ROOT_NOT_WRITABLE", "AO is not allowed to create the worktree root folder.", map[string]any{"worktreeRoot": root})
			}
			return "", apierr.Invalid("WORKTREE_ROOT_UNAVAILABLE", "AO could not create the worktree root folder.", map[string]any{"worktreeRoot": root})
		}
		return filepath.Join(parent, filepath.Base(root)), nil
	case err != nil:
		return "", apierr.Invalid("WORKTREE_ROOT_UNAVAILABLE", "The worktree root could not be inspected.", map[string]any{"worktreeRoot": root})
	case !info.IsDir():
		return "", apierr.Invalid("WORKTREE_ROOT_NOT_DIRECTORY", "The worktree root must be a folder.", map[string]any{"worktreeRoot": root})
	}
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", apierr.Invalid("WORKTREE_ROOT_UNAVAILABLE", "The worktree root could not be resolved.", map[string]any{"worktreeRoot": root})
	}
	if err := m.requireWorktreeRootUsable(physical); err != nil {
		return "", err
	}
	return physical, nil
}

// requireWorktreeRootUsable reports whether AO can write beneath root. An
// existing non-empty directory is refused unless it already holds AO-managed
// worktrees (directories laid out as AO's own root is), so pointing a project at
// a folder full of unrelated files fails instead of scattering worktrees through
// it. The AO-managed shape is accepted because it covers the installation
// default root, which every project shares.
func (m *Service) requireWorktreeRootUsable(root string) error {
	if err := m.requireWritableDirectory(root); err != nil {
		return err
	}
	// The installation default root holds one directory per project and a trash
	// folder; it is AO's own by construction, so it never needs the shape check.
	if defaultRoot := domain.DefaultWorktreeRoot(m.dataDir); defaultRoot != "" {
		if physicalDefault, err := physicalPathIfExists(defaultRoot); err == nil && samePath(physicalDefault, root) {
			return nil
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return apierr.Invalid("WORKTREE_ROOT_UNAVAILABLE", "The worktree root could not be read.", map[string]any{"worktreeRoot": root})
	}
	for _, entry := range entries {
		// The trash folder the workspace adapter keeps beside session worktrees.
		if entry.Name() == ".discarded" {
			continue
		}
		if !entry.IsDir() {
			return apierr.Invalid("WORKTREE_ROOT_NOT_EMPTY",
				"The worktree root must be empty or already hold AO-managed worktrees.",
				map[string]any{"worktreeRoot": root, "entry": entry.Name()})
		}
	}
	return nil
}

// requireWritableDirectory proves root accepts new files for AO by creating and
// removing a probe file. Mode bits alone are not evidence: ACLs, read-only
// mounts, and container bind mounts all deny writes on directories that look
// writable.
func (m *Service) requireWritableDirectory(root string) error {
	probe, err := os.CreateTemp(root, ".ao-write-check-")
	if err != nil {
		return apierr.Invalid("WORKTREE_ROOT_NOT_WRITABLE", "AO cannot write to the worktree root folder.", map[string]any{"worktreeRoot": root})
	}
	name := probe.Name()
	if _, err := io.Copy(io.Discard, probe); err != nil {
		_ = probe.Close()
		_ = os.Remove(name)
		return apierr.Invalid("WORKTREE_ROOT_NOT_WRITABLE", "AO cannot write to the worktree root folder.", map[string]any{"worktreeRoot": root})
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return apierr.Invalid("WORKTREE_ROOT_NOT_WRITABLE", "AO cannot write to the worktree root folder.", map[string]any{"worktreeRoot": root})
	}
	if err := os.Remove(name); err != nil {
		return apierr.Invalid("WORKTREE_ROOT_NOT_WRITABLE", "AO cannot write to the worktree root folder.", map[string]any{"worktreeRoot": root})
	}
	return nil
}

// rejectWorktreeRootInsideRepo refuses a root that lies inside the project's own
// repository: git cannot check out a worktree of a repository into that same
// repository without the checkout polluting its own working tree.
func rejectWorktreeRootInsideRepo(projectID domain.ProjectID, projectPath, root string) error {
	if strings.TrimSpace(projectPath) == "" {
		return nil
	}
	repoPhysical, err := physicalPathIfExists(projectPath)
	if err != nil {
		// The repository cannot be resolved right now (for example a detached
		// volume). Spawn re-checks this, and refusing a save because the repo is
		// temporarily unreadable would block unrelated settings edits.
		return nil
	}
	inside, err := pathWithin(repoPhysical, root)
	if err != nil {
		return apierr.Invalid("INVALID_WORKTREE_ROOT", "The worktree root could not be compared with the project repository.", map[string]any{"worktreeRoot": root})
	}
	if inside {
		return apierr.Invalid("WORKTREE_ROOT_INSIDE_REPO",
			"The worktree root cannot be inside the project repository.",
			map[string]any{"worktreeRoot": root, "projectId": string(projectID), "path": projectPath})
	}
	return nil
}

// rejectWorktreeRootConflicts refuses a root that equals another project's root,
// or nests with it in either direction: worktrees from two projects would
// otherwise become indistinguishable, and one project's cleanup could reach the
// other's checkouts.
func (m *Service) rejectWorktreeRootConflicts(ctx context.Context, projectID domain.ProjectID, root string) error {
	// The installation default root holds one directory per project by
	// construction, so every project shares it and none of them collides with
	// another. Only a root that moves a project OFF that layout must be unique.
	if defaultRoot := domain.DefaultWorktreeRoot(m.dataDir); defaultRoot != "" {
		if physicalDefault, err := physicalPathIfExists(defaultRoot); err == nil && samePath(physicalDefault, root) {
			return nil
		}
	}
	rows, err := m.store.ListProjects(ctx)
	if err != nil {
		return apierr.Internal("PROJECT_LOAD_FAILED", "Failed to load project")
	}
	for _, row := range rows {
		if domain.ProjectID(row.ID) == projectID || row.Kind.WithDefault() == domain.ProjectKindScratch {
			continue
		}
		other := EffectiveWorktreeRoot(row.Config, m.dataDir)
		if other == "" {
			continue
		}
		otherPhysical, err := physicalPathIfExists(other)
		if err != nil {
			// A root that does not exist yet can only collide by path spelling,
			// which Clean plus the equality check below still catches.
			otherPhysical = filepath.Clean(other)
		}
		nested, err := worktreeRootsNest(root, otherPhysical)
		if err != nil {
			return apierr.Invalid("INVALID_WORKTREE_ROOT", "The worktree root could not be compared with other projects.", map[string]any{"worktreeRoot": root})
		}
		if nested {
			return apierr.Conflict("WORKTREE_ROOT_CONFLICT",
				"That folder conflicts with another project's worktree root. Choose a folder that is neither the same as nor inside another project's root.",
				map[string]any{"worktreeRoot": root, "conflictingProjectId": row.ID, "conflictingRoot": other})
		}
	}
	return nil
}

// worktreeRootsNest reports whether two roots are equal or one contains the
// other. Two volumes never nest, so a Windows D:\ root and a C:\ root stay
// independent.
func worktreeRootsNest(a, b string) (bool, error) {
	insideAB, err := pathWithin(a, b)
	if err != nil {
		return false, err
	}
	if insideAB {
		return true, nil
	}
	return pathWithin(b, a)
}

// pathWithin reports whether target is parent or lies beneath it, comparing
// clean absolute paths. parent==target counts as within.
func pathWithin(parent, target string) (bool, error) {
	parentAbs, err := filepath.Abs(filepath.Clean(parent))
	if err != nil {
		return false, err
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false, err
	}
	if samePath(parentAbs, targetAbs) {
		return true, nil
	}
	rel, err := filepath.Rel(parentAbs, targetAbs)
	if err != nil {
		return false, nil
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// physicalPathIfExists resolves path's symlinks when it exists and otherwise
// returns its cleaned absolute form, so a not-yet-created root still compares
// predictably against existing ones.
func physicalPathIfExists(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return abs, nil
		}
		return "", err
	}
	return resolved, nil
}
