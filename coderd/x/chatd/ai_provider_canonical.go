package chatd

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
)

func canonicalAIProviderType(provider database.AIProvider) (database.AIProviderType, error) {
	settings, err := db2sdk.AIProviderSettings(provider.Settings)
	if err != nil {
		return "", xerrors.Errorf("decode AI provider settings: %w", err)
	}
	return database.AIProviderType(codersdk.CanonicalAIProviderType(codersdk.AIProviderType(provider.Type), settings)), nil
}
