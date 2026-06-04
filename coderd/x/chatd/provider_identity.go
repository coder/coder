package chatd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func effectiveAIProviderType(provider database.AIProvider) (database.AIProviderType, error) {
	effectiveType, err := db2sdk.EffectiveAIProviderType(provider)
	if err != nil {
		return "", err
	}
	return database.AIProviderType(effectiveType), nil
}

func effectiveAIProviderTypeString(provider database.AIProvider) (string, error) {
	effectiveType, err := effectiveAIProviderType(provider)
	if err != nil {
		return "", err
	}
	return string(effectiveType), nil
}

func bestEffortAIProviderType(ctx context.Context, logger slog.Logger, provider database.AIProvider) database.AIProviderType {
	effectiveType, err := effectiveAIProviderType(provider)
	if err != nil {
		logger.Warn(ctx, "parse AI provider settings", slog.F("provider_id", provider.ID), slog.Error(err))
		return provider.Type
	}
	return effectiveType
}

func bestEffortAIProviderTypeString(ctx context.Context, logger slog.Logger, provider database.AIProvider) string {
	return string(bestEffortAIProviderType(ctx, logger, provider))
}

func aiProviderTypeCanSatisfyRequest(candidateProviderType string, requestedProviderType string) bool {
	if candidateProviderType == requestedProviderType {
		return true
	}
	return requestedProviderType == string(database.AiProviderTypeAnthropic) &&
		candidateProviderType == string(database.AiProviderTypeBedrock)
}

func aiProviderMatchesEffectiveType(provider database.AIProvider, normalizedProviderType string) (bool, error) {
	effectiveType, err := effectiveAIProviderTypeString(provider)
	if err != nil {
		return false, err
	}
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(effectiveType), normalizedProviderType), nil
}

func aiProviderMatchesRawType(provider database.AIProvider, normalizedProviderType string) bool {
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(string(provider.Type)), normalizedProviderType)
}
