package agentcontext

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/buildinfo"
)

// mcpConnectTimeout bounds how long the runner waits for a single MCP
// server to start its transport, initialize, and report its tools.
const mcpConnectTimeout = 30 * time.Second

// mcpConnectConcurrency bounds how many MCP servers the runner connects
// to at once. .mcp.json files rarely declare many servers, but the cap
// keeps a pathological config from spawning an unbounded number of
// subprocesses simultaneously.
const mcpConnectConcurrency = 8

// mcpServerConfig is a single MCP server declaration parsed from a
// .mcp.json file. It is the runner's self-contained equivalent of the
// agent/x/agentmcp ServerConfig: agentcontext deliberately does not
// import that package so the two MCP paths stay completely separate.
type mcpServerConfig struct {
	Name      string
	Transport string
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Headers   map[string]string
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

// parseMCPConfig reads a .mcp.json file at path and returns the declared
// MCP servers sorted by name. It returns an empty slice when the
// mcpServers key is missing or empty. It is a self-contained copy of the
// agent/x/agentmcp parser so agentcontext can discover and start its own
// MCP servers without importing that package.
func parseMCPConfig(path string) ([]mcpServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf("read mcp config %q: %w", path, err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, xerrors.Errorf("parse mcp config %q: %w", path, err)
	}

	if len(cfg.MCPServers) == 0 {
		return []mcpServerConfig{}, nil
	}

	servers := make([]mcpServerConfig, 0, len(cfg.MCPServers))
	for name, raw := range cfg.MCPServers {
		var entry mcpServerEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, xerrors.Errorf("parse server %q in %q: %w", name, path, err)
		}

		tr := inferMCPTransport(entry)
		if tr == "" {
			return nil, xerrors.Errorf("server %q in %q has no command or url", name, path)
		}

		resolveMCPEnvVars(entry.Env)

		servers = append(servers, mcpServerConfig{
			Name:      name,
			Transport: tr,
			Command:   entry.Command,
			Args:      entry.Args,
			Env:       entry.Env,
			URL:       entry.URL,
			Headers:   entry.Headers,
		})
	}

	slices.SortFunc(servers, func(a, b mcpServerConfig) int {
		return strings.Compare(a.Name, b.Name)
	})

	return servers, nil
}

// inferMCPTransport determines the transport type for a server entry.
// An explicit "type" field takes priority; otherwise the presence of
// "command" implies stdio and "url" implies http.
func inferMCPTransport(e mcpServerEntry) string {
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

// resolveMCPEnvVars expands ${VAR} references in env map values using
// the current process environment.
func resolveMCPEnvVars(env map[string]string) {
	for k, v := range env {
		env[k] = os.Expand(v, os.Getenv)
	}
}

// mcpRunner connects to the MCP servers declared in the .mcp.json files
// the context resolver discovers, lists each server's tools, and caches
// a non-blocking per-server snapshot that buildMCPServerResources turns
// into KindMCPServer resources. It owns its own connection lifecycle and
// does not share state with agent/x/agentmcp: the two MCP paths run
// independently during the rollout. Connections are one-shot
// (connect, initialize, list tools, close) because the runner only needs
// each server's tool list to push to coderd, not a live tool-call proxy.
type mcpRunner struct {
	logger    slog.Logger
	execer    agentexec.Execer
	updateEnv func([]string) ([]string, error)
	onChange  func()

	// reloadMu serializes Reload so a slow reload cannot interleave
	// with a newer one and publish a stale cache. The sole production
	// caller (runMCPSync) already calls Reload sequentially; the mutex
	// is defensive.
	reloadMu sync.Mutex

	mu    sync.Mutex
	cache []MCPServerStatus
}

// newMCPRunner constructs a runner. onChange is invoked (outside the
// cache lock) after a Reload that changes the per-server snapshot, so
// the manager can re-resolve and push the updated KindMCPServer
// resources. updateEnv may be nil.
func newMCPRunner(logger slog.Logger, execer agentexec.Execer, updateEnv func([]string) ([]string, error), onChange func()) *mcpRunner {
	return &mcpRunner{
		logger:    logger,
		execer:    execer,
		updateEnv: updateEnv,
		onChange:  onChange,
	}
}

// Servers returns a deep copy of the current per-server MCP snapshot. It
// never blocks on I/O: the resolver calls it on every re-resolve.
func (r *mcpRunner) Servers() []MCPServerStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneMCPServers(r.cache)
}

// Reload reparses the supplied .mcp.json paths, connects to every
// declared server in parallel, lists its tools, and replaces the cached
// snapshot with the fresh result (including per-server failures). It
// fires onChange when the snapshot changed. Reload is best-effort: a
// server that fails to connect or list tools is recorded as a
// disconnected entry rather than aborting the whole reload.
func (r *mcpRunner) Reload(ctx context.Context, paths []string) {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	configs := r.parseConfigs(ctx, paths)
	statuses := r.connectAll(ctx, configs)

	r.mu.Lock()
	changed := !reflect.DeepEqual(r.cache, statuses)
	r.cache = statuses
	r.mu.Unlock()

	if changed && r.onChange != nil {
		r.onChange()
	}
}

// parseConfigs parses every path and returns the union of declared
// servers, deduplicated by name (first occurrence wins). Missing files
// are skipped silently; other parse errors are logged and skipped so one
// broken .mcp.json does not drop the servers declared in sibling files.
func (r *mcpRunner) parseConfigs(ctx context.Context, paths []string) []mcpServerConfig {
	var all []mcpServerConfig
	for _, path := range paths {
		configs, err := parseMCPConfig(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			r.logger.Warn(ctx, "failed to parse MCP config",
				slog.F("path", path),
				slog.Error(err),
			)
			continue
		}
		all = append(all, configs...)
	}

	seen := make(map[string]struct{}, len(all))
	deduped := make([]mcpServerConfig, 0, len(all))
	for _, cfg := range all {
		if _, ok := seen[cfg.Name]; ok {
			continue
		}
		seen[cfg.Name] = struct{}{}
		deduped = append(deduped, cfg)
	}
	return deduped
}

// connectAll connects to each server in parallel (bounded) and returns
// one status per server in name order. Per-server failures are isolated:
// a failed connect or list becomes a disconnected status carrying the
// error instead of failing the batch.
func (r *mcpRunner) connectAll(ctx context.Context, configs []mcpServerConfig) []MCPServerStatus {
	if len(configs) == 0 {
		return nil
	}
	statuses := make([]MCPServerStatus, len(configs))
	var eg errgroup.Group
	eg.SetLimit(mcpConnectConcurrency)
	for i, cfg := range configs {
		eg.Go(func() error {
			st := MCPServerStatus{Name: cfg.Name}
			tools, err := r.connectAndList(ctx, cfg)
			if err != nil {
				r.logger.Warn(ctx, "failed to connect MCP server",
					slog.F("server", cfg.Name),
					slog.Error(err),
				)
				st.Err = err.Error()
			} else {
				st.Connected = true
				st.Tools = tools
			}
			statuses[i] = st
			return nil
		})
	}
	_ = eg.Wait()
	return statuses
}

// connectAndList starts a single MCP server, completes the initialize
// handshake, lists its tools, and closes the connection. Tool names are
// returned exactly as the server reported them; the resource carries the
// server name separately, so any flattening into a single namespace is
// left to the control plane.
func (r *mcpRunner) connectAndList(ctx context.Context, cfg mcpServerConfig) ([]MCPTool, error) {
	tr, err := r.createTransport(ctx, cfg)
	if err != nil {
		return nil, xerrors.Errorf("create transport for %q: %w", cfg.Name, err)
	}

	c := client.NewClient(tr)

	// Tie the subprocess to cmdCtx. mcp-go's stdio Close() closes stdin
	// and then blocks on cmd.Wait() with no kill: a server that ignores
	// stdin-close would stall this reload indefinitely (the deferred
	// Close runs before connectAndList returns, so it would block the
	// errgroup and hold the reload lock). Canceling cmdCtx force-kills
	// the process via exec.CommandContext, so Close's Wait returns. The
	// deferred cleanup cancels before closing, and runs before the
	// connectCtx cancel because defers are LIFO.
	cmdCtx, cmdCancel := context.WithCancel(ctx)
	connectCtx, cancel := context.WithTimeout(cmdCtx, mcpConnectTimeout)
	defer cancel()

	if err := c.Start(cmdCtx); err != nil {
		cmdCancel()
		_ = c.Close()
		return nil, xerrors.Errorf("start %q: %w", cfg.Name, err)
	}
	defer func() {
		cmdCancel()
		_ = c.Close()
	}()

	if _, err := c.Initialize(connectCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "coder-agent",
				Version: buildinfo.Version(),
			},
		},
	}); err != nil {
		return nil, xerrors.Errorf("initialize %q: %w", cfg.Name, err)
	}

	result, err := c.ListTools(connectCtx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, xerrors.Errorf("list tools from %q: %w", cfg.Name, err)
	}

	tools := make([]MCPTool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, MCPTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: toolInputSchema(tool.InputSchema),
		})
	}
	return tools, nil
}

// createTransport builds the mcp-go transport for a server config.
func (r *mcpRunner) createTransport(ctx context.Context, cfg mcpServerConfig) (transport.Interface, error) {
	switch cfg.Transport {
	case "stdio":
		env := r.buildEnv(ctx, cfg.Env)
		return transport.NewStdioWithOptions(
			cfg.Command,
			env,
			cfg.Args,
			transport.WithCommandFunc(func(ctx context.Context, command string, cmdEnv []string, args []string) (*exec.Cmd, error) {
				cmd := r.execer.CommandContext(ctx, command, args...)
				cmd.Env = cmdEnv
				return cmd, nil
			}),
		), nil
	case "http", "":
		return transport.NewStreamableHTTP(cfg.URL, transport.WithHTTPHeaders(cfg.Headers))
	case "sse":
		return transport.NewSSE(cfg.URL, transport.WithHeaders(cfg.Headers))
	default:
		return nil, xerrors.Errorf("unsupported transport %q", cfg.Transport)
	}
}

// buildEnv enriches the process environment via the agent's updateEnv
// callback, then merges explicit overrides from the server config on
// top. Note: env enrichment is captured per Reload; an env change alone
// (without a .mcp.json change) does not trigger a re-list.
func (r *mcpRunner) buildEnv(ctx context.Context, explicit map[string]string) []string {
	env := usershell.SystemEnvInfo{}.Environ()
	if r.updateEnv != nil {
		updated, err := r.updateEnv(env)
		if err != nil {
			r.logger.Warn(ctx, "failed to enrich MCP server environment", slog.Error(err))
			env = usershell.SystemEnvInfo{}.Environ()
		} else {
			env = updated
		}
	}
	if len(explicit) == 0 {
		return env
	}

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

// toolInputSchema converts an mcp-go tool input schema into the
// JSON-Schema-shaped map MCPTool carries. Required is converted to
// []any (not []string) so the downstream structpb encoding accepts it.
// An empty schema yields nil so the tool ships with InputSchema unset.
func toolInputSchema(s mcp.ToolInputSchema) map[string]any {
	out := map[string]any{}
	if s.Type != "" {
		out["type"] = s.Type
	}
	if len(s.Properties) > 0 {
		out["properties"] = s.Properties
	}
	if len(s.Required) > 0 {
		required := make([]any, len(s.Required))
		for i, req := range s.Required {
			required[i] = req
		}
		out["required"] = required
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneMCPServers deep-copies a per-server snapshot so callers cannot
// mutate the runner's cache. Tool input schemas are treated as immutable
// and shared by reference.
func cloneMCPServers(in []MCPServerStatus) []MCPServerStatus {
	if len(in) == 0 {
		return nil
	}
	out := make([]MCPServerStatus, len(in))
	for i, s := range in {
		s.Tools = slices.Clone(s.Tools)
		out[i] = s
	}
	return out
}

// runMCPSync keeps the runner's connected servers in sync with the
// .mcp.json files the resolver discovers. It subscribes to snapshot
// changes, extracts the set of KindMCPConfig paths (keyed by content
// hash so in-place edits are detected), and reloads the runner only when
// that set changes. A reload fires the runner's onChange, which
// re-resolves and surfaces the updated KindMCPServer resources; that
// re-resolve does not change the config set, so it does not loop.
func (m *Manager) runMCPSync(ctx context.Context) {
	changes, unsubscribe := m.SubscribeChanges()
	defer unsubscribe()

	var lastKey string
	reload := func() {
		paths, key := mcpConfigSet(m.Snapshot())
		if key == lastKey {
			return
		}
		lastKey = key
		m.mcpRunner.Reload(ctx, paths)
	}

	// Pick up any .mcp.json discovered before we subscribed.
	reload()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closedCh:
			return
		case <-changes:
			reload()
		}
	}
}

// mcpConfigSet extracts the .mcp.json config files from a snapshot's
// KindMCPConfig resources. It returns the sorted unique source paths
// plus a key encoding path:contenthash pairs, so callers detect both
// path-set changes and in-place content edits. An empty set yields an
// empty key.
func mcpConfigSet(snap Snapshot) (paths []string, key string) {
	hashes := make(map[string]string, len(snap.Resources))
	for _, r := range snap.Resources {
		if r.Kind != KindMCPConfig || r.Source == "" {
			continue
		}
		hashes[r.Source] = hex.EncodeToString(r.ContentHash[:])
	}
	if len(hashes) == 0 {
		return nil, ""
	}
	paths = make([]string, 0, len(hashes))
	for p := range hashes {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	parts := make([]string, len(paths))
	for i, p := range paths {
		parts[i] = p + ":" + hashes[p]
	}
	return paths, strings.Join(parts, "\n")
}
