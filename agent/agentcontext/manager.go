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

// CurrentSchemaVersion is the on-wire shape version. Bump
// whenever the resource format changes in a way that requires
// coderd-side awareness.
const CurrentSchemaVersion uint64 = 1

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
	// BuiltinRoots are scan roots layered before user-added
	// sources. Typically: the working directory, ~/.coder,
	// ~/.coder/skills, .agents/skills,
	// ~/.claude/plugins/cache.
	BuiltinRoots []string
	// InitialSources seeds the Manager's source list at boot
	// time. Sources from CODER_AGENT_EXP_*_DIRS env vars or
	// startup scripts are layered here.
	InitialSources []Source
	// AllowedRoots restricts which paths may be added as
	// sources at runtime. Defaults to [~, ~/.coder, ~/.claude,
	// workingDir]. Empty falls back to the home directory.
	AllowedRoots []string
	// Resolver, when non-nil, replaces the default resolver.
	// Tests use this to inject MCP providers and tighten
	// caps.
	Resolver *Resolver
	// Debounce overrides the watcher's debounce window.
	Debounce time.Duration
	// SchemaVersion is the version stamped on each Snapshot.
	// Use CurrentSchemaVersion (the default) unless rolling
	// out a schema change.
	SchemaVersion uint64
}

// Manager orchestrates source CRUD, resolution, watching, and
// Pusher fan-out. Construct with NewManager; start its lifecycle
// goroutines with Run; tear down with Close.
type Manager struct {
	logger        slog.Logger
	clock         quartz.Clock
	workingDir    func() string
	builtinRoots  []string
	allowedRoots  []string
	resolver      *Resolver
	debounce      time.Duration
	schemaVersion uint64

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

	// running tracks Run lifetime.
	running      bool
	closed       bool
	closedCh     chan struct{}
	runDoneCh    chan struct{}
	runStartedCh chan struct{}

	watcher *Watcher
}

// NewManager validates options, canonicalizes initial sources,
// performs the first resolver pass synchronously, and returns
// the resulting Manager. Run must be called separately to start
// the watcher and re-resolve goroutine.
func NewManager(opts ManagerOptions) *Manager {
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}
	schemaVersion := opts.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = CurrentSchemaVersion
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = &Resolver{}
	}

	m := &Manager{
		logger:        opts.Logger,
		clock:         clock,
		workingDir:    opts.WorkingDir,
		builtinRoots:  append([]string(nil), opts.BuiltinRoots...),
		allowedRoots:  append([]string(nil), opts.AllowedRoots...),
		resolver:      resolver,
		debounce:      debounce,
		schemaVersion: schemaVersion,
		sources:       make([]Source, 0),
		sourceIndex:   make(map[string]int),
		subscribers:   make(map[chan struct{}]struct{}),
		trigger:       make(chan struct{}, 1),
		closedCh:      make(chan struct{}),
		runDoneCh:     make(chan struct{}),
		runStartedCh:  make(chan struct{}),
	}

	for _, s := range opts.InitialSources {
		canonical, err := CanonicalizePath(s.Path)
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
		if _, ok := m.sourceIndex[canonical]; ok {
			continue
		}
		m.sourceIndex[canonical] = len(m.sources)
		m.sources = append(m.sources, Source{Path: canonical})
	}

	// First snapshot is computed eagerly. The push protocol
	// requires a snapshot to be present before the agent signals
	// lifecycle = ready, so callers can rely on Snapshot() being
	// populated immediately after NewManager returns.
	m.resolveLocked()

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
		MaxDepth: m.resolver.MaxDepth,
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

// HasSource reports whether path matches an existing source
// after canonicalization. Returns the canonical path on
// success.
func (m *Manager) HasSource(path string) (canonical string, ok bool) {
	c, err := CanonicalizePath(path)
	if err != nil {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok = m.sourceIndex[c]
	return c, ok
}

// AddSource adds a new source. The path is canonicalized and
// validated against the AllowedRoots set. AddSource is
// idempotent.
func (m *Manager) AddSource(s Source) (Source, error) {
	canonical, err := CanonicalizePath(s.Path)
	if err != nil {
		return Source{}, xerrors.Errorf("canonicalize: %w", err)
	}
	if err := ValidateSourcePath(canonical, m.effectiveAllowedRoots()); err != nil {
		return Source{}, err
	}

	m.mu.Lock()
	if _, ok := m.sourceIndex[canonical]; ok {
		out := m.sources[m.sourceIndex[canonical]]
		m.mu.Unlock()
		return out, nil
	}
	m.sourceIndex[canonical] = len(m.sources)
	m.sources = append(m.sources, Source{Path: canonical})
	m.mu.Unlock()

	m.signal()
	return Source{Path: canonical}, nil
}

// RemoveSource removes the source matching path. Path is
// canonicalized before matching. Returns ErrSourceNotFound when
// no such source exists or when the path cannot be canonicalized.
func (m *Manager) RemoveSource(path string) error {
	canonical, err := CanonicalizePath(path)
	if err != nil {
		// A path that does not canonicalize cannot match any
		// existing source. Mirror HasSource semantics by
		// reporting not-found rather than leaking the
		// canonicalize error to API callers.
		return ErrSourceNotFound
	}

	m.mu.Lock()
	idx, ok := m.sourceIndex[canonical]
	if !ok {
		m.mu.Unlock()
		return ErrSourceNotFound
	}
	// O(n) compaction is fine for the typical handful of
	// user-added sources.
	m.sources = append(m.sources[:idx], m.sources[idx+1:]...)
	delete(m.sourceIndex, canonical)
	for i := idx; i < len(m.sources); i++ {
		m.sourceIndex[m.sources[i].Path] = i
	}
	m.mu.Unlock()

	m.signal()
	return nil
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
	roots := m.scanRootsLocked()
	resolver := m.resolver
	watcher := m.watcher
	schemaVersion := m.schemaVersion
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
	snap.SchemaVersion = schemaVersion

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
		// which is at least as fresh as ours.
		published := m.snapshot
		m.mu.Unlock()
		if watcher != nil {
			watcher.Sync(ctx, roots)
		}
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

// scanRootsLocked returns the list of ScanRoots to feed the
// resolver and watcher. The Manager's mutex must be held.
func (m *Manager) scanRootsLocked() []ScanRoot {
	out := make([]ScanRoot, 0, 1+len(m.builtinRoots)+len(m.sources))
	if m.workingDir != nil {
		if wd := strings.TrimSpace(m.workingDir()); wd != "" {
			out = append(out, ScanRoot{Path: wd})
		}
	}
	for _, r := range m.builtinRoots {
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
// When AllowedRoots is empty the home directory is added as a
// permissive fallback.
func (m *Manager) effectiveAllowedRoots() []string {
	var roots []string
	if len(m.allowedRoots) > 0 {
		roots = append(roots, m.allowedRoots...)
	} else {
		roots = append(roots, "~")
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
	roots := m.scanRootsLocked()
	resolver := m.resolver
	watcher := m.watcher
	schemaVersion := m.schemaVersion
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
	snap.SchemaVersion = schemaVersion

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

// resolveLocked runs the resolver inline while m.mu is held.
// It is used by the synchronous initial resolve in NewManager,
// where there is no concurrent reader. Background re-resolves
// must use resolveAndBroadcast, which drops the lock around
// filesystem I/O.
func (m *Manager) resolveLocked() {
	roots := m.scanRootsLocked()
	snap := m.resolver.Resolve(roots)
	m.version++
	snap.Version = m.version
	snap.SchemaVersion = m.schemaVersion
	// Surface watcher degradation as a snapshot-level error
	// when the resolver did not already emit one.
	if snap.SnapshotError == "" && m.watcher != nil {
		if d := m.watcher.Degraded(); d != "" {
			snap.SnapshotError = d
		}
	}
	m.snapshot = snap
}

// ErrSourceNotFound is returned by RemoveSource when the
// requested path is not in the source list.
var ErrSourceNotFound = xerrors.New("source not found")

// ErrManagerClosed is returned by methods called after Close.
var ErrManagerClosed = xerrors.New("agentcontext: manager closed")
