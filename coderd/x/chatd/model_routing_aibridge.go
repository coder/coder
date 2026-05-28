package chatd

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const (
	aibridgeLocalBaseURL = "http://coder-aibridge"
	// aibridgePlaceholderAPIKey satisfies fantasy clients that require a
	// non-empty API key before aibridged resolves the real credential.
	aibridgePlaceholderAPIKey   = "coder-aibridge"
	aibridgeDelegatedBYOKMarker = "delegated"
)

// Synthetic quickgen calls are still routed through AI Bridge, but they should
// not become promptless root cards in the user's chat session timeline.
type suppressAIBridgeSessionHeadersKey struct{}

func contextWithoutAIBridgeSessionHeaders(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressAIBridgeSessionHeadersKey{}, true)
}

func suppressAIBridgeSessionHeadersFromContext(ctx context.Context) bool {
	suppress, _ := ctx.Value(suppressAIBridgeSessionHeadersKey{}).(bool)
	return suppress
}

type aiGatewayModelRoute struct {
	Provider          database.AIProvider
	ModelProviderHint string
	ProviderAuth      aiGatewayProviderAuth
}

func newAIGatewayModelRoute(
	provider database.AIProvider,
	modelProviderHint string,
	auth aiGatewayProviderAuth,
) resolvedModelRoute {
	return resolvedModelRoute{
		kind: modelRouteKindAIGateway,
		aiGateway: aiGatewayModelRoute{
			Provider:          provider,
			ModelProviderHint: modelProviderHint,
			ProviderAuth:      auth,
		},
	}
}

type aiGatewayProviderAuth struct {
	Headers map[string]string
}

func (aiGatewayProviderAuth) String() string {
	return "aiGatewayProviderAuth{Headers:<redacted>}"
}

func (a aiGatewayProviderAuth) GoString() string {
	return a.String()
}

type aiGatewayRequestFormat int

const (
	aiGatewayRequestFormatOpenAI aiGatewayRequestFormat = iota
	aiGatewayRequestFormatAnthropic
)

type aiGatewayRoundTripper struct {
	base         http.RoundTripper
	apiKeyID     string
	providerAuth aiGatewayProviderAuth
}

func (t *aiGatewayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := aibridge.WithDelegatedAPIKeyID(req.Context(), t.apiKeyID)
	cloned := req.Clone(ctx)
	if suppressAIBridgeSessionHeadersFromContext(req.Context()) {
		cloned.Header.Del(chatprovider.HeaderCoderChatID)
		cloned.Header.Del(chatprovider.HeaderCoderSubchatID)
	}
	for name, value := range t.providerAuth.Headers {
		cloned.Header.Set(name, value)
	}
	if len(t.providerAuth.Headers) > 0 {
		cloned.Header.Set(aibridge.HeaderCoderToken, aibridgeDelegatedBYOKMarker)
	}
	return t.base.RoundTrip(cloned)
}

func (p *Server) newAIGatewayModel(
	_ context.Context,
	req modelClientRequest,
	route aiGatewayModelRoute,
	opts modelBuildOptions,
) (fantasy.LanguageModel, error) {
	if route.Provider.ID == uuid.Nil {
		return nil, xerrors.New("AI Gateway routing requires a concrete AI provider")
	}
	if route.Provider.Name == "" {
		return nil, xerrors.New("AI Gateway routing requires an AI provider name")
	}
	if opts.ActiveAPIKeyID == "" {
		return nil, xerrors.New("AI Gateway routing requires the active turn API key ID")
	}

	factoryPtr := p.aibridgeTransportFactory
	if factoryPtr == nil {
		return nil, xerrors.New("AI Gateway transport factory is not configured")
	}
	factory := factoryPtr.Load()
	if factory == nil || *factory == nil {
		return nil, xerrors.New("AI Gateway transport factory is not configured")
	}
	rt, err := (*factory).TransportFor(route.Provider.Name, aibridge.SourceAgents)
	if err != nil {
		return nil, xerrors.Errorf("create AI Gateway transport: %w", err)
	}
	baseRT := http.RoundTripper(&aiGatewayRoundTripper{
		base:         rt,
		apiKeyID:     opts.ActiveAPIKeyID,
		providerAuth: route.ProviderAuth,
	})
	if opts.RecordHTTP {
		baseRT = &chatdebug.RecordingTransport{Base: baseRT}
	}

	config := fantasyConfigForAIBridge(route.Provider.Type)
	return newLanguageModel(
		config.ProviderHint,
		req.ModelName,
		config.Keys,
		req.UserAgent,
		req.ExtraHeaders,
		&http.Client{Transport: baseRT},
	)
}

type aibridgeFantasyConfig struct {
	ProviderHint string
	Keys         chatprovider.ProviderAPIKeys
}

func fantasyConfigForAIBridge(providerType database.AIProviderType) aibridgeFantasyConfig {
	var fantasyProvider string
	baseURL := aibridgeLocalBaseURL + "/v1"
	switch providerType {
	case database.AiProviderTypeAnthropic, database.AiProviderTypeBedrock:
		fantasyProvider = fantasyanthropic.Name
		baseURL = aibridgeLocalBaseURL
	case database.AiProviderTypeOpenai:
		fantasyProvider = fantasyopenai.Name
	default:
		fantasyProvider = fantasyopenaicompat.Name
	}
	return aibridgeFantasyConfig{
		ProviderHint: fantasyProvider,
		Keys: chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				fantasyProvider: aibridgePlaceholderAPIKey,
			},
			BaseURLByProvider: map[string]string{
				fantasyProvider: baseURL,
			},
		},
	}
}

func aiGatewayRequestFormatForProviderType(providerType database.AIProviderType) aiGatewayRequestFormat {
	switch providerType {
	case database.AiProviderTypeAnthropic, database.AiProviderTypeBedrock:
		return aiGatewayRequestFormatAnthropic
	default:
		return aiGatewayRequestFormatOpenAI
	}
}

func (p *Server) aiGatewayProviderAuthForUser(
	ctx context.Context,
	ownerID uuid.UUID,
	provider database.AIProvider,
	format aiGatewayRequestFormat,
) (aiGatewayProviderAuth, error) {
	if !p.allowBYOK {
		return aiGatewayProviderAuth{}, nil
	}
	userKey, err := p.db.GetUserAIProviderKeyByProviderID(ctx, database.GetUserAIProviderKeyByProviderIDParams{
		UserID:       ownerID,
		AIProviderID: provider.ID,
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return aiGatewayProviderAuth{}, nil
		}
		return aiGatewayProviderAuth{}, xerrors.Errorf("get user AI provider key: %w", err)
	}
	apiKey := strings.TrimSpace(userKey.APIKey)
	if apiKey == "" {
		return aiGatewayProviderAuth{}, nil
	}

	headers := map[string]string{}
	switch format {
	case aiGatewayRequestFormatAnthropic:
		headers["X-Api-Key"] = apiKey
	default:
		headers["Authorization"] = "Bearer " + apiKey
	}
	return aiGatewayProviderAuth{Headers: headers}, nil
}

func (p *Server) resolveAIGatewayRoute(
	ctx context.Context,
	ownerID uuid.UUID,
	provider database.AIProvider,
	modelProviderHint string,
) (resolvedModelRoute, error) {
	auth, err := p.aiGatewayProviderAuthForUser(
		ctx,
		ownerID,
		provider,
		aiGatewayRequestFormatForProviderType(provider.Type),
	)
	if err != nil {
		return resolvedModelRoute{}, xerrors.Errorf("resolve AI Gateway provider auth: %w", err)
	}
	return newAIGatewayModelRoute(provider, modelProviderHint, auth), nil
}

func (p *Server) resolveAIGatewayModelRouteForConfig(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
) (resolvedModelRoute, error) {
	provider, err := p.gatewayProviderForConfig(ctx, modelConfig)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	return p.resolveAIGatewayRoute(ctx, ownerID, provider, string(provider.Type))
}

func (p *Server) resolveAIGatewayModelRouteForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (resolvedModelRoute, error) {
	provider, err := p.aiProviderForProviderType(ctx, providerType)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	return p.resolveAIGatewayRoute(
		ctx,
		ownerID,
		provider,
		chatprovider.NormalizeProvider(providerType),
	)
}

func (p *Server) gatewayProviderForConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (database.AIProvider, error) {
	if !modelConfig.AIProviderID.Valid {
		return database.AIProvider{}, xerrors.Errorf(
			"AI Gateway routing requires AI provider metadata for model config %s (%s)",
			modelConfig.ID,
			modelConfig.Model,
		)
	}
	return p.enabledAIProviderByID(ctx, modelConfig.AIProviderID.UUID)
}

func (p *Server) aiProviderForProviderType(
	ctx context.Context,
	providerType string,
) (database.AIProvider, error) {
	providers, err := p.db.GetAIProviders(ctx, database.GetAIProvidersParams{})
	if err != nil {
		return database.AIProvider{}, xerrors.Errorf("get enabled AI providers: %w", err)
	}
	normalizedProviderType := chatprovider.NormalizeProvider(providerType)
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		if chatprovider.NormalizeProvider(string(provider.Type)) != normalizedProviderType {
			continue
		}
		return provider, nil
	}
	return database.AIProvider{}, xerrors.Errorf(
		"AI Gateway routing requires a usable AI provider for provider type %q",
		providerType,
	)
}
