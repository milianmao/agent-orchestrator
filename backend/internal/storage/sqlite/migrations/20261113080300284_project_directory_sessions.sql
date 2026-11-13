-- +goose Up
ALTER TABLE sessions ADD COLUMN workspace_mode TEXT NOT NULL DEFAULT 'worktree' CHECK (workspace_mode IN ('worktree', 'project'));
ALTER TABLE sessions ADD COLUMN workspace_ownership TEXT NOT NULL DEFAULT 'owned' CHECK (workspace_ownership IN ('owned', 'external'));
ALTER TABLE sessions ADD COLUMN workspace_lease_key TEXT NOT NULL DEFAULT '';

-- The live session itself is the durable lease; termination releases it atomically.
CREATE UNIQUE INDEX sessions_project_directory_lease ON sessions(workspace_lease_key)
WHERE workspace_ownership = 'external' AND is_terminated = 0;

-- +goose StatementBegin
CREATE TRIGGER sessions_external_workspace_immutable
BEFORE UPDATE ON sessions
WHEN OLD.workspace_ownership = 'external' AND (
    NEW.workspace_mode <> OLD.workspace_mode OR NEW.workspace_ownership <> OLD.workspace_ownership OR
    NEW.workspace_lease_key <> OLD.workspace_lease_key OR NEW.workspace_path <> OLD.workspace_path OR
    NEW.workspace_repo_path <> OLD.workspace_repo_path OR
    NEW.diff_base_sha <> OLD.diff_base_sha OR NEW.diff_base_ref <> OLD.diff_base_ref
)
BEGIN
    SELECT RAISE(ABORT, 'external workspace identity is immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER sessions_external_workspace_immutable;
DROP INDEX sessions_project_directory_lease;
ALTER TABLE sessions DROP COLUMN workspace_lease_key;
ALTER TABLE sessions DROP COLUMN workspace_ownership;
ALTER TABLE sessions DROP COLUMN workspace_mode;
