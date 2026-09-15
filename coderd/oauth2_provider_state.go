package coderd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

const oauth2ProviderDisabledWithAppsMessage = "The OAuth2 provider is disabled but OAuth2 applications are registered. Existing applications, secrets and user authorizations are preserved, and new authorizations and token exchanges are blocked, but already-issued access tokens are not invalidated and remain usable until they expire or are revoked. Set CODER_OAUTH2_PROVIDER_ENABLE=true to re-enable the provider."

// LogOAuth2ProviderState logs whether the OAuth2 provider is enabled so
// every start leaves one line recording the value of
// CODER_OAUTH2_PROVIDER_ENABLE. When the provider is disabled it also warns
// if applications are still registered. Errors are logged and startup
// continues.
func LogOAuth2ProviderState(ctx context.Context, logger slog.Logger, db database.Store, cfg codersdk.OAuth2ProviderConfig) {
	flagField := slog.F("flag", "CODER_OAUTH2_PROVIDER_ENABLE")
	if cfg.Enable.Value() {
		logger.Info(ctx, "oauth2 provider enabled", flagField)
		return
	}
	logger.Info(ctx, "oauth2 provider disabled", flagField)

	//nolint:gocritic // Startup-only read; no user actor is present.
	apps, err := db.GetOAuth2ProviderApps(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		logger.Warn(ctx, "oauth2 provider: list registered applications", slog.Error(err))
		return
	}
	if len(apps) == 0 {
		return
	}
	logger.Warn(ctx, oauth2ProviderDisabledWithAppsMessage, flagField, slog.F("count", len(apps)))
}
