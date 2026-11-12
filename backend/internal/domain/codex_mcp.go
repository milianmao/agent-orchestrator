package domain

import (
	"errors"
	"net/url"
	"regexp"
)

// CodexMCPConfig 是机器共享的引用配置；任何字段都不得接收解析后的凭据。
type CodexMCPConfig struct {
	Name       string            `json:"name"`
	Transport  string            `json:"transport"`
	Enabled    bool              `json:"enabled"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	URL        string            `json:"url,omitempty"`
	EnvRefs    map[string]string `json:"envRefs,omitempty"`
	HeaderRefs map[string]string `json:"headerRefs,omitempty"`
}

var mcpName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var mcpEnvName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var mcpHeaderName = regexp.MustCompile("^[!#$%&'*+.^_`|~a-zA-Z0-9-]+$")

// Validate 在持久化前拒绝含混的 transport、保留名称及非引用凭据。
// 错误只描述约束，不回显调用方提供的值。
func (c CodexMCPConfig) Validate() error {
	if !mcpName.MatchString(c.Name) || c.Name == "ao" {
		return errors.New("MCP name must contain 1-64 letters, digits, underscores or hyphens and must not be ao")
	}
	switch c.Transport {
	case "stdio":
		if c.Command == "" || c.URL != "" || len(c.HeaderRefs) != 0 {
			return errors.New("stdio MCP requires command and does not accept URL or headers")
		}
	case "http":
		u, err := url.Parse(c.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.Command != "" || len(c.Args) != 0 || len(c.EnvRefs) != 0 {
			return errors.New("http MCP requires an http(s) URL without credentials, query or fragment and does not accept command, args or env")
		}
	default:
		return errors.New("MCP transport must be stdio or http")
	}
	for key, ref := range c.EnvRefs {
		if !mcpEnvName.MatchString(key) || !mcpEnvName.MatchString(ref) {
			return errors.New("MCP env keys and references must be environment variable names")
		}
	}
	for key, ref := range c.HeaderRefs {
		if !mcpHeaderName.MatchString(key) || !mcpEnvName.MatchString(ref) {
			return errors.New("MCP headers require valid header names and environment variable references")
		}
	}
	return nil
}
