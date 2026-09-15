package agentmcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// PluginScope identifies the Agent Plugin an mcp.json file belongs to.
// Name must be unique per reload; Root and DataDir must be absolute.
// Only a change to mcp.json itself triggers a reparse, so the plugin's
// files must be in place before its mcp.json is passed in.
type PluginScope struct {
	Name    string
	Root    string
	DataDir string
}

// ConfigSource names one MCP config file to load. A nil Plugin marks a
// legacy .mcp.json file; otherwise Path must be the mcp.json at the
// plugin root.
type ConfigSource struct {
	Path   string
	Plugin *PluginScope
}

// LegacySources wraps plain .mcp.json paths as legacy ConfigSources.
func LegacySources(paths []string) []ConfigSource {
	sources := make([]ConfigSource, 0, len(paths))
	for _, p := range paths {
		sources = append(sources, ConfigSource{Path: p})
	}
	return sources
}

// ServerConfig describes a single MCP server parsed from a config file.
type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"type"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	// Cwd is the absolute working directory for a stdio server. Empty
	// selects the workspace working directory at launch.
	Cwd string `json:"cwd,omitempty"`
	// Plugin is set for plugin mcp.json servers. A scope change
	// reconnects the server.
	Plugin *PluginScope `json:"plugin,omitempty"`
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

// ParseSource reads the config file named by src and returns its valid
// servers sorted by name. A legacy .mcp.json fails whole on any invalid
// entry. A plugin mcp.json fails whole on an invalid scope, path, or top
// level; otherwise invalid entries are returned keyed by name and their
// siblings still load.
func ParseSource(src ConfigSource) ([]ServerConfig, map[string]error, error) {
	if src.Plugin == nil {
		servers, err := parseLegacyConfig(src.Path)
		return servers, nil, err
	}
	if !filepath.IsAbs(src.Plugin.Root) || !filepath.IsAbs(src.Plugin.DataDir) {
		return nil, nil, xerrors.Errorf("mcp config %q: plugin root %q and data dir %q must be absolute", src.Path, src.Plugin.Root, src.Plugin.DataDir)
	}
	if filepath.Clean(src.Path) != filepath.Join(src.Plugin.Root, pluginMCPFileName) {
		return nil, nil, xerrors.Errorf("mcp config %q: a plugin mcp config must be %s at the plugin root", src.Path, pluginMCPFileName)
	}
	scope := *src.Plugin
	var err error
	if scope.Root, err = canonicalAllowMissing(scope.Root, scope.Root); err != nil {
		return nil, nil, xerrors.Errorf("resolve plugin root %q: %w", src.Plugin.Root, err)
	}
	// os.Root refuses a symlinked mcp.json that leads outside the root.
	data, err := readInRoot(scope.Root, pluginMCPFileName)
	if err != nil {
		return nil, nil, xerrors.Errorf("read mcp config %q: %w", src.Path, err)
	}
	return parsePluginConfig(data, ConfigSource{Path: src.Path, Plugin: &scope})
}

// parseLegacyConfig reads a .mcp.json file at path and returns the
// declared MCP servers sorted by name. It returns an empty slice when
// the mcpServers key is missing or empty.
func parseLegacyConfig(path string) ([]ServerConfig, error) {
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

		if reservedServerName(name) {
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

// reservedServerName reports whether name cannot be split back out of a
// prefixed "server__tool" name.
func reservedServerName(name string) bool {
	return strings.Contains(name, ToolNameSep) || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_")
}

func readInRoot(root, rel string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.ReadFile(rel)
}

// inferTransport determines the transport type for a server entry.
// An explicit "type" field takes priority; otherwise the presence
// of "command" implies stdio and "url" implies http.
func inferTransport(e mcpServerEntry) string {
	// "streamable-http" is the Agent Plugins spelling.
	if e.Type == "streamable-http" {
		return "http"
	}
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
