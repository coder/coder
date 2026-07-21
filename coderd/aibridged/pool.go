package aibridged

import (
	"context"
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
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

const (
	cacheCost = 1 // We can't know the actual size in bytes of the value (it'll change over time).
)

// Pooler describes a pool of [*aibridge.RequestBridge] instances from which instances can be retrieved.
// One [*aibridge.RequestBridge] instance is created per given key.
type Pooler interface {
	Acquire(ctx context.Context, req Request, clientFn ClientFunc, mcpBootstrapper MCPProxyBuilder) (http.Handler, error)
	// ReplaceProviders swaps the providers used to construct future
	// RequestBridge instances and clears the cache. Disabled providers
	// must be included; the bridge serves a 503 sentinel on their
	// routes.
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

type CachedBridgePool struct {
	cache *ristretto.Cache[string, *aibridge.RequestBridge]
	clock quartz.Clock
	// providers is the live provider set used by new RequestBridge
	// instances. Includes disabled providers.
	providers       atomic.Pointer[[]aibridge.Provider]
	providerVersion atomic.Int64
	logger          slog.Logger
	options         PoolOptions

	singleflight *singleflight.Group[string, *aibridge.RequestBridge]

	metrics *aibridge.Metrics
	tracer  trace.Tracer

	shutDownOnce   sync.Once
	shuttingDownCh chan struct{}

	// cacheMu + cacheWG order cache use against Shutdown. Without it,
	// (*ristretto.Cache).Close may race against cache usage.
	cacheMu sync.RWMutex
	cacheWG sync.WaitGroup
}

func NewCachedBridgePool(options PoolOptions, providers []aibridge.Provider, logger slog.Logger, metrics *aibridge.Metrics, tracer trace.Tracer) (*CachedBridgePool, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, *aibridge.RequestBridge]{
		NumCounters:        options.MaxItems * 10,        // Docs suggest setting this 10x number of keys.
		MaxCost:            options.MaxItems * cacheCost, // Up to n instances.
		IgnoreInternalCost: true,                         // Don't try estimate cost using bytes (ristretto does this naïvely anyway, just using the size of the value struct not the REAL memory usage).
		BufferItems:        64,                           // Sticking with recommendation from docs.
		Metrics:            true,                         // Collect metrics (only used in tests, for now).
		OnEvict: func(item *ristretto.Item[*aibridge.RequestBridge]) {
			if item == nil || item.Value == nil {
				return
			}
			// Capture the value synchronously: ristretto reuses the
			// item slot after OnEvict returns, so reading item.Value
			// from the goroutine below races with the caller of
			// Clear/Set. The shutdown still runs in the background to
			// avoid blocking ristretto's eviction loop.
			bridge := item.Value
			go func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				_ = bridge.Shutdown(shutdownCtx)
			}()
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("create cache: %w", err)
	}

	clk := options.Clock
	if clk == nil {
		clk = quartz.NewReal()
	}

	pool := &CachedBridgePool{
		cache:   cache,
		clock:   clk,
		options: options,
		metrics: metrics,
		tracer:  tracer,
		logger:  logger,

		singleflight: &singleflight.Group[string, *aibridge.RequestBridge]{},

		shuttingDownCh: make(chan struct{}),
	}
	initial := slices.Clone(providers)
	pool.providers.Store(&initial)
	return pool, nil
}

// ReplaceProviders swaps the provider snapshot used by future Acquires.
// It is safe to call concurrently with Acquire and is a no-op after
// Shutdown.
func (p *CachedBridgePool) ReplaceProviders(providers []aibridge.Provider) {
	p.cacheMu.RLock()
	select {
	case <-p.shuttingDownCh:
		p.cacheMu.RUnlock()
		return
	default:
	}
	p.cacheWG.Add(1)
	p.cacheMu.RUnlock()
	defer p.cacheWG.Done()

	snapshot := slices.Clone(providers)
	p.providers.Store(&snapshot)
	version := p.clock.Now("provider_reload_version").UnixNano()
	p.providerVersion.Store(version)
	// Clear evicts every cached bridge; OnEvict shuts each one down in
	// the background. Wait for buffered writes to drain so a replacement
	// immediately followed by an Acquire always sees the cleared cache.
	p.cache.Clear()
	p.cache.Wait()
	p.logger.Info(context.Background(), "request bridge pool reloaded",
		slog.F("provider_count", len(snapshot)),
		slog.F("provider_version", version),
	)
}

// loadProviders returns the current providers snapshot. The returned
// slice must not be mutated.
func (p *CachedBridgePool) loadProviders() []aibridge.Provider {
	if ptr := p.providers.Load(); ptr != nil {
		return *ptr
	}
	return nil
}

// KeyPools returns the key pools of the current live providers.
func (p *CachedBridgePool) KeyPools() []*keypool.Pool {
	providers := p.loadProviders()
	pools := make([]*keypool.Pool, 0, len(providers))
	for _, prov := range providers {
		if pool := prov.KeyPool(); pool != nil {
			pools = append(pools, pool)
		}
	}
	return pools
}

// Acquire retrieves or creates a [*aibridge.RequestBridge] instance per given key.
//
// Each returned [*aibridge.RequestBridge] is safe for concurrent use.
// Each [*aibridge.RequestBridge] is stateful because it has MCP clients which maintain sessions to the configured MCP server.
func (p *CachedBridgePool) Acquire(ctx context.Context, req Request, clientFn ClientFunc, mcpProxyFactory MCPProxyBuilder) (_ http.Handler, outErr error) {
	spanAttrs := []attribute.KeyValue{
		attribute.String(tracing.InitiatorID, req.InitiatorID.String()),
		attribute.String(tracing.APIKeyID, req.APIKeyID),
	}
	ctx, span := p.tracer.Start(ctx, "CachedBridgePool.Acquire", trace.WithAttributes(spanAttrs...))
	defer tracing.EndSpanErr(span, &outErr)
	ctx = tracing.WithRequestBridgeAttributesInContext(ctx, spanAttrs)

	if err := ctx.Err(); err != nil {
		return nil, xerrors.Errorf("acquire: %w", err)
	}

	p.cacheMu.RLock()
	select {
	case <-p.shuttingDownCh:
		p.cacheMu.RUnlock()
		return nil, xerrors.New("pool shutting down")
	default:
	}
	p.cacheWG.Add(1)
	p.cacheMu.RUnlock()
	defer p.cacheWG.Done()

	// Wait for all buffered writes to be applied, otherwise multiple calls in quick succession
	// may visit the slow path unnecessarily.
	defer p.cache.Wait()

	// Fast path.
	cacheKey := req.InitiatorID.String() + "|" + req.APIKeyID
	bridge, ok := p.cache.Get(cacheKey)
	if ok && bridge != nil {
		// TODO: future improvement:
		// Once we can detect token expiry against an MCP server, we no longer need to let these instances
		// expire after the original TTL; we can extend the TTL on each Acquire() call.
		// For now, we need to let the instance expiry to keep the MCP connections fresh.

		span.AddEvent("cache_hit")
		return bridge, nil
	}

	span.AddEvent("cache_miss")
	providerVersion := p.providerVersion.Load()
	recorder := aibridge.NewRecorder(p.logger.Named("recorder"), p.tracer, func() (aibridge.Recorder, error) {
		client, err := clientFn()
		if err != nil {
			return nil, xerrors.Errorf("acquire client: %w", err)
		}

		return &recorderTranslation{apiKeyID: req.APIKeyID, client: client}, nil
	})

	// Slow path.
	// Creating an *aibridge.RequestBridge may take some time, so gate all subsequent callers behind the initial request and return the resulting value.
	// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
	singleflightKey := cacheKey + "|" + strconv.FormatInt(providerVersion, 10)
	instance, err, _ := p.singleflight.Do(singleflightKey, func() (*aibridge.RequestBridge, error) {
		var (
			mcpServers mcp.ServerProxier
			err        error
		)

		mcpServers, err = mcpProxyFactory.Build(ctx, req, p.tracer)
		if err != nil {
			p.logger.Warn(ctx, "failed to create MCP server proxiers", slog.Error(err))
			// Don't fail here; MCP server injection can gracefully degrade.
		}

		if mcpServers != nil {
			// This will block while connections are established with upstream MCP server(s), and tools are listed.
			if err := mcpServers.Init(ctx); err != nil {
				p.logger.Warn(ctx, "failed to initialize MCP server proxier(s)", slog.Error(err))
			}
		}

		bridge, err := aibridge.NewRequestBridge(ctx, p.loadProviders(), recorder, mcpServers, p.logger, p.metrics, p.tracer, aibridge.WithClock(p.clock))
		if err != nil {
			return nil, xerrors.Errorf("create new request bridge: %w", err)
		}

		if p.providerVersion.Load() == providerVersion {
			p.cache.SetWithTTL(cacheKey, bridge, cacheCost, p.options.TTL)
		}

		return bridge, nil
	})

	return instance, err
}

func (p *CachedBridgePool) CacheMetrics() PoolMetrics {
	if p.cache == nil {
		return nil
	}

	return p.cache.Metrics
}

// Shutdown will close the cache which will trigger eviction of all the Bridge entries.
func (p *CachedBridgePool) Shutdown(_ context.Context) error {
	p.shutDownOnce.Do(func() {
		// Block new cache use, drain in-flight ops, then close (see cacheMu).
		p.cacheMu.Lock()
		close(p.shuttingDownCh)
		p.cacheMu.Unlock()

		p.cacheWG.Wait()

		p.cache.Close()
	})

	return nil
}
