package aibridged

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

const (
	// Each bridge has the same cache cost, so MaxCost is an item limit rather
	// than an estimate of bridge memory use.
	cacheCost = 1

	maxBridgeServeAttempts     = 3
	uncachedServeWarningPeriod = time.Minute
	bridgeRetirementTimeout    = 5 * time.Second
)

var (
	errGenerationRetired           = xerrors.New("provider generation retired")
	errBridgeBuildExited           = xerrors.New("request bridge build exited")
	errBridgeServeRetriesExhausted = xerrors.New("request bridge retry limit exceeded")
)

// Pooler describes a pool of [*aibridge.RequestBridge] instances.
type Pooler interface {
	// Serve retrieves or creates a request bridge for the current provider
	// generation, admits the request, and serves it. It returns after the
	// response has been written or the failure surfaced.
	Serve(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, rw http.ResponseWriter, r *http.Request) error
	// ReplaceProviders swaps the providers used to construct future
	// RequestBridge instances. Disabled providers must be included; the bridge
	// serves a 503 sentinel on their routes.
	ReplaceProviders(providers []aibridge.Provider)
	// Shutdown prevents new operations and drains inflight ones. Context
	// cancellation accelerates cleanup by canceling admitted requests.
	Shutdown(ctx context.Context) error
}

type PoolMetrics interface {
	Hits() uint64
	Misses() uint64
	KeysAdded() uint64
	KeysEvicted() uint64
}

type PoolOptions struct {
	MaxItems int64
	TTL      time.Duration
	Clock    quartz.Clock
}

var DefaultPoolOptions = PoolOptions{MaxItems: 5000, TTL: time.Minute * 15}

var _ Pooler = &CachedBridgePool{}

type providerGeneration struct {
	id        uint64
	providers []aibridge.Provider

	mu      sync.Mutex
	retired bool
	entries map[*bridgeEntry]struct{}
	// publishWG brackets cache publication so generation retirement cannot
	// finish until every in-flight publish attempt completes. retired is
	// mu-guarded (rather than atomic) because publish must fence publishWG.Add
	// behind the same mutex that retirement uses to observe retired.
	publishWG sync.WaitGroup
}

type bridgeEntry struct {
	key           string
	generation    *providerGeneration
	bridge        *aibridge.RequestBridge
	cacheRejected atomic.Bool
	uncached      bool
	retired       atomic.Bool
	retireOnce    sync.Once
}

type bridgeBuildCall struct {
	ready      chan struct{}
	entry      *bridgeEntry
	err        error
	panicValue any
	cleanup    func()
	users      int
}

// bridgeBuildGroup coalesces bridge construction and keeps an uncached result
// alive until every caller that joined the build has finished serving or trying
// to admit its request. Unlike singleflight, this gives rejected cache entries
// a defined shared lifetime instead of making each waiter build a fallback.
type bridgeBuildGroup struct {
	mu    sync.Mutex
	calls map[string]*bridgeBuildCall
}

func (g *bridgeBuildGroup) Do(key string, fn func() (*bridgeEntry, func(), error)) (*bridgeEntry, func(), error) {
	g.mu.Lock()
	if call := g.calls[key]; call != nil {
		call.users++
		g.mu.Unlock()
		<-call.ready
		return g.result(call)
	}
	if g.calls == nil {
		g.calls = make(map[string]*bridgeBuildCall)
	}
	call := &bridgeBuildCall{ready: make(chan struct{}), users: 1}
	g.calls[key] = call
	g.mu.Unlock()

	g.build(key, call, fn)
	return g.result(call)
}

func (g *bridgeBuildGroup) build(key string, call *bridgeBuildCall, fn func() (*bridgeEntry, func(), error)) {
	completed := false
	defer func() {
		panicValue := recover()
		switch {
		case panicValue != nil:
			call.panicValue = panicValue
		case !completed:
			// runtime.Goexit runs deferred functions without a recoverable panic.
			call.err = errBridgeBuildExited
		}

		g.mu.Lock()
		delete(g.calls, key)
		close(call.ready)
		g.mu.Unlock()

		if panicValue != nil {
			panic(panicValue)
		}
	}()

	call.entry, call.cleanup, call.err = fn()
	completed = true
}

func (g *bridgeBuildGroup) result(call *bridgeBuildCall) (*bridgeEntry, func(), error) {
	if call.panicValue != nil {
		panic(call.panicValue)
	}
	return call.entry, g.releaseFunc(call), call.err
}

func (g *bridgeBuildGroup) releaseFunc(call *bridgeBuildCall) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			var cleanup func()
			g.mu.Lock()
			call.users--
			if call.users == 0 {
				cleanup = call.cleanup
				call.cleanup = nil
			}
			g.mu.Unlock()
			if cleanup != nil {
				cleanup()
			}
		})
	}
}

// CachedBridgePool coordinates caching and retirement across three layers, in
// lock order: cacheMu/cacheWG order operations against Shutdown; replaceMu
// serializes ReplaceProviders against Shutdown; each providerGeneration guards
// retired/entries with mu and brackets publication with publishWG. Each
// bridgeEntry pairs retired with retireOnce so cache callbacks and generation
// retirement converge. bridgeBuildGroup keeps a rejected cache entry alive
// until every caller that joined its build releases it. retirementCtx and
// retirementWG bound bridge cleanup and cancel admitted requests during
// Shutdown.
type CachedBridgePool struct {
	cache *ristretto.Cache[string, *bridgeEntry]
	clock quartz.Clock

	generation atomic.Pointer[providerGeneration]
	replaceMu  sync.Mutex

	logger  slog.Logger
	options PoolOptions

	builds bridgeBuildGroup

	metrics *aibridge.Metrics
	tracer  trace.Tracer

	uncachedServeWarningAt atomic.Int64

	shutDownOnce   sync.Once
	shuttingDownCh chan struct{}

	// cacheMu and cacheWG order complete pool operations against Shutdown.
	// This prevents cache use and retirement registration after cache.Close.
	cacheMu sync.RWMutex
	cacheWG sync.WaitGroup

	// retirementCtx is canceled only by Shutdown. It has two cancellation
	// paths: Serve links its operation context to it via AfterFunc so Shutdown
	// can cut an active Serve short, and trackShutdown passes it to
	// bridge.Shutdown so Shutdown cancels admitted requests on timeout.
	retirementCtx    context.Context
	retirementCancel context.CancelFunc
	retirementWG     sync.WaitGroup
}

func NewCachedBridgePool(options PoolOptions, providers []aibridge.Provider, logger slog.Logger, metrics *aibridge.Metrics, tracer trace.Tracer) (*CachedBridgePool, error) {
	clk := options.Clock
	if clk == nil {
		clk = quartz.NewReal()
	}

	retirementCtx, retirementCancel := context.WithCancel(context.Background())
	pool := &CachedBridgePool{
		clock:   clk,
		options: options,
		metrics: metrics,
		tracer:  tracer,
		logger:  logger,

		shuttingDownCh: make(chan struct{}),

		retirementCtx:    retirementCtx,
		retirementCancel: retirementCancel,
	}

	cache, err := ristretto.NewCache(&ristretto.Config[string, *bridgeEntry]{
		// Ristretto recommends 10 counters per expected item to keep its
		// admission-frequency estimates accurate.
		NumCounters: options.MaxItems * 10,
		// Cache costs count bridge instances, not bytes.
		MaxCost: options.MaxItems * cacheCost,
		// Internal byte estimates do not match the memory retained by a bridge.
		IgnoreInternalCost: true,
		// Ristretto recommends 64 for normal Get contention.
		BufferItems: 64,
		Metrics:     true,
		OnReject: func(item *ristretto.Item[*bridgeEntry]) {
			if item != nil && item.Value != nil {
				item.Value.cacheRejected.Store(true)
			}
		},
		OnExit: func(entry *bridgeEntry) {
			if entry != nil && !entry.cacheRejected.Load() {
				entry.retire(pool)
			}
		},
	})
	if err != nil {
		retirementCancel()
		return nil, xerrors.Errorf("create cache: %w", err)
	}
	pool.cache = cache

	initial := &providerGeneration{
		id:        1,
		providers: slices.Clone(providers),
		entries:   make(map[*bridgeEntry]struct{}),
	}
	pool.generation.Store(initial)
	return pool, nil
}

// ReplaceProviders publishes a new provider generation and retires the old one.
// Already admitted requests continue to drain against their original generation.
// The old generation is retired eagerly rather than left to TTL expiry so its
// bridges, and their MCP sessions, are released promptly after a provider
// change such as a key rotation.
func (p *CachedBridgePool) ReplaceProviders(providers []aibridge.Provider) {
	if !p.beginOperation() {
		return
	}
	defer p.cacheWG.Done()

	p.replaceMu.Lock()
	defer p.replaceMu.Unlock()
	if p.isShuttingDown() {
		return
	}

	old := p.generation.Load()
	nextID := uint64(1)
	if old != nil {
		nextID = old.id + 1
	}
	next := &providerGeneration{
		id:        nextID,
		providers: slices.Clone(providers),
		entries:   make(map[*bridgeEntry]struct{}),
	}
	// Trap point for deterministic replacement and shutdown tests.
	_ = p.clock.Now("provider_generation_publish")
	p.generation.Store(next)

	if old != nil {
		old.retire(p)
	}

	p.logger.Info(context.Background(), "request bridge pool reloaded",
		slog.F("provider_count", len(next.providers)),
		slog.F("provider_generation", next.id),
	)
}

// KeyPools returns the key pools of the current live providers.
func (p *CachedBridgePool) KeyPools() []*keypool.Pool {
	generation := p.generation.Load()
	if generation == nil {
		return nil
	}

	pools := make([]*keypool.Pool, 0, len(generation.providers))
	for _, prov := range generation.providers {
		if pool := prov.KeyPool(); pool != nil {
			pools = append(pools, pool)
		}
	}
	return pools
}

// Serve retrieves or creates a request bridge for the current provider
// generation, admits the request, and serves it.
func (p *CachedBridgePool) Serve(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, rw http.ResponseWriter, r *http.Request) (outErr error) {
	spanAttrs := []attribute.KeyValue{
		attribute.String(tracing.InitiatorID, req.InitiatorID.String()),
		attribute.String(tracing.APIKeyID, req.APIKeyID),
	}
	ctx, span := p.tracer.Start(ctx, "CachedBridgePool.Serve", trace.WithAttributes(spanAttrs...))
	defer tracing.EndSpanErr(span, &outErr)
	ctx = tracing.WithRequestBridgeAttributesInContext(ctx, spanAttrs)

	if err := ctx.Err(); err != nil {
		return xerrors.Errorf("serve: %w", err)
	}
	if !p.beginOperation() {
		return xerrors.New("pool shutting down")
	}
	defer p.cacheWG.Done()

	operationCtx, cancelOperation := context.WithCancel(ctx)
	// Tie this Serve call's cancellation to pool retirement so Shutdown can
	// cut it short.
	stopRetirementCancel := context.AfterFunc(p.retirementCtx, cancelOperation)
	defer func() {
		stopRetirementCancel()
		cancelOperation()
	}()
	ctx = operationCtx

	for attempt := 1; attempt <= maxBridgeServeAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return xerrors.Errorf("serve: %w", err)
		}
		if p.isShuttingDown() {
			return xerrors.New("pool shutting down")
		}

		generation := p.generation.Load()
		if generation == nil {
			return xerrors.New("aibridged pool used before initialization completed")
		}

		entry, release, err := p.getOrBuild(ctx, req, clientFn, mcpProxyFactory, generation, span)
		if err != nil {
			release()
			if errors.Is(err, errGenerationRetired) {
				if attempt == maxBridgeServeAttempts {
					return p.retryExhausted(span, "generation")
				}
				p.recordRetry(span, "generation", attempt)
				continue
			}
			return err
		}

		served := func() bool {
			// A cache-rejected entry is leased from bridgeBuildGroup. Release it
			// even if request handling panics or exits its goroutine.
			defer release()
			if entry.uncached {
				p.recordUncachedServe(ctx, span, generation.id)
			}

			// Trap point for deterministic tests of the retirement window between
			// bridge selection and request admission.
			_ = p.clock.Now("bridge_serve_admission")
			return entry.bridge.TryServe(rw, r)
		}()
		if served {
			return nil
		}
		if attempt == maxBridgeServeAttempts {
			return p.retryExhausted(span, "admission")
		}
		p.recordRetry(span, "admission", attempt)
	}
	return errBridgeServeRetriesExhausted
}

func (p *CachedBridgePool) getOrBuild(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, generation *providerGeneration, span trace.Span) (*bridgeEntry, func(), error) {
	cacheKey := bridgeCacheKey(generation.id, req)
	if entry, ok := p.cache.Get(cacheKey); ok && entry != nil && !entry.retired.Load() {
		// Cache entries expire even when they remain active. This periodically
		// refreshes MCP connections until token expiry can be detected directly.
		span.AddEvent("cache_hit")
		return entry, func() {}, nil
	}
	span.AddEvent("cache_miss")

	return p.builds.Do(cacheKey, func() (*bridgeEntry, func(), error) {
		bridge, err := p.buildBridge(ctx, req, clientFn, mcpProxyFactory, generation.providers)
		if err != nil {
			return nil, nil, err
		}
		entry := &bridgeEntry{key: cacheKey, generation: generation, bridge: bridge}

		// Trap point for deterministic generation publication tests.
		_ = p.clock.Now("bridge_cache_publish")

		if !generation.publish(p.cache, entry, p.options.TTL) {
			// A dropped SetWithTTL does not invoke OnReject. Register the entry
			// explicitly so provider replacement can retire it while the callers
			// that joined this build are serving through it.
			entry.uncached = true
			if !generation.register(entry) {
				entry.retire(p)
				return nil, nil, errGenerationRetired
			}
			return entry, func() { entry.retire(p) }, nil
		}

		// Make publication visible before releasing callers that joined the
		// build. Policy rejection invokes OnReject followed by OnExit. OnExit
		// leaves rejected entries alive so those callers can share the bridge.
		p.cache.Wait()
		if entry.cacheRejected.Load() {
			entry.uncached = true
			if generation.isRetired() {
				entry.retire(p)
				return nil, nil, errGenerationRetired
			}
			return entry, func() { entry.retire(p) }, nil
		}
		if entry.retired.Load() && generation.isRetired() {
			return nil, nil, errGenerationRetired
		}
		return entry, nil, nil
	})
}

func (p *CachedBridgePool) recordRetry(span trace.Span, reason string, attempt int) {
	span.AddEvent(reason+"_retry", trace.WithAttributes(attribute.Int("attempt", attempt)))
	if p.metrics != nil {
		p.metrics.BridgePoolRetries.WithLabelValues(reason).Inc()
	}
}

func (p *CachedBridgePool) retryExhausted(span trace.Span, reason string) error {
	span.AddEvent("retry_exhausted", trace.WithAttributes(attribute.String("reason", reason)))
	if p.metrics != nil {
		p.metrics.BridgePoolRetryExhausted.WithLabelValues(reason).Inc()
	}
	return xerrors.Errorf("%w: %s", errBridgeServeRetriesExhausted, reason)
}

func (p *CachedBridgePool) recordUncachedServe(ctx context.Context, span trace.Span, generation uint64) {
	span.AddEvent("uncached_serve")
	if p.metrics != nil {
		p.metrics.BridgePoolUncachedServeAttempts.Inc()
	}

	now := p.clock.Now("uncached_serve_warning").UnixNano()
	for {
		previous := p.uncachedServeWarningAt.Load()
		if previous != 0 && time.Duration(now-previous) < uncachedServeWarningPeriod {
			return
		}
		if p.uncachedServeWarningAt.CompareAndSwap(previous, now) {
			p.logger.Warn(ctx, "request bridge cache rejected entry; serving uncached",
				slog.F("provider_generation", generation),
			)
			return
		}
	}
}

func (p *CachedBridgePool) buildBridge(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, providers []aibridge.Provider) (*aibridge.RequestBridge, error) {
	recorder := aibridge.NewRecorder(p.logger.Named("recorder"), p.tracer, func(clientCtx context.Context) (aibridge.Recorder, error) {
		// The recorder outlives this Serve call, so the client is acquired
		// against the context of the record call being served.
		client, err := clientFn(clientCtx)
		if err != nil {
			return nil, xerrors.Errorf("acquire client: %w", err)
		}
		return &recorderTranslation{apiKeyID: req.APIKeyID, client: client}, nil
	})

	mcpServers, err := mcpProxyFactory.Build(ctx, req, p.tracer)
	if err != nil {
		// Don't fail here; MCP server injection can gracefully degrade.
		p.logger.Warn(ctx, "failed to create MCP server proxiers", slog.Error(err))
	}
	if mcpServers != nil {
		// This blocks while connections are established with upstream MCP
		// server(s) and tools are listed. A canceled leader still publishes
		// the bridge so callers that joined its build are not stranded.
		if err := mcpServers.Init(ctx); err != nil {
			p.logger.Warn(ctx, "failed to initialize MCP server proxier(s)", slog.Error(err))
		}
	}

	bridge, err := aibridge.NewRequestBridge(ctx, providers, recorder, mcpServers, p.logger, p.metrics, p.tracer, aibridge.WithClock(p.clock))
	if err != nil {
		if mcpServers != nil {
			_ = mcpServers.Shutdown(ctx)
		}
		return nil, xerrors.Errorf("create new request bridge: %w", err)
	}
	return bridge, nil
}

func bridgeCacheKey(generation uint64, req Request) string {
	return strconv.FormatUint(generation, 10) + "|" + req.InitiatorID.String() + "|" + req.APIKeyID
}

func (g *providerGeneration) publish(cache *ristretto.Cache[string, *bridgeEntry], entry *bridgeEntry, ttl time.Duration) bool {
	g.mu.Lock()
	if g.retired {
		g.mu.Unlock()
		return false
	}
	g.entries[entry] = struct{}{}
	g.publishWG.Add(1)
	g.mu.Unlock()

	// SetWithTTL can synchronously invoke OnExit when replacing an existing
	// value. Do not hold g.mu while calling it. publishWG prevents generation
	// retirement from finishing until this publication attempt completes.
	ok := cache.SetWithTTL(entry.key, entry, cacheCost, ttl)
	g.publishWG.Done()
	if ok {
		return true
	}
	g.deregister(entry)
	return false
}

func (g *providerGeneration) register(entry *bridgeEntry) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retired {
		return false
	}
	g.entries[entry] = struct{}{}
	return true
}

func (g *providerGeneration) deregister(entry *bridgeEntry) {
	g.mu.Lock()
	delete(g.entries, entry)
	g.mu.Unlock()
}

func (g *providerGeneration) isRetired() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.retired
}

func (g *providerGeneration) retire(pool *CachedBridgePool) {
	g.mu.Lock()
	if g.retired {
		g.mu.Unlock()
		return
	}
	g.retired = true
	g.mu.Unlock()

	// publish registers with publishWG before releasing g.mu. Once retired is
	// set, no new publication can start, so this Wait cannot race an Add.
	g.publishWG.Wait()

	g.mu.Lock()
	entries := make([]*bridgeEntry, 0, len(g.entries))
	for entry := range g.entries {
		entries = append(entries, entry)
	}
	clear(g.entries)
	g.mu.Unlock()

	for _, entry := range entries {
		entry.retire(pool)
		if entry.key != "" {
			// Del invokes OnExit synchronously. entry.retireOnce makes the
			// converging callback a no-op.
			pool.cache.Del(entry.key)
		}
	}
	pool.cache.Wait()
}

func (entry *bridgeEntry) retire(pool *CachedBridgePool) {
	entry.retireOnce.Do(func() {
		entry.retired.Store(true)
		// Removal matters for eviction and rejection paths. Generation retirement
		// clears the registry before reaching here, so this is then a no-op.
		entry.generation.deregister(entry)
		pool.trackShutdown(entry.bridge)
	})
}

func (p *CachedBridgePool) trackShutdown(bridge *aibridge.RequestBridge) {
	bridge.Retire()
	p.retirementWG.Go(func() {
		// Bound the drain so a retired bridge with a long-lived streaming
		// response cannot hold its MCP session indefinitely. Pool Shutdown
		// still cancels immediately via retirementCtx.
		ctx, cancel := context.WithTimeout(p.retirementCtx, bridgeRetirementTimeout)
		defer cancel()
		_ = bridge.Shutdown(ctx)
	})
}

func (p *CachedBridgePool) isShuttingDown() bool {
	select {
	case <-p.shuttingDownCh:
		return true
	default:
		return false
	}
}

func (p *CachedBridgePool) beginOperation() bool {
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
	select {
	case <-p.shuttingDownCh:
		return false
	default:
		p.cacheWG.Add(1)
		return true
	}
}

func (p *CachedBridgePool) CacheMetrics() PoolMetrics {
	if p.cache == nil {
		return nil
	}
	return p.cache.Metrics
}

// Shutdown prevents new pool operations, retires the current generation, and
// waits for operations and bridge cleanup. Context cancellation accelerates
// retirement by canceling admitted requests.
func (p *CachedBridgePool) Shutdown(ctx context.Context) error {
	var outErr error
	p.shutDownOnce.Do(func() {
		p.cacheMu.Lock()
		close(p.shuttingDownCh)
		p.cacheMu.Unlock()

		// Serialize with replacement so the generation observed here is the last
		// one that can be published. Retire before waiting for Serve operations to
		// close admission and let context cancellation unblock active handlers.
		p.replaceMu.Lock()
		if generation := p.generation.Load(); generation != nil {
			generation.retire(p)
		}
		p.replaceMu.Unlock()

		operationsDone := make(chan struct{})
		go func() {
			p.cacheWG.Wait()
			close(operationsDone)
		}()

		select {
		case <-operationsDone:
		case <-ctx.Done():
			p.retirementCancel()
			<-operationsDone
			outErr = ctx.Err()
		}

		p.cache.Close()

		retirementDone := make(chan struct{})
		go func() {
			p.retirementWG.Wait()
			close(retirementDone)
		}()

		select {
		case <-retirementDone:
			p.retirementCancel()
		case <-ctx.Done():
			p.retirementCancel()
			<-retirementDone
			outErr = ctx.Err()
		}
	})
	return outErr
}
