package agentmcp

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// ServerConfig describes a single MCP server parsed from a .mcp.json file.
type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"type"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
}

// mcpConfigFile mirrors the on-disk .mcp.json schema.
type mcpConfigFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// mcpServerEntry is a single server block inside mcpServers.
type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ParseConfig reads a .mcp.json file at path and returns the declared
// MCP servers sorted by name. It returns an empty slice when the
// mcpServers key is missing or empty.
func ParseConfig(path string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf("read mcp config %q: %w", path, err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, xerrors.Errorf("parse mcp config %q: %w", path, err)
	}

	if len(cfg.MCPServers) == 0 {
		return []ServerConfig{}, nil
	}

	servers := make([]ServerConfig, 0, len(cfg.MCPServers))
	for name, raw := range cfg.MCPServers {
		var entry mcpServerEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, xerrors.Errorf("parse server %q in %q: %w", name, path, err)
		}

		if strings.Contains(name, ToolNameSep) || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_") {
			return nil, xerrors.Errorf("server name %q in %q contains reserved separator %q or leading/trailing underscore", name, path, ToolNameSep)
		}

		transport := inferTransport(entry)

		if transport == "" {
			return nil, xerrors.Errorf("server %q in %q has no command or url", name, path)
		}

		resolveEnvVars(entry.Env)

		servers = append(servers, ServerConfig{
			Name:      name,
			Transport: transport,
			Command:   entry.Command,
			Args:      entry.Args,
			Env:       entry.Env,
			URL:       entry.URL,
			Headers:   entry.Headers,
		})
	}

	slices.SortFunc(servers, func(a, b ServerConfig) int {
		return strings.Compare(a.Name, b.Name)
	})

	return servers, nil
}

// inferTransport determines the transport type for a server entry.
// An explicit "type" field takes priority; otherwise the presence
// of "command" implies stdio and "url" implies http.
func inferTransport(e mcpServerEntry) string {
	if e.Type != "" {
		return e.Type
	}
	if e.Command != "" {
		return "stdio"
	}
	if e.URL != "" {
		return "http"
	}
	return ""
}

// resolveEnvVars expands ${VAR} references in env map values
// using the current process environment.
func resolveEnvVars(env map[string]string) {
	for k, v := range env {
		env[k] = os.Expand(v, os.Getenv)
	}
}
