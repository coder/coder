package gitsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/quartz"
)

const (
	// DiffStatusTTL is how long a successfully refreshed
	// diff status remains fresh before becoming stale again.
	DiffStatusTTL = 120 * time.Second

	// defaultConcurrency is the maximum number of HTTP calls
	// made in parallel during a single Refresh batch.
	defaultConcurrency = 10
)

// ProviderResolver maps a git remote origin to the gitprovider
// that handles it. Returns nil if no provider matches.
type ProviderResolver func(origin string) gitprovider.Provider

var ErrNoTokenAvailable error = errors.New("no token available")

// ErrRateLimitSkipped indicates that a row was skipped because
// a prior request in the same group hit a rate limit.
var ErrRateLimitSkipped error = errors.New("skipped due to rate limit")

// TokenResolver obtains the user's git access token for a given
// remote origin. Should return nil if no token is available, in
// which case ErrNoTokenAvailable will be returned.
type TokenResolver func(
	ctx context.Context,
	userID uuid.UUID,
	origin string,
) (*string, error)

// RefresherOption configures a Refresher.
type RefresherOption func(*Refresher)

// WithConcurrency sets the maximum number of concurrent HTTP
// calls per Refresh batch. Defaults to defaultConcurrency.
func WithConcurrency(n int) RefresherOption {
	return func(r *Refresher) {
		if n > 0 {
			r.concurrency = n
		}
	}
}

// Refresher contains the stateless business logic for fetching
// fresh PR data from a git provider given a stale
// database.ChatDiffStatus row.
type Refresher struct {
	providers   ProviderResolver
	tokens      TokenResolver
	logger      slog.Logger
	clock       quartz.Clock
	concurrency int
}

// NewRefresher creates a Refresher with the given dependency
// functions.
func NewRefresher(
	providers ProviderResolver,
	tokens TokenResolver,
	logger slog.Logger,
	clock quartz.Clock,
	opts ...RefresherOption,
) *Refresher {
	r := &Refresher{
		providers:   providers,
		tokens:      tokens,
		logger:      logger,
		clock:       clock,
		concurrency: defaultConcurrency,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// RefreshRequest pairs a stale row with the chat owner who
// holds the git token needed for API calls.
type RefreshRequest struct {
	Row     database.ChatDiffStatus
	OwnerID uuid.UUID
}

// RefreshResult is the outcome for a single row.
//   - Params != nil, Error == nil  → success, caller should upsert.
//   - Params == nil, Error == nil  → no PR yet, caller should skip.
//   - Params == nil, Error != nil  → row-level failure.
type RefreshResult struct {
	Request RefreshRequest
	Params  *database.UpsertChatDiffStatusParams
	Error   error
}

// groupKey identifies a unique (owner, origin) pair so that
// provider and token resolution happen once per group.
type groupKey struct {
	ownerID uuid.UUID
	origin  string
}

// resolvedGroup holds the pre-resolved provider and token for
// a group of requests that share the same (owner, origin).
type resolvedGroup struct {
	provider gitprovider.Provider
	token    string
	indices  []int
}

// Refresh fetches fresh PR data for a batch of stale rows.
// Rows are grouped internally by (ownerID, origin) so that
// provider and token resolution happen once per group. HTTP
// calls within and across groups run concurrently, bounded by
// the Refresher's concurrency limit.
//
// A top-level error is returned only when the entire batch
// fails catastrophically. Per-row outcomes are in the
// returned RefreshResult slice (one per input request, same
// order).
func (r *Refresher) Refresh(
	ctx context.Context,
	requests []RefreshRequest,
) ([]RefreshResult, error) {
	results := make([]RefreshResult, len(requests))
	for i, req := range requests {
		results[i].Request = req
	}

	// Group request indices by (ownerID, origin).
	groups := make(map[groupKey][]int)
	for i, req := range requests {
		key := groupKey{
			ownerID: req.OwnerID,
			origin:  req.Row.GitRemoteOrigin,
		}
		groups[key] = append(groups[key], i)
	}

	// Pre-resolve providers and tokens sequentially. This is
	// fast (DB + in-memory config lookups) and avoids
	// duplicate resolution for rows in the same group.
	var resolved []resolvedGroup
	for key, indices := range groups {
		provider := r.providers(key.origin)
		if provider == nil {
			err := xerrors.Errorf("no provider for origin %q", key.origin)
			for _, i := range indices {
				results[i].Error = err
			}
			continue
		}

		token, err := r.tokens(ctx, key.ownerID, key.origin)
		if err != nil {
			err = xerrors.Errorf("resolve token: %w", err)
		} else if token == nil || len(*token) == 0 {
			err = ErrNoTokenAvailable
		}
		if err != nil {
			for _, i := range indices {
				results[i].Error = err
			}
			continue
		}

		resolved = append(resolved, resolvedGroup{
			provider: provider,
			token:    *token,
			indices:  indices,
		})
	}

	// Process all HTTP calls concurrently with a shared
	// semaphore. Each group tracks rate-limit errors
	// independently so that a limit hit on one provider
	// doesn't stall requests to other providers.
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup

	for _, grp := range resolved {
		var rateLimitErr atomic.Pointer[gitprovider.RateLimitError]

		for _, idx := range grp.indices {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Best-effort rate-limit check before acquiring
				// the semaphore to avoid unnecessary blocking.
				if rl := rateLimitErr.Load(); rl != nil {
					results[idx] = RefreshResult{
						Request: requests[idx],
						Error:   fmt.Errorf("%w: %w", ErrRateLimitSkipped, rl),
					}
					return
				}

				// Acquire semaphore slot.
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					results[idx] = RefreshResult{
						Request: requests[idx],
						Error:   ctx.Err(),
					}
					return
				}

				// Best-effort rate-limit check after acquiring
				// in case it was set while we waited.
				if rl := rateLimitErr.Load(); rl != nil {
					results[idx] = RefreshResult{
						Request: requests[idx],
						Error:   fmt.Errorf("%w: %w", ErrRateLimitSkipped, rl),
					}
					return
				}

				params, err := r.refreshOne(ctx, grp.provider, grp.token, requests[idx].Row)
				results[idx] = RefreshResult{
					Request: requests[idx],
					Params:  params,
					Error:   err,
				}

				var rlErr *gitprovider.RateLimitError
				if errors.As(err, &rlErr) {
					rateLimitErr.Store(rlErr)
				}
			}()
		}
	}

	wg.Wait()
	return results, nil
}

// refreshOne processes a single row using an already-resolved
// provider and token.
func (r *Refresher) refreshOne(
	ctx context.Context,
	provider gitprovider.Provider,
	token string,
	row database.ChatDiffStatus,
) (*database.UpsertChatDiffStatusParams, error) {
	var ref gitprovider.PRRef
	var prURL string

	if row.Url.Valid && row.Url.String != "" {
		// Row already has a PR URL — parse it directly.
		parsed, ok := provider.ParsePullRequestURL(row.Url.String)
		if !ok {
			return nil, xerrors.Errorf("parse pull request URL %q", row.Url.String)
		}
		ref = parsed
		prURL = row.Url.String
	} else {
		// No PR URL — resolve owner/repo from the remote origin,
		// then look up the open PR for this branch.
		owner, repo, _, ok := provider.ParseRepositoryOrigin(row.GitRemoteOrigin)
		if !ok {
			return nil, xerrors.Errorf("parse repository origin %q", row.GitRemoteOrigin)
		}

		resolved, err := provider.ResolveBranchPullRequest(ctx, token, gitprovider.BranchRef{
			Owner:  owner,
			Repo:   repo,
			Branch: row.GitBranch,
		})
		if err != nil {
			return nil, xerrors.Errorf("resolve branch pull request: %w", err)
		}
		if resolved == nil {
			// No PR exists yet for this branch.
			return nil, nil
		}
		ref = *resolved
		prURL = provider.BuildPullRequestURL(ref)
	}

	status, err := provider.FetchPullRequestStatus(ctx, token, ref)
	if err != nil {
		return nil, xerrors.Errorf("fetch pull request status: %w", err)
	}

	now := r.clock.Now().UTC()
	params := &database.UpsertChatDiffStatusParams{
		ChatID: row.ChatID,
		Url:    sql.NullString{String: prURL, Valid: prURL != ""},
		PullRequestState: sql.NullString{
			String: string(status.State),
			Valid:  status.State != "",
		},
		PullRequestTitle: status.Title,
		PullRequestDraft: status.Draft,
		ChangesRequested: status.ChangesRequested,
		Additions:        status.DiffStats.Additions,
		Deletions:        status.DiffStats.Deletions,
		ChangedFiles:     status.DiffStats.ChangedFiles,
		RefreshedAt:      now,
		StaleAt:          now.Add(DiffStatusTTL),
	}

	return params, nil
}
