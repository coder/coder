package agentmcp

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// PluginScope identifies the Agent Plugin an mcp.json file belongs to.
// Root is the plugin directory that relative commands, cwd, and the
// PLUGIN_ROOT placeholder resolve against. DataDir is the writable
// per-installation directory exported as PLUGIN_DATA; see PluginDataDir.
type PluginScope struct {
	Name    string
	Root    string
	DataDir string
}

// ConfigSource names one MCP config file to load. A nil Plugin means
// Path is a legacy .mcp.json file; a non-nil Plugin means Path is a
// plugin mcp.json parsed under the plugin rules.
type ConfigSource struct {
	Path   string
	Plugin *PluginScope
}

// PathSources wraps plain .mcp.json paths as legacy ConfigSources.
func PathSources(paths []string) []ConfigSource {
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
	// Plugin is set for servers declared by a plugin mcp.json. It is
	// part of the config identity, so a server that moves between a
	// legacy file and a plugin is reconnected rather than kept.
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

// ParseConfig reads a legacy .mcp.json file at path. See ParseSource.
func ParseConfig(path string) ([]ServerConfig, map[string]error, error) {
	return ParseSource(ConfigSource{Path: path})
}

// ParseSource reads the config file named by src and returns the valid
// servers sorted by name plus, keyed by entry name, the error that
// excluded each invalid entry. The returned error is non-nil only when
// the whole file is unusable: unreadable, malformed JSON, or, for plugin
// files, a top level that does not match the plugin mcp.json schema.
func ParseSource(src ConfigSource) ([]ServerConfig, map[string]error, error) {
	if src.Plugin != nil {
		// A plugin's mcp.json is read through its resolved path, which
		// must stay inside the plugin root.
		resolved, err := canonicalExisting(src.Path)
		if err != nil {
			return nil, nil, xerrors.Errorf("resolve mcp config %q: %w", src.Path, err)
		}
		if err := requireInside(src.Plugin.Root, resolved); err != nil {
			return nil, nil, xerrors.Errorf("mcp config %q: %w", src.Path, err)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, nil, xerrors.Errorf("read mcp config %q: %w", src.Path, err)
		}
		return parsePluginConfig(data, src)
	}
	data, err := os.ReadFile(src.Path)
	if err != nil {
		return nil, nil, xerrors.Errorf("read mcp config %q: %w", src.Path, err)
	}
	return parseLegacyConfig(data, src.Path)
}

// parseLegacyConfig parses a .mcp.json body. Each entry is validated
// on its own; an invalid entry is reported in the returned map and
// does not affect its siblings.
func parseLegacyConfig(data []byte, path string) ([]ServerConfig, map[string]error, error) {
	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, xerrors.Errorf("parse mcp config %q: %w", path, err)
	}

	servers := make([]ServerConfig, 0, len(cfg.MCPServers))
	entryErrs := make(map[string]error)
	for name, raw := range cfg.MCPServers {
		server, err := parseLegacyEntry(name, raw)
		if err != nil {
			entryErrs[name] = err
			continue
		}
		servers = append(servers, server)
	}

	sortServers(servers)
	return servers, entryErrs, nil
}

func parseLegacyEntry(name string, raw json.RawMessage) (ServerConfig, error) {
	var entry mcpServerEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ServerConfig{}, xerrors.Errorf("parse server %q: %w", name, err)
	}

	if err := validateServerName(name); err != nil {
		return ServerConfig{}, err
	}

	transport := inferTransport(entry)
	if transport == "" {
		return ServerConfig{}, xerrors.Errorf("server %q has no command or url", name)
	}

	resolveEnvVars(entry.Env)

	return ServerConfig{
		Name:      name,
		Transport: transport,
		Command:   entry.Command,
		Args:      entry.Args,
		Env:       entry.Env,
		URL:       entry.URL,
		Headers:   entry.Headers,
	}, nil
}

// validateServerName rejects names that cannot be split back out of a
// prefixed "server__tool" name.
func validateServerName(name string) error {
	if strings.Contains(name, ToolNameSep) || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_") {
		return xerrors.Errorf("server name %q contains reserved separator %q or leading/trailing underscore", name, ToolNameSep)
	}
	return nil
}

func sortServers(servers []ServerConfig) {
	slices.SortFunc(servers, func(a, b ServerConfig) int {
		return strings.Compare(a.Name, b.Name)
	})
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
