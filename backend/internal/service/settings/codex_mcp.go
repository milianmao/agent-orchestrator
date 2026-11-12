package settings

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sort"
	"sync"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// CodexMCPStore 只持久化引用；凭据解析不属于存储边界。
type CodexMCPStore interface {
	ListCodexMCPConfigs(context.Context) ([]domain.CodexMCPConfig, error)
	SaveCodexMCPConfig(context.Context, domain.CodexMCPConfig, bool) error
	DeleteCodexMCPConfig(context.Context, string) error
}

// CodexMCPStatus 将缺失引用反馈给设置页，不暴露解析值。
type CodexMCPStatus struct {
	domain.CodexMCPConfig
	StartupFailure    string   `json:"startupFailure,omitempty"`
	MissingReferences []string `json:"missingReferences"`
}

// CodexMCPService 管理仅对新会话生效的机器级配置。
type CodexMCPService struct {
	Store CodexMCPStore
	// mutationMu 串行化局部更新的读取与写入，避免同时启停和编辑丢失字段。
	mutationMu sync.Mutex
}

// CodexMCPPatch 的指针字段区分省略与显式清空，尤其保留 enabled:false。
type CodexMCPPatch struct {
	Transport  *string            `json:"transport,omitempty"`
	Enabled    *bool              `json:"enabled,omitempty"`
	Command    *string            `json:"command,omitempty"`
	Args       *[]string          `json:"args,omitempty"`
	URL        *string            `json:"url,omitempty"`
	EnvRefs    *map[string]string `json:"envRefs,omitempty"`
	HeaderRefs *map[string]string `json:"headerRefs,omitempty"`
}

// Patch 保留未提供的字段，完成合并后统一校验 transport 约束。
func (s *CodexMCPService) Patch(ctx context.Context, name string, patch CodexMCPPatch) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	configs, err := s.Store.ListCodexMCPConfigs(ctx)
	if err != nil {
		return apierr.New(apierr.KindInternal, "MCP_CONFIG_READ_FAILED", "Unable to read MCP configurations", nil)
	}
	for _, config := range configs {
		if config.Name != name {
			continue
		}
		if patch.Transport != nil {
			config.Transport = *patch.Transport
		}
		if patch.Enabled != nil {
			config.Enabled = *patch.Enabled
		}
		if patch.Command != nil {
			config.Command = *patch.Command
		}
		if patch.Args != nil {
			config.Args = *patch.Args
		}
		if patch.URL != nil {
			config.URL = *patch.URL
		}
		if patch.EnvRefs != nil {
			config.EnvRefs = *patch.EnvRefs
		}
		if patch.HeaderRefs != nil {
			config.HeaderRefs = *patch.HeaderRefs
		}
		return s.save(ctx, config, false)
	}
	return apierr.NotFound("MCP_CONFIG_NOT_FOUND", "MCP configuration not found")
}

// List 返回配置和进程环境中的引用可用性，不返回环境值。
func (s *CodexMCPService) List(ctx context.Context) ([]CodexMCPStatus, error) {
	failures := map[string]string{}
	if reader, ok := s.Store.(interface {
		CodexMCPStartupFailures(context.Context) (map[string]string, error)
	}); ok {
		var err error
		failures, err = reader.CodexMCPStartupFailures(ctx)
		if err != nil {
			return nil, apierr.New(apierr.KindInternal, "MCP_STATUS_READ_FAILED", "Unable to read MCP startup status", nil)
		}
	}
	configs, err := s.Store.ListCodexMCPConfigs(ctx)
	if err != nil {
		return nil, apierr.New(apierr.KindInternal, "MCP_CONFIG_READ_FAILED", "Unable to read MCP configurations", nil)
	}
	result := make([]CodexMCPStatus, 0, len(configs))
	for _, c := range configs {
		missing := make(map[string]bool)
		for _, refs := range []map[string]string{c.EnvRefs, c.HeaderRefs} {
			for _, ref := range refs {
				if value, ok := os.LookupEnv(ref); !ok || value == "" {
					missing[ref] = true
				}
			}
		}
		status := CodexMCPStatus{CodexMCPConfig: c, MissingReferences: []string{}}
		status.StartupFailure = failures[c.Name]
		for ref := range missing {
			status.MissingReferences = append(status.MissingReferences, ref)
		}
		sort.Strings(status.MissingReferences)
		result = append(result, status)
	}
	return result, nil
}

// Save 校验后区分创建、更新；数据库错误不向调用方回显配置内容。
func (s *CodexMCPService) Save(ctx context.Context, config domain.CodexMCPConfig, create bool) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.save(ctx, config, create)
}

// save 在持有 mutationMu 时校验并写入，避免局部更新与删除交错。
func (s *CodexMCPService) save(ctx context.Context, config domain.CodexMCPConfig, create bool) error {
	if err := config.Validate(); err != nil {
		return apierr.Invalid("MCP_CONFIG_INVALID", err.Error(), nil)
	}
	configs, err := s.Store.ListCodexMCPConfigs(ctx)
	if err != nil {
		return apierr.New(apierr.KindInternal, "MCP_CONFIG_READ_FAILED", "Unable to read MCP configurations", nil)
	}
	found := false
	for _, c := range configs {
		if c.Name == config.Name {
			found = true
			break
		}
	}
	if create && found {
		return apierr.Conflict("MCP_CONFIG_EXISTS", "MCP configuration already exists", nil)
	}
	if !create && !found {
		return apierr.NotFound("MCP_CONFIG_NOT_FOUND", "MCP configuration not found")
	}
	if err := s.Store.SaveCodexMCPConfig(ctx, config, create); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apierr.NotFound("MCP_CONFIG_NOT_FOUND", "MCP configuration not found")
		}
		return apierr.New(apierr.KindInternal, "MCP_CONFIG_SAVE_FAILED", "Unable to save MCP configuration", nil)
	}
	return nil
}

// Delete 只删除 AO 的引用记录，不操作正在运行的服务器。
func (s *CodexMCPService) Delete(ctx context.Context, name string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.Store.DeleteCodexMCPConfig(ctx, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apierr.NotFound("MCP_CONFIG_NOT_FOUND", "MCP configuration not found")
		}
		return apierr.New(apierr.KindInternal, "MCP_CONFIG_DELETE_FAILED", "Unable to delete MCP configuration", nil)
	}
	return nil
}
