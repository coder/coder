package agentmcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	mu      sync.RWMutex
	logger  slog.Logger
	closed  bool
	servers map[string]*serverEntry // keyed by server name
	tools   []workspacesdk.MCPToolInfo

	// snapshot records (mtime, size) for each config file path
	// at the end of the last successful reload. Protected by mu.
	snapshot map[string]fileSnapshot

	// ctx is the Manager's lifetime context, derived from the
	// agent's gracefulCtx. It outlives any individual request
	// and is used as the context for Connect so subprocesses
	// survive caller cancellation.
	ctx context.Context

	// reloadGroup coalesces concurrent Reload calls so at most
	// one connect pass runs at a time.
	reloadGroup tailscalesingleflight.Group[string, error]
}

// serverEntry pairs a server config with its connected client.
type serverEntry struct {
	config ServerConfig
	client *client.Client
}

// NewManager creates a new MCP client manager. The ctx parameter
// is the Manager's lifetime context (typically the agent's
// gracefulCtx); it outlives individual requests and is used as
// the context for subprocess startup.
func NewManager(ctx context.Context, logger slog.Logger) *Manager {
	return &Manager{
		logger:   logger,
		servers:  make(map[string]*serverEntry),
		snapshot: make(map[string]fileSnapshot),
		ctx:      ctx,
	}
}

// connect reads MCP config files and performs a differential
// reconnect. Unchanged servers keep their existing client;
// new or changed servers get a fresh connection; removed servers
// are closed. This is the core reconnect logic called by Reload.
func (m *Manager) connect(ctx context.Context, mcpConfigFiles []string) error {
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
	allConfigs = deduped

	// Build a map for quick lookup by name.
	wantedConfigs := make(map[string]ServerConfig, len(allConfigs))
	for _, cfg := range allConfigs {
		wantedConfigs[cfg.Name] = cfg
	}

	// Classify entries under lock: keep, connect, close.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return xerrors.New("manager closed")
	}

	var (
		toConnect []ServerConfig
		toClose   []*serverEntry
		keepSet   = make(map[string]*serverEntry)
	)

	// Check each wanted config against existing servers.
	for name, wantCfg := range wantedConfigs {
		if existing, ok := m.servers[name]; ok {
			if reflect.DeepEqual(existing.config, wantCfg) {
				// Unchanged: keep the existing client.
				keepSet[name] = existing
			} else {
				// Changed: need to reconnect.
				toConnect = append(toConnect, wantCfg)
			}
		} else {
			// New server.
			toConnect = append(toConnect, wantCfg)
		}
	}

	// Servers in m.servers but not in wantedConfigs are removed.
	for name, entry := range m.servers {
		if _, wanted := wantedConfigs[name]; !wanted {
			toClose = append(toClose, entry)
		}
	}

	// Snapshot the previous servers for the changed-server
	// rollback path.
	prevServers := make(map[string]*serverEntry, len(m.servers))
	for k, v := range m.servers {
		prevServers[k] = v
	}
	m.mu.Unlock()

	// Connect new/changed servers in parallel without holding
	// the lock, since each connectServer call may block on
	// network I/O for up to connectTimeout.
	type connectedServer struct {
		name   string
		config ServerConfig
		client *client.Client
	}
	var (
		cmu       sync.Mutex
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
			cmu.Lock()
			connected = append(connected, connectedServer{
				name: cfg.Name, config: cfg, client: c,
			})
			cmu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()

	// Re-acquire lock to install the new server map.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		// Close freshly-connected clients since we're shutting
		// down.
		for _, cs := range connected {
			_ = cs.client.Close()
		}
		return xerrors.New("manager closed")
	}

	newConnected := make(map[string]connectedServer, len(connected))
	for _, cs := range connected {
		newConnected[cs.name] = cs
	}

	// Build the new server map.
	newServers := make(map[string]*serverEntry, len(wantedConfigs))

	// Keep entries: reuse existing client pointer.
	for name, entry := range keepSet {
		newServers[name] = entry
	}

	// Connect entries: use freshly-connected client, or fall
	// back to the previous client on connect failure.
	var closePrev []*serverEntry
	for _, cfg := range toConnect {
		if cs, ok := newConnected[cfg.Name]; ok {
			// New connection succeeded.
			newServers[cfg.Name] = &serverEntry{
				config: cs.config,
				client: cs.client,
			}
			// If this was a changed server (existed before),
			// close the old client after releasing the lock.
			if prev, existed := prevServers[cfg.Name]; existed {
				closePrev = append(closePrev, prev)
			}
		} else {
			// Connect failed. If the server existed before
			// (changed config), retain the old client.
			if prev, existed := prevServers[cfg.Name]; existed {
				newServers[cfg.Name] = prev
			}
			// If the server is new and connect failed, it is
			// simply not in the map.
		}
	}

	m.servers = newServers

	// Update the snapshot for the config files.
	m.snapshot = captureSnapshot(mcpConfigFiles)

	m.mu.Unlock()

	// Close removed and replaced servers outside the lock to
	// avoid blocking concurrent readers on subprocess I/O.
	for _, entry := range toClose {
		_ = entry.client.Close()
	}
	for _, entry := range closePrev {
		_ = entry.client.Close()
	}

	// Refresh tools outside the lock to avoid blocking
	// concurrent reads during network I/O.
	if err := m.RefreshTools(ctx); err != nil {
		m.logger.Warn(ctx, "failed to refresh MCP tools after connect", slog.Error(err))
	}
	return nil
}

// captureSnapshot stats each path and returns the current
// snapshot map.
func captureSnapshot(paths []string) map[string]fileSnapshot {
	snap := make(map[string]fileSnapshot, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			snap[p] = fileSnapshot{exists: false}
		} else {
			snap[p] = fileSnapshot{
				exists:  true,
				modTime: info.ModTime(),
				size:    info.Size(),
			}
		}
	}
	return snap
}

// SnapshotChanged checks whether any config file has changed
// since the last reload by comparing os.Stat results against
// the stored snapshot.
func (m *Manager) SnapshotChanged(paths []string) bool {
	m.mu.RLock()
	// Copy the snapshot under the lock to avoid holding it
	// during os.Stat syscalls, which can block on slow
	// filesystems (e.g., NFS).
	snapshotLen := len(m.snapshot)
	snap := make(map[string]fileSnapshot, snapshotLen)
	for k, v := range m.snapshot {
		snap[k] = v
	}
	m.mu.RUnlock()

	// Different number of paths is a change.
	if len(paths) != snapshotLen {
		return true
	}

	for _, p := range paths {
		prev, ok := snap[p]
		if !ok {
			// Path not in the snapshot: new path added.
			return true
		}

		info, err := os.Stat(p)
		if err != nil {
			// File is currently absent.
			if prev.exists {
				// Was present before: changed.
				return true
			}
			// Was absent before and still absent: unchanged.
			continue
		}

		// File is currently present.
		if !prev.exists {
			// Was absent before: changed.
			return true
		}

		// Both present: compare mtime and size.
		if !info.ModTime().Equal(prev.modTime) || info.Size() != prev.size {
			return true
		}
	}

	return false
}

// Reload checks whether config files have changed and, if so,
// performs a differential reconnect. Concurrent callers are
// coalesced via singleflight; the reload body runs under the
// Manager's lifetime context so it survives caller cancellation.
func (m *Manager) Reload(callerCtx context.Context, paths []string) error {
	m.mu.RLock()
	closed := m.closed
	hasSnapshot := len(m.snapshot) > 0
	m.mu.RUnlock()
	if closed {
		return xerrors.New("manager closed")
	}

	// Fast path: if nothing changed, return immediately.
	if hasSnapshot && !m.SnapshotChanged(paths) {
		return nil
	}

	// Use singleflight to coalesce concurrent reloads.
	// The sentinel key "" means all callers share the same
	// in-flight reload.
	ch := m.reloadGroup.DoChan("", func() (error, error) {
		err := m.connect(m.ctx, paths)
		return err, nil
	})

	select {
	case <-callerCtx.Done():
		return callerCtx.Err()
	case res := <-ch:
		return res.Val
	}
}

// connectServer establishes a connection to a single MCP server
// and returns the connected client. It does not modify any Manager
// state.
func (*Manager) connectServer(ctx context.Context, cfg ServerConfig) (*client.Client, error) {
	tr, err := createTransport(cfg)
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
func createTransport(cfg ServerConfig) (transport.Interface, error) {
	switch cfg.Transport {
	case "stdio":
		return transport.NewStdio(
			cfg.Command,
			buildEnv(cfg.Env),
			cfg.Args...,
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

// buildEnv merges the current process environment with explicit
// overrides, returning the result as KEY=VALUE strings suitable
// for the stdio transport.
func buildEnv(explicit map[string]string) []string {
	env := os.Environ()
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
	m.tools = merged
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
		errs = append(errs, entry.client.Close())
	}
	m.servers = make(map[string]*serverEntry)
	m.tools = nil
	return errors.Join(errs...)
}
