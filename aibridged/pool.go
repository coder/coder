package aibridged

import (
	"context"
	"fmt"
	"time"

	"github.com/ammario/tlru"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/aibridge"
)

const (
	bridgeCacheTTL = time.Minute // TODO: configurable. // This is intentionally short right now, should probably be 1hr by default or something.
)

// pooler describes a pool of *aibridge.RequestBridge instances from which instances can be retrieved.
// One *aibridge.RequestBridge instance is created per given key.
type pooler interface {
	Acquire(ctx context.Context, req Request, clientFn func() (aibridge.Recorder, error)) (*aibridge.RequestBridge, error)
}

type CachedBridgePool struct {
	cache     *tlru.Cache[string, *aibridge.RequestBridge]
	providers []aibridge.Provider
	mcpCfg    MCPConfigurator
	logger    slog.Logger

	singleflight *singleflight.Group[string, *aibridge.RequestBridge]
}

func NewCachedBridgePool(cfg codersdk.AIBridgeConfig, instances int, mcpCfg MCPConfigurator, logger slog.Logger) *CachedBridgePool {
	return &CachedBridgePool{
		cache: tlru.New[string](tlru.ConstantCost[*aibridge.RequestBridge], instances),
		providers: []aibridge.Provider{
			aibridge.NewOpenAIProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String()),
			aibridge.NewAnthropicProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String()),
		},
		mcpCfg: mcpCfg,
		logger: logger,

		singleflight: &singleflight.Group[string, *aibridge.RequestBridge]{},
	}
}

// Acquire retrieves or creates a Bridge instance per given key.
//
// Each returned Bridge is safe for concurrent use.
// Each Bridge is stateful because it has MCP clients which maintain sessions to the configured MCP server.
func (m *CachedBridgePool) Acquire(ctx context.Context, req Request, clientFn func() (aibridge.Recorder, error)) (*aibridge.RequestBridge, error) {
	if err := ctx.Err(); err != nil {
		return nil, xerrors.Errorf("acquire: %w", err)
	}

	// Fast path.
	bridge, _, ok := m.cache.Get(req.SessionKey)
	if ok && bridge != nil {
		// TODO: remove.
		m.logger.Debug(ctx, "reusing existing bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))
		return bridge, nil
	}

	// Slow path.
	// Creating an *aibridge.RequestBridge may take some time, so gate all subsequent callers behind the initial request and return the resulting value.
	instance, err, _ := m.singleflight.Do(req.SessionKey, func() (*aibridge.RequestBridge, error) {
		// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
		tools, err := m.fetchTools(ctx, req.SessionKey, req.InitiatorID)
		if err != nil {
			m.logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err = aibridge.NewRequestBridge(m.providers, m.logger, clientFn, aibridge.NewInjectedToolManager(tools))
		if err != nil {
			return nil, xerrors.Errorf("create new request bridge: %w", err)
		}
		// TODO: remove.
		m.logger.Debug(ctx, "created new request bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		m.cache.Set(req.SessionKey, bridge, bridgeCacheTTL)
		return bridge, nil
	})

	return instance, err
}

func (m *CachedBridgePool) fetchTools(ctx context.Context, key string, userID uuid.UUID) ([]*aibridge.MCPServerProxy, error) {
	var (
		proxies []*aibridge.MCPServerProxy
		eg      errgroup.Group
	)

	cfgs, err := m.mcpCfg.GetMCPConfigs(ctx, key, userID)
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

			linkBridge, err := aibridge.NewMCPServerProxy(c.Name, c.URL, map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", c.AccessToken),
			}, m.logger.Named(fmt.Sprintf("mcp-bridge-%s", c.Name)))
			if err != nil {
				return xerrors.Errorf("%s MCP bridge setup: %w", c.Name, err)
			}
			proxies = append(proxies, linkBridge)

			ctx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()

			err = linkBridge.Init(ctx)
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
