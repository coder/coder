package chatd

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
)

func canonicalAIProviderType(provider database.AIProvider) (database.AIProviderType, error) {
	return db2sdk.CanonicalAIProviderType(provider)
}
