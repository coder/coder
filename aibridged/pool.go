package aibridged

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/aibridge"
	"github.com/coder/aibridge/mcp"
)

const (
	bridgeCacheTTL = time.Hour // TODO: configurable.
	cacheCost      = 1         // We can't know the actual size in bytes of the value (it'll change over time).
)

// pooler describes a pool of *aibridge.RequestBridge instances from which instances can be retrieved.
// One *aibridge.RequestBridge instance is created per given key.
type pooler interface {
	Acquire(ctx context.Context, req Request, recorder aibridge.Recorder) (*aibridge.RequestBridge, error)
	Shutdown(ctx context.Context) error
}

type CachedBridgePool struct {
	cache     *ristretto.Cache[string, *aibridge.RequestBridge]
	providers []aibridge.Provider
	mcpCfg    MCPConfigurator
	logger    slog.Logger

	singleflight *singleflight.Group[string, *aibridge.RequestBridge]

	shutDownOnce   sync.Once
	shuttingDownCh chan struct{}
}

func NewCachedBridgePool(cfg codersdk.AIBridgeConfig, instances int64, mcpCfg MCPConfigurator, logger slog.Logger) (*CachedBridgePool, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, *aibridge.RequestBridge]{
		// TODO: the cost seems to actually take into account the size of the object in bytes...? Stop at breakpoint and see.
		NumCounters: instances * 10,        // Docs suggest setting this 10x number of keys.
		MaxCost:     instances * cacheCost, // Up to n instances.
		BufferItems: 64,                    // Sticking with recommendation from docs.
		OnEvict: func(item *ristretto.Item[*aibridge.RequestBridge]) {
			if item == nil || item.Value == nil {
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			if err := item.Value.Shutdown(ctx); err != nil {
				logger.Debug(ctx, "bridge shutdown failed", slog.Error(err))
			}
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("create cache: %w", err)
	}

	return &CachedBridgePool{
		cache: cache,
		providers: []aibridge.Provider{
			aibridge.NewOpenAIProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String()),
			aibridge.NewAnthropicProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String()),
		},
		mcpCfg: mcpCfg,
		logger: logger,

		singleflight: &singleflight.Group[string, *aibridge.RequestBridge]{},

		shuttingDownCh: make(chan struct{}),
	}, nil
}

// Acquire retrieves or creates a [*aibridge.RequestBridge] instance per given key.
//
// Each returned [*aibridge.RequestBridge] is safe for concurrent use.
// Each [*aibridge.RequestBridge] is stateful because it has MCP clients which maintain sessions to the configured MCP server.
func (p *CachedBridgePool) Acquire(ctx context.Context, req Request, recorder aibridge.Recorder) (*aibridge.RequestBridge, error) {
	if err := ctx.Err(); err != nil {
		return nil, xerrors.Errorf("acquire: %w", err)
	}

	select {
	case <-p.shuttingDownCh:
		return nil, xerrors.New("pool shutting down")
	default:
	}

	// Fast path.
	bridge, ok := p.cache.Get(req.InitiatorID.String())
	if ok && bridge != nil {
		// TODO: remove.
		p.logger.Debug(ctx, "reusing existing bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		// Set key again to refresh its TTL.
		//
		// It's possible that two calls can race here, but since they'll both be setting the same value and
		// approximately the same TTL, we don't really care. We could debounce this to prevent unnecessary writes
		// but it'll likely never be an issue.
		p.cache.SetWithTTL(req.InitiatorID.String(), bridge, cacheCost, bridgeCacheTTL)

		return bridge, nil
	}

	// Slow path.
	// Creating an *aibridge.RequestBridge may take some time, so gate all subsequent callers behind the initial request and return the resulting value.
	// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
	instance, err, _ := p.singleflight.Do(req.InitiatorID.String(), func() (*aibridge.RequestBridge, error) {
		proxiers, err := p.setupMCPServerProxiers(ctx, req.SessionKey, req.InitiatorID)
		if err != nil {
			p.logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err = aibridge.NewRequestBridge(ctx, p.providers, p.logger, recorder, mcp.NewServerProxyManager(proxiers))
		if err != nil {
			return nil, xerrors.Errorf("create new request bridge: %w", err)
		}
		// TODO: remove.
		p.logger.Debug(ctx, "created new request bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		p.cache.SetWithTTL(req.InitiatorID.String(), bridge, cacheCost, bridgeCacheTTL)

		return bridge, nil
	})

	return instance, err
}

func (p *CachedBridgePool) setupMCPServerProxiers(ctx context.Context, key string, userID uuid.UUID) ([]mcp.ServerProxier, error) {
	var (
		proxies []mcp.ServerProxier
		eg      errgroup.Group
	)

	cfgs, err := p.mcpCfg.GetMCPConfigs(ctx, key, userID)
	if err != nil {
		return nil, xerrors.Errorf("get mcp configs: %w", err)
	}

	for _, c := range cfgs {
		eg.Go(func() error {
			valid, err := c.ValidateFn(ctx)
			if !valid {
				if c.RefreshFn != nil {
					// TODO: refresh token.
					return xerrors.Errorf("%q token is not valid and cannot be refreshed currently: %w", c.Name, err)
				}
				return xerrors.Errorf("%q external auth token invalid: %w", c.Name, err)
			}

			mcpProxy, err := mcp.NewStreamableHTTPServerProxy(c.Name, c.URL, map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", c.AccessToken),
			}, p.logger.Named(fmt.Sprintf("mcp-bridge-%s", c.Name)))
			if err != nil {
				return xerrors.Errorf("%s MCP bridge setup: %w", c.Name, err)
			}
			proxies = append(proxies, mcpProxy)

			ctx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()

			err = mcpProxy.Init(ctx)
			if err == nil {
				return nil
			}
			return xerrors.Errorf("%s MCP init: %w", c.Name, err)
		})
	}

	if err := eg.Wait(); err != nil {
		// Still return proxies even if there's an error; some is better than none.
		return proxies, xerrors.Errorf("MCP proxy init: %w", err)
	}

	return proxies, nil
}

// Shutdown will close the cache which will trigger eviction of all the Bridge entries.
func (p *CachedBridgePool) Shutdown(_ context.Context) error {
	p.shutDownOnce.Do(func() {
		// Prevent new requests from being served.
		close(p.shuttingDownCh)

		p.cache.Close()
	})

	return nil
}
