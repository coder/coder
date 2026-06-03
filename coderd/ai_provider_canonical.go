package coderd

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
)

func canonicalDatabaseAIProviderType(providerType database.AIProviderType, settings codersdk.AIProviderSettings) database.AIProviderType {
	return database.AIProviderType(codersdk.CanonicalAIProviderType(codersdk.AIProviderType(providerType), settings))
}

func canonicalAIProviderTypeForRow(provider database.AIProvider) (database.AIProviderType, error) {
	return db2sdk.CanonicalAIProviderType(provider)
}
