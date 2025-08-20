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
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/aibridge"
)

const (
	bridgeCacheTTL = time.Minute // TODO: configurable.
)

type Manager interface {
	Acquire(ctx context.Context, key string, userID uuid.UUID, apiClientFn func() (proto.DRPCRecorderClient, error)) (*aibridge.Bridge, error)
}

type AIBridgeManager struct {
	cache     *tlru.Cache[string, *aibridge.Bridge]
	providers []aibridge.Provider
	mcpCfg    MCPConfigurator
	logger    slog.Logger

	singleflight *singleflight.Group[string, *aibridge.Bridge]
}

func NewAIBridgeManager(cfg codersdk.AIBridgeConfig, instances int, mcpCfg MCPConfigurator, logger slog.Logger) *AIBridgeManager {
	return &AIBridgeManager{
		cache: tlru.New[string](tlru.ConstantCost[*aibridge.Bridge], instances),
		providers: []aibridge.Provider{
			aibridge.NewOpenAIProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String()),
			aibridge.NewAnthropicProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String()),
		},
		mcpCfg: mcpCfg,
		logger: logger,

		singleflight: &singleflight.Group[string, *aibridge.Bridge]{},
	}
}

// Acquire retrieves or creates a Bridge instance per given key.
//
// Each returned Bridge is safe for concurrent use.
// Each Bridge is stateful because it has MCP clients which maintain sessions to the configured MCP server.
func (m *AIBridgeManager) Acquire(ctx context.Context, key string, userID uuid.UUID, apiClientFn func() (proto.DRPCRecorderClient, error)) (*aibridge.Bridge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// TODO: log *obfuscated* key here for tracking?

	// Fast path.
	bridge, _, ok := m.cache.Get(key)
	if ok && bridge != nil {
		// TODO: remove.
		m.logger.Debug(ctx, "reusing existing bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))
		return bridge, nil
	}

	// Slow path.
	// Creating an *aibridge.Bridge may take some time, so gate all subsequent callers behind the initial request and return the resulting value.
	instance, err, _ := m.singleflight.Do(key, func() (*aibridge.Bridge, error) {
		// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
		tools, err := m.fetchTools(ctx, key, userID)
		if err != nil {
			m.logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err = aibridge.NewBridge(m.providers, m.logger, func() (aibridge.RecorderClient, error) {
			client, err := apiClientFn()
			if err != nil {
				return nil, xerrors.Errorf("acquire client: %w", err)
			}

			return &translator{client: client}, nil
		}, aibridge.NewInjectedToolManager(tools))
		if err != nil {
			return nil, xerrors.Errorf("create new bridge server: %w", err)
		}
		// TODO: remove.
		m.logger.Debug(ctx, "created new bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		m.cache.Set(key, bridge, bridgeCacheTTL)
		return bridge, nil
	})

	return instance, err
}

func (m *AIBridgeManager) fetchTools(ctx context.Context, key string, userID uuid.UUID) ([]*aibridge.MCPServerProxy, error) {
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
				return xerrors.Errorf("%s external auth token invalid: %w", c.Name, err)
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

	// This MUST block requests until all MCP proxies are setup.
	if err := eg.Wait(); err != nil {
		return nil, xerrors.Errorf("MCP proxy init: %w", err)
	}

	return proxies, nil
}
