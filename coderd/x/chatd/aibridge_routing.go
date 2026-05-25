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

type modelBuildRequest struct {
	Chat         database.Chat
	ProviderHint string
	ModelName    string
	ProviderKeys chatprovider.ProviderAPIKeys
	UserAgent    string
	ExtraHeaders map[string]string
	AIProvider   *database.AIProvider
}

type modelBuildOptions struct {
	ActiveAPIKeyID string
	RecordHTTP     bool
}

func modelBuildOptionsFromMessages(messages []database.ChatMessage) modelBuildOptions {
	apiKeyID, _ := activeTurnAPIKeyIDFromMessages(messages)
	return modelBuildOptions{ActiveAPIKeyID: apiKeyID}
}

type delegatedAPIKeyRoundTripper struct {
	base     http.RoundTripper
	apiKeyID string
}

func (t *delegatedAPIKeyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := aibridge.WithDelegatedAPIKeyID(req.Context(), t.apiKeyID)
	return t.base.RoundTrip(req.WithContext(ctx))
}

func (p *Server) shouldRouteModelsThroughAIBridge() bool {
	return p.aiGatewayRoutingEnabled
}

func (p *Server) newModel(
	_ context.Context,
	req modelBuildRequest,
	opts modelBuildOptions,
) (fantasy.LanguageModel, error) {
	providerHint := req.ProviderHint
	providerKeys := req.ProviderKeys

	var httpClient *http.Client
	if p.shouldRouteModelsThroughAIBridge() {
		if req.AIProvider == nil || req.AIProvider.ID == uuid.Nil {
			return nil, xerrors.New("AI Gateway routing requires a concrete AI provider")
		}
		if req.AIProvider.Name == "" {
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
		rt, err := (*factory).TransportFor(req.AIProvider.Name, aibridge.SourceAgents)
		if err != nil {
			return nil, xerrors.Errorf("create AI Gateway transport: %w", err)
		}
		baseRT := http.RoundTripper(&delegatedAPIKeyRoundTripper{base: rt, apiKeyID: opts.ActiveAPIKeyID})
		if opts.RecordHTTP {
			baseRT = &chatdebug.RecordingTransport{Base: baseRT}
		}
		httpClient = &http.Client{Transport: baseRT}

		config := fantasyConfigForAIBridge(req.AIProvider.Type)
		providerHint = config.ProviderHint
		providerKeys = config.Keys
	} else if opts.RecordHTTP {
		httpClient = &http.Client{Transport: &chatdebug.RecordingTransport{}}
	}

	model, err := chatprovider.ModelFromConfig(
		providerHint,
		req.ModelName,
		providerKeys,
		req.UserAgent,
		req.ExtraHeaders,
		httpClient,
	)
	if err != nil {
		return nil, err
	}
	if model == nil {
		provider, resolvedModel, resolveErr := chatprovider.ResolveModelWithProviderHint(req.ModelName, providerHint)
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

func (p *Server) aiProviderForConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (string, *database.AIProvider, error) {
	if !modelConfig.AIProviderID.Valid {
		if p.shouldRouteModelsThroughAIBridge() {
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

func (p *Server) resolveModelConfigProviderHintKeysAndProvider(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
	fallbackKeys chatprovider.ProviderAPIKeys,
) (string, chatprovider.ProviderAPIKeys, *database.AIProvider, error) {
	providerHint, provider, err := p.aiProviderForConfig(ctx, modelConfig)
	if err != nil {
		return "", chatprovider.ProviderAPIKeys{}, nil, err
	}
	if provider == nil {
		if !fallbackKeys.Empty() && userCanUseProviderKeys(fallbackKeys, providerHint) {
			return providerHint, fallbackKeys, nil, nil
		}
		keys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
		if err != nil {
			return "", chatprovider.ProviderAPIKeys{}, nil, xerrors.Errorf("resolve provider API keys: %w", err)
		}
		return providerHint, keys, nil, nil
	}
	providerKeys, err := p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, *provider)
	if err != nil {
		return "", chatprovider.ProviderAPIKeys{}, nil, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	return providerHint, providerKeys, provider, nil
}
