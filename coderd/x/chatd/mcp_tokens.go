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

// refreshMCPTokenIfNeeded delegates to mcpoauth.RefreshOAuth2Token
// and persists the result to the database when a refresh occurs.
// A transient refresh failure is logged and keeps the current token,
// so every returned error is a database error.
// The logger should carry chat-scoped fields so log lines can be
// correlated with specific chat requests.
func (p *Server) refreshMCPTokenIfNeeded(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (database.MCPServerUserToken, error) {
	result, err := mcpoauth.RefreshOAuth2Token(ctx, p.mcpHTTPClient, cfg, tok)
	if err != nil {
		if mcpoauth.IsPermanentRefreshError(err) {
			return p.markMCPTokenRefreshFailure(ctx, logger, cfg, tok, err)
		}
		logger.Warn(ctx, "failed to refresh MCP oauth2 token",
			slog.F("server_slug", cfg.Slug),
			slog.Error(err),
		)
		return tok, nil
	}

	if !result.Refreshed {
		return tok, nil
	}

	logger.Info(ctx, "refreshed MCP oauth2 token",
		slog.F("server_slug", cfg.Slug),
		slog.F("user_id", tok.UserID),
	)

	var expiry sql.NullTime
	if !result.Expiry.IsZero() {
		expiry = sql.NullTime{Time: result.Expiry, Valid: true}
	}

	// The chatd subject has no personal-write access; persist the
	// refresh as a subject scoped to this token's owner.
	updated, err := p.db.UpdateMCPServerUserTokenFromRefresh(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		database.UpdateMCPServerUserTokenFromRefreshParams{
			ID:                tok.ID,
			UpdatedAt:         tok.UpdatedAt,
			AccessToken:       result.AccessToken,
			AccessTokenKeyID:  sql.NullString{},
			RefreshToken:      result.RefreshToken,
			RefreshTokenKeyID: sql.NullString{},
			TokenType:         result.TokenType,
			Expiry:            expiry,
		},
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// A disconnect or re-authentication can win the optimistic update.
			current, readErr := p.db.GetMCPServerUserToken(
				ctx,
				database.GetMCPServerUserTokenParams{
					MCPServerConfigID: tok.MCPServerConfigID,
					UserID:            tok.UserID,
				},
			)
			if readErr == nil {
				return current, nil
			}
			if !xerrors.Is(readErr, sql.ErrNoRows) {
				return database.MCPServerUserToken{}, xerrors.Errorf("get MCP oauth2 token after refresh conflict: %w", readErr)
			}
			tok.AccessToken = ""
			tok.RefreshToken = ""
			tok.Expiry = sql.NullTime{}
			return tok, nil
		}
		return database.MCPServerUserToken{}, xerrors.Errorf("persist refreshed MCP oauth2 token: %w", err)
	}

	return updated, nil
}

// markMCPTokenRefreshFailure persists a permanent refresh failure
// (e.g. the upstream grant was revoked) so the dead token is never
// attached again and the UI can prompt the user to reconnect.
func (p *Server) markMCPTokenRefreshFailure(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
	refreshErr error,
) (database.MCPServerUserToken, error) {
	logger.Warn(ctx, "mcp oauth2 grant permanently unusable, marking token for reconnect",
		slog.F("server_slug", cfg.Slug),
		slog.F("user_id", tok.UserID),
		slog.Error(refreshErr),
	)

	// The chatd subject has no personal-write access; persist the
	// failure as a subject scoped to this token's owner.
	marked, err := p.db.MarkMCPServerUserTokenRefreshFailure(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		database.MarkMCPServerUserTokenRefreshFailureParams{
			ID:                        tok.ID,
			UpdatedAt:                 tok.UpdatedAt,
			OauthRefreshFailureReason: mcpoauth.RefreshFailureReason(refreshErr),
		},
	)
	if err == nil {
		return marked, nil
	}

	if xerrors.Is(err, sql.ErrNoRows) {
		// Optimistic lock miss: a concurrent request refreshed or
		// replaced the token after we read it, so our failure is
		// stale. Use the winner's row instead.
		current, readErr := p.db.GetMCPServerUserToken(
			ctx,
			database.GetMCPServerUserTokenParams{
				MCPServerConfigID: tok.MCPServerConfigID,
				UserID:            tok.UserID,
			},
		)
		if readErr == nil {
			return current, nil
		}
		if !xerrors.Is(readErr, sql.ErrNoRows) {
			return database.MCPServerUserToken{}, xerrors.Errorf("get MCP oauth2 token after refresh failure conflict: %w", readErr)
		}
		tok.AccessToken = ""
		tok.RefreshToken = ""
		tok.Expiry = sql.NullTime{}
		tok.OauthRefreshFailureReason = mcpoauth.RefreshFailureReason(refreshErr)
		return tok, nil
	}
	return database.MCPServerUserToken{}, xerrors.Errorf("mark MCP oauth2 token refresh failure: %w", err)
}
