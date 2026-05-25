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
	// non-empty API key. AI Gateway resolves the real provider credential from
	// its central policy unless chatd forwards an explicit user BYOK key.
	aibridgePlaceholderAPIKey = "coder-aibridge"
)

type modelClientRequest struct {
	Chat         database.Chat
	ModelName    string
	UserAgent    string
	ExtraHeaders map[string]string
}

type modelBuildOptions struct {
	ActiveAPIKeyID string
	RecordHTTP     bool
}

func modelBuildOptionsFromMessages(messages []database.ChatMessage) modelBuildOptions {
	apiKeyID, _ := activeTurnAPIKeyIDFromMessages(messages)
	return modelBuildOptions{ActiveAPIKeyID: apiKeyID}
}

type resolvedModelRoute struct {
	Direct    *directModelRoute
	AIGateway *aiGatewayModelRoute
}

type directModelRoute struct {
	ProviderHint string
	Keys         chatprovider.ProviderAPIKeys
}

type aiGatewayModelRoute struct {
	Provider     database.AIProvider
	OriginalHint string
	ProviderAuth aiGatewayProviderAuth
}

type aiGatewayProviderAuth struct {
	Headers              map[string]string
	PreserveProviderAuth bool
}

type aiGatewayRequestFormat int

const (
	aiGatewayRequestFormatOpenAI aiGatewayRequestFormat = iota
	aiGatewayRequestFormatAnthropic
)

func (r resolvedModelRoute) providerHint() (string, error) {
	switch {
	case r.Direct != nil && r.AIGateway != nil:
		return "", xerrors.New("model route cannot be both direct and AI Gateway")
	case r.Direct != nil:
		return r.Direct.ProviderHint, nil
	case r.AIGateway != nil:
		return r.AIGateway.OriginalHint, nil
	default:
		return "", xerrors.New("model route is not configured")
	}
}

func (r resolvedModelRoute) withProviderHint(providerHint string) resolvedModelRoute {
	if r.Direct != nil {
		direct := *r.Direct
		direct.ProviderHint = providerHint
		r.Direct = &direct
	}
	if r.AIGateway != nil {
		aiGateway := *r.AIGateway
		aiGateway.OriginalHint = providerHint
		r.AIGateway = &aiGateway
	}
	return r
}

func (r resolvedModelRoute) directProviderKeys() chatprovider.ProviderAPIKeys {
	if r.Direct == nil {
		return chatprovider.ProviderAPIKeys{}
	}
	return r.Direct.Keys
}

func (r resolvedModelRoute) aiProvider() *database.AIProvider {
	if r.AIGateway == nil {
		return nil
	}
	provider := r.AIGateway.Provider
	return &provider
}

type aiGatewayRoundTripper struct {
	base         http.RoundTripper
	apiKeyID     string
	providerAuth aiGatewayProviderAuth
}

func (t *aiGatewayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := aibridge.WithDelegatedAPIKeyID(req.Context(), t.apiKeyID)
	if t.providerAuth.PreserveProviderAuth {
		ctx = aibridge.WithPreserveProviderAuth(ctx)
	}
	cloned := req.Clone(ctx)
	cloned.Header.Del("Authorization")
	cloned.Header.Del("X-Api-Key")
	for name, value := range t.providerAuth.Headers {
		if value == "" {
			continue
		}
		cloned.Header.Set(name, value)
	}
	return t.base.RoundTrip(cloned)
}

func (p *Server) shouldUseAIGatewayRouting() bool {
	return p.aiGatewayRoutingEnabled
}

func (p *Server) newModel(
	ctx context.Context,
	req modelClientRequest,
	route resolvedModelRoute,
	opts modelBuildOptions,
) (fantasy.LanguageModel, error) {
	switch {
	case route.Direct != nil && route.AIGateway != nil:
		return nil, xerrors.New("model route cannot be both direct and AI Gateway")
	case route.Direct != nil:
		return p.newDirectModel(ctx, req, *route.Direct, opts)
	case route.AIGateway != nil:
		return p.newAIGatewayModel(ctx, req, *route.AIGateway, opts)
	default:
		return nil, xerrors.New("model route is not configured")
	}
}

func (*Server) newDirectModel(
	_ context.Context,
	req modelClientRequest,
	route directModelRoute,
	opts modelBuildOptions,
) (fantasy.LanguageModel, error) {
	var httpClient *http.Client
	if opts.RecordHTTP {
		httpClient = &http.Client{Transport: &chatdebug.RecordingTransport{}}
	}
	return newLanguageModel(
		route.ProviderHint,
		req.ModelName,
		route.Keys,
		req.UserAgent,
		req.ExtraHeaders,
		httpClient,
	)
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

func newLanguageModel(
	providerHint string,
	modelName string,
	providerKeys chatprovider.ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
	httpClient *http.Client,
) (fantasy.LanguageModel, error) {
	model, err := chatprovider.ModelFromConfig(
		providerHint,
		modelName,
		providerKeys,
		userAgent,
		extraHeaders,
		httpClient,
	)
	if err != nil {
		return nil, err
	}
	if model == nil {
		provider, resolvedModel, resolveErr := chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return nil, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			resolvedModel,
		)
	}
	return model, nil
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
	return aiGatewayProviderAuth{
		Headers:              headers,
		PreserveProviderAuth: true,
	}, nil
}

func (p *Server) aiProviderForConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (string, *database.AIProvider, error) {
	if !modelConfig.AIProviderID.Valid {
		if p.shouldUseAIGatewayRouting() {
			return "", nil, xerrors.Errorf(
				"AI Gateway routing requires AI provider metadata for model config %s (%s)",
				modelConfig.ID,
				modelConfig.Model,
			)
		}
		return modelConfig.Provider, nil, nil
	}
	provider, err := p.db.GetAIProviderByID(ctx, modelConfig.AIProviderID.UUID)
	if err != nil {
		return "", nil, xerrors.Errorf("get AI provider: %w", err)
	}
	if !provider.Enabled {
		return "", nil, xerrors.Errorf("AI provider %s is disabled", provider.ID)
	}
	return string(provider.Type), &provider, nil
}

func (p *Server) resolveModelRouteForConfig(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
	fallbackKeys chatprovider.ProviderAPIKeys,
) (resolvedModelRoute, error) {
	providerHint, provider, err := p.aiProviderForConfig(ctx, modelConfig)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	if p.shouldUseAIGatewayRouting() {
		if provider == nil || provider.ID == uuid.Nil {
			return resolvedModelRoute{}, xerrors.Errorf(
				"AI Gateway routing requires AI provider metadata for model config %s (%s)",
				modelConfig.ID,
				modelConfig.Model,
			)
		}
		auth, err := p.aiGatewayProviderAuthForUser(
			ctx,
			ownerID,
			*provider,
			aiGatewayRequestFormatForProviderType(provider.Type),
		)
		if err != nil {
			return resolvedModelRoute{}, xerrors.Errorf("resolve AI Gateway provider auth: %w", err)
		}
		return resolvedModelRoute{AIGateway: &aiGatewayModelRoute{
			Provider:     *provider,
			OriginalHint: providerHint,
			ProviderAuth: auth,
		}}, nil
	}
	if provider == nil {
		if !fallbackKeys.Empty() && userCanUseProviderKeys(fallbackKeys, providerHint) {
			return resolvedModelRoute{Direct: &directModelRoute{ProviderHint: providerHint, Keys: fallbackKeys}}, nil
		}
		keys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
		if err != nil {
			return resolvedModelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
		}
		return resolvedModelRoute{Direct: &directModelRoute{ProviderHint: providerHint, Keys: keys}}, nil
	}
	providerKeys, err := p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, *provider)
	if err != nil {
		return resolvedModelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	return resolvedModelRoute{Direct: &directModelRoute{ProviderHint: providerHint, Keys: providerKeys}}, nil
}

func (p *Server) resolveModelRouteForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (resolvedModelRoute, error) {
	normalizedProviderType := chatprovider.NormalizeProvider(providerType)
	if p.shouldUseAIGatewayRouting() {
		provider, err := p.aiProviderForProviderType(ctx, providerType)
		if err != nil {
			return resolvedModelRoute{}, err
		}
		auth, err := p.aiGatewayProviderAuthForUser(
			ctx,
			ownerID,
			provider,
			aiGatewayRequestFormatForProviderType(provider.Type),
		)
		if err != nil {
			return resolvedModelRoute{}, xerrors.Errorf("resolve AI Gateway provider auth: %w", err)
		}
		return resolvedModelRoute{AIGateway: &aiGatewayModelRoute{
			Provider:     provider,
			OriginalHint: normalizedProviderType,
			ProviderAuth: auth,
		}}, nil
	}
	keys, _, err := p.resolveUserProviderAPIKeysAndProviderForProviderType(ctx, ownerID, providerType)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	return resolvedModelRoute{Direct: &directModelRoute{ProviderHint: normalizedProviderType, Keys: keys}}, nil
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
