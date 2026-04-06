package agentmcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

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

// Manager manages connections to MCP servers discovered from a
// workspace's .mcp.json file. It caches the aggregated tool list
// and proxies tool calls to the appropriate server.
type Manager struct {
	mu      sync.RWMutex
	logger  slog.Logger
	closed  bool
	servers map[string]*serverEntry // keyed by server name
	tools   []workspacesdk.MCPToolInfo
}

// serverEntry pairs a server config with its connected client.
type serverEntry struct {
	config ServerConfig
	client *client.Client
}

// NewManager creates a new MCP client manager.
func NewManager(logger slog.Logger) *Manager {
	return &Manager{
		logger:  logger,
		servers: make(map[string]*serverEntry),
	}
}

// Connect reads MCP config files at the given absolute paths and
// connects to all configured servers. Failed servers are logged
// and skipped. Missing config files are silently skipped.
func (m *Manager) Connect(ctx context.Context, mcpConfigFiles []string) error {
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

	if len(allConfigs) == 0 {
		return nil
	}

	// Connect to servers in parallel without holding the
	// lock, since each connectServer call may block on
	// network I/O for up to connectTimeout.
	type connectedServer struct {
		name   string
		config ServerConfig
		client *client.Client
	}
	var (
		mu        sync.Mutex
		connected []connectedServer
	)
	var eg errgroup.Group
	for _, cfg := range allConfigs {
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

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		// Close the freshly-connected clients since we're
		// shutting down.
		for _, cs := range connected {
			_ = cs.client.Close()
		}
		return xerrors.New("manager closed")
	}

	// Close previous connections to avoid leaking child
	// processes on agent reconnect.
	for _, entry := range m.servers {
		_ = entry.client.Close()
	}
	m.servers = make(map[string]*serverEntry, len(connected))

	for _, cs := range connected {
		m.servers[cs.name] = &serverEntry{
			config: cs.config,
			client: cs.client,
		}
	}
	m.mu.Unlock()

	// Refresh tools outside the lock to avoid blocking
	// concurrent reads during network I/O.
	if err := m.RefreshTools(ctx); err != nil {
		m.logger.Warn(ctx, "failed to refresh MCP tools after connect", slog.Error(err))
	}
	return nil
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
