package chatd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func canonicalAIProviderType(provider database.AIProvider) (database.AIProviderType, error) {
	return db2sdk.CanonicalAIProviderType(provider)
}

func canonicalAIProviderTypeString(provider database.AIProvider) (string, error) {
	providerType, err := canonicalAIProviderType(provider)
	if err != nil {
		return "", err
	}
	return string(providerType), nil
}

func bestEffortCanonicalAIProviderType(ctx context.Context, logger slog.Logger, provider database.AIProvider) database.AIProviderType {
	providerType, err := canonicalAIProviderType(provider)
	if err != nil {
		logger.Warn(ctx, "parse AI provider settings", slog.F("provider_id", provider.ID), slog.Error(err))
		return provider.Type
	}
	return providerType
}

func bestEffortCanonicalAIProviderTypeString(ctx context.Context, logger slog.Logger, provider database.AIProvider) string {
	return string(bestEffortCanonicalAIProviderType(ctx, logger, provider))
}

func aiProviderTypeCanSatisfyRequest(candidateProviderType string, requestedProviderType string) bool {
	if candidateProviderType == requestedProviderType {
		return true
	}
	return requestedProviderType == string(database.AiProviderTypeAnthropic) &&
		candidateProviderType == string(database.AiProviderTypeBedrock)
}

func aiProviderMatchesCanonicalType(provider database.AIProvider, normalizedProviderType string) (bool, error) {
	providerType, err := canonicalAIProviderTypeString(provider)
	if err != nil {
		return false, err
	}
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(providerType), normalizedProviderType), nil
}

func aiProviderMatchesRawType(provider database.AIProvider, normalizedProviderType string) bool {
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(string(provider.Type)), normalizedProviderType)
}
