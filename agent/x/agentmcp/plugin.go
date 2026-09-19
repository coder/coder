package agentmcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/xerrors"
)

// pluginMCPSchema is the only $schema accepted in a plugin mcp.json.
const pluginMCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

// pluginServerEntry is one mcpServers entry of a plugin mcp.json.
// Fields not listed here are ignored.
type pluginServerEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// parsePluginConfig parses a plugin mcp.json body. The top level must
// hold exactly $schema and mcpServers with the supported schema URL, or
// the whole file is rejected. Entries are validated independently.
func parsePluginConfig(data []byte, src ConfigSource) ([]ServerConfig, map[string]error, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, xerrors.Errorf("parse plugin mcp config %q: %w", src.Path, err)
	}
	if len(top) != 2 {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: top level must contain exactly $schema and mcpServers", src.Path)
	}
	rawSchema, ok := top["$schema"]
	if !ok {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: missing $schema", src.Path)
	}
	rawServers, ok := top["mcpServers"]
	if !ok {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: missing mcpServers", src.Path)
	}

	var schema string
	if err := json.Unmarshal(rawSchema, &schema); err != nil || schema != pluginMCPSchema {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: unsupported $schema %s", src.Path, rawSchema)
	}

	var entries map[string]json.RawMessage
	if err := json.Unmarshal(rawServers, &entries); err != nil {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: mcpServers must be an object: %w", src.Path, err)
	}

	scope := *src.Plugin
	servers := make([]ServerConfig, 0, len(entries))
	entryErrs := make(map[string]error)
	for name, raw := range entries {
		server, err := parsePluginEntry(name, raw, scope)
		if err != nil {
			entryErrs[name] = err
			continue
		}
		servers = append(servers, server)
	}

	sortServers(servers)
	return servers, entryErrs, nil
}

func parsePluginEntry(name string, raw json.RawMessage, scope PluginScope) (ServerConfig, error) {
	var entry pluginServerEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ServerConfig{}, xerrors.Errorf("parse server %q: %w", name, err)
	}
	if err := validateServerName(name); err != nil {
		return ServerConfig{}, err
	}

	cfg := ServerConfig{
		Name:      name,
		Transport: entry.Type,
		Plugin:    &scope,
	}
	switch entry.Type {
	case "stdio":
		if err := fillPluginStdio(&cfg, entry, scope); err != nil {
			return ServerConfig{}, xerrors.Errorf("server %q: %w", name, err)
		}
	case "streamable-http":
		if err := validatePluginURL(entry.URL); err != nil {
			return ServerConfig{}, xerrors.Errorf("server %q: %w", name, err)
		}
		cfg.URL = entry.URL
		cfg.Headers = entry.Headers
	case "sse":
		// The agent's SSE client does not keep its event stream alive past
		// the connect handshake, so an sse plugin server would connect and
		// then silently stop receiving. Legacy .mcp.json entries are not
		// affected by this check.
		return ServerConfig{}, xerrors.Errorf("server %q: transport sse is deprecated in MCP and not supported for plugins; use streamable-http", name)
	case "":
		return ServerConfig{}, xerrors.Errorf("server %q: missing type", name)
	default:
		return ServerConfig{}, xerrors.Errorf("server %q: unsupported type %q", name, entry.Type)
	}
	return cfg, nil
}

// fillPluginStdio validates and resolves a stdio entry's command, args,
// env, and cwd into cfg. Command and Cwd are stored as absolute paths
// (Command only when it was ./-relative).
func fillPluginStdio(cfg *ServerConfig, entry pluginServerEntry, scope PluginScope) error {
	command, err := resolvePluginCommand(entry.Command, scope.Root)
	if err != nil {
		return err
	}

	for key := range entry.Env {
		if key == pluginRootEnv || key == pluginDataEnv {
			return xerrors.Errorf("env may not set %s", key)
		}
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return xerrors.Errorf("invalid env name %q", key)
		}
	}

	cwd, err := resolvePluginCwd(entry.Cwd, scope)
	if err != nil {
		return err
	}

	var args []string
	if entry.Args != nil {
		args = make([]string, 0, len(entry.Args))
		for _, arg := range entry.Args {
			args = append(args, expandPluginPlaceholders(arg, scope))
		}
	}
	var env map[string]string
	if entry.Env != nil {
		env = make(map[string]string, len(entry.Env))
		for key, value := range entry.Env {
			env[key] = expandPluginPlaceholders(value, scope)
		}
	}

	cfg.Command = command
	cfg.Args = args
	cfg.Env = env
	cfg.Cwd = cwd
	return nil
}

// resolvePluginCommand accepts a bare executable name, returned as is
// for PATH lookup, or a ./-relative path, returned as the absolute
// symlink-resolved path inside root. Anything else is rejected.
func resolvePluginCommand(command, root string) (string, error) {
	if command == "" {
		return "", xerrors.New("missing command")
	}
	if strings.ContainsFunc(command, unicode.IsSpace) {
		return "", xerrors.Errorf("command %q must be a single token", command)
	}
	if strings.HasPrefix(command, "./") {
		resolved, err := canonicalExisting(filepath.Join(root, command))
		if err != nil {
			return "", xerrors.Errorf("command %q: %w", command, err)
		}
		if err := requireInside(root, resolved); err != nil {
			return "", xerrors.Errorf("command %q: %w", command, err)
		}
		return resolved, nil
	}
	if filepath.IsAbs(command) || strings.ContainsRune(command, '/') ||
		strings.ContainsRune(command, filepath.Separator) || command == "." || command == ".." {
		return "", xerrors.Errorf("command %q must be a bare name or a ./-relative path", command)
	}
	return command, nil
}

// resolvePluginCwd returns the absolute working directory for a stdio
// entry. Empty selects the plugin root. Otherwise cwd must begin with
// ./, ${PLUGIN_ROOT}, or ${PLUGIN_DATA}; the first two must resolve to
// an existing directory inside Root and the last inside DataDir, which
// is created at launch and so may not exist yet.
func resolvePluginCwd(cwd string, scope PluginScope) (string, error) {
	if cwd == "" {
		return canonicalDir(scope.Root)
	}
	var (
		base, target string
		resolve      = canonicalExisting
	)
	switch {
	case strings.HasPrefix(cwd, "./"):
		base, target = scope.Root, filepath.Join(scope.Root, cwd)
	case strings.HasPrefix(cwd, pluginRootPlaceholder):
		base, target = scope.Root, expandPluginPlaceholders(cwd, scope)
	case strings.HasPrefix(cwd, pluginDataPlaceholder):
		base, target = scope.DataDir, expandPluginPlaceholders(cwd, scope)
		resolve = resolveExistingPrefix
	default:
		return "", xerrors.Errorf("cwd %q must start with ./, %s, or %s", cwd, pluginRootPlaceholder, pluginDataPlaceholder)
	}
	resolved, err := resolve(target)
	if err != nil {
		return "", xerrors.Errorf("cwd %q: %w", cwd, err)
	}
	if err := requireInside(base, resolved); err != nil {
		return "", xerrors.Errorf("cwd %q: %w", cwd, err)
	}
	return resolved, nil
}

// canonicalDir returns dir with symlinks resolved. A missing tail is
// appended lexically to its longest existing ancestor, so a base that
// does not exist yet (DataDir before launch) canonicalizes the same way
// as paths resolved beneath it.
func canonicalDir(dir string) (string, error) {
	return resolveExistingPrefix(dir)
}

// requireInside reports an error unless resolved, an already
// symlink-resolved absolute path, is base itself or lies beneath it.
func requireInside(base, resolved string) error {
	canonBase, err := canonicalDir(base)
	if err != nil {
		return xerrors.Errorf("resolve %q: %w", base, err)
	}
	if resolved != canonBase && !strings.HasPrefix(resolved, canonBase+string(filepath.Separator)) {
		return xerrors.Errorf("%q resolves outside %q", resolved, canonBase)
	}
	return nil
}

// canonicalExisting resolves symlinks in an existing path. When the
// filesystem cannot report symlink targets (Windows substituted drives
// fail EvalSymlinks even for plain directories) and the path itself is
// not a symlink, the cleaned path is used instead; a path that is a
// symlink must resolve or it is rejected.
func canonicalExisting(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	// The failure is reported as a not-found error on those drives too,
	// so the path itself decides: missing or a symlink means the error
	// stands, anything else falls back to the cleaned path.
	info, lerr := os.Lstat(path)
	if lerr != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", err
	}
	return filepath.Clean(path), nil
}

// resolveExistingPrefix is canonicalExisting that tolerates a
// missing tail: the longest existing ancestor is resolved and the
// remaining components are appended lexically, so a symlinked ancestor
// of a not-yet-created directory is still followed.
func resolveExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, err := canonicalExisting(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent, name := filepath.Split(path)
		parent = filepath.Clean(parent)
		if parent == path || name == "" {
			return "", err
		}
		tail = append(tail, name)
		path = parent
	}
}

// validatePluginURL enforces the plugin HTTP transport URL rules: an
// absolute URL, https except for loopback hosts, no userinfo, and no
// fragment.
func validatePluginURL(raw string) error {
	if raw == "" {
		return xerrors.New("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return xerrors.Errorf("invalid url %q: %w", raw, err)
	}
	if !u.IsAbs() || u.Host == "" {
		return xerrors.Errorf("url %q must be absolute", raw)
	}
	if u.User != nil {
		return xerrors.Errorf("url %q must not contain userinfo", raw)
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return xerrors.Errorf("url %q must not contain a fragment", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return xerrors.Errorf("url %q must use https for non-loopback hosts", raw)
	default:
		return xerrors.Errorf("url %q has unsupported scheme %q", raw, u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ensurePluginDataDir creates the plugin's DataDir, readable only by the
// agent's user.
func ensurePluginDataDir(scope PluginScope) error {
	if scope.DataDir == "" {
		return xerrors.New("plugin data dir is not set")
	}
	if err := os.MkdirAll(scope.DataDir, 0o700); err != nil {
		return xerrors.Errorf("create plugin data dir %q: %w", scope.DataDir, err)
	}
	return nil
}
