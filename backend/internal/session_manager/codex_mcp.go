package sessionmanager

import (
	"context"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// codexMCPStore 是引用快照边界；其他 harness 不读取统一配置。
type codexMCPStore interface {
	CaptureCodexMCPConfigs(context.Context, domain.SessionID) ([]domain.CodexMCPConfig, error)
	SessionCodexMCPConfigs(context.Context, domain.SessionID) ([]domain.CodexMCPConfig, error)
}

// codexMCPServers 新会话冻结配置，恢复读取原快照，始终只传递引用名。
func (m *Manager) codexMCPServers(ctx context.Context, id domain.SessionID, harness domain.AgentHarness, fresh bool) ([]ports.ChatMCPServerConfig, error) {
	if harness != domain.HarnessCodex {
		return nil, nil
	}
	store, ok := m.store.(codexMCPStore)
	if !ok {
		return nil, nil
	}
	var configs []domain.CodexMCPConfig
	var err error
	if fresh {
		configs, err = store.CaptureCodexMCPConfigs(ctx, id)
	} else {
		configs, err = store.SessionCodexMCPConfigs(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	servers := make([]ports.ChatMCPServerConfig, 0, len(configs))
	for _, c := range configs {
		if !c.Enabled {
			continue
		}
		servers = append(servers, ports.ChatMCPServerConfig{Name: c.Name, Type: c.Transport, Command: c.Command, Args: c.Args, URL: c.URL, EnvRefs: c.EnvRefs, HeaderRefs: c.HeaderRefs})
	}
	return servers, nil
}
