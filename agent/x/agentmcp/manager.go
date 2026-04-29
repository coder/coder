package agentmcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	tailscalesingleflight "tailscale.com/util/singleflight"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ToolNameSep separates the server name from the original tool name
// in prefixed tool names. Double underscore avoids collisions with
// tool names that may contain single underscores.
const ToolNameSep = "__"

// connectTimeout bounds how long we wait for a single MCP server
// to start its transport and complete initialization.
const connectTimeout = 30 * time.Second

// toolCallTimeout bounds how long a single tool invocation may
// take before being canceled.
const toolCallTimeout = 60 * time.Second

var (
	// ErrInvalidToolName is returned when the tool name format
	// is not "server__tool".
	ErrInvalidToolName = xerrors.New("invalid tool name format")
	// ErrUnknownServer is returned when no MCP server matches
	// the prefix in the tool name.
	ErrUnknownServer = xerrors.New("unknown MCP server")
)

// fileSnapshot records the identity of a config file at the time
// it was last read.
type fileSnapshot struct {
	exists  bool
	modTime time.Time
	size    int64
}

// Manager manages connections to MCP servers discovered from a
// workspace's .mcp.json file. It caches the aggregated tool list
// and proxies tool calls to the appropriate server.
type Manager struct {
	ctx       context.Context
	execer    agentexec.Execer
	updateEnv func(current []string) ([]string, error)

	mu        sync.RWMutex
	logger    slog.Logger
	closed    bool
	servers   map[string]*serverEntry
	tools     []workspacesdk.MCPToolInfo
	snapshot  map[string]fileSnapshot
	serverGen uint64
	sf        tailscalesingleflight.Group[string, struct{}]
}

// serverEntry pairs a server config with its connected client.
type serverEntry struct {
	config ServerConfig
	client *client.Client
}

// NewManager creates a new MCP client manager. The ctx bounds
// subprocess lifetime. The execer applies resource limits to
// MCP server subprocesses. The updateEnv callback enriches the
// subprocess environment to match interactive sessions.
func NewManager(
	ctx context.Context,
	logger slog.Logger,
	execer agentexec.Execer,
	updateEnv func([]string) ([]string, error),
) *Manager {
	return &Manager{
		ctx:       ctx,
		logger:    logger,
		execer:    execer,
		updateEnv: updateEnv,
		servers:   make(map[string]*serverEntry),
		snapshot:  make(map[string]fileSnapshot),
	}
}

// Reload checks whether config files have changed and, if so,
// performs a differential reconnect. Concurrent callers are
// coalesced via singleflight; the reload body runs under the
// Manager's lifetime context so it survives caller cancellation.
func (m *Manager) Reload(ctx context.Context, paths []string) error {
	m.mu.RLock()
	closed := m.closed
	hasSnapshot := len(m.snapshot) > 0
	m.mu.RUnlock()
	if closed {
		return xerrors.New("manager closed")
	}

	// Double-check: another goroutine may have completed a
	// reload between the caller's SnapshotChanged and this
	// call. The singleflight body uses its own resolved paths.
	if hasSnapshot && !m.SnapshotChanged(paths) {
		return nil
	}

	// All concurrent callers share one in-flight reload keyed
	// by "". If a concurrent caller resolves different paths
	// (e.g. after a manifest reconnect), its paths are not
	// consulted; the next SnapshotChanged check after this
	// reload completes will detect the mismatch and trigger
	// a fresh reload.
	ch := m.sf.DoChan("reload", func() (struct{}, error) {
		err := m.doReload(m.ctx, paths)
		return struct{}{}, err
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}

// SnapshotChanged checks whether any config file has changed
// since the last reload by comparing os.Stat results against
// the stored snapshot.
func (m *Manager) SnapshotChanged(paths []string) bool {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			unique = append(unique, p)
		}
	}
	paths = unique

	m.mu.RLock()
	snap := maps.Clone(m.snapshot)
	snapshotLen := len(snap)
	m.mu.RUnlock()

	if len(paths) != snapshotLen {
		return true
	}

	for _, p := range paths {
		prev, ok := snap[p]
		if !ok {
			return true
		}

		info, err := os.Stat(p)
		if err != nil {
			// Stat failed; changed only if the file existed before.
			if prev.exists {
				return true
			}
			continue
		}

		// Stat succeeded but file was absent before: it appeared.
		if !prev.exists {
			return true
		}

		if !info.ModTime().Equal(prev.modTime) || info.Size() != prev.size {
			return true
		}
	}

	return false
}

// serverDiff is the output of classifyServers: which servers to
// connect, which to close, which to keep, and a snapshot of the
// previous map for fallback on connect failure.
type serverDiff struct {
	toConnect []ServerConfig
	toClose   []*serverEntry
	keep      map[string]*serverEntry
	prev      map[string]*serverEntry
}

type connectedServer struct {
	name   string
	config ServerConfig
	client *client.Client
}

// doReload reads MCP config files and performs a differential
// reconnect. Unchanged servers keep their existing client; new or
// changed servers get a fresh connection; removed servers are
// closed.
func (m *Manager) doReload(ctx context.Context, mcpConfigFiles []string) error {
	allConfigs, snap := m.parseAndDedup(ctx, mcpConfigFiles)

	wanted := make(map[string]ServerConfig, len(allConfigs))
	for _, cfg := range allConfigs {
		wanted[cfg.Name] = cfg
	}

	diff, err := m.classifyServers(wanted)
	if err != nil {
		return err
	}

	connected := m.connectAll(ctx, diff.toConnect)

	replaced, err := m.installServers(wanted, diff, connected, snap)
	if err != nil {
		return err
	}

	// Close removed and replaced servers outside the lock to
	// avoid leaking child processes and to avoid blocking
	// concurrent readers on subprocess I/O.
	// Note: a concurrent CallTool that captured a removed
	// entry's client before the swap may call a closed client.
	// This is a narrow race that self-heals on the next request.
	for _, entry := range diff.toClose {
		_ = entry.client.Close()
	}
	for _, entry := range replaced {
		_ = entry.client.Close()
	}

	// Refresh tools outside the lock to avoid blocking
	// concurrent reads during network I/O.
	if err := m.RefreshTools(ctx); err != nil {
		m.logger.Warn(ctx, "failed to refresh MCP tools after connect", slog.Error(err))
	}
	return nil
}

// parseAndDedup reads all config files and returns a deduplicated
// list of server configs. Missing files are silently skipped;
// parse errors are logged and skipped.
func (m *Manager) parseAndDedup(ctx context.Context, mcpConfigFiles []string) ([]ServerConfig, map[string]fileSnapshot) {
	// Stat before reading so the snapshot is conservatively old.
	// If a file changes between stat and read, the snapshot
	// records the old mtime, SnapshotChanged detects a mismatch
	// on the next check, and triggers a re-read. False positives
	// (extra reload) are safe; false negatives (missed change)
	// are not.
	snap := captureSnapshot(mcpConfigFiles)

	var allConfigs []ServerConfig
	for _, configPath := range mcpConfigFiles {
		configs, err := ParseConfig(configPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			m.logger.Warn(ctx, "failed to parse MCP config",
				slog.F("path", configPath),
				slog.Error(err),
			)
			continue
		}
		allConfigs = append(allConfigs, configs...)
	}

	// Deduplicate by server name; first occurrence wins.
	seen := make(map[string]struct{})
	deduped := make([]ServerConfig, 0, len(allConfigs))
	for _, cfg := range allConfigs {
		if _, ok := seen[cfg.Name]; ok {
			continue
		}
		seen[cfg.Name] = struct{}{}
		deduped = append(deduped, cfg)
	}
	return deduped, snap
}

// classifyServers compares wanted configs against the current
// server map and returns a diff describing what changed.
// Acquires and releases m.mu for reading.
func (m *Manager) classifyServers(wanted map[string]ServerConfig) (*serverDiff, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, xerrors.New("manager closed")
	}

	diff := &serverDiff{
		keep: make(map[string]*serverEntry),
	}

	for name, wantCfg := range wanted {
		if existing, ok := m.servers[name]; ok {
			if reflect.DeepEqual(existing.config, wantCfg) {
				diff.keep[name] = existing
			} else {
				diff.toConnect = append(diff.toConnect, wantCfg)
			}
		} else {
			diff.toConnect = append(diff.toConnect, wantCfg)
		}
	}

	for name, entry := range m.servers {
		if _, ok := wanted[name]; !ok {
			diff.toClose = append(diff.toClose, entry)
		}
	}

	diff.prev = maps.Clone(m.servers)
	return diff, nil
}

// connectAll runs connectServer in parallel for the given configs.
// Failed connects are logged and skipped.
func (m *Manager) connectAll(ctx context.Context, toConnect []ServerConfig) []connectedServer {
	var (
		mu        sync.Mutex
		connected []connectedServer
	)
	var eg errgroup.Group
	for _, cfg := range toConnect {
		eg.Go(func() error {
			c, err := m.connectServer(ctx, cfg)
			if err != nil {
				m.logger.Warn(ctx, "skipping MCP server",
					slog.F("server", cfg.Name),
					slog.F("transport", cfg.Transport),
					slog.Error(err),
				)
				return nil // Don't fail the group.
			}
			mu.Lock()
			connected = append(connected, connectedServer{
				name: cfg.Name, config: cfg, client: c,
			})
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()
	return connected
}

// installServers builds the new server map from diff.keep and the
// connected list, falling back to diff.prev when a connect failed.
// Returns old entries replaced by successful connects (caller
// closes them). Acquires and releases m.mu.
func (m *Manager) installServers(
	wanted map[string]ServerConfig,
	diff *serverDiff,
	connected []connectedServer,
	snap map[string]fileSnapshot,
) ([]*serverEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		for _, cs := range connected {
			_ = cs.client.Close()
		}
		return nil, xerrors.New("manager closed")
	}

	newConnected := make(map[string]connectedServer, len(connected))
	for _, cs := range connected {
		newConnected[cs.name] = cs
	}

	newServers := make(map[string]*serverEntry, len(wanted))
	for name, entry := range diff.keep {
		newServers[name] = entry
	}

	var replaced []*serverEntry
	for name, wantCfg := range wanted {
		if _, kept := diff.keep[name]; kept {
			continue
		}
		if cs, ok := newConnected[wantCfg.Name]; ok {
			newServers[wantCfg.Name] = &serverEntry{
				config: cs.config,
				client: cs.client,
			}
			if prev, existed := diff.prev[wantCfg.Name]; existed {
				replaced = append(replaced, prev)
			}
		} else if prev, existed := diff.prev[wantCfg.Name]; existed {
			// Connect failed; retain the old client.
			newServers[wantCfg.Name] = prev
		}
	}

	m.servers = newServers
	m.serverGen++
	m.snapshot = snap
	return replaced, nil
}

// captureSnapshot stats each path and returns the current
// snapshot map.
func captureSnapshot(paths []string) map[string]fileSnapshot {
	snap := make(map[string]fileSnapshot, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			snap[p] = fileSnapshot{exists: false}
			continue
		}
		snap[p] = fileSnapshot{
			exists:  true,
			modTime: info.ModTime(),
			size:    info.Size(),
		}
	}
	return snap
}

// Tools returns the cached tool list. Thread-safe.
func (m *Manager) Tools() []workspacesdk.MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Clone(m.tools)
}

// CallTool proxies a tool call to the appropriate MCP server.
func (m *Manager) CallTool(ctx context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
	serverName, originalName, err := splitToolName(req.ToolName)
	if err != nil {
		return workspacesdk.CallMCPToolResponse{}, err
	}

	m.mu.RLock()
	entry, ok := m.servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return workspacesdk.CallMCPToolResponse{}, xerrors.Errorf("%w: %q", ErrUnknownServer, serverName)
	}

	callCtx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()

	result, err := entry.client.CallTool(callCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      originalName,
			Arguments: req.Arguments,
		},
	})
	if err != nil {
		return workspacesdk.CallMCPToolResponse{}, xerrors.Errorf("call tool %q on %q: %w", originalName, serverName, err)
	}

	return convertResult(result), nil
}

// RefreshTools re-fetches tool lists from all connected servers
// in parallel and rebuilds the cache. On partial failure, tools
// from servers that responded successfully are merged with the
// existing cached tools for servers that failed, so a single
// dead server doesn't block updates from healthy ones.
func (m *Manager) RefreshTools(ctx context.Context) error {
	// Snapshot servers under read lock.
	m.mu.RLock()
	servers := make(map[string]*serverEntry, len(m.servers))
	for k, v := range m.servers {
		servers[k] = v
	}
	gen := m.serverGen
	m.mu.RUnlock()

	// Fetch tool lists in parallel without holding any lock.
	type serverTools struct {
		name  string
		tools []workspacesdk.MCPToolInfo
	}
	var (
		mu      sync.Mutex
		results []serverTools
		failed  []string
		errs    []error
	)
	var eg errgroup.Group
	for name, entry := range servers {
		eg.Go(func() error {
			listCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			result, err := entry.client.ListTools(listCtx, mcp.ListToolsRequest{})
			cancel()
			if err != nil {
				m.logger.Warn(ctx, "failed to list tools from MCP server",
					slog.F("server", name),
					slog.Error(err),
				)
				mu.Lock()
				errs = append(errs, xerrors.Errorf("list tools from %q: %w", name, err))
				failed = append(failed, name)
				mu.Unlock()
				return nil
			}
			var tools []workspacesdk.MCPToolInfo
			for _, tool := range result.Tools {
				tools = append(tools, workspacesdk.MCPToolInfo{
					ServerName:  name,
					Name:        name + ToolNameSep + tool.Name,
					Description: tool.Description,
					Schema:      tool.InputSchema.Properties,
					Required:    tool.InputSchema.Required,
				})
			}
			mu.Lock()
			results = append(results, serverTools{name: name, tools: tools})
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()

	// Build the new tool list. For servers that failed, preserve
	// their tools from the existing cache so a single dead server
	// doesn't remove healthy tools.
	var merged []workspacesdk.MCPToolInfo
	for _, st := range results {
		merged = append(merged, st.tools...)
	}
	if len(failed) > 0 {
		failedSet := make(map[string]struct{}, len(failed))
		for _, f := range failed {
			failedSet[f] = struct{}{}
		}
		m.mu.RLock()
		for _, t := range m.tools {
			if _, ok := failedSet[t.ServerName]; ok {
				merged = append(merged, t)
			}
		}
		m.mu.RUnlock()
	}
	slices.SortFunc(merged, func(a, b workspacesdk.MCPToolInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	m.mu.Lock()
	// Skip the write if the server map changed since the
	// snapshot. A doReload that bumped the generation will
	// produce a correct tool list; this write would be stale.
	if m.serverGen == gen {
		m.tools = merged
	}
	m.mu.Unlock()

	return errors.Join(errs...)
}

// Close terminates all MCP server connections and child
// processes.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	var errs []error
	for _, entry := range m.servers {
		if err := entry.client.Close(); err != nil {
			// Subprocess kill signals are expected during shutdown.
			// The stdio transport returns cmd.Wait() which surfaces
			// "signal: killed" as an exec.ExitError.
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				errs = append(errs, err)
			}
		}
	}
	m.servers = make(map[string]*serverEntry)
	m.tools = nil
	return errors.Join(errs...)
}

// connectServer establishes a connection to a single MCP server
// and returns the connected client. It does not modify any Manager
// state.
func (m *Manager) connectServer(ctx context.Context, cfg ServerConfig) (*client.Client, error) {
	tr, err := m.createTransport(ctx, cfg)
	if err != nil {
		return nil, xerrors.Errorf("create transport for %q: %w", cfg.Name, err)
	}

	c := client.NewClient(tr)

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	// Use the parent ctx (not connectCtx) so the subprocess outlives
	// the connect/initialize handshake. connectCtx bounds only the
	// Initialize call below. The subprocess is cleaned up when the
	// Manager is closed or ctx is canceled.
	if err := c.Start(ctx); err != nil {
		_ = c.Close()
		return nil, xerrors.Errorf("start %q: %w", cfg.Name, err)
	}

	_, err = c.Initialize(connectCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "coder-agent",
				Version: buildinfo.Version(),
			},
		},
	})
	if err != nil {
		_ = c.Close()
		return nil, xerrors.Errorf("initialize %q: %w", cfg.Name, err)
	}

	return c, nil
}

// createTransport builds the mcp-go transport for a server config.
func (m *Manager) createTransport(ctx context.Context, cfg ServerConfig) (transport.Interface, error) {
	switch cfg.Transport {
	case "stdio":
		env := m.buildEnv(ctx, cfg.Env)
		return transport.NewStdioWithOptions(
			cfg.Command,
			env,
			cfg.Args,
			transport.WithCommandFunc(func(ctx context.Context, command string, cmdEnv []string, args []string) (*exec.Cmd, error) {
				cmd := m.execer.CommandContext(ctx, command, args...)
				cmd.Env = cmdEnv
				return cmd, nil
			}),
		), nil
	case "http", "":
		return transport.NewStreamableHTTP(
			cfg.URL,
			transport.WithHTTPHeaders(cfg.Headers),
		)
	case "sse":
		return transport.NewSSE(
			cfg.URL,
			transport.WithHeaders(cfg.Headers),
		)
	default:
		return nil, xerrors.Errorf("unsupported transport %q", cfg.Transport)
	}
}

// buildEnv enriches the process environment via the agent's
// updateEnv callback, then merges explicit overrides from the
// server config on top.
func (m *Manager) buildEnv(ctx context.Context, explicit map[string]string) []string {
	env := usershell.SystemEnvInfo{}.Environ()
	if m.updateEnv != nil {
		var err error
		env, err = m.updateEnv(env)
		if err != nil {
			m.logger.Warn(ctx, "failed to enrich MCP server environment",
				slog.Error(err),
			)
			env = usershell.SystemEnvInfo{}.Environ()
		}
	}
	if len(explicit) == 0 {
		return env
	}

	// Index existing env so explicit keys can override in-place.
	existing := make(map[string]int, len(env))
	for i, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			existing[k] = i
		}
	}

	for k, v := range explicit {
		entry := k + "=" + v
		if idx, ok := existing[k]; ok {
			env[idx] = entry
		} else {
			env = append(env, entry)
		}
	}
	return env
}

// splitToolName extracts the server name and original tool name
// from a prefixed tool name like "server__tool".
func splitToolName(prefixed string) (serverName, toolName string, err error) {
	server, tool, ok := strings.Cut(prefixed, ToolNameSep)
	if !ok || server == "" || tool == "" {
		return "", "", xerrors.Errorf("%w: expected format \"server%stool\", got %q", ErrInvalidToolName, ToolNameSep, prefixed)
	}
	return server, tool, nil
}

// convertResult translates an MCP CallToolResult into a
// workspacesdk.CallMCPToolResponse. It iterates over content
// items and maps each recognized type.
func convertResult(result *mcp.CallToolResult) workspacesdk.CallMCPToolResponse {
	if result == nil {
		return workspacesdk.CallMCPToolResponse{}
	}

	var content []workspacesdk.MCPToolContent
	for _, item := range result.Content {
		switch c := item.(type) {
		case mcp.TextContent:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "text",
				Text: c.Text,
			})
		case mcp.ImageContent:
			content = append(content, workspacesdk.MCPToolContent{
				Type:      "image",
				Data:      c.Data,
				MediaType: c.MIMEType,
			})
		case mcp.AudioContent:
			content = append(content, workspacesdk.MCPToolContent{
				Type:      "audio",
				Data:      c.Data,
				MediaType: c.MIMEType,
			})
		case mcp.EmbeddedResource:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "resource",
				Text: fmt.Sprintf("[embedded resource: %T]", c.Resource),
			})
		case mcp.ResourceLink:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "resource",
				Text: fmt.Sprintf("[resource link: %s]", c.URI),
			})
		default:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "text",
				Text: fmt.Sprintf("[unsupported content type: %T]", item),
			})
		}
	}

	return workspacesdk.CallMCPToolResponse{
		Content: content,
		IsError: result.IsError,
	}
}
