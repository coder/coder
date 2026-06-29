package agentcontext

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// ManagerOptions configures a Manager. Zero values get sensible
// defaults.
type ManagerOptions struct {
	// Logger receives diagnostic messages. Required.
	Logger slog.Logger
	// Clock is the time source used for the watcher's
	// debounce timer. Optional; defaults to quartz.NewReal().
	Clock quartz.Clock
	// WorkingDir is evaluated on every resolve, mirroring the
	// existing agent convention. The result is used as a
	// scan root.
	WorkingDir func() string
	// InitialSources seeds the Manager's source list at boot
	// time. Sources from CODER_AGENT_EXP_*_DIRS env vars or
	// startup scripts are layered here.
	InitialSources []Source
	// AllowedRoots restricts which paths may be added as
	// sources at runtime. When empty the package falls back
	// to [~, ~/.coder, ~/.claude] plus the working directory.
	// Tests override this to exercise the validation logic
	// directly; production callers leave it unset.
	AllowedRoots []string
	// Resolver, when non-nil, replaces the default resolver.
	// Tests use this to inject MCP resources (via
	// Resolver.MCPResources) and tighten caps.
	Resolver *Resolver
	// MCPCatalog, when non-nil, supplies the per-server MCP snapshot
	// the Manager surfaces as KindMCPServer resources on every
	// resolve. The agent injects the shared MCP engine's catalog here
	// so discovery and execution use one set of server connections.
	// It is ignored when the resolver already has an MCP provider
	// (e.g. a test injecting one via Resolver).
	MCPCatalog func() []MCPServerStatus
	// Debounce overrides the watcher's debounce window.
	Debounce time.Duration
}

// Source is a user-declared scan root added to the agent's
// in-memory list via the HTTP API or boot-time env seeding.
// Identity is the canonical absolute path.
type Source struct {
	// Path is the canonical absolute path (symlinks resolved,
	// ~ expanded). Empty means the zero value.
	Path string
}

// Manager orchestrates source CRUD, resolution, watching, and
// Pusher fan-out. Construct with NewManager; start its lifecycle
// goroutines with Run; tear down with Close.
type Manager struct {
	logger       slog.Logger
	clock        quartz.Clock
	workingDir   func() string
	allowedRoots []string
	resolver     *Resolver
	debounce     time.Duration

	mu      sync.Mutex
	sources []Source
	// sourceIndex maps canonical path -> position in sources
	// for O(1) lookups during AddSource / RemoveSource.
	sourceIndex map[string]int

	// snapshot is the latest result of a resolver pass. It is
	// replaced atomically under mu.
	snapshot Snapshot
	// version monotonically increases per resolve pass.
	version uint64
	// resolveEpoch increments at the start of every resolver
	// pass that drops m.mu around the filesystem walk. Each
	// pass captures the epoch it claimed; at publish time it
	// compares its captured epoch against the current epoch and
	// skips the publish if a newer pass has started, preventing
	// an old walk's stale result from overwriting a newer one's
	// fresh result at a higher version number.
	resolveEpoch uint64

	// subscribers receive a non-blocking signal whenever the
	// snapshot changes. Subscribers must drain their channel
	// promptly; the Manager drops sends to full channels.
	subscribers map[chan struct{}]struct{}

	// trigger fires when AddSource / RemoveSource / watcher
	// observe a change.
	trigger chan struct{}

	// ready gates collection. While false (until the first SetReady
	// call) the Manager does not scan; Snapshot() returns the empty
	// version-0 value, which the push loop never sends to coderd.
	// Guarded by mu.
	ready bool

	// running tracks Run lifetime.
	running      bool
	closed       bool
	closedCh     chan struct{}
	runDoneCh    chan struct{}
	runStartedCh chan struct{}

	watcher *Watcher
}

// NewManager validates options and canonicalizes initial sources. The
// returned Manager is gated, so its first snapshot is the empty
// version-0 placeholder and the first real resolve runs on SetReady.
// Call Run to start the watcher and re-resolve goroutine.
func NewManager(opts ManagerOptions) *Manager {
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = &Resolver{}
	}

	m := &Manager{
		logger:       opts.Logger,
		clock:        clock,
		workingDir:   opts.WorkingDir,
		allowedRoots: append([]string(nil), opts.AllowedRoots...),
		resolver:     resolver,
		debounce:     debounce,
		sources:      make([]Source, 0),
		sourceIndex:  make(map[string]int),
		subscribers:  make(map[chan struct{}]struct{}),
		trigger:      make(chan struct{}, 1),
		closedCh:     make(chan struct{}),
		runDoneCh:    make(chan struct{}),
		runStartedCh: make(chan struct{}),
	}

	// Surface the shared MCP engine's catalog as KindMCPServer
	// resources unless the resolver already has a provider (tests
	// inject one via Resolver). The engine owns the connection
	// lifecycle and notifies this Manager via Trigger when its
	// catalog changes (see agent wiring). Wire it before SetReady runs
	// the first resolve.
	if resolver.MCPResources == nil && opts.MCPCatalog != nil {
		resolver.MCPResources = func() []Resource {
			return buildMCPServerResources(opts.MCPCatalog())
		}
	}

	for _, s := range opts.InitialSources {
		identity, err := lexicalPath(s.Path)
		if err != nil {
			// Initial sources may not exist yet at boot
			// time; log and skip rather than abort the
			// agent.
			m.logger.Warn(context.Background(),
				"skipping invalid initial source",
				slog.F("path", s.Path),
				slog.Error(err))
			continue
		}
		m.addSourceLocked(identity)
	}

	// Start gated: m.snapshot stays the zero value (version 0) until
	// SetReady runs the first resolve.
	return m
}

// Run starts the watcher and the re-resolve goroutine. Run
// blocks until ctx is canceled or Close is called. It is safe
// to call Run at most once per Manager.
func (m *Manager) Run(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return xerrors.New("agentcontext: Manager.Run called more than once")
	}
	if m.closed {
		m.mu.Unlock()
		return xerrors.New("agentcontext: Manager already closed")
	}
	m.running = true
	close(m.runStartedCh)
	m.mu.Unlock()
	// Close any early-exit path so Close does not block on
	// runDoneCh after Run already set running=true. The deferred
	// close runs even when NewWatcher fails.
	defer close(m.runDoneCh)

	watcher, err := NewWatcher(WatcherOptions{
		Logger:   m.logger.Named("watcher"),
		Clock:    m.clock,
		Debounce: m.debounce,
		OnChange: m.signal,
	})
	if err != nil {
		// NewWatcher already falls back to degraded mode on
		// init failure, so an actual error here is
		// exceptional.
		return xerrors.Errorf("create watcher: %w", err)
	}
	m.mu.Lock()
	m.watcher = watcher
	roots := m.scanRootsLocked()
	m.mu.Unlock()
	watcher.Sync(ctx, roots)

	defer watcher.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.closedCh:
			return nil
		case <-m.trigger:
			m.mu.Lock()
			roots := m.scanRootsLocked()
			m.mu.Unlock()
			watcher.Sync(ctx, roots)
			m.resolveAndBroadcast(ctx)
		}
	}
}

// started returns a channel that is closed once Run has
// claimed the running flag. Tests use it to coordinate with
// the watcher loop without polling; a closed channel never
// blocks, so this is safe to call repeatedly.
func (m *Manager) started() <-chan struct{} {
	return m.runStartedCh
}

// Close stops the Manager. Close is idempotent; subsequent
// calls block until Run exits.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		running := m.running
		m.mu.Unlock()
		if running {
			<-m.runDoneCh
		}
		return nil
	}
	m.closed = true
	running := m.running
	close(m.closedCh)
	m.mu.Unlock()
	if running {
		<-m.runDoneCh
	}
	return nil
}

// Sources returns a defensive copy of the current source list.
// The returned slice is safe to mutate.
func (m *Manager) Sources() []Source {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, len(m.sources))
	copy(out, m.sources)
	return out
}

// HasSource reports whether path matches a registered source,
// returning its lexical identity.
func (m *Manager) HasSource(path string) (canonical string, ok bool) {
	c, err := lexicalPath(path)
	if err != nil {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok = m.sourceIndex[c]
	return c, ok
}

// AddSource validates a new source against the AllowedRoots set
// and registers it by its lexical identity. AddSource is
// idempotent.
func (m *Manager) AddSource(s Source) (Source, error) {
	identity, err := lexicalPath(s.Path)
	if err != nil {
		return Source{}, xerrors.Errorf("canonicalize: %w", err)
	}
	// Validate the resolved path so symlinks can't escape the allowed roots.
	resolved, err := CanonicalizePath(s.Path)
	if err != nil {
		return Source{}, xerrors.Errorf("canonicalize: %w", err)
	}
	if err := ValidateSourcePath(resolved, m.effectiveAllowedRoots()); err != nil {
		return Source{}, err
	}

	m.mu.Lock()
	if idx, ok := m.sourceIndex[identity]; ok {
		out := m.sources[idx]
		m.mu.Unlock()
		return out, nil
	}
	m.sourceIndex[identity] = len(m.sources)
	m.sources = append(m.sources, Source{Path: identity})
	m.mu.Unlock()

	m.signal()
	return Source{Path: identity}, nil
}

// SeedSources registers a batch of trusted sources without
// AllowedRoots validation, the late-binding equivalent of
// ManagerOptions.InitialSources for when the working directory
// is only known after Run starts. Invalid paths are skipped and
// duplicates are ignored.
//
// Untrusted callers must use AddSource; SeedSources exists only
// for manifest-triggered seeding from CODER_AGENT_EXP_*_DIRS.
func (m *Manager) SeedSources(sources []Source) {
	if len(sources) == 0 {
		return
	}
	m.mu.Lock()
	changed := false
	for _, s := range sources {
		identity, err := lexicalPath(s.Path)
		if err != nil {
			m.logger.Warn(context.Background(),
				"skipping invalid seeded source",
				slog.F("path", s.Path),
				slog.Error(err))
			continue
		}
		if m.addSourceLocked(identity) {
			changed = true
		}
	}
	m.mu.Unlock()
	if changed {
		m.signal()
	}
}

// RemoveSource removes the source matching path by its lexical
// identity, returning ErrSourceNotFound if none matches.
func (m *Manager) RemoveSource(path string) error {
	identity, err := lexicalPath(path)
	if err != nil {
		// A path that does not canonicalize cannot match any
		// existing source. Mirror HasSource semantics by
		// reporting not-found rather than leaking the
		// canonicalize error to API callers.
		return ErrSourceNotFound
	}

	m.mu.Lock()
	idx, ok := m.sourceIndex[identity]
	if !ok {
		m.mu.Unlock()
		return ErrSourceNotFound
	}
	// O(n) compaction is fine for the typical handful of
	// user-added sources.
	m.sources = append(m.sources[:idx], m.sources[idx+1:]...)
	delete(m.sourceIndex, identity)
	for i := idx; i < len(m.sources); i++ {
		m.sourceIndex[m.sources[i].Path] = i
	}
	m.mu.Unlock()

	m.signal()
	return nil
}

// addSourceLocked registers identity unless already present,
// reporting whether it was added. m.mu must be held.
func (m *Manager) addSourceLocked(identity string) bool {
	if _, ok := m.sourceIndex[identity]; ok {
		return false
	}
	m.sourceIndex[identity] = len(m.sources)
	m.sources = append(m.sources, Source{Path: identity})
	return true
}

// Snapshot returns the latest Snapshot. The returned value is
// safe to share but shares the same Resources slice as the
// internal state; callers must not mutate it.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

// SubscribeChanges returns a buffered channel that receives a
// signal whenever the snapshot changes. The unsubscribe
// callback is safe to call from any goroutine and is
// idempotent.
func (m *Manager) SubscribeChanges() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.subscribers[ch] = struct{}{}
	m.mu.Unlock()

	// OnceFunc returns a closure that runs the underlying
	// function at most once. Subsequent invocations are no-ops,
	// matching the idempotency contract callers rely on.
	unsub := sync.OnceFunc(func() {
		m.mu.Lock()
		delete(m.subscribers, ch)
		m.mu.Unlock()
		// Don't close ch: readers may still be in flight.
	})
	return ch, unsub
}

// Resync forces an immediate re-resolve and returns the new
// Snapshot. Resync is safe to call regardless of whether Run is
// active. Like resolveAndBroadcast, Resync drops the Manager's
// mutex around the resolver pass so concurrent Sources,
// AddSource, RemoveSource, and Snapshot calls do not block on
// filesystem I/O. When the watcher is active, Resync also
// re-arms it so newly added scan roots are observed for
// subsequent edits.
func (m *Manager) Resync(ctx context.Context) (Snapshot, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return m.Snapshot(), ctxErr
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return m.Snapshot(), ErrManagerClosed
	}
	if !m.ready {
		// Gated until SetReady: return the version-0 placeholder, no scan.
		snap := m.snapshot
		m.mu.Unlock()
		return snap, nil
	}
	roots := m.scanRootsLocked()
	resolver := m.resolver
	watcher := m.watcher
	m.resolveEpoch++
	myEpoch := m.resolveEpoch
	m.mu.Unlock()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return m.Snapshot(), ctxErr
	}
	snap := resolver.ResolveContext(ctx, roots)
	if ctxErr := ctx.Err(); ctxErr != nil {
		// Cancellation mid-walk yields a partial or empty
		// Snapshot whose SnapshotError is set to
		// "context canceled". Publishing it would replace
		// the live Snapshot with empty resources until the
		// next trigger, so bail without touching state.
		return m.Snapshot(), ctxErr
	}
	if snap.SnapshotError == "" && watcher != nil {
		if d := watcher.Degraded(); d != "" {
			snap.SnapshotError = d
		}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return m.Snapshot(), ErrManagerClosed
	}
	if m.resolveEpoch != myEpoch {
		// A newer resolve pass started while this one was
		// walking the filesystem. The newer pass's data
		// strictly supersedes ours, so skip the publish to
		// avoid overwriting a fresher Snapshot at a higher
		// version. Return the currently published Snapshot,
		// which is at least as fresh as ours. The watcher
		// is NOT re-armed: the winning pass already synced
		// with the current roots, and replaying our stale
		// root set here would drop watches on sources that
		// only the newer pass knows about.
		published := m.snapshot
		m.mu.Unlock()
		return published, nil
	}
	m.version++
	snap.Version = m.version
	m.snapshot = snap
	subs := make([]chan struct{}, 0, len(m.subscribers))
	for ch := range m.subscribers {
		subs = append(subs, ch)
	}
	m.mu.Unlock()

	if watcher != nil {
		watcher.Sync(ctx, roots)
	}

	// The broadcast is unconditional: Resync waiters that
	// triggered the pass without an actual content change
	// still need to wake up. Subscribers compare snapshots via
	// AggregateHash if they want to filter.
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return snap, nil
}

// signal triggers a re-resolve. Sends are non-blocking; the
// trigger channel has a depth of 1, which coalesces bursts.
func (m *Manager) signal() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Trigger queues an asynchronous re-resolve. Trigger returns
// immediately; the Run goroutine performs the filesystem walk
// in the background and broadcasts when it finishes. Use
// Trigger when the caller wants the watcher to pick up an
// updated working directory or scan-root set but does not need
// the new Snapshot synchronously. Trigger is a no-op when Run
// has not started or the Manager is closed.
func (m *Manager) Trigger() {
	m.signal()
}

// SetReady starts context collection: the agent calls it once startup
// scripts finish (or terminally fail) so context is never collected
// from a half-built workspace. Idempotent; the first call triggers the
// first resolve and push.
func (m *Manager) SetReady() {
	m.mu.Lock()
	if m.ready || m.closed {
		m.mu.Unlock()
		return
	}
	m.ready = true
	running := m.running
	m.mu.Unlock()

	if running {
		// The Run loop owns the watcher; signal it to re-sync and resolve
		// with ready=true.
		m.signal()
		return
	}
	// No Run loop yet (embedders or tests driving the Manager directly):
	// resolve inline.
	m.resolveAndBroadcast(context.Background())
}

// scanRootsLocked returns the list of ScanRoots to feed the
// resolver and watcher. The Manager's mutex must be held.
func (m *Manager) scanRootsLocked() []ScanRoot {
	builtinRoots := defaultBuiltinRoots()
	out := make([]ScanRoot, 0, 1+len(builtinRoots)+len(m.sources))
	if m.workingDir != nil {
		if wd := strings.TrimSpace(m.workingDir()); wd != "" {
			// The working directory is a single scan root. The
			// resolver reads its top-level instruction files and
			// .mcp.json plus the fixed skill containers under it;
			// it neither descends into subdirectories nor climbs
			// to parent directories. Additional directories are
			// added explicitly as Sources or via the seeding env
			// vars.
			out = append(out, ScanRoot{Path: wd})
		}
	}
	for _, r := range builtinRoots {
		canonical, err := CanonicalizePath(r)
		if err != nil {
			continue
		}
		out = append(out, ScanRoot{Path: canonical})
	}
	for _, s := range m.sources {
		out = append(out, ScanRoot{Path: s.Path, UserSource: s.Path})
	}
	return out
}

// effectiveAllowedRoots returns the AllowedRoots augmented
// with the current working directory. The working directory is
// evaluated on every call so it picks up the workspace's
// resolved path after the agent's manifest finishes loading.
// When AllowedRoots is empty the package falls back to its
// default policy ([~, ~/.coder, ~/.claude]).
func (m *Manager) effectiveAllowedRoots() []string {
	var roots []string
	if len(m.allowedRoots) > 0 {
		roots = append(roots, m.allowedRoots...)
	} else {
		roots = append(roots, defaultAllowedRoots()...)
	}
	if m.workingDir != nil {
		if wd := strings.TrimSpace(m.workingDir()); wd != "" {
			roots = append(roots, wd)
		}
	}
	return roots
}

// resolveAndBroadcast computes a fresh snapshot and notifies
// every subscriber. The broadcast is unconditional: Resync
// waiters that triggered the pass without an actual content
// change still need to wake up. Subscribers compare snapshots
// via AggregateHash if they want to filter.
func (m *Manager) resolveAndBroadcast(ctx context.Context) {
	// Snapshot the inputs under the lock, then release it
	// before running the resolver. The resolver walks the
	// filesystem, reads files, and hashes them; holding
	// m.mu across that would block Sources, AddSource,
	// RemoveSource, Snapshot, and SubscribeChanges for the
	// duration of the pass.
	m.mu.Lock()
	if !m.ready {
		// Gated until SetReady: no scan, no broadcast.
		m.mu.Unlock()
		return
	}
	roots := m.scanRootsLocked()
	resolver := m.resolver
	watcher := m.watcher
	m.resolveEpoch++
	myEpoch := m.resolveEpoch
	m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return
	}
	snap := resolver.ResolveContext(ctx, roots)
	if err := ctx.Err(); err != nil {
		// Cancellation mid-walk yields a partial or empty
		// Snapshot. Publishing it would replace the live
		// Snapshot with empty resources, so bail without
		// touching state. The Run loop's gracefulCtx is
		// canceled only at shutdown, but defensive checks
		// keep the publish contract uniform with Resync.
		return
	}
	// Surface watcher degradation as a snapshot-level error
	// when the resolver did not already emit one.
	if snap.SnapshotError == "" && watcher != nil {
		if d := watcher.Degraded(); d != "" {
			snap.SnapshotError = d
		}
	}

	m.mu.Lock()
	if m.resolveEpoch != myEpoch {
		// A newer resolve pass started while this one was
		// walking the filesystem. Skip the publish so a
		// stale-epoch result does not overwrite a fresher
		// Snapshot at a higher version number. The newer
		// pass will broadcast its own result.
		m.mu.Unlock()
		return
	}
	m.version++
	snap.Version = m.version
	m.snapshot = snap
	subs := make([]chan struct{}, 0, len(m.subscribers))
	for ch := range m.subscribers {
		subs = append(subs, ch)
	}
	m.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// ErrSourceNotFound is returned by RemoveSource when the
// requested path is not in the source list.
var ErrSourceNotFound = xerrors.New("source not found")

// ErrManagerClosed is returned by methods called after Close.
var ErrManagerClosed = xerrors.New("agentcontext: manager closed")
