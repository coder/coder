package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

const (
	aibridgeLocalBaseURL = "http://coder-aibridge"
	// aibridgePlaceholderAPIKey satisfies fantasy clients that require a
	// non-empty API key before aibridged resolves the real credential.
	aibridgePlaceholderAPIKey   = "coder-aibridge"
	aibridgeDelegatedBYOKMarker = "delegated"
)

type aiGatewayModelRoute struct {
	Provider          database.AIProvider
	ModelProviderHint string
	ProviderAuth      aiGatewayProviderAuth
}

func newAIGatewayModelRoute(
	provider database.AIProvider,
	modelProviderHint string,
	auth aiGatewayProviderAuth,
) aiGatewayModelRoute {
	return aiGatewayModelRoute{
		Provider:          provider,
		ModelProviderHint: modelProviderHint,
		ProviderAuth:      auth,
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

// stageSpanRoundTripper emits one provider_attempt stage per HTTP
// round trip to the model provider, so retried requests each get
// their own span. model labels every attempt with the identity the
// client was built for.
type stageSpanRoundTripper struct {
	base   http.RoundTripper
	stages *chatloop.StageTracer
	model  chatloop.StageModel
}

var _ http.RoundTripper = (*stageSpanRoundTripper)(nil)

func (t *stageSpanRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := t.stages.Start(req.Context(), chatloop.StageProviderAttempt,
		attribute.String(chatloop.AttrHTTPMethod, req.Method),
		attribute.String(chatloop.AttrHTTPHost, req.URL.Host),
	)
	span.SetModel(t.model)
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if resp != nil {
		span.SetAttributes(attribute.Int(chatloop.AttrHTTPStatusCode, resp.StatusCode))
		if err == nil && resp.StatusCode >= http.StatusBadRequest {
			err = xerrors.Errorf("provider returned status %d", resp.StatusCode)
			// The status error only marks the span; the response and the
			// transport's own error are returned untouched.
			span.End(err)
			return resp, nil
		}
	}
	// The span closes on response headers, not on body completion: the
	// streamed body outlives this call and is measured by the stream
	// stage.
	span.End(err)
	return resp, err
}

type aiGatewayRoundTripper struct {
	base         http.RoundTripper
	apiKeyID     string
	providerAuth aiGatewayProviderAuth
}

func (t *aiGatewayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := aibridge.WithDelegatedAPIKeyID(req.Context(), t.apiKeyID)
	cloned := req.Clone(ctx)
	for name, value := range t.providerAuth.Headers {
		cloned.Header.Set(name, value)
	}
	if len(t.providerAuth.Headers) > 0 {
		cloned.Header.Set(aibridge.HeaderCoderToken, aibridgeDelegatedBYOKMarker)
	}
	return t.base.RoundTrip(cloned)
}

// ValidateAIGatewayProviderModel rejects slash-namespaced models when an
// OpenRouter-like gateway is configured with the OpenAI provider type.
func ValidateAIGatewayProviderModel(provider database.AIProvider, model string) error {
	if provider.Type != database.AIProviderTypeOpenai {
		return nil
	}
	if !isSlashNamespacedAIGatewayModel(model) || !isOpenRouterLikeAIGatewayProvider(provider) {
		return nil
	}
	return xerrors.New("OpenRouter-like provider configured as type openai does not support slash-namespaced models")
}

func isSlashNamespacedAIGatewayModel(model string) bool {
	prefix, suffix, ok := strings.Cut(strings.TrimSpace(model), "/")
	return ok && strings.TrimSpace(prefix) != "" && strings.TrimSpace(suffix) != ""
}

func isOpenRouterLikeAIGatewayProvider(provider database.AIProvider) bool {
	if strings.EqualFold(strings.TrimSpace(provider.Name), "openrouter") {
		return true
	}
	host := aibridged.BaseURLHostname(provider.BaseUrl)
	return host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai")
}

func (p *Server) newModel(
	_ context.Context,
	req modelClientRequest,
	route aiGatewayModelRoute,
	opts modelBuildOptions,
) (chatprovider.Model, error) {
	if route.Provider.ID == uuid.Nil {
		return chatprovider.Model{}, xerrors.New("AI Gateway routing requires a concrete AI provider")
	}
	if route.Provider.Name == "" {
		return chatprovider.Model{}, xerrors.New("AI Gateway routing requires an AI provider name")
	}
	if opts.ActiveAPIKeyID == "" {
		return chatprovider.Model{}, chaterror.WithClassification(
			xerrors.New("AI Gateway routing requires the active turn API key ID"),
			chaterror.ClassifiedError{
				Kind:      codersdk.ChatErrorKindMissingKey,
				Retryable: false,
				Detail:    "If this error persists after resending, please report it as a bug.",
			},
		)
	}

	if err := ValidateAIGatewayProviderModel(route.Provider, req.ModelName); err != nil {
		return chatprovider.Model{}, chaterror.WithClassification(
			err,
			chaterror.ClassifiedError{
				Kind:      codersdk.ChatErrorKindConfig,
				Retryable: false,
				Detail:    "Ask an administrator to change the AI provider type to openrouter or openai-compat.",
			},
		)
	}

	factoryPtr := p.aibridgeTransportFactory
	if factoryPtr == nil {
		return chatprovider.Model{}, xerrors.New("AI Gateway transport factory is not configured")
	}
	factory := factoryPtr.Load()
	if factory == nil || *factory == nil {
		return chatprovider.Model{}, xerrors.New("AI Gateway transport factory is not configured")
	}
	rt, err := (*factory).TransportFor(route.Provider.Name, aibridge.SourceAgents)
	if err != nil {
		return chatprovider.Model{}, xerrors.Errorf("create AI Gateway transport: %w", err)
	}
	baseRT := http.RoundTripper(&aiGatewayRoundTripper{
		base:         rt,
		apiKeyID:     opts.ActiveAPIKeyID,
		providerAuth: route.ProviderAuth,
	})
	if opts.RecordHTTP {
		baseRT = &chatdebug.RecordingTransport{Base: baseRT}
	}
	baseRT = &stageSpanRoundTripper{base: baseRT, stages: p.stages, model: opts.StageModel}

	config := fantasyConfigForAIBridge(route.Provider.Type)
	extraHeaders := mergeConfigBetaHeaders(req.ExtraHeaders, config.ProviderHint, req.CallConfig)
	return newLanguageModel(
		config.ProviderHint,
		req.ModelName,
		config.Keys,
		req.UserAgent,
		extraHeaders,
		&http.Client{Transport: baseRT},
		req.CallConfig.OpenAIConfig,
	)
}

func parseModelConfigOptions(configOptions json.RawMessage) (codersdk.ChatModelCallConfig, error) {
	var callConfig codersdk.ChatModelCallConfig
	if len(configOptions) == 0 {
		return callConfig, nil
	}
	if err := json.Unmarshal(configOptions, &callConfig); err != nil {
		return codersdk.ChatModelCallConfig{}, xerrors.Errorf("parse model config options: %w", err)
	}
	return callConfig, nil
}

// mergeConfigBetaHeaders never mutates extraHeaders; existing entries win
// over config-derived ones.
func mergeConfigBetaHeaders(
	extraHeaders map[string]string,
	providerHint string,
	callConfig codersdk.ChatModelCallConfig,
) map[string]string {
	betaHeaders := chatprovider.BetaHeadersFromCallConfig(providerHint, &callConfig)
	if len(betaHeaders) == 0 {
		return extraHeaders
	}
	merged := make(map[string]string, len(extraHeaders)+len(betaHeaders))
	for name, value := range betaHeaders {
		merged[name] = value
	}
	for name, value := range extraHeaders {
		merged[name] = value
	}
	return merged
}

type aibridgeFantasyConfig struct {
	ProviderHint string
	Keys         chatprovider.ProviderAPIKeys
}

func fantasyConfigForAIBridge(providerType database.AIProviderType) aibridgeFantasyConfig {
	var fantasyProvider string
	baseURL := aibridgeLocalBaseURL + "/v1"
	switch providerType {
	case database.AIProviderTypeAnthropic, database.AIProviderTypeBedrock:
		fantasyProvider = fantasyanthropic.Name
		baseURL = aibridgeLocalBaseURL
	case database.AIProviderTypeOpenai:
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
	case database.AIProviderTypeAnthropic, database.AIProviderTypeBedrock:
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
) (aiGatewayModelRoute, error) {
	auth, err := p.aiGatewayProviderAuthForUser(
		ctx,
		ownerID,
		provider,
		aiGatewayRequestFormatForProviderType(provider.Type),
	)
	if err != nil {
		return aiGatewayModelRoute{}, xerrors.Errorf("resolve AI Gateway provider auth: %w", err)
	}
	return newAIGatewayModelRoute(provider, modelProviderHint, auth), nil
}

func (p *Server) resolveModelRouteForConfig(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
) (aiGatewayModelRoute, error) {
	provider, err := p.gatewayProviderForConfig(ctx, modelConfig)
	if err != nil {
		return aiGatewayModelRoute{}, err
	}
	return p.resolveAIGatewayRoute(ctx, ownerID, provider, string(provider.Type))
}

func (p *Server) resolveModelRouteForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (aiGatewayModelRoute, error) {
	provider, err := p.aiProviderForProviderType(ctx, providerType)
	if err != nil {
		return aiGatewayModelRoute{}, err
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
