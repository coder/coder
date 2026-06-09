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

// BackfillChatModelConfigProviderStrings fixes stale provider strings on
// chat_model_configs rows created when the linked ai_providers row had
// type=anthropic. After BackfillBedrockProviderType promotes those provider
// rows to type=bedrock, the stored provider strings remain "anthropic"; the
// frontend uses the provider field to select icons, so stale strings cause
// Anthropic styling to appear for Bedrock-backed models.
//
// Non-blocking: errors are logged and startup continues. Must run after
// BackfillBedrockProviderType so provider types are already correct when
// the JOIN executes.
func BackfillChatModelConfigProviderStrings(ctx context.Context, db database.Store, logger slog.Logger) {
	//nolint:gocritic // Startup-only backfill; no user actor is present.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	result, err := db.BackfillChatModelConfigProvider(sysCtx, database.BackfillChatModelConfigProviderParams{
		OldProvider: "anthropic",
		NewProvider: "bedrock",
	})
	if err != nil {
		logger.Error(ctx, "backfill chat model config provider strings", slog.Error(err))
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		logger.Info(ctx, "backfilled chat model config provider strings", slog.F("count", n))
	}
}

// BackfillBedrockProviderType promotes legacy ai_providers rows stored as
// type=anthropic with Bedrock settings to type=bedrock. It must be called
// after newAPI so that options.Database is dbcrypt-wrapped; encrypted settings
// are decrypted transparently by that wrapper, making the Bedrock discriminator
// visible for comparison.
//
// The function is idempotent: rows already typed as bedrock are skipped.
// Any error is logged and the startup sequence continues.
//
// Fixing the provider row type restores correct routing: when a
// chat_model_configs row has a valid ai_provider_id, chatd resolves the
// provider name from the live provider row's type column, not from the stored
// provider string. BackfillChatModelConfigProviderStrings corrects those
// stored strings separately, which matters for frontend icon selection.
//
// Trade-off: the function reads all providers then updates them individually.
// A concurrent PATCH between the read and a write could have non-type field
// changes (display_name, enabled, etc.) silently overwritten. This is
// acceptable: the provider type cannot be changed via the API, the startup
// window is short, and a subsequent PATCH corrects any overwritten field.
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
		_, err = db.UpdateAIProvider(sysCtx, database.UpdateAIProviderParams{
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
