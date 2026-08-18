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
	"tailscale.com/util/singleflight"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

const cacheCost = 1

var (
	errGenerationRetired = xerrors.New("provider generation retired")
	errCacheRejected     = xerrors.New("request bridge rejected by cache")
)

// Pooler describes a pool of [*aibridge.RequestBridge] instances.
type Pooler interface {
	Serve(ctx context.Context, req Request, clientFn ClientFunc, mcpBootstrapper MCPProxyBuilder, rw http.ResponseWriter, r *http.Request) error
	// ReplaceProviders swaps the providers used to construct future
	// RequestBridge instances. Disabled providers must be included; the bridge
	// serves a 503 sentinel on their routes.
	ReplaceProviders(providers []aibridge.Provider)
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

	mu        sync.Mutex
	retired   bool
	entries   map[*bridgeEntry]struct{}
	publishWG sync.WaitGroup
}

type bridgeEntry struct {
	key        string
	generation *providerGeneration
	bridge     *aibridge.RequestBridge
	retired    atomic.Bool
	retireOnce sync.Once
}

type CachedBridgePool struct {
	cache *ristretto.Cache[string, *bridgeEntry]
	clock quartz.Clock

	generation atomic.Pointer[providerGeneration]
	replaceMu  sync.Mutex

	logger  slog.Logger
	options PoolOptions

	singleflight *singleflight.Group[string, *bridgeEntry]

	metrics *aibridge.Metrics
	tracer  trace.Tracer

	shutDownOnce   sync.Once
	shuttingDownCh chan struct{}

	// cacheMu and cacheWG order complete pool operations against Shutdown.
	// This prevents cache use and retirement registration after cache.Close.
	cacheMu sync.RWMutex
	cacheWG sync.WaitGroup

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

		singleflight: &singleflight.Group[string, *bridgeEntry]{},

		shuttingDownCh: make(chan struct{}),

		retirementCtx:    retirementCtx,
		retirementCancel: retirementCancel,
	}

	cache, err := ristretto.NewCache(&ristretto.Config[string, *bridgeEntry]{
		NumCounters:        options.MaxItems * 10,
		MaxCost:            options.MaxItems * cacheCost,
		IgnoreInternalCost: true,
		BufferItems:        64,
		Metrics:            true,
		OnExit: func(entry *bridgeEntry) {
			if entry != nil {
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
	ctx, span := p.tracer.Start(ctx, "CachedBridgePool.Acquire", trace.WithAttributes(spanAttrs...))
	defer tracing.EndSpanErr(span, &outErr)
	ctx = tracing.WithRequestBridgeAttributesInContext(ctx, spanAttrs)

	if err := ctx.Err(); err != nil {
		return xerrors.Errorf("acquire: %w", err)
	}
	if !p.beginOperation() {
		return xerrors.New("pool shutting down")
	}
	defer p.cacheWG.Done()

	operationCtx, cancelOperation := context.WithCancel(ctx)
	stopRetirementCancel := context.AfterFunc(p.retirementCtx, cancelOperation)
	defer func() {
		stopRetirementCancel()
		cancelOperation()
	}()
	ctx = operationCtx

	for {
		if err := ctx.Err(); err != nil {
			return xerrors.Errorf("acquire: %w", err)
		}
		if p.isShuttingDown() {
			return xerrors.New("pool shutting down")
		}

		generation := p.generation.Load()
		if generation == nil {
			return xerrors.New("no provider generation")
		}

		entry, err := p.getOrBuild(ctx, req, clientFn, mcpProxyFactory, generation, span)
		switch {
		case err == nil:
			if entry.bridge.TryServe(rw, r) {
				return nil
			}
			span.AddEvent("admission_retry")
			continue
		case errors.Is(err, errGenerationRetired):
			span.AddEvent("generation_retry")
			continue
		case errors.Is(err, errCacheRejected):
			err = p.serveUncached(ctx, req, clientFn, mcpProxyFactory, generation, rw, r)
			if errors.Is(err, errGenerationRetired) {
				span.AddEvent("generation_retry")
				continue
			}
			return err
		default:
			return err
		}
	}
}

func (p *CachedBridgePool) getOrBuild(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, generation *providerGeneration, span trace.Span) (*bridgeEntry, error) {
	cacheKey := bridgeCacheKey(generation.id, req)
	if entry, ok := p.cache.Get(cacheKey); ok && entry != nil && !entry.retired.Load() {
		span.AddEvent("cache_hit")
		return entry, nil
	}
	span.AddEvent("cache_miss")

	entry, err, _ := p.singleflight.Do(cacheKey, func() (*bridgeEntry, error) {
		bridge, err := p.buildBridge(ctx, req, clientFn, mcpProxyFactory, generation.providers)
		if err != nil {
			return nil, err
		}
		entry := &bridgeEntry{key: cacheKey, generation: generation, bridge: bridge}

		// Trap point for deterministic generation publication tests.
		_ = p.clock.Now("bridge_cache_publish")

		if !generation.publish(p.cache, entry, p.options.TTL) {
			entry.retire(p)
			if generation.isRetired() {
				return nil, errGenerationRetired
			}
			return nil, errCacheRejected
		}

		// Make publication visible before singleflight releases its waiters.
		// Policy rejection invokes OnExit, which marks the entry retired.
		p.cache.Wait()
		if entry.retired.Load() {
			return nil, errCacheRejected
		}
		return entry, nil
	})
	return entry, err
}

func (p *CachedBridgePool) serveUncached(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, generation *providerGeneration, rw http.ResponseWriter, r *http.Request) error {
	bridge, err := p.buildBridge(ctx, req, clientFn, mcpProxyFactory, generation.providers)
	if err != nil {
		return err
	}
	entry := &bridgeEntry{generation: generation, bridge: bridge}
	if !generation.register(entry) {
		entry.retire(p)
		return errGenerationRetired
	}
	defer entry.retire(p)

	if !bridge.TryServe(rw, r) {
		return errGenerationRetired
	}
	return nil
}

func (p *CachedBridgePool) buildBridge(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder, providers []aibridge.Provider) (*aibridge.RequestBridge, error) {
	recorder := aibridge.NewRecorder(p.logger.Named("recorder"), p.tracer, func(clientCtx context.Context) (aibridge.Recorder, error) {
		client, err := clientFn(clientCtx)
		if err != nil {
			return nil, xerrors.Errorf("acquire client: %w", err)
		}
		return &recorderTranslation{apiKeyID: req.APIKeyID, client: client}, nil
	})

	mcpServers, err := mcpProxyFactory.Build(ctx, req, p.tracer)
	if err != nil {
		p.logger.Warn(ctx, "failed to create MCP server proxiers", slog.Error(err))
	}
	if mcpServers != nil {
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
	g.remove(entry)
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

func (g *providerGeneration) remove(entry *bridgeEntry) {
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
		entry.generation.remove(entry)
		pool.trackShutdown(entry.bridge)
	})
}

func (p *CachedBridgePool) trackShutdown(bridge *aibridge.RequestBridge) {
	bridge.Retire()
	p.retirementWG.Add(1)
	go func() {
		defer p.retirementWG.Done()
		_ = bridge.Shutdown(p.retirementCtx)
	}()
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
