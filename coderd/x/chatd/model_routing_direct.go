package chatd

import (
	"context"
	"net/http"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

type directModelRoute struct {
	ProviderHint string
	Keys         chatprovider.ProviderAPIKeys
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

func (p *Server) resolveDirectModelRouteForConfig(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfig database.ChatModelConfig,
	fallbackKeys chatprovider.ProviderAPIKeys,
) (resolvedModelRoute, error) {
	providerHint, provider, err := p.directProviderHintAndProviderForConfig(ctx, modelConfig)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	if provider == nil {
		if !fallbackKeys.Empty() && userCanUseProviderKeys(fallbackKeys, providerHint) {
			return newDirectModelRoute(providerHint, fallbackKeys), nil
		}
		keys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
		if err != nil {
			return resolvedModelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
		}
		return newDirectModelRoute(providerHint, keys), nil
	}
	providerKeys, err := p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, *provider)
	if err != nil {
		return resolvedModelRoute{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	return newDirectModelRoute(providerHint, providerKeys), nil
}

func (p *Server) resolveDirectModelRouteForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (resolvedModelRoute, error) {
	normalizedProviderType := chatprovider.NormalizeProvider(providerType)
	keys, _, err := p.resolveUserProviderAPIKeysAndProviderForProviderType(ctx, ownerID, providerType)
	if err != nil {
		return resolvedModelRoute{}, err
	}
	return newDirectModelRoute(normalizedProviderType, keys), nil
}

func (p *Server) directProviderHintAndProviderForConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (string, *database.AIProvider, error) {
	if !modelConfig.AIProviderID.Valid {
		return modelConfig.Provider, nil, nil
	}
	provider, err := p.enabledAIProviderByID(ctx, modelConfig.AIProviderID.UUID)
	if err != nil {
		return "", nil, err
	}
	return string(provider.Type), &provider, nil
}
