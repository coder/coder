package chatd

import (
	"context"
	"database/sql"
	"slices"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpoauth"
)

// refreshExpiredMCPTokens checks each MCP OAuth2 token and refreshes
// any that are expired (or about to expire). Tokens without a
// refresh_token or that fail to refresh are returned unchanged so the
// caller can still attempt the connection (which will likely fail with
// a 401 for the expired ones). outcomes holds, by token ID, how each
// refresh attempt ended.
func (p *Server) refreshExpiredMCPTokens(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) (refreshed []database.MCPServerUserToken, outcomes map[uuid.UUID]mcpoauth.LeasedRefreshOutcome, err error) {
	configsByID := make(map[uuid.UUID]database.MCPServerConfig, len(configs))
	for _, cfg := range configs {
		configsByID[cfg.ID] = cfg
	}

	result := slices.Clone(tokens)
	// The zero outcome marks a token that was not attempted.
	attempts := make([]mcpoauth.LeasedRefreshOutcome, len(result))

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
			refreshed, outcome, err := p.refreshMCPTokenIfNeeded(ctx, logger, cfg, tok)
			if err != nil {
				return err
			}
			result[i] = refreshed
			attempts[i] = outcome
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}

	outcomes = make(map[uuid.UUID]mcpoauth.LeasedRefreshOutcome, len(result))
	for i, outcome := range attempts {
		if outcome != 0 {
			outcomes[tokens[i].ID] = outcome
		}
	}
	return result, outcomes, nil
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
) (database.MCPServerUserToken, mcpoauth.LeasedRefreshOutcome, error) {
	return p.refreshMCPToken(ctx, logger, cfg, tok, mcpoauth.LeasedRefreshOptions{
		WaitForLease: true,
	})
}

// refreshMCPToken runs mcpoauth.RefreshOAuth2TokenWithLease and logs how
// it ended. After a deleted row or a lost save or mark it reads the
// current row.
func (p *Server) refreshMCPToken(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
	opts mcpoauth.LeasedRefreshOptions,
) (database.MCPServerUserToken, mcpoauth.LeasedRefreshOutcome, error) {
	// The chatd subject has no personal-write access; the lease, the save
	// and the failure mark run as a subject scoped to this token's owner.
	result, err := mcpoauth.RefreshOAuth2TokenWithLease(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		logger,
		p.db,
		p.mcpHTTPClient,
		cfg,
		tok,
		opts,
	)
	if err != nil {
		return database.MCPServerUserToken{}, 0, err
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
		current, err := p.currentMCPToken(ctx, tok, result.RefreshErr)
		return current, result.Outcome, err
	}
	return result.Token, result.Outcome, nil
}

// mcpTokenRefresher refreshes and reconnects MCP OAuth2 tokens that a
// server rejected during one generation step's connect.
type mcpTokenRefresher struct {
	server  *Server
	logger  slog.Logger
	configs map[uuid.UUID]database.MCPServerConfig
	// expiryRefreshes is how this step's expiry refreshes ended, by
	// token ID.
	expiryRefreshes map[uuid.UUID]mcpoauth.LeasedRefreshOutcome
	tracker         *mcpTokenRefreshTracker
	// turnKey identifies the step's turn in tracker.
	turnKey int64
}

var _ mcpclient.TokenRefresher = (*mcpTokenRefresher)(nil)

// newMCPTokenRefresher returns the refresher for one generation step's
// connect to the servers in configs. expiryRefreshes is how the step's
// expiry refreshes ended, by token ID, and turnKey identifies the step's
// turn in tracker (see stopNudgeKey).
func (p *Server) newMCPTokenRefresher(
	logger slog.Logger,
	configs []database.MCPServerConfig,
	expiryRefreshes map[uuid.UUID]mcpoauth.LeasedRefreshOutcome,
	tracker *mcpTokenRefreshTracker,
	turnKey int64,
) *mcpTokenRefresher {
	configsByID := make(map[uuid.UUID]database.MCPServerConfig, len(configs))
	for _, cfg := range configs {
		configsByID[cfg.ID] = cfg
	}
	return &mcpTokenRefresher{
		server:          p,
		logger:          logger,
		configs:         configsByID,
		expiryRefreshes: expiryRefreshes,
		tracker:         tracker,
		turnKey:         turnKey,
	}
}

// RefreshRejected returns the token for the next connect after a 401.
// A token that this step's expiry refresh just issued, or one without a
// refresh token, is sent again as it is, which ConnectAllWithRefresh
// does only after a short grace. A token whose expiry refresh failed in
// this step, or that already had its forced refresh or its grace this
// turn, gets no reconnect. Otherwise it refreshes the token whatever its
// stored expiry, under the token's refresh lease; when the leased row
// already holds another access token, it reconnects with that row
// without a refresh. There is no reconnect when it cannot help: the
// token is marked for reconnect or gone, or the refresh failed
// temporarily.
func (r *mcpTokenRefresher) RefreshRejected(
	ctx context.Context,
	srv mcpclient.Server,
	tok database.MCPServerUserToken,
) (database.MCPServerUserToken, bool) {
	cfg, ok := r.configs[srv.ID]
	if !ok {
		return tok, false
	}
	switch r.expiryRefreshes[tok.ID] {
	case mcpoauth.LeasedRefreshSaved, mcpoauth.LeasedRefreshChanged:
		return tok, true
	case mcpoauth.LeasedRefreshFailed, mcpoauth.LeasedRefreshLeaseHeld:
		// The failed expiry refresh is this turn's attempt.
		_ = r.tracker.claim(r.turnKey, tok.ID)
		return tok, false
	}
	if !r.tracker.claim(r.turnKey, tok.ID) {
		return tok, false
	}
	if tok.RefreshToken == "" {
		// Nothing can renew the token, but the server may still be
		// catching up with it.
		return tok, true
	}
	r.logger.Warn(ctx, "mcp server rejected the oauth2 token, refreshing it",
		slog.F("server_slug", srv.Slug),
	)
	next, outcome, err := r.server.refreshMCPToken(ctx, r.logger, cfg, tok, mcpoauth.LeasedRefreshOptions{
		WaitForLease: true,
		Force:        true,
	})
	if err != nil {
		r.logger.Warn(ctx, "failed to refresh rejected MCP oauth2 token",
			slog.F("server_slug", cfg.Slug),
			slog.Error(err),
		)
		return tok, false
	}
	switch outcome {
	case mcpoauth.LeasedRefreshSaved, mcpoauth.LeasedRefreshChanged, mcpoauth.LeasedRefreshLost:
		return next, mcpclient.TokenAttachable(next)
	default:
		return next, false
	}
}

// RejectedAgain marks a token that the MCP server still rejected after
// a refresh or the grace, so the token is never sent again and the user
// is asked to reconnect. The mark runs under the token's refresh lease
// and is skipped when another caller holds the lease or the row no
// longer holds tok's access token, because that caller may have saved a
// new token.
func (r *mcpTokenRefresher) RejectedAgain(
	ctx context.Context,
	srv mcpclient.Server,
	tok database.MCPServerUserToken,
) {
	reason := "mcp server rejected the refreshed access token (HTTP 401)"
	if tok.RefreshToken == "" {
		reason = "mcp server rejected the access token (HTTP 401): no refresh token"
	}
	// The chatd subject has no personal-write access; the lease and the
	// mark run as a subject scoped to this token's owner.
	result, err := mcpoauth.MarkRejectedOAuth2TokenWithLease(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		r.logger,
		r.server.db,
		tok,
		reason,
	)
	switch {
	case err != nil:
		r.logger.Warn(ctx, "failed to mark rejected MCP oauth2 token for reconnect",
			slog.F("server_slug", srv.Slug),
			slog.Error(err),
		)
	case result.Outcome == mcpoauth.LeasedRefreshMarkedDead:
		r.logger.Warn(ctx, "mcp server rejected the oauth2 token again, marked token for reconnect",
			slog.F("server_slug", srv.Slug),
			slog.F("user_id", tok.UserID),
			slog.F("reason", reason),
		)
	case result.Outcome == mcpoauth.LeasedRefreshLeaseHeld:
		r.logger.Info(ctx, "mcp server rejected the oauth2 token again, not marked while another refresh holds the lease",
			slog.F("server_slug", srv.Slug),
			slog.F("user_id", tok.UserID),
		)
	}
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

// mcpTokenRefreshTracker allows at most one forced refresh of each MCP
// OAuth2 token per turn, or for a token without a refresh token one
// reconnect after the grace, so an MCP server that keeps rejecting a
// token costs one attempt per turn, not one per step. It lives on the
// runner, which keeps its chat across queued-message promotions, so
// claims are keyed by the turn's prompt (see stopNudgeKey) and a new key
// clears them. On one runner the key changes only with a newly inserted
// prompt: a queued message promoted when a turn ends, or a steered
// message delivered within a turn. A chat that stops running is
// abandoned, and its next turn runs on a new runner. Tokens are keyed by
// ID. A nil tracker allows every attempt.
type mcpTokenRefreshTracker struct {
	mu      sync.Mutex
	turnKey int64
	tried   map[uuid.UUID]struct{}
}

// claim reports whether the token may get a forced refresh now in the
// turn identified by turnKey (see stopNudgeKey), and records the attempt.
func (t *mcpTokenRefreshTracker) claim(turnKey int64, tokenID uuid.UUID) bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.turnKey != turnKey {
		t.turnKey = turnKey
		t.tried = nil
	}
	if _, tried := t.tried[tokenID]; tried {
		return false
	}
	if t.tried == nil {
		t.tried = make(map[uuid.UUID]struct{})
	}
	t.tried[tokenID] = struct{}{}
	return true
}
