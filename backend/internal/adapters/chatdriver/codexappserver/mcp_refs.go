package codexappserver

import (
	"errors"
	"fmt"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"maps"
	"os"
	"strings"
)

// mcpDiagnosticError 对带引用会话隐藏 RPC 原始错误，防止 provider 回显凭据。
func (c *conversation) mcpDiagnosticError(err error) error {
	sensitive := false
	c.sensitiveMCP.Range(func(_, _ any) bool { sensitive = true; return false })
	if sensitive {
		return errors.New("Codex MCP operation failed; check referenced credentials and server availability")
	}
	return err
}

// registerMCPReferences 在启动请求前标记诊断敏感性，不保存凭据值。
func (c *conversation) registerMCPReferences(servers []ports.ChatMCPServerConfig) {
	for _, server := range servers {
		if len(server.EnvRefs) > 0 || len(server.HeaderRefs) > 0 {
			c.sensitiveMCP.Store(server.Name, true)
		}
	}
}

// resolveMCPReferences 在发送 thread 配置前复制并解析引用，不修改可持久化的启动快照。
// 错误仅包含引用名；缺失引用时不向 provider 发送部分配置。
func resolveMCPReferences(servers []ports.ChatMCPServerConfig) ([]ports.ChatMCPServerConfig, error) {
	resolved := make([]ports.ChatMCPServerConfig, 0, len(servers))
	for _, server := range servers {
		server.Env = maps.Clone(server.Env)
		server.Headers = maps.Clone(server.Headers)
		if server.Env == nil {
			server.Env = map[string]string{}
		}
		if server.Headers == nil {
			server.Headers = map[string]string{}
		}
		for _, binding := range []struct {
			refs, target map[string]string
			header       bool
		}{{server.EnvRefs, server.Env, false}, {server.HeaderRefs, server.Headers, true}} {
			for key, ref := range binding.refs {
				value, ok := os.LookupEnv(ref)
				if !ok || value == "" {
					return nil, fmt.Errorf("MCP environment reference %s is missing", ref)
				}
				if binding.header && strings.ContainsAny(value, "\r\n") {
					return nil, fmt.Errorf("MCP header reference %s has an invalid value", ref)
				}
				binding.target[key] = value
			}
		}
		resolved = append(resolved, server)
	}
	return resolved, nil
}
