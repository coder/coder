package agentmcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"
)

// pluginMCPSchema is the only $schema accepted in a plugin mcp.json.
const pluginMCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

// pluginServerEntry is one mcpServers entry of a plugin mcp.json.
type pluginServerEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func parsePluginConfig(data []byte, src ConfigSource) ([]ServerConfig, map[string]error, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, xerrors.Errorf("parse plugin mcp config %q: %w", src.Path, err)
	}
	rawSchema, hasSchema := top["$schema"]
	rawServers, hasServers := top["mcpServers"]
	if len(top) != 2 || !hasSchema || !hasServers {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: top level must contain exactly $schema and mcpServers, found %q", src.Path, slices.Sorted(maps.Keys(top)))
	}

	var schema string
	if err := json.Unmarshal(rawSchema, &schema); err != nil || schema != pluginMCPSchema {
		return nil, nil, xerrors.Errorf("plugin mcp config %q: unsupported $schema %s; want %q", src.Path, rawSchema, pluginMCPSchema)
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

	slices.SortFunc(servers, func(a, b ServerConfig) int {
		return strings.Compare(a.Name, b.Name)
	})
	return servers, entryErrs, nil
}

func validateServerName(name string) error {
	if reservedServerName(name) {
		return xerrors.Errorf("server name %q contains reserved separator %q or leading/trailing underscore", name, ToolNameSep)
	}
	return nil
}

// pluginEntryFields lists the fields each entry type allows. Any other
// field, including one belonging to another type, makes the entry
// invalid.
var pluginEntryFields = map[string][]string{
	"stdio":           {"type", "command", "args", "env", "cwd"},
	"streamable-http": {"type", "url", "headers"},
	"sse":             {"type", "url", "headers"},
}

func parsePluginEntry(name string, raw json.RawMessage, scope PluginScope) (ServerConfig, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ServerConfig{}, xerrors.Errorf("parse server %q: %w", name, err)
	}
	var entry pluginServerEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ServerConfig{}, xerrors.Errorf("parse server %q: %w", name, err)
	}
	if err := validateServerName(name); err != nil {
		return ServerConfig{}, err
	}
	if allowed, ok := pluginEntryFields[entry.Type]; ok {
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			if !slices.Contains(allowed, key) {
				return ServerConfig{}, xerrors.Errorf("server %q: field %q is not allowed for type %q", name, key, entry.Type)
			}
		}
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
	case "streamable-http", "sse":
		if err := validatePluginURL(entry.URL); err != nil {
			return ServerConfig{}, xerrors.Errorf("server %q: %w", name, err)
		}
		if entry.Type == "streamable-http" {
			cfg.Transport = "http"
		}
		if err := validatePluginHeaders(entry.Headers); err != nil {
			return ServerConfig{}, xerrors.Errorf("server %q: %w", name, err)
		}
		cfg.URL = entry.URL
		cfg.Headers = entry.Headers
	case "":
		return ServerConfig{}, xerrors.Errorf(`server %q: missing type; set "type" to "stdio", "streamable-http", or "sse"`, name)
	case "http":
		return ServerConfig{}, xerrors.Errorf(`server %q: unsupported type "http"; use "streamable-http"`, name)
	default:
		return ServerConfig{}, xerrors.Errorf(`server %q: unsupported type %q; set "type" to "stdio", "streamable-http", or "sse"`, name, entry.Type)
	}
	return cfg, nil
}

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
		resolved, err := canonicalExisting(root, filepath.Join(root, command))
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
// entry. Empty selects the plugin root. Otherwise cwd must be ./-relative
// or be ${PLUGIN_ROOT} or ${PLUGIN_DATA}, alone or followed by /. The
// first two must resolve to an existing directory inside Root. The last
// must resolve inside DataDir and may not exist yet; it is created at
// launch.
func resolvePluginCwd(cwd string, scope PluginScope) (string, error) {
	if cwd == "" {
		return canonicalAllowMissing(scope.Root, scope.Root)
	}
	var (
		base, target string
		resolve      = canonicalExisting
	)
	switch {
	case strings.HasPrefix(cwd, "./"):
		base, target = scope.Root, filepath.Join(scope.Root, cwd)
	case hasPlaceholderPrefix(cwd, pluginRootPlaceholder):
		base, target = scope.Root, expandPluginPlaceholders(cwd, scope)
	case hasPlaceholderPrefix(cwd, pluginDataPlaceholder):
		base, target = scope.DataDir, expandPluginPlaceholders(cwd, scope)
		resolve = canonicalAllowMissing
	default:
		return "", xerrors.Errorf("cwd %q must start with ./, %s, or %s", cwd, pluginRootPlaceholder, pluginDataPlaceholder)
	}
	resolved, err := resolve(base, target)
	if err != nil {
		return "", xerrors.Errorf("cwd %q: %w", cwd, err)
	}
	if err := requireInside(base, resolved); err != nil {
		return "", xerrors.Errorf("cwd %q: %w", cwd, err)
	}
	if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
		return "", xerrors.Errorf("cwd %q is not a directory", cwd)
	}
	return resolved, nil
}

func hasPlaceholderPrefix(s, placeholder string) bool {
	return s == placeholder || strings.HasPrefix(s, placeholder+"/")
}

// requireInside reports an error unless resolved, an already
// symlink-resolved absolute path, is base itself or lies beneath it.
func requireInside(base, resolved string) error {
	canonBase, err := canonicalAllowMissing(base, base)
	if err != nil {
		return xerrors.Errorf("resolve %q: %w", base, err)
	}
	if resolved != canonBase && !strings.HasPrefix(resolved, canonBase+string(filepath.Separator)) {
		return xerrors.Errorf("%q resolves outside %q", resolved, canonBase)
	}
	return nil
}

// canonicalExisting resolves symlinks in an existing path that is
// checked for containment in base. When the filesystem cannot report
// symlink targets (Windows substituted drives fail EvalSymlinks even for
// plain directories) and no component from base down to path is a link,
// the cleaned path is used instead; otherwise the error stands.
func canonicalExisting(base, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	// Substituted drives also fail with a not-found error, so Lstat
	// decides whether the error stands.
	if hasLinkInside(base, path) {
		return "", err
	}
	return filepath.Clean(path), nil
}

// hasLinkInside reports whether path, or any component between it and
// base, is a symlink or another reparse point such as a Windows
// junction, or cannot be read. Lstat follows links in all but the last
// component, so each one is checked. Components above base are trusted;
// for a path outside base only its last component is checked.
func hasLinkInside(base, path string) bool {
	base = filepath.Clean(base)
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
		if !strings.HasPrefix(p, base+string(filepath.Separator)) {
			return false
		}
	}
}

// canonicalAllowMissing is canonicalExisting that tolerates a missing
// tail: the longest existing ancestor is resolved and the remaining
// components are appended lexically, so a symlinked ancestor of a
// not-yet-created directory is still followed.
func canonicalAllowMissing(base, path string) (string, error) {
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, err := canonicalExisting(base, path)
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

// validatePluginURL rejects a URL with a fragment even when the fragment
// is empty, which url.Parse does not report.
func validatePluginURL(raw string) error {
	if raw == "" {
		return xerrors.New("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return xerrors.New("invalid url")
	}
	if !u.IsAbs() || u.Host == "" {
		return xerrors.New("url must be absolute")
	}
	if u.User != nil {
		return xerrors.New("url must not contain userinfo")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return xerrors.New("url must not contain a fragment")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return xerrors.New("url must use https for non-loopback hosts")
	default:
		return xerrors.Errorf("url has unsupported scheme %q", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validatePluginHeaders rejects invalid HTTP field names and values and
// names that repeat under different casing, since only one of them would
// be sent.
func validatePluginHeaders(headers map[string]string) error {
	seen := make(map[string]string, len(headers))
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		if !httpguts.ValidHeaderFieldName(name) {
			return xerrors.Errorf("invalid header name %q", name)
		}
		if !httpguts.ValidHeaderFieldValue(headers[name]) {
			return xerrors.Errorf("header %q has an invalid value", name)
		}
		canonical := http.CanonicalHeaderKey(name)
		if prev, ok := seen[canonical]; ok {
			return xerrors.Errorf("headers %q and %q differ only in case", prev, name)
		}
		seen[canonical] = name
	}
	return nil
}

// preparePluginDirs creates the plugin's DataDir and, when the server's
// cwd lies inside it, the cwd. The 0700 mode applies only to directories
// created here; an existing directory keeps its mode.
func preparePluginDirs(cfg ServerConfig) error {
	dataDir := cfg.Plugin.DataDir
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return xerrors.Errorf("create plugin data dir %q: %w", dataDir, err)
	}
	if cfg.Cwd == "" || requireInside(dataDir, cfg.Cwd) != nil {
		return nil
	}
	if err := os.MkdirAll(cfg.Cwd, 0o700); err != nil {
		return xerrors.Errorf("create plugin cwd %q: %w", cfg.Cwd, err)
	}
	return nil
}
