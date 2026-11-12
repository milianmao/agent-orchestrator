package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// CodexMCPStartupFailures 从已有会话投影派生诊断，不持久化显示状态或返回原始错误。
func (s *Store) CodexMCPStartupFailures(ctx context.Context) (map[string]string, error) {
	rows, err := s.readDB.QueryContext(ctx, `SELECT c.mcp_servers_json FROM conversations c
		JOIN session_codex_mcp_configs snapshot ON snapshot.session_id = c.current_session_id
		WHERE c.mcp_servers_json IS NOT NULL ORDER BY c.updated_at DESC, c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	seen := map[string]bool{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var servers []domain.ConversationMCPServer
		if err := json.Unmarshal([]byte(data), &servers); err != nil {
			return nil, errors.New("invalid MCP startup projection")
		}
		for _, server := range servers {
			if seen[server.Name] {
				continue
			}
			seen[server.Name] = true
			if server.Status == "failed" {
				result[server.Name] = "Provider reported startup failure; check referenced credentials and server availability"
			}
		}
	}
	return result, rows.Err()
}

// CaptureCodexMCPConfigs 在创建会话时冻结引用列表，后续编辑不影响恢复中的旧会话。
func (s *Store) CaptureCodexMCPConfigs(ctx context.Context, id domain.SessionID) ([]domain.CodexMCPConfig, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.writeDB.ExecContext(ctx, `INSERT INTO session_codex_mcp_configs(session_id, config_json)
		SELECT ?, COALESCE(json_group_array(json(config_json)), '[]') FROM codex_mcp_configs WHERE true
		ON CONFLICT(session_id) DO NOTHING`, string(id))
	if err != nil {
		return nil, err
	}
	return s.SessionCodexMCPConfigs(ctx, id)
}

// SessionCodexMCPConfigs 旧会话无快照时返回空配置，不加载新保存的全局配置。
func (s *Store) SessionCodexMCPConfigs(ctx context.Context, id domain.SessionID) ([]domain.CodexMCPConfig, error) {
	var data string
	err := s.readDB.QueryRowContext(ctx, "SELECT config_json FROM session_codex_mcp_configs WHERE session_id = ?", string(id)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []domain.CodexMCPConfig
	if err := json.Unmarshal([]byte(data), &configs); err != nil {
		return nil, errors.New("invalid session MCP reference snapshot")
	}
	return configs, nil
}

// ListCodexMCPConfigs 读取机器共享的引用配置；不解析或保存凭据。
func (s *Store) ListCodexMCPConfigs(ctx context.Context) ([]domain.CodexMCPConfig, error) {
	rows, err := s.readDB.QueryContext(ctx, "SELECT config_json FROM codex_mcp_configs ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := make([]domain.CodexMCPConfig, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var config domain.CodexMCPConfig
		if err := json.Unmarshal([]byte(data), &config); err != nil {
			return nil, errors.New("invalid stored MCP configuration")
		}
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

// SaveCodexMCPConfig 区分创建和更新，避免并发保存悄然覆盖现有配置。
func (s *Store) SaveCodexMCPConfig(ctx context.Context, config domain.CodexMCPConfig, create bool) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if create {
		_, err = s.writeDB.ExecContext(ctx, "INSERT INTO codex_mcp_configs (name, config_json) VALUES (?, ?)", config.Name, string(data))
		return err
	}
	result, err := s.writeDB.ExecContext(ctx, "UPDATE codex_mcp_configs SET config_json = ? WHERE name = ?", string(data), config.Name)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

// DeleteCodexMCPConfig 删除引用配置，不影响任何运行中的 provider。
func (s *Store) DeleteCodexMCPConfig(ctx context.Context, name string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	result, err := s.writeDB.ExecContext(ctx, "DELETE FROM codex_mcp_configs WHERE name = ?", name)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}
