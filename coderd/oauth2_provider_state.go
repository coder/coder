package coderd

import (
	"context"
	"fmt"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// oauth2ProviderDisabledWithAppsMessage is logged at startup when the
// provider is off but applications are still registered.
const oauth2ProviderDisabledWithAppsMessage = "The OAuth2 provider is disabled but %d OAuth2 application(s) are registered. Existing applications, secrets and user authorizations are preserved but cannot be used until the provider is enabled. Set CODER_OAUTH2_PROVIDER_ENABLE=true to enable it."

// LogOAuth2ProviderState logs whether the OAuth2 provider is enabled so
// every start leaves one line recording the value of
// CODER_OAUTH2_PROVIDER_ENABLE. When the provider is disabled it also warns
// if applications are still registered. Errors are logged and startup
// continues.
func LogOAuth2ProviderState(ctx context.Context, logger slog.Logger, db database.Store, cfg codersdk.OAuth2ProviderConfig) {
	flag := slog.F("flag", "CODER_OAUTH2_PROVIDER_ENABLE")
	if cfg.Enable.Value() {
		logger.Info(ctx, "oauth2 provider enabled", flag)
		return
	}
	logger.Info(ctx, "oauth2 provider disabled", flag)

	//nolint:gocritic // Startup-only read; no user actor is present.
	apps, err := db.GetOAuth2ProviderApps(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		logger.Warn(ctx, "oauth2 provider: list registered applications", slog.Error(err))
		return
	}
	if len(apps) == 0 {
		return
	}
	logger.Warn(ctx, fmt.Sprintf(oauth2ProviderDisabledWithAppsMessage, len(apps)))
}
