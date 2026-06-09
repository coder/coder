package coderd

import (
	"context"
	"database/sql"
	"errors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
)

// BackfillBedrockProviderType promotes legacy ai_providers rows stored as
// type=anthropic with Bedrock settings to type=bedrock. It must be called
// after newAPI so that options.Database is dbcrypt-wrapped; encrypted settings
// are decrypted transparently by that wrapper, making the Bedrock discriminator
// visible for comparison. db must be the raw (pre-dbauthz) store: this
// function runs during startup with no actor in context, so dbauthz checks
// would fail.
//
// The function is idempotent: rows already typed as bedrock are skipped.
// Any error is logged and the startup sequence continues.
//
// Trade-off: the function reads all providers then updates them individually.
// A concurrent PATCH between the read and a write could have non-type field
// changes (display_name, enabled, etc.) silently overwritten. This is
// acceptable: the provider type cannot be changed via the API, the startup
// window is short, and a subsequent PATCH corrects any overwritten field.
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
			if errors.Is(err, sql.ErrNoRows) {
				logger.Debug(ctx, "backfill bedrock provider type: provider deleted during backfill",
					slog.F("provider_id", provider.ID))
				continue
			}
			logger.Error(ctx, "backfill bedrock provider type: update provider",
				slog.F("provider_id", provider.ID), slog.Error(err))
		}
	}
}
