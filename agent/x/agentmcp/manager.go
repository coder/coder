package agentmcp

import (
	"context"
	"encoding/base64"
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	tailscalesingleflight "tailscale.com/util/singleflight"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
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
	// ErrManagerClosed is returned by Reload and Tools after
	// Close. Close cancels the Manager's derived context, so this
	// sentinel keeps explicit Close distinguishable from parent
	// context cancellation.
	ErrManagerClosed = xerrors.New("manager closed")
	// errConnectTimeout is the cause recorded when a server does not
	// finish its handshake within connectTimeout.
	errConnectTimeout = xerrors.Errorf("no response within %s", connectTimeout)
)

// fileSnapshot records the identity of a config file at the time
// it was last read.
type fileSnapshot struct {
	exists  bool
	modTime time.Time
	size    int64
}

type reloadResult = tailscalesingleflight.Result[struct{}]

// Manager manages connections to MCP servers discovered from a
// workspace's .mcp.json file. It caches the aggregated tool list
// and proxies tool calls to the appropriate server.
type Manager struct {
	ctx       context.Context
	cancel    context.CancelFunc
	execer    agentexec.Execer
	fs        afero.Fs
	envInfo   usershell.EnvInfoer
	updateEnv func(current []string) ([]string, error)

	// workingDir reports the workspace working directory for stdio
	// servers, read at connect time. See resolveWorkingDir.
	workingDir func() string
	// inheritedSecrets reports values updateEnv injects into every
	// stdio server's environment (agent token, user secrets); they
	// are redacted from published diagnostics. See SetInheritedSecrets.
	inheritedSecrets func() []string

	mu      sync.RWMutex
	logger  slog.Logger
	clock   quartz.Clock
	closed  bool
	servers map[string]*serverEntry
	// catalog and configErrors together form the published Report.
	catalog      []ServerStatus
	configErrors []ConfigError
	snapshot     map[string]fileSnapshot
	serverGen    uint64
	sf           tailscalesingleflight.Group[string, struct{}]

	// onChange, when non-nil, is invoked (outside the cache lock)
	// after a reload changes the discovery report (servers, tools,
	// warnings, or config errors), so the agentcontext manager can
	// re-resolve and re-push the MCP resources.
	onChange func()

	// firstSyncSettled records that a reload body reached a
	// terminal result, successful or not. It gates whether the
	// SnapshotChanged short-circuit may skip a reload.
	firstSyncSettled bool

	// closedCh is closed by Close to unblock waiters that do not
	// otherwise observe Close (the parent ctx is owned by the
	// caller and may outlive Close).
	closedCh  chan struct{}
	closeOnce sync.Once

	// lastPaths records the most recent config paths passed to
	// Reload/Tools. The fsnotify-backed watcher uses these to
	// drive its own reloads when ~/.mcp.json appears late on
	// dual-agent workspaces.
	lastPaths []string

	// watcher fires a debounced Reload when any watched config
	// file is created, written, removed, or renamed. It is armed
	// lazily on the first Reload call so tests that never call
	// Reload do not pay for an extra goroutine and file
	// descriptor.
	watcher       *configWatcher
	watcherOnce   sync.Once
	watchDebounce time.Duration

	// connectStartedHook is a test hook invoked at the start of
	// connectAll, before any client is dialed. Production code
	// leaves this nil; tests set it to coordinate with an
	// in-flight reload (for example, to verify Close()'s
	// shutdown ordering does not stall on a stuck connect).
	connectStartedHook func()
}

type serverEntry struct {
	config ServerConfig
	client *mcp.ClientSession
}

// NewManager creates a new MCP client manager. The ctx bounds
// subprocess lifetime. The execer applies resource limits to
// MCP server subprocesses. The fs and envInfo report the filesystem and
// the user's environment, home, and shell; nil values default to the OS
// filesystem and the current process. The updateEnv callback enriches
// the subprocess environment to match interactive sessions. The
// workingDir callback reports the workspace working directory for stdio
// servers.
func NewManager(
	ctx context.Context,
	logger slog.Logger,
	execer agentexec.Execer,
	filesystem afero.Fs,
	envInfo usershell.EnvInfoer,
	updateEnv func([]string) ([]string, error),
	workingDir func() string,
) *Manager {
	if filesystem == nil {
		filesystem = afero.NewOsFs()
	}
	if envInfo == nil {
		envInfo = &usershell.SystemEnvInfo{}
	}
	managerCtx, cancel := context.WithCancel(ctx)
	return &Manager{
		ctx:           managerCtx,
		cancel:        cancel,
		logger:        logger,
		clock:         quartz.NewReal(),
		execer:        execer,
		fs:            filesystem,
		envInfo:       envInfo,
		updateEnv:     updateEnv,
		workingDir:    workingDir,
		servers:       make(map[string]*serverEntry),
		snapshot:      make(map[string]fileSnapshot),
		closedCh:      make(chan struct{}),
		watchDebounce: defaultWatchDebounce,
	}
}

// Reload ensures the tool cache reflects the current config.
//
// If config files differ from the last snapshot, a singleflight
// differential reconnect is driven and Reload waits for it. If the
// snapshot is current, Reload returns immediately.
//
// Starting and running the reload is manager-scoped. Caller contexts
// may bound only that caller's wait for the reload result. They are
// never passed to, and must not suppress, the reload body.
func (m *Manager) Reload(ctx context.Context, paths []string) error {
	ch, started, err := m.startReloadIfNeeded(paths)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	return m.waitReload(ctx, ch, 0)
}

// SetOnReload registers a callback fired (outside the cache lock) after
// a reload changes the discovery report. The agent wires this to the
// agentcontext manager's Trigger so discovery re-resolves and re-pushes
// the MCP resources. It must be called before the first Reload.
func (m *Manager) SetOnReload(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// SetInheritedSecrets registers the source of secret values the agent
// injects into every stdio server's environment through updateEnv. A
// server can echo its environment back in a JSON-RPC error, so these
// values are redacted from published diagnostics like configured ones.
// It must be called before the first Reload.
func (m *Manager) SetInheritedSecrets(fn func() []string) {
	m.mu.Lock()
	m.inheritedSecrets = fn
	m.mu.Unlock()
}

// sanitizeError is sanitizeMCPError with the manager's inherited
// secrets.
func (m *Manager) sanitizeError(cfg ServerConfig, err error) string {
	m.mu.RLock()
	fn := m.inheritedSecrets
	m.mu.RUnlock()
	var inherited []string
	if fn != nil {
		inherited = fn()
	}
	return sanitizeMCPError(cfg, inherited, err)
}

// startReloadIfNeeded registers the reload with the singleflight group
// using a fixed key so concurrent triggers share one body. The body
// always runs under m.ctx. The returned channel yields the body's result
// exactly once.
//
// All concurrent callers share one in-flight reload keyed by "reload".
// If a concurrent caller resolves different paths, its paths are not
// consulted. The next SnapshotChanged check after this reload completes
// will detect the mismatch and trigger a fresh reload.
func (m *Manager) startReloadIfNeeded(paths []string) (<-chan reloadResult, bool, error) {
	m.mu.RLock()
	closed := m.closed
	firstSyncSettled := m.firstSyncSettled
	m.mu.RUnlock()
	if closed {
		return nil, false, ErrManagerClosed
	}
	if err := m.ctx.Err(); err != nil {
		if closeErr := m.closeErr(); closeErr != nil {
			return nil, false, closeErr
		}
		return nil, false, err
	}
	// Arm the fsnotify watcher before deciding whether to short
	// circuit. The first call lazily creates it; subsequent calls
	// re-sync the watched path set if it changed. Arming before
	// the SnapshotChanged check ensures any Create event that
	// races with parseAndDedup is still delivered: the watcher
	// is running when parseAndDedup returns the empty snapshot.
	m.armWatcher(paths)

	if firstSyncSettled && !m.SnapshotChanged(paths) {
		return nil, false, nil
	}

	ch := m.sf.DoChan("reload", func() (struct{}, error) {
		defer m.markFirstSyncSettled()
		err := m.doReload(m.ctx, paths)
		return struct{}{}, err
	})
	return ch, true, nil
}

// armWatcher lazily initializes the fsnotify-backed configWatcher
// and syncs it to the latest config paths. Lazy initialization
// keeps unit tests that never call Reload free of extra goroutines
// and file descriptors.
//
// If the underlying watcher cannot be created (e.g. inotify limit
// reached), the error is logged once and the manager continues
// without a watcher. The lazy stat-on-request path remains the
// primary mechanism; the watcher is an optimization that closes
// the dual-agent race window.
func (m *Manager) armWatcher(paths []string) {
	m.watcherOnce.Do(func() {
		cw, err := newConfigWatcher(
			m.logger.Named("config_watcher"),
			m.clock,
			m.watchDebounce,
			m.handleWatchedConfigChange,
		)
		if err != nil {
			m.logger.Warn(m.ctx,
				"failed to start MCP config watcher; falling back to lazy stat",
				slog.Error(err))
			return
		}
		// Close the watcher if the manager was closed between
		// newConfigWatcher returning and us acquiring m.mu.
		// Otherwise its goroutine and inotify fd leak.
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			_ = cw.Close()
			return
		}
		m.watcher = cw
		m.mu.Unlock()
	})

	m.mu.Lock()
	m.lastPaths = slices.Clone(paths)
	w := m.watcher
	closed := m.closed
	m.mu.Unlock()
	if w == nil || closed {
		return
	}
	w.Sync(paths)
}

// handleWatchedConfigChange is invoked by the watcher on a
// debounced fire. It triggers a singleflight Reload using the
// most recently observed path set so the cached server map and
// snapshot are refreshed without waiting for the next HTTP
// request.
func (m *Manager) handleWatchedConfigChange() {
	m.mu.RLock()
	paths := slices.Clone(m.lastPaths)
	closed := m.closed
	m.mu.RUnlock()
	if closed || len(paths) == 0 {
		return
	}

	logger := m.logger.With(slog.F("trigger", "fsnotify"))
	logger.Debug(m.ctx, "reloading due to config change")
	if err := m.Reload(m.ctx, paths); err != nil {
		if errors.Is(err, ErrManagerClosed) ||
			errors.Is(err, context.Canceled) {
			logger.Debug(m.ctx,
				"watched reload short-circuited by shutdown",
				slog.Error(err))
			return
		}
		logger.Warn(m.ctx, "watched reload failed", slog.Error(err))
	}
}

func (m *Manager) waitReload(ctx context.Context, ch <-chan reloadResult, timeout time.Duration) error {
	// Prefer caller cancellation when it already happened before the
	// wait. Otherwise select may choose a ready reload result instead.
	if err := ctx.Err(); err != nil {
		return err
	}

	var timeoutC <-chan time.Time
	if timeout > 0 {
		timer := m.clock.NewTimer(timeout, "agentmcp", "tools_reload")
		defer timer.Stop()
		timeoutC = timer.C
	}

	select {
	case res := <-ch:
		return res.Err
	case <-ctx.Done():
		return ctx.Err()
	case <-timeoutC:
		return xerrors.Errorf("tools reload timed out after %s: %w", timeout, context.DeadlineExceeded)
	case <-m.ctx.Done():
		if err := m.closeErr(); err != nil {
			return err
		}
		return m.ctx.Err()
	case <-m.closedCh:
		return ErrManagerClosed
	}
}

func (m *Manager) closeErr() error {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return ErrManagerClosed
	}
	return nil
}

func (m *Manager) markFirstSyncSettled() {
	m.mu.Lock()
	m.firstSyncSettled = true
	m.mu.Unlock()
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
	client *mcp.ClientSession
}

// doReload reads MCP config files and performs a differential
// reconnect. Unchanged servers keep their existing client; new or
// changed servers get a fresh connection; removed servers are
// closed.
func (m *Manager) doReload(ctx context.Context, mcpConfigFiles []string) error {
	allConfigs, configErrors, snap := m.parseAndDedup(ctx, mcpConfigFiles)

	wanted := make(map[string]ServerConfig, len(allConfigs))
	for _, cfg := range allConfigs {
		wanted[cfg.Name] = cfg
	}

	diff, err := m.classifyServers(wanted)
	if err != nil {
		return err
	}

	connected, connectErrors := m.connectAll(ctx, diff.toConnect, func(cs connectedServer) {
		m.publishConnected(ctx, wanted, configErrors, cs)
	})

	replaced, warnings, err := m.installServers(wanted, diff, connected, connectErrors, snap)
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

	// Rebuild the per-server catalog outside the lock to avoid
	// blocking concurrent reads during network I/O, then notify the
	// agentcontext manager when the report changed so it re-resolves
	// and re-pushes the MCP resources.
	if m.refreshCatalog(ctx, wanted, configErrors, connectErrors, warnings) {
		m.fireOnChange()
	}
	return nil
}

// parseAndDedup reads all config files and returns a deduplicated
// list of server configs plus one ConfigError per file that exists
// but could not be used (unreadable, malformed JSON, or a server
// entry the config schema rejects). Missing files are skipped.
func (m *Manager) parseAndDedup(ctx context.Context, mcpConfigFiles []string) ([]ServerConfig, []ConfigError, map[string]fileSnapshot) {
	logger := m.logger.With(agentchat.Fields(ctx)...)

	// Stat before reading so the snapshot is conservatively old.
	// If a file changes between stat and read, the snapshot
	// records the old mtime, SnapshotChanged detects a mismatch
	// on the next check, and triggers a re-read. False positives
	// (extra reload) are safe; false negatives (missed change)
	// are not.
	snap := captureSnapshot(mcpConfigFiles)

	var (
		allConfigs   []ServerConfig
		configErrors []ConfigError
		seenPaths    = make(map[string]struct{}, len(mcpConfigFiles))
	)
	for _, configPath := range mcpConfigFiles {
		if _, ok := seenPaths[configPath]; ok {
			continue
		}
		seenPaths[configPath] = struct{}{}
		configs, err := ParseConfig(configPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			logger.Warn(ctx, "failed to parse MCP config",
				slog.F("path", configPath),
				slog.Error(err),
			)
			configErrors = append(configErrors, ConfigError{Path: configPath, Err: boundDiagnostic(err.Error())})
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
	return deduped, configErrors, snap
}

// classifyServers compares wanted configs against the current
// server map and returns a diff describing what changed.
// Acquires and releases m.mu for reading.
func (m *Manager) classifyServers(wanted map[string]ServerConfig) (*serverDiff, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrManagerClosed
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
// Failed connects are logged and reported by name with a sanitized
// error so the discovery report can attribute them. onConnected runs
// for each successful connect while siblings may still be connecting.
func (m *Manager) connectAll(ctx context.Context, toConnect []ServerConfig, onConnected func(connectedServer)) ([]connectedServer, map[string]string) {
	logger := m.logger.With(agentchat.Fields(ctx)...)

	if hook := m.connectStartedHook; hook != nil {
		hook()
	}

	var (
		mu        sync.Mutex
		connected []connectedServer
		failed    = make(map[string]string)
	)
	var eg errgroup.Group
	for _, cfg := range toConnect {
		eg.Go(func() error {
			c, err := m.connectServer(ctx, cfg)
			if err != nil {
				logger.Warn(ctx, "skipping MCP server",
					slog.F("server", cfg.Name),
					slog.F("transport", cfg.Transport),
					slog.Error(err),
				)
				mu.Lock()
				failed[cfg.Name] = m.sanitizeError(cfg, err)
				mu.Unlock()
				return nil // Don't fail the group.
			}
			cs := connectedServer{name: cfg.Name, config: cfg, client: c}
			mu.Lock()
			connected = append(connected, cs)
			mu.Unlock()
			onConnected(cs)
			return nil
		})
	}
	_ = eg.Wait()
	return connected, failed
}

// publishConnected installs a server that connected while the reload is
// still waiting on its siblings and publishes it into the report next to
// the servers already published, so one hung server cannot hold back a
// healthy one past the chat's bounded first-turn wait. Only the new
// server is listed; the reload's final refreshCatalog re-lists everything
// once, so N successful connects cost N interim list calls rather than
// N squared. The phase stays pending until the whole reload settles. A
// server that already has an entry (a changed configuration
// reconnecting) keeps serving its previous client until installServers
// swaps and closes it, so the old client is never orphaned if the
// manager closes mid-reload.
func (m *Manager) publishConnected(ctx context.Context, wanted map[string]ServerConfig, configErrors []ConfigError, cs connectedServer) {
	m.mu.Lock()
	if _, exists := m.servers[cs.name]; m.closed || exists {
		m.mu.Unlock()
		return
	}
	m.servers[cs.name] = &serverEntry{config: cs.config, client: cs.client}
	m.serverGen++
	m.mu.Unlock()

	res := m.listServerTools(ctx, cs.name, cs.client)
	st := ServerStatus{Name: cs.name, Connected: res.err == nil, Tools: res.tools}
	if res.err != nil {
		st.Err = m.sanitizeError(cs.config, res.err)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	// Keep the published statuses of the other wanted servers that have
	// a live client and add this one; servers still connecting stay
	// absent until they publish or the final refresh reports them.
	catalog := make([]ServerStatus, 0, len(m.catalog)+1)
	for _, prev := range m.catalog {
		if _, wantedStill := wanted[prev.Name]; !wantedStill || prev.Name == cs.name {
			continue
		}
		if _, live := m.servers[prev.Name]; live {
			catalog = append(catalog, prev)
		}
	}
	catalog = append(catalog, st)
	slices.SortFunc(catalog, func(a, b ServerStatus) int {
		return strings.Compare(a.Name, b.Name)
	})
	if len(configErrors) == 0 {
		configErrors = nil
	}
	changed := !reflect.DeepEqual(m.catalog, catalog) || !reflect.DeepEqual(m.configErrors, configErrors)
	if changed {
		m.catalog = catalog
		m.configErrors = configErrors
	}
	m.mu.Unlock()
	if changed {
		m.fireOnChange()
	}
}

// installServers builds the new server map from diff.keep and the
// connected list, falling back to diff.prev when a connect failed.
// Returns old entries replaced by successful connects (caller
// closes them) and, per server that kept its previous client after a
// failed reconnect, a warning explaining that its tools come from the
// previous connection. Acquires and releases m.mu.
func (m *Manager) installServers(
	wanted map[string]ServerConfig,
	diff *serverDiff,
	connected []connectedServer,
	connectErrors map[string]string,
	snap map[string]fileSnapshot,
) ([]*serverEntry, map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		for _, cs := range connected {
			_ = cs.client.Close()
		}
		return nil, nil, ErrManagerClosed
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
	warnings := make(map[string]string)
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
			// Connect failed; retain the old client. The retained
			// entry keeps its old config, so the next reload retries
			// the reconnect and clears the warning on success.
			newServers[wantCfg.Name] = prev
			warnings[wantCfg.Name] = boundDiagnostic("reconnect with the updated configuration failed, tools are from the previous connection: " + connectErrors[wantCfg.Name])
		}
	}

	m.servers = newServers
	m.serverGen++
	m.snapshot = snap
	return replaced, warnings, nil
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

// Report returns a deep copy of the current discovery report: one
// status per declared server and one entry per unusable config file.
// It never blocks on I/O: the agentcontext resolver calls it on every
// re-resolve to build the MCP resources.
func (m *Manager) Report() Report {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Report{
		Servers:      cloneServerStatuses(m.catalog),
		ConfigErrors: slices.Clone(m.configErrors),
	}
}

// fireOnChange invokes the registered reload callback, if any, without
// holding the cache lock.
func (m *Manager) fireOnChange() {
	m.mu.RLock()
	fn := m.onChange
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
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

	result, err := entry.client.CallTool(callCtx, &mcp.CallToolParams{
		Name:      originalName,
		Arguments: req.Arguments,
	})
	if err != nil {
		return workspacesdk.CallMCPToolResponse{}, xerrors.Errorf("call tool %q on %q: %w", originalName, serverName, err)
	}

	return convertResult(result), nil
}

type listResult struct {
	tools []ToolInfo
	err   error
}

// listServerTools performs one bounded tools/list call against a live
// client.
func (m *Manager) listServerTools(ctx context.Context, name string, client *mcp.ClientSession) listResult {
	listCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	result, err := client.ListTools(listCtx, nil)
	cancel()
	if err != nil {
		m.logger.With(agentchat.Fields(ctx)...).Warn(ctx, "failed to list tools from MCP server",
			slog.F("server", name),
			slog.Error(err),
		)
		return listResult{err: err}
	}
	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: toolInputSchemaMap(tool.InputSchema),
		})
	}
	return listResult{tools: tools}
}

// refreshCatalog re-lists tools from the connected servers and rebuilds
// the per-server catalog the agentcontext resolver consumes. Every
// declared server in wanted appears in the result: a server with a live
// client contributes its listed tools (or its list error) plus any
// retained-client warning, and a server that never connected appears as
// an unreadable entry carrying its connect error so it surfaces in the
// snapshot instead of vanishing. It returns whether the report (catalog
// or config errors) changed so the caller can fire the reload callback.
func (m *Manager) refreshCatalog(
	ctx context.Context,
	wanted map[string]ServerConfig,
	configErrors []ConfigError,
	connectErrors map[string]string,
	warnings map[string]string,
) bool {
	// Snapshot the connected servers under the read lock.
	m.mu.RLock()
	servers := make(map[string]*serverEntry, len(m.servers))
	for k, v := range m.servers {
		servers[k] = v
	}
	gen := m.serverGen
	m.mu.RUnlock()

	// List tools from every connected server in parallel, without
	// holding any lock.
	var (
		mu      sync.Mutex
		results = make(map[string]listResult, len(servers))
	)
	var eg errgroup.Group
	for name, entry := range servers {
		eg.Go(func() error {
			res := m.listServerTools(ctx, name, entry.client)
			mu.Lock()
			results[name] = res
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()

	// Build one status per declared server so a server that never
	// connected surfaces as an unreadable entry rather than vanishing.
	catalog := make([]ServerStatus, 0, len(wanted))
	for name := range wanted {
		st := ServerStatus{Name: name}
		switch res, ok := results[name]; {
		case ok && res.err == nil:
			st.Connected = true
			st.Tools = res.tools
			st.Warning = warnings[name]
		case ok:
			// The listed session may be a retained previous connection,
			// so its error is sanitized with the config it was opened
			// with, not the config that failed to replace it.
			st.Err = m.sanitizeError(servers[name].config, res.err)
		case connectErrors[name] != "":
			st.Err = connectErrors[name]
		default:
			st.Err = "failed to connect"
		}
		catalog = append(catalog, st)
	}
	slices.SortFunc(catalog, func(a, b ServerStatus) int {
		return strings.Compare(a.Name, b.Name)
	})

	m.mu.Lock()
	defer m.mu.Unlock()
	// Skip the write if the server map changed since the snapshot. A
	// doReload that bumped the generation will rebuild the catalog.
	if m.serverGen != gen {
		return false
	}
	if len(catalog) == 0 {
		catalog = nil
	}
	if len(configErrors) == 0 {
		configErrors = nil
	}
	if reflect.DeepEqual(m.catalog, catalog) && reflect.DeepEqual(m.configErrors, configErrors) {
		return false
	}
	m.catalog = catalog
	m.configErrors = configErrors
	return true
}

// Close terminates all MCP server connections and child
// processes, stops the config file watcher, and waits for any
// in-flight watcher-driven reload to complete.
func (m *Manager) Close() error {
	// Mark the manager closed and signal closedCh first, then
	// hand the watcher off and release the lock. Marking closed
	// before w.Close() ensures that any in-flight
	// handleWatchedConfigChange short-circuits and any Reload
	// blocked in waitReload observes m.closedCh, instead of
	// blocking firesWG.Wait() inside w.Close() until a 30 s
	// connectAll times out.
	m.mu.Lock()
	m.closed = true
	m.closeOnce.Do(func() { close(m.closedCh) })
	w := m.watcher
	m.watcher = nil
	m.mu.Unlock()

	// Close the watcher outside the manager lock. Its goroutine
	// may call handleWatchedConfigChange, which takes m.mu, so
	// holding m.mu while waiting for the watcher to drain would
	// deadlock. Close on a nil watcher is a no-op.
	if w != nil {
		_ = w.Close()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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
	// Prevent an in-flight refreshCatalog from repopulating the
	// catalog after Close clears it.
	m.serverGen++
	m.catalog = nil
	m.configErrors = nil

	// Cancel while holding the lock so waiters that observe
	// m.ctx.Done also observe m.closed when checking closeErr.
	m.cancel()
	return errors.Join(errs...)
}

// connectServer does not modify Manager state.
func (m *Manager) connectServer(ctx context.Context, cfg ServerConfig) (*mcp.ClientSession, error) {
	// Use ctx for the transport so a stdio subprocess outlives the
	// connect handshake. connectCtx bounds only Connect; closing the
	// session or canceling ctx stops the subprocess.
	tr, err := m.createTransport(ctx, cfg)
	if err != nil {
		return nil, xerrors.Errorf("create transport for %q: %w", cfg.Name, err)
	}

	c := mcp.NewClient(&mcp.Implementation{
		Name:    "coder-agent",
		Version: buildinfo.Version(),
	}, nil)

	// The timeout runs on the manager clock so tests can expire a hung
	// handshake deterministically instead of waiting out connectTimeout.
	connectCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := m.clock.AfterFunc(connectTimeout, func() { cancel(errConnectTimeout) }, "agentmcp", "connect")
	defer timer.Stop()

	session, err := c.Connect(connectCtx, tr, nil)
	if err != nil {
		if cause := context.Cause(connectCtx); errors.Is(cause, errConnectTimeout) {
			err = cause
		}
		return nil, xerrors.Errorf("connect %q: %w", cfg.Name, err)
	}

	return session, nil
}

func (m *Manager) createTransport(ctx context.Context, cfg ServerConfig) (mcp.Transport, error) {
	switch cfg.Transport {
	case "stdio":
		env := m.buildEnv(ctx, cfg.Env)
		cmd := m.execer.CommandContext(ctx, cfg.Command, cfg.Args...)
		cmd.Env = env
		cmd.Dir = m.resolveWorkingDir()
		return &mcp.CommandTransport{Command: cmd}, nil
	case "http", "":
		return &mcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClientWithHeaders(cfg.Headers),
		}, nil
	case "sse":
		return &mcp.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClientWithHeaders(cfg.Headers),
		}, nil
	default:
		return nil, xerrors.Errorf("unsupported transport %q", cfg.Transport)
	}
}

// resolveWorkingDir returns the directory to launch stdio MCP servers in
// so relative Args resolve against the workspace, not the agent's temp
// cwd: the configured workspace dir if it exists, otherwise the user's
// home dir. It mirrors SSH and the process API via
// usershell.ResolveWorkingDirectory so the three cannot drift.
func (m *Manager) resolveWorkingDir() string {
	var configured string
	if m.workingDir != nil {
		configured = m.workingDir()
	}
	dir, err := usershell.ResolveWorkingDirectory(m.fs, m.envInfo, configured)
	if err != nil {
		return ""
	}
	return dir
}

// buildEnv enriches the process environment via the agent's
// updateEnv callback, then merges explicit overrides from the
// server config on top.
func (m *Manager) buildEnv(ctx context.Context, explicit map[string]string) []string {
	logger := m.logger.With(agentchat.Fields(ctx)...)

	env := m.envInfo.Environ()
	if m.updateEnv != nil {
		var err error
		env, err = m.updateEnv(env)
		if err != nil {
			logger.Warn(ctx, "failed to enrich MCP server environment",
				slog.Error(err),
			)
			env = m.envInfo.Environ()
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
		case *mcp.TextContent:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "text",
				Text: c.Text,
			})
		case *mcp.ImageContent:
			// The SDK decodes base64 during unmarshal; re-encode to
			// keep the agent API's base64 wire format.
			content = append(content, workspacesdk.MCPToolContent{
				Type:      "image",
				Data:      base64.StdEncoding.EncodeToString(c.Data),
				MediaType: c.MIMEType,
			})
		case *mcp.AudioContent:
			content = append(content, workspacesdk.MCPToolContent{
				Type:      "audio",
				Data:      base64.StdEncoding.EncodeToString(c.Data),
				MediaType: c.MIMEType,
			})
		case *mcp.EmbeddedResource:
			content = append(content, workspacesdk.MCPToolContent{
				Type: "resource",
				Text: fmt.Sprintf("[embedded resource: %T]", c.Resource),
			})
		case *mcp.ResourceLink:
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

// ServerStatus is a point-in-time view of one MCP server's connection
// state and tools, used by the agentcontext resolver to build
// KindMCPServer resources. Tool names are exactly as the server
// reported them (no server prefix); the resource carries the server
// name separately.
type ServerStatus struct {
	Name      string
	Connected bool
	// Err carries the sanitized connect or list failure when Connected
	// is false.
	Err string
	// Warning is a non-fatal note on a connected server, set when a
	// reconnect after a config change failed and the previous client
	// was retained, so Tools describe the previous connection.
	Warning string
	Tools   []ToolInfo
}

// ConfigError records one .mcp.json file that exists but could not be
// used: unreadable, malformed JSON, or a server entry the config schema
// rejects (for example neither command nor url). The whole file is
// skipped, so every server it declares is missing from Servers.
type ConfigError struct {
	Path string
	Err  string
}

// Report is the manager's complete discovery result. Servers holds one
// status per declared server, including connected servers with zero
// tools; ConfigErrors holds one entry per unusable config file.
type Report struct {
	Servers      []ServerStatus
	ConfigErrors []ConfigError
}

// ToolInfo is one tool exposed by an MCP server. InputSchema is the
// JSON-Schema-shaped object the server reported for the tool's
// arguments, or nil when the schema is empty.
type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Only type, properties, and required are exposed through ToolInfo;
// empty schemas leave InputSchema unset.
func toolInputSchemaMap(schema any) map[string]any {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if typ, ok := m["type"].(string); ok && typ != "" {
		out["type"] = typ
	}
	// Preserve an empty "properties" object: dropping it collapses the
	// schema to nil by the time the tool definition is rebuilt, and a
	// nil properties serializes to JSON null, which OpenAI rejects.
	if properties, ok := m["properties"].(map[string]any); ok {
		out["properties"] = properties
	}
	if required, ok := m["required"].([]any); ok && len(required) > 0 {
		out["required"] = required
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneServerStatuses deep-copies a catalog so callers cannot mutate the
// Manager's cache. Tool input schemas are treated as immutable and
// shared by reference.
func cloneServerStatuses(in []ServerStatus) []ServerStatus {
	if len(in) == 0 {
		return nil
	}
	out := make([]ServerStatus, len(in))
	for i, s := range in {
		s.Tools = slices.Clone(s.Tools)
		out[i] = s
	}
	return out
}
