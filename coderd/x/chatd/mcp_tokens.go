package chatd

import (
	"context"
	"database/sql"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpoauth"
)

// refreshExpiredMCPTokens checks each MCP OAuth2 token and refreshes
// any that are expired (or about to expire). Tokens without a
// refresh_token or that fail to refresh are returned unchanged so the
// caller can still attempt the connection (which will likely fail with
// a 401 for the expired ones).
func (p *Server) refreshExpiredMCPTokens(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) ([]database.MCPServerUserToken, error) {
	configsByID := make(map[uuid.UUID]database.MCPServerConfig, len(configs))
	for _, cfg := range configs {
		configsByID[cfg.ID] = cfg
	}

	result := slices.Clone(tokens)

	var eg errgroup.Group
	for i, tok := range result {
		cfg, ok := configsByID[tok.MCPServerConfigID]
		if !ok || cfg.AuthType != "oauth2" {
			continue
		}
		if tok.RefreshToken == "" {
			continue
		}
		if tok.OauthRefreshFailureReason != "" {
			// A previous refresh already failed permanently (e.g.
			// revoked grant); the user must reconnect. Skip the
			// provider call entirely.
			continue
		}

		eg.Go(func() error {
			refreshed, err := p.refreshMCPTokenIfNeeded(ctx, logger, cfg, tok)
			if err != nil {
				return err
			}
			result[i] = refreshed
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

// refreshMCPTokenIfNeeded refreshes an expired token through
// mcpoauth.RefreshOAuth2TokenWithLease, waiting while another request
// holds the token's refresh lease. A transient refresh failure or a lease
// wait that runs out is logged and keeps the current token, so every
// returned error is a database error.
// The logger should carry chat-scoped fields so log lines can be
// correlated with specific chat requests.
func (p *Server) refreshMCPTokenIfNeeded(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (database.MCPServerUserToken, error) {
	// The chatd subject has no personal-write access; the lease, the save
	// and the failure mark run as a subject scoped to this token's owner.
	result, err := mcpoauth.RefreshOAuth2TokenWithLease(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		logger,
		p.db,
		p.mcpHTTPClient,
		cfg,
		tok,
		mcpoauth.LeasedRefreshOptions{WaitForLease: true},
	)
	if err != nil {
		return database.MCPServerUserToken{}, err
	}

	switch result.Outcome {
	case mcpoauth.LeasedRefreshSaved:
		logger.Info(ctx, "refreshed MCP oauth2 token",
			slog.F("server_slug", cfg.Slug),
			slog.F("user_id", tok.UserID),
		)
	case mcpoauth.LeasedRefreshMarkedDead:
		logger.Warn(ctx, "mcp oauth2 grant permanently unusable, marked token for reconnect",
			slog.F("server_slug", cfg.Slug),
			slog.F("user_id", tok.UserID),
			slog.Error(result.RefreshErr),
		)
	case mcpoauth.LeasedRefreshFailed:
		logger.Warn(ctx, "failed to refresh MCP oauth2 token",
			slog.F("server_slug", cfg.Slug),
			slog.Error(result.RefreshErr),
		)
	case mcpoauth.LeasedRefreshLeaseHeld:
		logger.Warn(ctx, "gave up waiting for another MCP oauth2 token refresh",
			slog.F("server_slug", cfg.Slug),
		)
	case mcpoauth.LeasedRefreshLost:
		return p.currentMCPToken(ctx, tok, result.RefreshErr)
	}
	return result.Token, nil
}

// currentMCPToken reads the token after a disconnect or re-authentication
// deleted it before the lease, or won over a leased save or failure mark.
// A deleted token comes back with its token material cleared, and with
// refreshErr as its failure reason when the lost write was a failure
// mark.
func (p *Server) currentMCPToken(
	ctx context.Context,
	tok database.MCPServerUserToken,
	refreshErr error,
) (database.MCPServerUserToken, error) {
	current, err := p.db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
		MCPServerConfigID: tok.MCPServerConfigID,
		UserID:            tok.UserID,
	})
	if err == nil {
		return current, nil
	}
	if !xerrors.Is(err, sql.ErrNoRows) {
		return database.MCPServerUserToken{}, xerrors.Errorf("get MCP oauth2 token after refresh conflict: %w", err)
	}
	tok.AccessToken = ""
	tok.RefreshToken = ""
	tok.Expiry = sql.NullTime{}
	if refreshErr != nil {
		tok.OauthRefreshFailureReason = mcpoauth.RefreshFailureReason(refreshErr)
	}
	return tok, nil
}
