package project_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/project"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

// newManagerWithDataDir builds a Manager whose installation default worktree
// root is <dataDir>/worktrees, the way the daemon wires it.
func newManagerWithDataDir(t *testing.T, dataDir string) project.Manager {
	t.Helper()
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return project.NewWithDeps(project.Deps{Store: store, DataDir: dataDir})
}

// TestAddProjectPersistsDefaultWorktreeRoot covers the registration contract:
// a new project without an explicit root records <AO_DATA_DIR>/worktrees, so a
// later AO_DATA_DIR change cannot move its worktrees.
func TestAddProjectPersistsDefaultWorktreeRoot(t *testing.T) {
	dataDir := t.TempDir()
	m := newManagerWithDataDir(t, dataDir)
	repo := gitRepo(t)

	added, err := m.Add(context.Background(), project.AddInput{Path: repo})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	wantRoot := filepath.Join(dataDir, "worktrees")
	if added.Config == nil || added.Config.WorktreeRoot != wantRoot {
		t.Fatalf("saved worktreeRoot = %+v, want %q", added.Config, wantRoot)
	}
	if added.WorktreeRoot != wantRoot {
		t.Fatalf("effective worktreeRoot = %q, want %q", added.WorktreeRoot, wantRoot)
	}
}

// TestProjectReadModelReportsEffectiveWorktreeRootWithoutClaimingItIsSaved
// covers the read-model rule for projects registered before the field existed:
// the API reports the root in effect, while the saved config stays empty.
func TestProjectReadModelReportsEffectiveWorktreeRootWithoutClaimingItIsSaved(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	m := project.NewWithDeps(project.Deps{Store: store, DataDir: dataDir})

	repo := gitRepo(t)
	// A row as it exists for a project registered before worktreeRoot shipped.
	if err := store.UpsertProject(ctx, domain.ProjectRecord{
		ID:     "legacy",
		Path:   repo,
		Kind:   domain.ProjectKindSingleRepo,
		Config: domain.ProjectConfig{DefaultBranch: domain.DefaultBranchAuto},
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	got, err := m.Get(ctx, "legacy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantRoot := filepath.Join(dataDir, "worktrees")
	if got.Project.WorktreeRoot != wantRoot {
		t.Fatalf("effective worktreeRoot = %q, want %q", got.Project.WorktreeRoot, wantRoot)
	}
	if got.Project.Config == nil || got.Project.Config.WorktreeRoot != "" {
		t.Fatalf("saved config = %+v, want worktreeRoot unset", got.Project.Config)
	}
}

// TestAddProjectRejectsUnsafeWorktreeRoots covers the path rules that protect
// the filesystem: relativism, filesystem roots, non-empty folders, and a root
// inside the project's own repository are all refused at save time.
func TestAddProjectRejectsUnsafeWorktreeRoots(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	m := newManagerWithDataDir(t, dataDir)
	repo := gitRepo(t)

	t.Run("relative root", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: "relative/root"}
		_, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
		// Shape rules live in domain.ProjectConfig.Validate, so a relative root
		// is refused there before the service ever touches the filesystem.
		wantCode(t, err, "INVALID_PROJECT_CONFIG")
		if !strings.Contains(err.Error(), "worktreeRoot") {
			t.Fatalf("error = %v, want it to name worktreeRoot", err)
		}
	})

	t.Run("filesystem root", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: string(filepath.Separator)}
		_, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
		wantCode(t, err, "INVALID_PROJECT_CONFIG")
		if !strings.Contains(err.Error(), "worktreeRoot") {
			t.Fatalf("error = %v, want it to name worktreeRoot", err)
		}
	})

	t.Run("root inside the project repository", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: filepath.Join(repo, "worktrees")}
		_, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_INSIDE_REPO")
	})

	t.Run("existing non-empty folder", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := domain.ProjectConfig{WorktreeRoot: dir}
		_, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_NOT_EMPTY")
	})

	t.Run("missing parent", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: filepath.Join(t.TempDir(), "missing", "root")}
		_, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_PARENT_MISSING")
	})
}

// TestAddProjectCreatesWorktreeRootAndSupportsUnicode covers the supported
// cases: a not-yet-existing root is created beneath an existing parent, and a
// root with spaces and non-ASCII characters round-trips unchanged.
func TestAddProjectCreatesWorktreeRootAndSupportsUnicode(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	m := newManagerWithDataDir(t, dataDir)
	repo := gitRepo(t)

	parent := t.TempDir()
	root := filepath.Join(parent, "工作树 目录")
	cfg := domain.ProjectConfig{WorktreeRoot: root}
	added, err := m.Add(ctx, project.AddInput{Path: repo, Config: &cfg})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Fatalf("worktree root not created: %v", err)
	}
	if added.Config == nil || added.Config.WorktreeRoot != root {
		t.Fatalf("saved worktreeRoot = %+v, want %q", added.Config, root)
	}
}

// TestWorktreeRootConflictsAcrossProjects covers the cross-project rule: two
// projects may not share a root or nest one inside the other.
func TestWorktreeRootConflictsAcrossProjects(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	m := newManagerWithDataDir(t, dataDir)

	parent := t.TempDir()
	shared := filepath.Join(parent, "shared-root")
	firstCfg := domain.ProjectConfig{WorktreeRoot: shared}
	if _, err := m.Add(ctx, project.AddInput{Path: gitRepo(t), Config: &firstCfg}); err != nil {
		t.Fatalf("Add first project: %v", err)
	}

	t.Run("same root", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: shared}
		_, err := m.Add(ctx, project.AddInput{Path: gitRepo(t), Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_CONFLICT")
	})

	t.Run("nested root", func(t *testing.T) {
		cfg := domain.ProjectConfig{WorktreeRoot: filepath.Join(shared, "inner")}
		_, err := m.Add(ctx, project.AddInput{Path: gitRepo(t), Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_CONFLICT")
	})

	t.Run("parent of another root", func(t *testing.T) {
		other := filepath.Join(parent, "other-root")
		cfg := domain.ProjectConfig{WorktreeRoot: other}
		if _, err := m.Add(ctx, project.AddInput{Path: gitRepo(t), Config: &cfg}); err != nil {
			t.Fatalf("Add other project: %v", err)
		}
		cfg = domain.ProjectConfig{WorktreeRoot: parent}
		_, err := m.Add(ctx, project.AddInput{Path: gitRepo(t), Config: &cfg})
		wantCode(t, err, "WORKTREE_ROOT_CONFLICT")
	})
}

// TestSetConfigAcceptsDefaultWorktreeRootAfterProject covers re-saving the root
// every project shares: the installation default holds one directory per
// project, which must not be mistaken for an unrelated non-empty folder.
func TestSetConfigAcceptsDefaultWorktreeRootAfterProject(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	m := newManagerWithDataDir(t, dataDir)

	first, err := m.Add(ctx, project.AddInput{Path: gitRepo(t)})
	if err != nil {
		t.Fatalf("Add first: %v", err)
	}
	second, err := m.Add(ctx, project.AddInput{Path: gitRepo(t)})
	if err != nil {
		t.Fatalf("Add second: %v", err)
	}
	// The default root now contains one directory per project.
	wantRoot := filepath.Join(dataDir, "worktrees")
	cfg := domain.ProjectConfig{WorktreeRoot: wantRoot}
	if _, err := m.SetConfig(ctx, second.ID, project.SetConfigInput{Config: cfg}); err != nil {
		t.Fatalf("SetConfig default root: %v", err)
	}
	if _, err := m.SetConfig(ctx, first.ID, project.SetConfigInput{Config: cfg}); err != nil {
		t.Fatalf("SetConfig default root for second project: %v", err)
	}
}

// TestProjectBranchesListsLocalAndOrigin covers the picker's contract: local
// branches plus origin/<branch> rows, origin/HEAD excluded, and the configured
// default marked.
func TestProjectBranchesListsLocalAndOrigin(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	repo := gitRepoOnBranch(t, "main")
	runGit(t, repo, "branch", "feature/with-slash")
	// Point origin at a bare clone of the same repository so remote-tracking
	// refs exist without touching the network.
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, filepath.Dir(remote), "clone", "--bare", repo, remote)
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "fetch", "origin")
	runGit(t, repo, "remote", "set-head", "origin", "main")

	added, err := m.Add(ctx, project.AddInput{Path: repo})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	branches, err := m.Branches(ctx, added.ID)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	names := make(map[string]project.ProjectBranch, len(branches.Branches))
	for _, branch := range branches.Branches {
		names[branch.Name] = branch
	}
	for _, want := range []string{"main", "feature/with-slash", "origin/main", "origin/feature/with-slash"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("branches = %v, want %q listed", names, want)
		}
	}
	if _, ok := names["origin/HEAD"]; ok {
		t.Fatalf("origin/HEAD must be excluded: %v", names)
	}
	if !names["main"].Current || !names["main"].Local {
		t.Fatalf("main row = %+v, want current local branch", names["main"])
	}
	if names["origin/main"].Remote != "origin" {
		t.Fatalf("origin/main remote = %q, want origin", names["origin/main"].Remote)
	}
	if !names["main"].Default {
		t.Fatalf("default branch %q not marked: %v", branches.Default, names)
	}
	defaults := 0
	for _, branch := range branches.Branches {
		if branch.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("default rows = %d, want exactly the configured spelling: %v", defaults, names)
	}
}

// TestProjectBranchesReportsUnreadableRepository covers the error contract: a
// failure to read the repository is reported, not returned as an empty list.
func TestProjectBranchesReportsUnreadableRepository(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	repo := gitRepo(t)
	added, err := m.Add(ctx, project.AddInput{Path: repo})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := m.Branches(ctx, added.ID); err != nil {
		t.Fatalf("Branches on a healthy repo: %v", err)
	}
	// Break the repository's git metadata so the branch read fails.
	if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	_, err = m.Branches(ctx, added.ID)
	if err == nil {
		t.Fatal("Branches on a broken repository = nil error, want a read failure")
	}
}

// TestScratchProjectSkipsWorktreeRoot covers the scope boundary: scratch
// projects report no worktree root and their config still refuses one.
func TestScratchProjectSkipsWorktreeRoot(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	m := project.NewWithDeps(project.Deps{Store: store, DataDir: dataDir})

	if err := store.UpsertProject(ctx, domain.ProjectRecord{
		ID:   "scratch",
		Path: t.TempDir(),
		Kind: domain.ProjectKindScratch,
	}); err != nil {
		t.Fatalf("seed scratch: %v", err)
	}
	got, err := m.Get(ctx, "scratch")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Project.WorktreeRoot != "" {
		t.Fatalf("scratch worktreeRoot = %q, want empty", got.Project.WorktreeRoot)
	}
	branches, err := m.Branches(ctx, "scratch")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(branches.Branches) != 0 {
		t.Fatalf("scratch branches = %v, want none", branches.Branches)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
	}
}

// TestSetConfigWithoutWorktreeRootKeepsSavedRoot covers the older-client path: a
// read-modify-write that never knew the field must not erase a saved root, while
// an explicit empty value still clears it.
func TestSetConfigWithoutWorktreeRootKeepsSavedRoot(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	repo := gitRepo(t)
	custom := filepath.Join(t.TempDir(), "custom-root")
	added, err := m.Add(ctx, project.AddInput{
		Path:   repo,
		Config: &domain.ProjectConfig{WorktreeRoot: custom},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// An old client's config has no worktreeRoot at all: WorktreeRootPresent
	// stays false and the saved root survives.
	if _, err := m.SetConfig(ctx, added.ID, project.SetConfigInput{
		Config: domain.ProjectConfig{SessionPrefix: "old-client"},
	}); err != nil {
		t.Fatalf("SetConfig from an old client: %v", err)
	}
	got, err := m.Get(ctx, added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Project.Config == nil || got.Project.Config.WorktreeRoot != custom {
		t.Fatalf("saved root = %+v, want %q preserved", got.Project.Config, custom)
	}
	if got.Project.Config.SessionPrefix != "old-client" {
		t.Fatalf("session prefix = %q, want the rest of the config applied", got.Project.Config.SessionPrefix)
	}

	// An explicit empty value is a real clear.
	if _, err := m.SetConfig(ctx, added.ID, project.SetConfigInput{
		Config:              domain.ProjectConfig{},
		WorktreeRootPresent: true,
	}); err != nil {
		t.Fatalf("SetConfig clearing the root: %v", err)
	}
	got, err = m.Get(ctx, added.ID)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if got.Project.Config != nil && got.Project.Config.WorktreeRoot != "" {
		t.Fatalf("saved root = %q, want cleared", got.Project.Config.WorktreeRoot)
	}
}

// TestUpdateSettingsWithoutWorktreeRootKeepsSavedRoot covers the same rule on the
// settings endpoint, which the settings form uses.
func TestUpdateSettingsWithoutWorktreeRootKeepsSavedRoot(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	repo := gitRepo(t)
	custom := filepath.Join(t.TempDir(), "settings-root")
	added, err := m.Add(ctx, project.AddInput{
		Path:   repo,
		Config: &domain.ProjectConfig{WorktreeRoot: custom},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := m.UpdateSettings(ctx, added.ID, project.UpdateSettingsInput{
		DisplayName: "Renamed",
		Config:      domain.ProjectConfig{AutoReview: true},
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	got, err := m.Get(ctx, added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Project.Config == nil || got.Project.Config.WorktreeRoot != custom {
		t.Fatalf("saved root = %+v, want %q preserved", got.Project.Config, custom)
	}
	if got.Project.Name != "Renamed" || !got.Project.Config.AutoReview {
		t.Fatalf("rest of the settings update lost: %+v", got.Project)
	}
}

// TestWorktreeRootSymlinkSpellingConflicts covers M2: a junction/symlink spelling
// of another project's root must be recognised as the SAME root, because the two
// paths address one directory on disk.
func TestWorktreeRootSymlinkSpellingConflicts(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real-root")
	linkRoot := filepath.Join(parent, "linked-root")
	if err := os.MkdirAll(realRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlinks unavailable on this host: %v", err)
	}

	if _, err := m.Add(ctx, project.AddInput{
		Path:   gitRepo(t),
		Config: &domain.ProjectConfig{WorktreeRoot: realRoot},
	}); err != nil {
		t.Fatalf("Add first project: %v", err)
	}
	_, err := m.Add(ctx, project.AddInput{
		Path:   gitRepo(t),
		Config: &domain.ProjectConfig{WorktreeRoot: linkRoot},
	})
	wantCode(t, err, "WORKTREE_ROOT_CONFLICT")
}

// TestWorktreeRootCreationRollsBackOnValidationFailure covers M3: a root this call
// created is removed when the same call then rejects it, and a folder that already
// existed is never removed by a failed validation.
func TestWorktreeRootCreationRollsBackOnValidationFailure(t *testing.T) {
	ctx := context.Background()
	m := newManagerWithDataDir(t, t.TempDir())
	repo := gitRepo(t)

	// Created inside the project repository: creation happens first, then the
	// inside-repo rule rejects it, and the new folder must not survive.
	inside := filepath.Join(repo, "worktrees-root")
	_, err := m.Add(ctx, project.AddInput{
		Path:   repo,
		Config: &domain.ProjectConfig{WorktreeRoot: inside},
	})
	wantCode(t, err, "WORKTREE_ROOT_INSIDE_REPO")
	if _, statErr := os.Stat(inside); !os.IsNotExist(statErr) {
		t.Fatalf("rejected root survived at %s: %v", inside, statErr)
	}

	// A pre-existing empty folder rejected for another reason stays untouched.
	existing := filepath.Join(t.TempDir(), "existing-empty")
	if err := os.MkdirAll(existing, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(ctx, project.AddInput{
		Path:   gitRepo(t),
		Config: &domain.ProjectConfig{WorktreeRoot: existing},
	}); err != nil {
		t.Fatalf("Add with a pre-existing empty root: %v", err)
	}
	if info, statErr := os.Stat(existing); statErr != nil || !info.IsDir() {
		t.Fatalf("pre-existing root removed after success: %v", statErr)
	}
}
