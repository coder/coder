package coderd

import (
	"context"
	"database/sql"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
)

// BackfillBedrockProviderType promotes legacy ai_providers rows stored as
// type=anthropic with Bedrock settings to type=bedrock. It must be called
// after newAPI so that options.Database is dbcrypt-wrapped; encrypted settings
// are decrypted transparently by that wrapper, making the Bedrock discriminator
// visible for comparison.
//
// The function is idempotent: rows already typed as bedrock are skipped.
// Any error is logged and the startup sequence continues; the runtime
// CanonicalAIProviderType shim in codersdk handles rows not yet promoted.
func BackfillBedrockProviderType(ctx context.Context, db database.Store, logger slog.Logger) {
	providers, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
		IncludeDeleted:  false,
		IncludeDisabled: true,
	})
	if err != nil {
		logger.Error(ctx, "backfill bedrock provider type: list providers", slog.Error(err))
		return
	}
	for _, provider := range providers {
		if provider.Type != database.AiProviderTypeAnthropic {
			continue
		}
		settings, err := db2sdk.AIProviderSettings(provider.Settings)
		if err != nil {
			logger.Warn(ctx, "backfill bedrock provider type: skip provider with unparsable settings",
				slog.F("provider_id", provider.ID), slog.Error(err))
			continue
		}
		if settings.Bedrock == nil {
			continue
		}
		_, err = db.UpdateAIProvider(ctx, database.UpdateAIProviderParams{
			ID:          provider.ID,
			Type:        database.AiProviderTypeBedrock,
			DisplayName: provider.DisplayName,
			Enabled:     provider.Enabled,
			BaseUrl:     provider.BaseUrl,
			Settings:    provider.Settings,
			// SettingsKeyID is re-set by the dbcrypt wrapper on write.
			SettingsKeyID: sql.NullString{},
		})
		if err != nil {
			logger.Error(ctx, "backfill bedrock provider type: update provider",
				slog.F("provider_id", provider.ID), slog.Error(err))
		}
	}
}
