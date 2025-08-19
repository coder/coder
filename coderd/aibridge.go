package coderd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"

	"github.com/ammario/tlru"
	"github.com/google/uuid"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
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

	// Inject the initiator's scope into the scope.
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, initiatorID, rbac.ScopeAll)
	if err != nil {
		api.Logger.Error(ctx, "failed to setup user RBAC context", slog.Error(err), slog.F("userID", initiatorID))
		http.Error(rw, "internal server error", http.StatusInternalServerError) // Don't leak reason as this might have security implications.
		return
	}

	ctx = dbauthz.As(ctx, actor)

	bridge, err := api.AIBridgeManager.acquire(ctx, api, sessionKey, initiatorID, server.Client)
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
	cache               *tlru.Cache[string, *aibridge.Bridge]
	providers           []aibridge.Provider
	store               store
	externalAuthConfigs []*externalauth.Config

	singleflight *singleflight.Group[string, *aibridge.Bridge]
}

// store is a minimal database interface.
type store interface {
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)
}

func NewAIBridgeManager(cfg codersdk.AIBridgeConfig, instances int, store store, externalAuthConfigs []*externalauth.Config) *AIBridgeManager {
	return &AIBridgeManager{
		cache: tlru.New[string](tlru.ConstantCost[*aibridge.Bridge], instances),
		providers: []aibridge.Provider{
			aibridge.NewOpenAIProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String()),
			aibridge.NewAnthropicProvider(cfg.Anthropic.BaseURL.String(), cfg.Anthropic.Key.String()),
		},
		store:               store,
		externalAuthConfigs: externalAuthConfigs,
		singleflight:        &singleflight.Group[string, *aibridge.Bridge]{},
	}
}

func (m *AIBridgeManager) fetchTools(ctx context.Context, logger slog.Logger, accessURL, key string, userID uuid.UUID) ([]*aibridge.MCPServerProxy, error) {
	url, err := url.JoinPath(accessURL, "/api/experimental/mcp/http")
	if err != nil {
		return nil, xerrors.Errorf("failed to build coder MCP url: %w", err)
	}

	var proxies []*aibridge.MCPServerProxy

	coderMCP, err := aibridge.NewMCPServerProxy("coder", url, map[string]string{
		"Authorization": "Bearer " + key,
	}, logger.Named("mcp-bridge-coder"))
	if err != nil {
		return nil, xerrors.Errorf("coder MCP bridge setup: %w", err)
	}
	proxies = append(proxies, coderMCP)

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

	externalAuthLinks, err := m.store.GetExternalAuthLinksByUserID(ctx, userID)
	if err == nil {
		for _, link := range externalAuthLinks {
			eg.Go(func() error {
				var externalAuthConfig *externalauth.Config
				for _, eac := range m.externalAuthConfigs {
					if eac.ID == link.ProviderID {
						externalAuthConfig = eac
					}
				}

				if externalAuthConfig == nil {
					logger.Warn(ctx, "failed to find external auth config matching known external auth link", slog.F("id", link.ProviderID))
					return nil
				}

				valid, _, err := externalAuthConfig.ValidateToken(ctx, link.OAuthToken())
				if !valid {
					if strings.TrimSpace(link.OAuthRefreshToken) != "" {
						// TODO: refresh token.
						return xerrors.Errorf("%q token is not valid and cannot be refreshed currently: %w", link.ProviderID, err)
					}
					return xerrors.Errorf("%s external auth token invalid: %w", link.ProviderID, err)
				}

				linkBridge, err := aibridge.NewMCPServerProxy(link.ProviderID, externalAuthConfig.MCPURL, map[string]string{
					"Authorization": fmt.Sprintf("Bearer %s", link.OAuthAccessToken),
				}, logger.Named(fmt.Sprintf("mcp-bridge-%s", link.ProviderID)))
				if err != nil {
					return xerrors.Errorf("%s MCP bridge setup: %w", link.ProviderID, err)
				}
				proxies = append(proxies, linkBridge)

				ctx, cancel := context.WithTimeout(ctx, time.Second*30)
				defer cancel()

				err = linkBridge.Init(ctx)
				if err == nil {
					return nil
				}
				return xerrors.Errorf("%s MCP init: %w", link.ProviderID, err)
			})
		}
	} else {
		// Degrade gracefully, let request proceed.
		logger.Error(ctx, "failed to load external auth links", slog.Error(err), slog.F("userID", userID))
	}

	// This MUST block requests until all MCP proxies are setup.
	if err := eg.Wait(); err != nil {
		return nil, xerrors.Errorf("MCP proxy init: %w", err)
	}

	return proxies, nil
}

// acquire retrieves or creates a Bridge instance per given session key; Bridge is safe for concurrent use.
// Each Bridge is stateful in that it has MCP tools which are scoped to the initiator.
func (m *AIBridgeManager) acquire(ctx context.Context, api *API, sessionKey string, initiatorID uuid.UUID, apiClientFn func() (proto.DRPCRecorderClient, error)) (*aibridge.Bridge, error) {
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
	instance, err, _ := m.singleflight.Do(sessionKey, func() (*aibridge.Bridge, error) {
		// TODO: track startup time since it adds latency to first request (histogram count will also help us see how often this occurs).
		tools, err := m.fetchTools(ctx, logger, api.DeploymentValues.AccessURL.String(), sessionKey, initiatorID)
		if err != nil {
			logger.Warn(ctx, "failed to load tools", slog.Error(err))
		}

		bridge, err = aibridge.NewBridge(m.providers, logger, func() (aibridge.RecorderClient, error) {
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
		logger.Debug(ctx, "created new bridge", slog.F("ptr", fmt.Sprintf("%p", bridge)))

		m.cache.Set(sessionKey, bridge, bridgeCacheTTL)
		return bridge, nil
	})

	return instance, err
}

var _ aibridge.RecorderClient = &translator{}

// translator satisfies the aibridge.RecorderClient interface and translates calls into dRPC calls to aibridgedserver.
type translator struct {
	client proto.DRPCRecorderClient
}

func (t *translator) RecordSession(ctx context.Context, req *aibridge.SessionRequest) error {
	_, err := t.client.RecordSession(ctx, &proto.RecordSessionRequest{
		SessionId:   req.SessionID,
		InitiatorId: req.InitiatorID,
		Provider:    req.Provider,
		Model:       req.Model,
	})
	return err
}

func (t *translator) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRequest) error {
	_, err := t.client.RecordPromptUsage(ctx, &proto.RecordPromptUsageRequest{
		SessionId: req.SessionID,
		MsgId:     req.MsgID,
		Prompt:    req.Prompt,
		Metadata:  MarshalForProto(req.Metadata),
	})
	return err
}

func (t *translator) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRequest) error {
	_, err := t.client.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		SessionId:    req.SessionID,
		MsgId:        req.MsgID,
		InputTokens:  req.Input,
		OutputTokens: req.Output,
		Metadata:     MarshalForProto(req.Metadata),
	})
	return err
}

func (t *translator) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRequest) error {
	serialized, err := json.Marshal(req.Args)
	if err != nil {
		return xerrors.Errorf("serialize tool %q args: %w", req.Name, err)
	}

	_, err = t.client.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		SessionId: req.SessionID,
		MsgId:     req.MsgID,
		Tool:      req.Name,
		Input:     string(serialized),
		Injected:  req.Injected,
		Metadata:  MarshalForProto(req.Metadata),
	})
	return err
}

func MarshalForProto(in aibridge.Metadata) map[string]*anypb.Any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*anypb.Any, len(in))
	for k, v := range in {
		if sv, err := structpb.NewValue(v); err == nil {
			if av, err := anypb.New(sv); err == nil {
				out[k] = av
			}
		}
	}
	return out
}
