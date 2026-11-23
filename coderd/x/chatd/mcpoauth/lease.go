package mcpoauth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/retry"
)

const (
	// refreshBudget bounds one leased refresh: the token endpoint call,
	// which requestOAuth2Refresh caps at tokenRequestTimeout, plus the
	// save.
	refreshBudget = 15 * time.Second
	// refreshLeaseLength exceeds refreshBudget, so a holder always runs
	// out of time before its lease expires and another caller takes it.
	refreshLeaseLength = refreshBudget + 5*time.Second
	// refreshLeaseReleaseTimeout lets a holder that used its whole
	// budget still release the lease.
	refreshLeaseReleaseTimeout = 2 * time.Second
	// The lease wait polls with the external auth backoff.
	refreshLeaseInitialBackoff = 50 * time.Millisecond
	refreshLeaseMaxBackoff     = 500 * time.Millisecond
	// refreshLeaseActiveConstraint is the check violation that
	// acquire_mcp_server_user_token_refresh_lease raises while another
	// caller holds the lease.
	refreshLeaseActiveConstraint database.CheckConstraint = "mcp_server_user_token_active_lease"
)

// RefreshLeaseStore is the part of database.Store that
// RefreshOAuth2TokenWithLease uses.
type RefreshLeaseStore interface {
	AcquireMCPServerUserTokenRefreshLease(ctx context.Context, arg database.AcquireMCPServerUserTokenRefreshLeaseParams) (database.MCPServerUserToken, error)
	ReleaseMCPServerUserTokenRefreshLease(ctx context.Context, arg database.ReleaseMCPServerUserTokenRefreshLeaseParams) error
	UpdateMCPServerUserTokenFromRefresh(ctx context.Context, arg database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error)
	MarkMCPServerUserTokenRefreshFailure(ctx context.Context, arg database.MarkMCPServerUserTokenRefreshFailureParams) (database.MCPServerUserToken, error)
}

// expiredTokenChanged is the comparison for refreshing an expired
// token: any new access token, refresh token or expiry means another
// caller already refreshed it, or marked it dead and cleared all three.
// An unchanged row is still expired.
func expiredTokenChanged(read, leased database.MCPServerUserToken) bool {
	return leased.AccessToken != read.AccessToken ||
		leased.RefreshToken != read.RefreshToken ||
		leased.Expiry.Valid != read.Expiry.Valid ||
		!leased.Expiry.Time.Equal(read.Expiry.Time)
}

// LeasedRefreshOptions configures RefreshOAuth2TokenWithLease.
type LeasedRefreshOptions struct {
	// WaitForLease waits, for at most the lease length, while another
	// caller holds the lease. Without it a held lease returns at once.
	WaitForLease bool
}

// LeasedRefreshOutcome says how RefreshOAuth2TokenWithLease ended. The
// zero value is no outcome, for a token that no refresh was attempted
// for.
type LeasedRefreshOutcome int

const (
	// LeasedRefreshNotNeeded means the token is valid by the clock: no
	// lease was taken and the provider was not called.
	LeasedRefreshNotNeeded LeasedRefreshOutcome = iota + 1
	// LeasedRefreshLeaseHeld means another caller holds the lease, and
	// this caller did not wait or its wait ended.
	LeasedRefreshLeaseHeld
	// LeasedRefreshChanged means the leased row already differs from
	// the caller's token, so the provider was not called and nothing
	// was written.
	LeasedRefreshChanged
	// LeasedRefreshSaved means the token endpoint answered with a new
	// token and it was saved.
	LeasedRefreshSaved
	// LeasedRefreshMarkedDead means the token was marked for reconnect,
	// because the provider rejected the grant permanently.
	LeasedRefreshMarkedDead
	// LeasedRefreshFailed means the provider call failed temporarily, or
	// that this caller waited while another caller held the lease and
	// that caller did not change the row, so this caller made no
	// provider call. Nothing was saved.
	LeasedRefreshFailed
	// LeasedRefreshLost means the token row no longer exists, or the
	// save or the failure mark matched no row, because a
	// re-authentication or disconnect replaced or deleted the token
	// while the lease was held. The caller reads the current row.
	LeasedRefreshLost
)

// LeasedRefresh is the result of RefreshOAuth2TokenWithLease.
type LeasedRefresh struct {
	Outcome LeasedRefreshOutcome
	// Token is the row to use next. It is the caller's token for
	// NotNeeded and LeaseHeld, the saved or marked row for Saved and
	// MarkedDead, and the leased row for Changed and Failed. For Lost
	// the caller reads the current row instead.
	Token database.MCPServerUserToken
	// RefreshErr is the token endpoint error for a refresh that ended
	// MarkedDead or Failed, and for Lost when the lost write was the
	// refresh's failure mark.
	RefreshErr error
}

// RefreshOAuth2TokenWithLease refreshes an expired MCP user token while
// holding the row's refresh lease, so concurrent callers on any replica
// never spend the same refresh token twice. Under the lease it reads the
// row again: a row that another caller refreshed or marked is returned
// without a provider call. An unchanged row after a wait for another
// holder means that holder did not change the row, so it returns
// LeasedRefreshFailed without a provider call rather than retrying the
// provider once per waiting caller. Otherwise it refreshes with the
// leased row's refresh token, then saves any successful answer or marks
// a permanent failure, both only while the row and the lease are
// unchanged.
//
// ctx carries the database subject for the lease, the save and the mark.
// The lease wait ends with ctx, but the refresh, the save and the release
// run on detached contexts that always end before the lease does.
// Release errors are logged, never returned. Returned errors are
// database errors.
func RefreshOAuth2TokenWithLease(
	ctx context.Context,
	logger slog.Logger,
	store RefreshLeaseStore,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
	opts LeasedRefreshOptions,
) (LeasedRefresh, error) {
	if oauth2TokenFromRow(tok).Valid() {
		return LeasedRefresh{Outcome: LeasedRefreshNotNeeded, Token: tok}, nil
	}
	var maxWait time.Duration
	if opts.WaitForLease {
		maxWait = refreshLeaseLength
	}
	return withRefreshLease(ctx, logger, store, tok, maxWait,
		func(ctx context.Context, leased database.MCPServerUserToken, waited bool) (LeasedRefresh, error) {
			if expiredTokenChanged(tok, leased) {
				return LeasedRefresh{Outcome: LeasedRefreshChanged, Token: leased}, nil
			}
			if waited {
				return LeasedRefresh{Outcome: LeasedRefreshFailed, Token: leased, RefreshErr: errLeaseHolderLeftTokenUnchanged}, nil
			}
			return refreshLeasedToken(ctx, store, httpClient, cfg, leased)
		})
}

// errLeaseHolderLeftTokenUnchanged is the refresh error of a caller that
// waited for another holder's lease and found the row unchanged.
var errLeaseHolderLeftTokenUnchanged = xerrors.New("refresh oauth2 token: another refresh of this token held the lease and did not change it")

// withRefreshLease runs fn on tok's row while holding its refresh lease,
// then releases the lease. It retries a held lease for at most maxWait;
// with no time left a held lease returns LeasedRefreshLeaseHeld. A
// deleted row returns LeasedRefreshLost. fn gets the row read under the
// lease, a detached context that ends before the lease does, and whether
// another caller held the lease first.
func withRefreshLease(
	ctx context.Context,
	logger slog.Logger,
	store RefreshLeaseStore,
	tok database.MCPServerUserToken,
	maxWait time.Duration,
	fn func(ctx context.Context, leased database.MCPServerUserToken, waited bool) (LeasedRefresh, error),
) (LeasedRefresh, error) {
	waitCtx, cancelWait := context.WithTimeout(ctx, maxWait)
	defer cancelWait()
	backoff := retry.New(refreshLeaseInitialBackoff, refreshLeaseMaxBackoff)
	var (
		waited        bool
		refreshCtx    context.Context
		cancelRefresh context.CancelFunc
		leased        database.MCPServerUserToken
	)
	for {
		// The budget starts before the acquire, so it ends before a lease
		// granted by this acquire does.
		refreshCtx, cancelRefresh = context.WithTimeout(context.WithoutCancel(ctx), refreshBudget)
		var err error
		leased, err = store.AcquireMCPServerUserTokenRefreshLease(refreshCtx, database.AcquireMCPServerUserTokenRefreshLeaseParams{
			ID:        tok.ID,
			TimeoutMs: refreshLeaseLength.Milliseconds(),
		})
		if err == nil {
			break
		}
		cancelRefresh()
		switch {
		case database.IsCheckViolation(err, refreshLeaseActiveConstraint):
			waited = true
			// The backoff's first delay is zero, so it can retry even on
			// an expired waitCtx; a zero maxWait must not retry at all.
			if maxWait <= 0 || !backoff.Wait(waitCtx) {
				return LeasedRefresh{Outcome: LeasedRefreshLeaseHeld, Token: tok}, nil
			}
		case errors.Is(err, sql.ErrNoRows):
			return LeasedRefresh{Outcome: LeasedRefreshLost, Token: tok}, nil
		default:
			return LeasedRefresh{}, xerrors.Errorf("acquire MCP oauth2 token refresh lease: %w", err)
		}
	}
	defer cancelRefresh()
	defer releaseRefreshLease(ctx, logger, store, leased)
	return fn(refreshCtx, leased, waited)
}

// refreshLeasedToken refreshes and saves leased, the row read under the
// lease.
func refreshLeasedToken(
	ctx context.Context,
	store RefreshLeaseStore,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	leased database.MCPServerUserToken,
) (LeasedRefresh, error) {
	result, refreshErr := refreshOAuth2Token(ctx, httpClient, cfg, leased)
	if refreshErr != nil {
		if !isPermanentRefreshError(refreshErr) {
			return LeasedRefresh{Outcome: LeasedRefreshFailed, Token: leased, RefreshErr: refreshErr}, nil
		}
		return markRefreshFailure(ctx, store, leased, refreshErr)
	}

	var expiry sql.NullTime
	if !result.Expiry.IsZero() {
		expiry = sql.NullTime{Time: result.Expiry, Valid: true}
	}
	saved, err := store.UpdateMCPServerUserTokenFromRefresh(ctx, database.UpdateMCPServerUserTokenFromRefreshParams{
		ID:                leased.ID,
		UpdatedAt:         leased.UpdatedAt,
		AccessToken:       result.AccessToken,
		AccessTokenKeyID:  sql.NullString{},
		RefreshToken:      result.RefreshToken,
		RefreshTokenKeyID: sql.NullString{},
		TokenType:         result.TokenType,
		Expiry:            expiry,
		Lease:             leased.RefreshLeaseExpiresAt.Time,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return LeasedRefresh{Outcome: LeasedRefreshLost, Token: leased}, nil
	}
	if err != nil {
		return LeasedRefresh{}, xerrors.Errorf("persist refreshed MCP oauth2 token: %w", err)
	}
	return LeasedRefresh{Outcome: LeasedRefreshSaved, Token: saved}, nil
}

// markRefreshFailure marks leased, the row read under the refresh lease,
// for reconnect after refreshErr, and returns the mark with refreshErr.
func markRefreshFailure(
	ctx context.Context,
	store RefreshLeaseStore,
	leased database.MCPServerUserToken,
	refreshErr error,
) (LeasedRefresh, error) {
	marked, err := markLeasedToken(ctx, store, leased, RefreshFailureReason(refreshErr))
	if err != nil {
		return LeasedRefresh{}, err
	}
	marked.RefreshErr = refreshErr
	return marked, nil
}

// markLeasedToken marks leased, the row read under the refresh lease,
// for reconnect with reason, only while the row and the lease are
// unchanged.
func markLeasedToken(
	ctx context.Context,
	store RefreshLeaseStore,
	leased database.MCPServerUserToken,
	reason string,
) (LeasedRefresh, error) {
	marked, err := store.MarkMCPServerUserTokenRefreshFailure(ctx, database.MarkMCPServerUserTokenRefreshFailureParams{
		ID:                        leased.ID,
		UpdatedAt:                 leased.UpdatedAt,
		OauthRefreshFailureReason: reason,
		Lease:                     leased.RefreshLeaseExpiresAt.Time,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return LeasedRefresh{Outcome: LeasedRefreshLost, Token: leased}, nil
	}
	if err != nil {
		return LeasedRefresh{}, xerrors.Errorf("mark MCP oauth2 token refresh failure: %w", err)
	}
	return LeasedRefresh{Outcome: LeasedRefreshMarkedDead, Token: marked}, nil
}

// releaseRefreshLease ends the lease only if it is still the current
// one.
func releaseRefreshLease(
	ctx context.Context,
	logger slog.Logger,
	store RefreshLeaseStore,
	leased database.MCPServerUserToken,
) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshLeaseReleaseTimeout)
	defer cancel()
	err := store.ReleaseMCPServerUserTokenRefreshLease(ctx, database.ReleaseMCPServerUserTokenRefreshLeaseParams{
		ID:                    leased.ID,
		RefreshLeaseExpiresAt: leased.RefreshLeaseExpiresAt,
	})
	// A deleted row has no lease left to release.
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Warn(ctx, "failed to release MCP oauth2 token refresh lease",
			slog.F("token_id", leased.ID),
			slog.Error(err),
		)
	}
}
