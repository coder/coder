package coderd

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// oidcMCPTokenSource implements mcpclient.UserOIDCTokenSource by
// delegating to provisionerdserver.ObtainOIDCAccessToken, which already
// handles refresh-on-expiry and writing the refreshed token back to
// user_links. Keeping the adapter in coderd avoids leaking the
// provisionerdserver dependency into the chatd / mcpclient packages.
type oidcMCPTokenSource struct {
	db     database.Store
	config promoauth.OAuth2Config
	logger slog.Logger
}

// newOIDCMCPTokenSource returns nil if the deployment has no OIDC
// provider configured. The chatd MCP client treats a nil source the
// same as a missing user_links row: no Authorization header is sent.
func newOIDCMCPTokenSource(db database.Store, config promoauth.OAuth2Config, logger slog.Logger) mcpclient.UserOIDCTokenSource {
	if config == nil {
		return nil
	}
	return &oidcMCPTokenSource{
		db:     db,
		config: config,
		logger: logger,
	}
}

// OIDCAccessToken returns the user's OIDC access token, refreshing it
// transparently if it has expired. It returns ("", nil) when the user
// has no OIDC link or a non-fatal refresh error occurred, matching the
// contract documented on mcpclient.UserOIDCTokenSource.
func (s *oidcMCPTokenSource) OIDCAccessToken(ctx context.Context, userID uuid.UUID) (string, error) {
	return provisionerdserver.ObtainOIDCAccessToken(ctx, s.logger, s.db, s.config, userID)
}
