package chatd

import (
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

func bestEffortAIProviderType(provider database.AIProvider) database.AIProviderType {
	effectiveType, err := effectiveAIProviderType(provider)
	if err != nil {
		return provider.Type
	}
	return effectiveType
}

func bestEffortAIProviderTypeString(provider database.AIProvider) string {
	return string(bestEffortAIProviderType(provider))
}

func aiProviderMatchesEffectiveType(provider database.AIProvider, normalizedProviderType string) (bool, error) {
	effectiveType, err := effectiveAIProviderTypeString(provider)
	if err != nil {
		return false, err
	}
	return chatprovider.NormalizeProvider(effectiveType) == normalizedProviderType, nil
}

func aiProviderMatchesRawType(provider database.AIProvider, normalizedProviderType string) bool {
	return chatprovider.NormalizeProvider(string(provider.Type)) == normalizedProviderType
}
