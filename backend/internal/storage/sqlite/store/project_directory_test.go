package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

// TestProjectDirectoryLeasePersists exercises the unique lease, restart, release, and reacquisition.
func TestProjectDirectoryLeasePersists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := sqlitetest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	seedProject(t, s, "project")
	rec := sampleRecord("project")
	rec.Metadata.WorkspaceMode = domain.WorkspaceModeProject
	rec.Metadata.WorkspaceOwnership = domain.WorkspaceExternal
	rec.Metadata.WorkspaceLeaseKey = "canonical-project-path"
	rec.Metadata.DiffBaseSHA, rec.Metadata.DiffBaseRef = "start-sha", "start-sha"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.CreateSession(ctx, rec); results <- err })
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ports.ErrProjectDirectoryBusy) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = sqlite.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateSession(ctx, rec)
	if !errors.Is(err, ports.ErrProjectDirectoryBusy) {
		t.Fatalf("restart lost lease: %v", err)
	}
	persisted, found, err := s.GetSession(ctx, "project-1")
	if err != nil || !found || persisted.Metadata.WorkspaceMode != domain.WorkspaceModeProject || persisted.Metadata.WorkspaceOwnership != domain.WorkspaceExternal || persisted.Metadata.WorkspaceLeaseKey != rec.Metadata.WorkspaceLeaseKey {
		t.Fatalf("external metadata did not survive reopen: %+v found=%v err=%v", persisted.Metadata, found, err)
	}
	worktree, err := s.CreateSession(ctx, sampleRecord("project"))
	if err != nil || worktree.Metadata.WorkspaceMode != domain.WorkspaceModeWorktree || worktree.Metadata.WorkspaceOwnership != domain.WorkspaceOwned {
		t.Fatalf("worktree regression: %+v %v", worktree, err)
	}
	rows, err := s.ListSessions(ctx, "project")
	if err != nil {
		t.Fatal(err)
	}
	owner := rows[0]
	owner.IsTerminated = true
	owner.Metadata.DiffBaseSHA, owner.Metadata.DiffBaseRef, owner.Metadata.WorkspacePath = "changed", "changed", "changed"
	if err := s.UpdateSession(ctx, owner); err != nil {
		t.Fatal(err)
	}
	owner, _, err = s.GetSession(ctx, owner.ID)
	if err != nil || owner.Metadata.DiffBaseSHA != "start-sha" || owner.Metadata.DiffBaseRef != "start-sha" || owner.Metadata.WorkspacePath != rec.Metadata.WorkspacePath {
		t.Fatalf("identity changed: %+v %v", owner.Metadata, err)
	}
	newOwner, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("termination did not release lease: %v", err)
	}
	owner.IsTerminated = false
	if err := s.UpdateSession(ctx, owner); !errors.Is(err, ports.ErrProjectDirectoryBusy) {
		t.Fatalf("restore stole live lease: %v", err)
	}
	newOwner.IsTerminated = true
	if err := s.UpdateSession(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSession(ctx, owner); err != nil {
		t.Fatalf("restore could not reacquire free lease: %v", err)
	}
}
