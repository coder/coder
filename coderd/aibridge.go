package coderd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"

	"github.com/ammario/tlru"
	"github.com/google/uuid"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	if api.AIBridgeManager == nil {
		http.Error(rw, "not ready", http.StatusBadGateway)
		return
	}

	ctx := r.Context()

	if len(api.AIBridgeDaemons) == 0 {
		http.Error(rw, "no AI bridge daemons running", http.StatusBadGateway)
		return
	}

	// Random loadbalancing.
	// TODO: introduce better strategy.
	server, err := slice.PickRandom(api.AIBridgeDaemons)
	if err != nil {
		api.Logger.Error(ctx, "failed to pick random AI bridge server", slog.Error(err))
		http.Error(rw, "failed to select AI bridge", http.StatusInternalServerError)
		return
	}

	sessionKey, ok := r.Context().Value(aibridged.ContextKeyBridgeAPIKey{}).(string)
	if sessionKey == "" || !ok {
		http.Error(rw, "unable to retrieve request session key", http.StatusBadRequest)
		return
	}

	initiatorID, ok := r.Context().Value(aibridged.ContextKeyBridgeUserID{}).(uuid.UUID)
	if !ok {
		api.Logger.Error(r.Context(), "missing initiator ID in context")
		http.Error(rw, "unable to retrieve initiator", http.StatusBadRequest)
		return
	}

	r.Header.Set(aibridge.InitiatorHeaderKey, initiatorID.String())

	bridge, err := api.AIBridgeManager.acquire(ctx, api, sessionKey, initiatorID.String(), server.Client)
	if err != nil {
		api.Logger.Error(ctx, "failed to acquire aibridge", slog.Error(err))
		http.Error(rw, "failed to acquire aibridge", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/api/v2/aibridge", bridge.Handler()).ServeHTTP(rw, r)
}

const (
	bridgeCacheTTL = time.Minute // TODO: configurable.
)

type AIBridgeManager struct {
	cache     *tlru.Cache[string, *aibridge.Bridge]
	providers []aibridge.Provider

	retrieval *singleflight.Group[string, *aibridge.Bridge]
}

func NewAIBridgeManager(cfg codersdk.AIBridgeConfig, instances int) *AIBridgeManager {
	return &AIBridgeManager{
		cache: tlru.New[string](tlru.ConstantCost[*aibridge.Bridge], instances),
		providers: []aibridge.Provider{
			aibridge.NewOpenAIProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String()),
			aibridge.NewAnthropicProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String()),
		},
		retrieval: &singleflight.Group[string, *aibridge.Bridge]{},
	}
}

// TODO: expand to support more MCP servers.
// TODO: can this move into coder/aibridge?
func (m *AIBridgeManager) fetchTools(ctx context.Context, logger slog.Logger, accessURL, key string) (aibridge.ToolRegistry, error) {
	url, err := url.JoinPath(accessURL, "/api/experimental/mcp/http")
	if err != nil {
		return nil, xerrors.Errorf("failed to build coder MCP url: %w", err)
	}

	coderMCP, err := aibridge.NewMCPToolBridge("coder", url, map[string]string{
		codersdk.SessionTokenHeader: key,
	}, logger.Named("mcp-bridge-coder"))
	if err != nil {
		return nil, xerrors.Errorf("coder MCP bridge setup: %w", err)
	}

	// TODO: add github mcp if external auth is configured.
	var eg errgroup.Group
	eg.Go(func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		err := coderMCP.Init(ctx)
		if err == nil {
			return nil
		}
		return xerrors.Errorf("coder: %w", err)
	})

	// This must block requests until MCP proxies are setup.
	if err := eg.Wait(); err != nil {
		return nil, xerrors.Errorf("MCP proxy init: %w", err)
	}

	return aibridge.ToolRegistry{
		"coder": coderMCP.ListTools(),
	}, nil
}

// acquire retrieves or creates a Bridge instance per given session key; Bridge is safe for concurrent use.
// Each Bridge is stateful in that it has MCP tools which are scoped to the initiator.
func (m *AIBridgeManager) acquire(ctx context.Context, api *API, sessionKey, initiatorID string, apiClientFn func() (aibridge.APIClient, error)) (*aibridge.Bridge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger := api.Logger.Named("aibridge").With(slog.F("initiator_id", initiatorID))

	// Fast path.
	bridge, _, ok := m.cache.Get(sessionKey)
	if ok && bridge != nil {
		// TODO: remove.
		logger.Debug(ctx, "reusing existing bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))
		return bridge, nil
	}

	// Slow path.
	// Creating an *aibridge.Bridge may take some time, so gate all subsequent callers behind the initial request and return the resulting value.
	instance, err, _ := m.retrieval.Do(sessionKey, func() (*aibridge.Bridge, error) {
		// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
		tools, err := m.fetchTools(ctx, logger, api.DeploymentValues.AccessURL.String(), sessionKey)
		if err != nil {
			logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err = aibridge.NewBridge(m.providers, logger, apiClientFn, aibridge.NewInjectedToolManager(tools))
		if err != nil {
			return nil, xerrors.Errorf("create new bridge server: %w", err)
		}
		// TODO: remove.
		logger.Debug(ctx, "created new bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		m.cache.Set(sessionKey, bridge, bridgeCacheTTL)
		return bridge, nil
	})

	return instance, err
}
