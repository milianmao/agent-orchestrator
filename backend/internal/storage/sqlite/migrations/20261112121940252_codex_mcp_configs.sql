-- +goose Up
CREATE TABLE codex_mcp_configs (
    name TEXT PRIMARY KEY NOT NULL,
    config_json TEXT NOT NULL CHECK (json_valid(config_json))
);
CREATE TABLE session_codex_mcp_configs (
    session_id TEXT PRIMARY KEY NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    config_json TEXT NOT NULL CHECK (json_valid(config_json))
);

-- +goose Down
DROP TABLE session_codex_mcp_configs;
DROP TABLE codex_mcp_configs;
