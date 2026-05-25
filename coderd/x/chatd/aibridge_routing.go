package chatd

import (
	"context"
	"net/http"

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
	// non-empty API key. Aibridge resolves the real provider credential from
	// the routed provider and delegated API key context.
	aibridgePlaceholderAPIKey = "coder-aibridge"
)

type delegatedAPIKeyRoundTripper struct {
	base     http.RoundTripper
	apiKeyID string
}

func (t *delegatedAPIKeyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := aibridge.WithDelegatedAPIKeyID(req.Context(), t.apiKeyID)
	return t.base.RoundTrip(req.WithContext(ctx))
}

type modelRoute struct {
	aiProvider database.AIProvider
}

func modelRouteFromAIProvider(provider database.AIProvider) modelRoute {
	return modelRoute{aiProvider: provider}
}

func (p *Server) shouldRouteModelsThroughAIBridge() bool {
	return p.aiGatewayRoutingEnabled
}

func (r modelRoute) hasProviderID() bool {
	return r.aiProvider.ID != uuid.Nil
}

func (p *Server) newModelFromConfig(
	ctx context.Context,
	chat database.Chat,
	providerHint string,
	modelName string,
	providerKeys chatprovider.ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
	route modelRoute,
) (fantasy.LanguageModel, bool, error) {
	debugSvc := p.debugService()
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

	var httpClient *http.Client
	if p.shouldRouteModelsThroughAIBridge() {
		if !route.hasProviderID() {
			return nil, debugEnabled, xerrors.New("AI Gateway routing requires a concrete AI provider")
		}
		if route.aiProvider.Name == "" {
			return nil, debugEnabled, xerrors.New("AI Gateway routing requires an AI provider name")
		}
		apiKeyID, ok := aibridge.DelegatedAPIKeyIDFromContext(ctx)
		if !ok || apiKeyID == "" {
			return nil, debugEnabled, xerrors.New("AI Gateway routing requires the active turn API key ID")
		}

		factoryPtr := p.aibridgeTransportFactory
		if factoryPtr == nil {
			return nil, debugEnabled, xerrors.New("AI Gateway transport factory is not configured")
		}
		factory := factoryPtr.Load()
		if factory == nil || *factory == nil {
			return nil, debugEnabled, xerrors.New("AI Gateway transport factory is not configured")
		}
		rt, err := (*factory).TransportFor(route.aiProvider.Name, aibridge.SourceAgents)
		if err != nil {
			return nil, debugEnabled, xerrors.Errorf("create AI Gateway transport: %w", err)
		}
		delegatedRT := &delegatedAPIKeyRoundTripper{base: rt, apiKeyID: apiKeyID}
		if debugEnabled {
			httpClient = &http.Client{Transport: &chatdebug.RecordingTransport{Base: delegatedRT}}
		} else {
			httpClient = &http.Client{Transport: delegatedRT}
		}

		providerHint, providerKeys = aibridgeModelProviderInput(route.aiProvider.Type)
	} else if debugEnabled {
		httpClient = &http.Client{Transport: &chatdebug.RecordingTransport{}}
	}

	model, err := chatprovider.ModelFromConfig(
		providerHint,
		modelName,
		providerKeys,
		userAgent,
		extraHeaders,
		httpClient,
	)
	if err != nil {
		return nil, debugEnabled, err
	}
	if model == nil {
		provider, resolvedModel, resolveErr := chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
		if resolveErr != nil {
			return nil, debugEnabled, resolveErr
		}
		return nil, debugEnabled, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			resolvedModel,
		)
	}
	return model, debugEnabled, nil
}

func aibridgeModelProviderInput(providerType database.AIProviderType) (string, chatprovider.ProviderAPIKeys) {
	fantasyProvider := aibridgeFantasyProviderForType(providerType)
	keys := chatprovider.ProviderAPIKeys{
		ByProvider: map[string]string{
			fantasyProvider: aibridgePlaceholderAPIKey,
		},
		BaseURLByProvider: map[string]string{
			fantasyProvider: aibridgeBaseURLForProviderType(providerType),
		},
	}
	return fantasyProvider, keys
}

func aibridgeFantasyProviderForType(providerType database.AIProviderType) string {
	switch providerType {
	case database.AiProviderTypeOpenai:
		return fantasyopenai.Name
	case database.AiProviderTypeAnthropic, database.AiProviderTypeBedrock:
		return fantasyanthropic.Name
	default:
		return fantasyopenaicompat.Name
	}
}

func aibridgeBaseURLForProviderType(providerType database.AIProviderType) string {
	switch aibridgeFantasyProviderForType(providerType) {
	case fantasyanthropic.Name:
		return aibridgeLocalBaseURL
	default:
		return aibridgeLocalBaseURL + "/v1"
	}
}

func (p *Server) modelRouteForConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (string, modelRoute, error) {
	if !modelConfig.AIProviderID.Valid {
		if p.shouldRouteModelsThroughAIBridge() {
			return "", modelRoute{}, xerrors.Errorf(
				"AI Gateway routing requires AI provider metadata for model config %s (%s)",
				modelConfig.ID,
				modelConfig.Model,
			)
		}
		return modelConfig.Provider, modelRoute{}, nil
	}
	provider, err := p.db.GetAIProviderByID(ctx, modelConfig.AIProviderID.UUID)
	if err != nil {
		return "", modelRoute{}, xerrors.Errorf("get AI provider: %w", err)
	}
	if !provider.Enabled {
		return "", modelRoute{}, xerrors.Errorf("AI provider %s is disabled", provider.ID)
	}
	return string(provider.Type), modelRouteFromAIProvider(provider), nil
}

func (p *Server) resolveModelConfigProviderHintKeysAndRoute(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
	fallbackKeys chatprovider.ProviderAPIKeys,
) (string, chatprovider.ProviderAPIKeys, modelRoute, error) {
	providerHint, route, err := p.modelRouteForConfig(ctx, modelConfig)
	if err != nil {
		return "", chatprovider.ProviderAPIKeys{}, modelRoute{}, err
	}
	if !route.hasProviderID() {
		if !fallbackKeys.Empty() && userCanUseProviderKeys(fallbackKeys, providerHint) {
			return providerHint, fallbackKeys, route, nil
		}
		keys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
		if err != nil {
			return "", chatprovider.ProviderAPIKeys{}, modelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
		}
		return providerHint, keys, route, nil
	}
	providerKeys, err := p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, route.aiProvider)
	if err != nil {
		return "", chatprovider.ProviderAPIKeys{}, modelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	return providerHint, providerKeys, route, nil
}
