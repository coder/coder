package coderd

import (
	"context"
	"database/sql"
	"errors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// BackfillBedrockProviderType promotes legacy ai_providers rows stored as
// type=anthropic with Bedrock settings to type=bedrock. Must run after newAPI
// so options.Database is dbcrypt-wrapped. Idempotent; errors are logged and
// startup continues.
func BackfillBedrockProviderType(ctx context.Context, db database.Store, logger slog.Logger) {
	//nolint:gocritic // Startup-only backfill; no user actor is present.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	providers, err := db.GetAIProviders(sysCtx, database.GetAIProvidersParams{
		IncludeDeleted:  false,
		IncludeDisabled: true,
	})
	if err != nil {
		logger.Error(ctx, "backfill bedrock provider type: list providers", slog.Error(err))
		return
	}
	var promoted int
	for _, provider := range providers {
		if provider.Type != database.AIProviderTypeAnthropic {
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
		_, err = db.UpdateAIProvider(sysCtx, database.UpdateAIProviderParams{
			ID:          provider.ID,
			Type:        database.AIProviderTypeBedrock,
			DisplayName: provider.DisplayName,
			Icon:        provider.Icon,
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
			logger.Error(ctx, "backfill bedrock provider type: provider update failed and will re-attempt on next server startup",
				slog.F("provider_id", provider.ID), slog.Error(err))
			continue
		}
		promoted++
	}
	if promoted > 0 {
		logger.Info(ctx, "backfilled bedrock provider types", slog.F("count", promoted))
	}
}
