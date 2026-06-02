package agentcontext

import (
	"context"
	"sort"
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
	// workingDir]. Empty disables validation.
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

	// subscribers receive a non-blocking signal whenever the
	// snapshot changes. Subscribers must drain their channel
	// promptly; the Manager drops sends to full channels.
	subscribers map[chan struct{}]struct{}

	// trigger fires when AddSource / RemoveSource / watcher
	// observe a change.
	trigger chan struct{}

	// running tracks Run lifetime.
	running   bool
	closed    bool
	closedCh  chan struct{}
	runDoneCh chan struct{}

	watcher *Watcher
}

// NewManager validates options, canonicalizes initial sources,
// performs the first resolver pass synchronously, and returns
// the resulting Manager. Run must be called separately to start
// the watcher and re-resolve goroutine.
func NewManager(opts ManagerOptions) (*Manager, error) {
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
	}

	for _, s := range opts.InitialSources {
		canonical, err := CanonicalizePath(s.Path)
		if err != nil {
			// Initial sources may not exist yet at boot
			// time; log and skip rather than abort the
			// agent.
			m.logger.Warn(context.Background(),
				"agentcontext: skipping invalid initial source",
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

	return m, nil
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
	m.mu.Unlock()

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

	defer close(m.runDoneCh)
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
// no such source exists.
func (m *Manager) RemoveSource(path string) error {
	canonical, err := CanonicalizePath(path)
	if err != nil {
		return xerrors.Errorf("canonicalize: %w", err)
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

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subscribers, ch)
			m.mu.Unlock()
			// Don't close ch: readers may still be in flight.
		})
	}
	return ch, unsub
}

// Resync forces an immediate re-resolve and returns the new
// Snapshot. Resync is safe to call regardless of whether Run
// is active; the work is done synchronously under the
// Manager's mutex either way.
func (m *Manager) Resync(ctx context.Context) (Snapshot, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return m.Snapshot(), ctxErr
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return m.Snapshot(), ErrManagerClosed
	}
	m.resolveLocked()
	snap := m.snapshot
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

// effectiveAllowedRoots returns the AllowedRoots augmented with
// a sensible fallback (~ and the working directory) when the
// caller did not configure any.
func (m *Manager) effectiveAllowedRoots() []string {
	if len(m.allowedRoots) > 0 {
		return append([]string{}, m.allowedRoots...)
	}
	roots := []string{"~"}
	if m.workingDir != nil {
		if wd := strings.TrimSpace(m.workingDir()); wd != "" {
			roots = append(roots, wd)
		}
	}
	return roots
}

// resolveAndBroadcast computes a fresh snapshot and notifies
// subscribers if the aggregate hash changed.
func (m *Manager) resolveAndBroadcast(ctx context.Context) {
	m.mu.Lock()
	m.resolveLocked()
	subs := make([]chan struct{}, 0, len(m.subscribers))
	for ch := range m.subscribers {
		subs = append(subs, ch)
	}
	m.mu.Unlock()

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
	_ = ctx
}

// resolveLocked runs the resolver and stamps the snapshot.
// The Manager's mutex must be held.
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
	// Ensure resources are stable-sorted by ID even when the
	// resolver did not run them through caps.
	sort.Slice(snap.Resources, func(i, j int) bool {
		return snap.Resources[i].ID < snap.Resources[j].ID
	})
	m.snapshot = snap
}

// ErrSourceNotFound is returned by RemoveSource when the
// requested path is not in the source list.
var ErrSourceNotFound = xerrors.New("source not found")

// ErrManagerClosed is returned by methods called after Close.
var ErrManagerClosed = xerrors.New("agentcontext: manager closed")
