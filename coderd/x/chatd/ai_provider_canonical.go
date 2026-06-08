package chatd

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func aiProviderTypeCanSatisfyRequest(candidateProviderType string, requestedProviderType string) bool {
	if candidateProviderType == requestedProviderType {
		return true
	}
	return requestedProviderType == string(codersdk.AIProviderTypeAnthropic) &&
		candidateProviderType == string(codersdk.AIProviderTypeBedrock)
}

func aiProviderMatchesCanonicalType(provider database.AIProvider, normalizedProviderType string) (bool, error) {
	providerType, err := db2sdk.CanonicalAIProviderType(provider)
	if err != nil {
		return false, err
	}
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(string(providerType)), normalizedProviderType), nil
}

func aiProviderMatchesRawType(provider database.AIProvider, normalizedProviderType string) bool {
	return aiProviderTypeCanSatisfyRequest(chatprovider.NormalizeProvider(string(provider.Type)), normalizedProviderType)
}
