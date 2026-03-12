package gitsync_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/gitsync"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/quartz"
)

// mockProvider implements gitprovider.Provider with function fields
// so each test can wire only the methods it needs. Any method left
// nil panics with "unexpected call".
type mockProvider struct {
	fetchPullRequestStatus  func(ctx context.Context, token string, ref gitprovider.PRRef) (*gitprovider.PRStatus, error)
	resolveBranchPR         func(ctx context.Context, token string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error)
	fetchPullRequestDiff    func(ctx context.Context, token string, ref gitprovider.PRRef) (string, error)
	fetchBranchDiff         func(ctx context.Context, token string, ref gitprovider.BranchRef) (string, error)
	parseRepositoryOrigin   func(raw string) (string, string, string, bool)
	parsePullRequestURL     func(raw string) (gitprovider.PRRef, bool)
	normalizePullRequestURL func(raw string) string
	buildBranchURL          func(owner, repo, branch string) string
	buildRepositoryURL      func(owner, repo string) string
	buildPullRequestURL     func(ref gitprovider.PRRef) string
}

func (m *mockProvider) FetchPullRequestStatus(ctx context.Context, token string, ref gitprovider.PRRef) (*gitprovider.PRStatus, error) {
	if m.fetchPullRequestStatus == nil {
		panic("unexpected call to FetchPullRequestStatus")
	}
	return m.fetchPullRequestStatus(ctx, token, ref)
}

func (m *mockProvider) ResolveBranchPullRequest(ctx context.Context, token string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
	if m.resolveBranchPR == nil {
		panic("unexpected call to ResolveBranchPullRequest")
	}
	return m.resolveBranchPR(ctx, token, ref)
}

func (m *mockProvider) FetchPullRequestDiff(ctx context.Context, token string, ref gitprovider.PRRef) (string, error) {
	if m.fetchPullRequestDiff == nil {
		panic("unexpected call to FetchPullRequestDiff")
	}
	return m.fetchPullRequestDiff(ctx, token, ref)
}

func (m *mockProvider) FetchBranchDiff(ctx context.Context, token string, ref gitprovider.BranchRef) (string, error) {
	if m.fetchBranchDiff == nil {
		panic("unexpected call to FetchBranchDiff")
	}
	return m.fetchBranchDiff(ctx, token, ref)
}

func (m *mockProvider) ParseRepositoryOrigin(raw string) (string, string, string, bool) {
	if m.parseRepositoryOrigin == nil {
		panic("unexpected call to ParseRepositoryOrigin")
	}
	return m.parseRepositoryOrigin(raw)
}

func (m *mockProvider) ParsePullRequestURL(raw string) (gitprovider.PRRef, bool) {
	if m.parsePullRequestURL == nil {
		panic("unexpected call to ParsePullRequestURL")
	}
	return m.parsePullRequestURL(raw)
}

func (m *mockProvider) NormalizePullRequestURL(raw string) string {
	if m.normalizePullRequestURL == nil {
		panic("unexpected call to NormalizePullRequestURL")
	}
	return m.normalizePullRequestURL(raw)
}

func (m *mockProvider) BuildBranchURL(owner, repo, branch string) string {
	if m.buildBranchURL == nil {
		panic("unexpected call to BuildBranchURL")
	}
	return m.buildBranchURL(owner, repo, branch)
}

func (m *mockProvider) BuildRepositoryURL(owner, repo string) string {
	if m.buildRepositoryURL == nil {
		panic("unexpected call to BuildRepositoryURL")
	}
	return m.buildRepositoryURL(owner, repo)
}

func (m *mockProvider) BuildPullRequestURL(ref gitprovider.PRRef) string {
	if m.buildPullRequestURL == nil {
		panic("unexpected call to BuildPullRequestURL")
	}
	return m.buildPullRequestURL(ref)
}

func TestRefresher_WithPRURL(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parsePullRequestURL: func(raw string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42}, true
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return &gitprovider.PRStatus{
				State: gitprovider.PRStateOpen,
				DiffStats: gitprovider.DiffStats{
					Additions:    10,
					Deletions:    5,
					ChangedFiles: 3,
				},
			}, nil
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	chatID := uuid.New()
	row := database.ChatDiffStatus{
		ChatID:          chatID,
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/42", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	require.NoError(t, res.Error)
	require.NotNil(t, res.Params)

	assert.Equal(t, chatID, res.Params.ChatID)
	assert.Equal(t, "open", res.Params.PullRequestState.String)
	assert.True(t, res.Params.PullRequestState.Valid)
	assert.Equal(t, int32(10), res.Params.Additions)
	assert.Equal(t, int32(5), res.Params.Deletions)
	assert.Equal(t, int32(3), res.Params.ChangedFiles)

	// StaleAt should be ~120s after RefreshedAt.
	diff := res.Params.StaleAt.Sub(res.Params.RefreshedAt)
	assert.InDelta(t, 120, diff.Seconds(), 5)
}

func TestRefresher_BranchResolvesToPR(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parseRepositoryOrigin: func(_ string) (string, string, string, bool) {
			return "org", "repo", "https://github.com/org/repo", true
		},
		resolveBranchPR: func(_ context.Context, _ string, _ gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return &gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 7}, nil
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return &gitprovider.PRStatus{State: gitprovider.PRStateOpen}, nil
		},
		buildPullRequestURL: func(_ gitprovider.PRRef) string {
			return "https://github.com/org/repo/pull/7"
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	require.NoError(t, res.Error)
	require.NotNil(t, res.Params)

	assert.Contains(t, res.Params.Url.String, "pull/7")
	assert.True(t, res.Params.Url.Valid)
	assert.Equal(t, "open", res.Params.PullRequestState.String)
}

func TestRefresher_BranchNoPRYet(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parseRepositoryOrigin: func(_ string) (string, string, string, bool) {
			return "org", "repo", "https://github.com/org/repo", true
		},
		resolveBranchPR: func(_ context.Context, _ string, _ gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return nil, nil
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.NoError(t, res.Error)
	assert.Nil(t, res.Params)
}

func TestRefresher_NoProviderForOrigin(t *testing.T) {
	t.Parallel()

	providers := func(_ string) gitprovider.Provider { return nil }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{String: "https://example.com/pr/1", Valid: true},
		GitRemoteOrigin: "https://example.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.Nil(t, res.Params)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "no provider")
}

func TestRefresher_TokenResolutionFails(t *testing.T) {
	t.Parallel()

	var fetchCalled atomic.Bool
	mp := &mockProvider{
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			fetchCalled.Store(true)
			return nil, errors.New("should not be called")
		},
		parsePullRequestURL: func(_ string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1}, true
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return nil, errors.New("token lookup failed")
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.Nil(t, res.Params)
	require.Error(t, res.Error)
	assert.False(t, fetchCalled.Load(), "FetchPullRequestStatus should not be called when token resolution fails")
}

func TestRefresher_EmptyToken(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref(""), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.Nil(t, res.Params)
	require.ErrorIs(t, res.Error, gitsync.ErrNoTokenAvailable)
}

func TestRefresher_ProviderFetchFails(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parsePullRequestURL: func(_ string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42}, true
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return nil, errors.New("api error")
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/42", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.Nil(t, res.Params)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "api error")
}

func TestRefresher_PRURLParseFailure(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parsePullRequestURL: func(_ string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{}, false
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	row := database.ChatDiffStatus{
		ChatID:          uuid.New(),
		Url:             sql.NullString{String: "https://github.com/org/repo/not-a-pr", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	assert.Nil(t, res.Params)
	require.Error(t, res.Error)
}

func TestRefresher_BatchGroupsByOwnerAndOrigin(t *testing.T) {
	t.Parallel()

	mp := &mockProvider{
		parsePullRequestURL: func(_ string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1}, true
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return &gitprovider.PRStatus{State: gitprovider.PRStateOpen}, nil
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }

	var tokenCalls atomic.Int32
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		tokenCalls.Add(1)
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	ownerID := uuid.New()
	originA := "https://github.com/org/repo"
	originB := "https://gitlab.com/org/repo"

	requests := []gitsync.RefreshRequest{
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
				GitRemoteOrigin: originA,
				GitBranch:       "feature-1",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
				GitRemoteOrigin: originA,
				GitBranch:       "feature-2",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://gitlab.com/org/repo/pull/1", Valid: true},
				GitRemoteOrigin: originB,
				GitBranch:       "feature-3",
			},
			OwnerID: ownerID,
		},
	}

	results, err := r.Refresh(context.Background(), requests)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, res := range results {
		require.NoError(t, res.Error, "result[%d] should not have an error", i)
		require.NotNil(t, res.Params, "result[%d] should have params", i)
	}

	// Two distinct (ownerID, origin) groups → exactly 2 token
	// resolution calls.
	assert.Equal(t, int32(2), tokenCalls.Load(),
		"TokenResolver should be called once per (owner, origin) group")
}

func TestRefresher_UsesInjectedClock(t *testing.T) {
	t.Parallel()

	mClock := quartz.NewMock(t)
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	mClock.Set(fixedTime)

	mp := &mockProvider{
		parsePullRequestURL: func(raw string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42}, true
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, _ gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return &gitprovider.PRStatus{
				State: gitprovider.PRStateOpen,
				DiffStats: gitprovider.DiffStats{
					Additions:    10,
					Deletions:    5,
					ChangedFiles: 3,
				},
			}, nil
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), mClock)

	chatID := uuid.New()
	row := database.ChatDiffStatus{
		ChatID:          chatID,
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/42", Valid: true},
		GitRemoteOrigin: "https://github.com/org/repo",
		GitBranch:       "feature",
	}

	ownerID := uuid.New()
	results, err := r.Refresh(context.Background(), []gitsync.RefreshRequest{
		{Row: row, OwnerID: ownerID},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]

	require.NoError(t, res.Error)
	require.NotNil(t, res.Params)

	// The mock clock is deterministic, so times must be exact.
	assert.Equal(t, fixedTime, res.Params.RefreshedAt)
	assert.Equal(t, fixedTime.Add(gitsync.DiffStatusTTL), res.Params.StaleAt)
}

func TestRefresher_RateLimitSkipsRemainingInGroup(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	mp := &mockProvider{
		parsePullRequestURL: func(raw string) (gitprovider.PRRef, bool) {
			var num int
			switch {
			case strings.HasSuffix(raw, "/pull/1"):
				num = 1
			case strings.HasSuffix(raw, "/pull/2"):
				num = 2
			case strings.HasSuffix(raw, "/pull/3"):
				num = 3
			default:
				return gitprovider.PRRef{}, false
			}
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: num}, true
		},
		fetchPullRequestStatus: func(_ context.Context, _ string, ref gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			call := callCount.Add(1)
			switch call {
			case 1:
				// First call succeeds.
				return &gitprovider.PRStatus{
					State: gitprovider.PRStateOpen,
					DiffStats: gitprovider.DiffStats{
						Additions:    5,
						Deletions:    2,
						ChangedFiles: 1,
					},
				}, nil
			case 2:
				// Second call hits rate limit.
				return nil, &gitprovider.RateLimitError{
					RetryAfter: time.Now().Add(60 * time.Second),
				}
			default:
				// Third call should never happen.
				t.Fatal("FetchPullRequestStatus called more than 2 times")
				return nil, nil
			}
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }
	tokens := func(_ context.Context, _ uuid.UUID, _ string) (*string, error) {
		return ptr.Ref("test-token"), nil
	}

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	ownerID := uuid.New()
	origin := "https://github.com/org/repo"

	requests := []gitsync.RefreshRequest{
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
				GitRemoteOrigin: origin,
				GitBranch:       "feat-1",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/2", Valid: true},
				GitRemoteOrigin: origin,
				GitBranch:       "feat-2",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/3", Valid: true},
				GitRemoteOrigin: origin,
				GitBranch:       "feat-3",
			},
			OwnerID: ownerID,
		},
	}

	results, err := r.Refresh(context.Background(), requests)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Row 0: success.
	assert.NoError(t, results[0].Error)
	assert.NotNil(t, results[0].Params)

	// Row 1: rate-limited.
	require.Error(t, results[1].Error)
	var rlErr1 *gitprovider.RateLimitError
	assert.True(t, errors.As(results[1].Error, &rlErr1),
		"result[1] error should be *RateLimitError")

	// Row 2: skipped due to rate limit.
	require.Error(t, results[2].Error)
	var rlErr2 *gitprovider.RateLimitError
	assert.True(t, errors.As(results[2].Error, &rlErr2),
		"result[2] error should wrap *RateLimitError")
	assert.Contains(t, results[2].Error.Error(), "skipped")

	// Provider should have been called exactly twice.
	assert.Equal(t, int32(2), callCount.Load(),
		"FetchPullRequestStatus should be called exactly 2 times")
}

func TestRefresher_CorrectTokenPerOrigin(t *testing.T) {
	t.Parallel()

	var tokenCalls atomic.Int32
	tokens := func(_ context.Context, _ uuid.UUID, origin string) (*string, error) {
		tokenCalls.Add(1)
		switch {
		case strings.Contains(origin, "github.com"):
			return ptr.Ref("gh-public-token"), nil
		case strings.Contains(origin, "ghes.corp.com"):
			return ptr.Ref("ghe-private-token"), nil
		default:
			return nil, fmt.Errorf("unexpected origin: %s", origin)
		}
	}

	// Track which token each FetchPullRequestStatus call received,
	// keyed by chat ID. We pass the chat ID through the PRRef.Number
	// field (unique per request) so FetchPullRequestStatus can
	// identify which row it's processing.
	var mu sync.Mutex
	tokensByPR := make(map[int]string)

	mp := &mockProvider{
		parsePullRequestURL: func(raw string) (gitprovider.PRRef, bool) {
			// Extract a unique PR number from the URL to identify
			// each row inside FetchPullRequestStatus.
			var num int
			switch {
			case strings.HasSuffix(raw, "/pull/1"):
				num = 1
			case strings.HasSuffix(raw, "/pull/2"):
				num = 2
			case strings.HasSuffix(raw, "/pull/10"):
				num = 10
			default:
				return gitprovider.PRRef{}, false
			}
			return gitprovider.PRRef{Owner: "org", Repo: "repo", Number: num}, true
		},
		fetchPullRequestStatus: func(_ context.Context, token string, ref gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			mu.Lock()
			tokensByPR[ref.Number] = token
			mu.Unlock()
			return &gitprovider.PRStatus{State: gitprovider.PRStateOpen}, nil
		},
	}

	providers := func(_ string) gitprovider.Provider { return mp }

	r := gitsync.NewRefresher(providers, tokens, slogtest.Make(t, nil), quartz.NewReal())

	ownerID := uuid.New()

	requests := []gitsync.RefreshRequest{
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
				GitRemoteOrigin: "https://github.com/org/repo",
				GitBranch:       "feature-1",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://github.com/org/repo/pull/2", Valid: true},
				GitRemoteOrigin: "https://github.com/org/repo",
				GitBranch:       "feature-2",
			},
			OwnerID: ownerID,
		},
		{
			Row: database.ChatDiffStatus{
				ChatID:          uuid.New(),
				Url:             sql.NullString{String: "https://ghes.corp.com/org/repo/pull/10", Valid: true},
				GitRemoteOrigin: "https://ghes.corp.com/org/repo",
				GitBranch:       "feature-3",
			},
			OwnerID: ownerID,
		},
	}

	results, err := r.Refresh(context.Background(), requests)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, res := range results {
		require.NoError(t, res.Error, "result[%d] should not have an error", i)
		require.NotNil(t, res.Params, "result[%d] should have params", i)
	}

	// github.com rows (PR #1 and #2) should use the public token.
	assert.Equal(t, "gh-public-token", tokensByPR[1],
		"github.com PR #1 should use gh-public-token")
	assert.Equal(t, "gh-public-token", tokensByPR[2],
		"github.com PR #2 should use gh-public-token")

	// ghes.corp.com row (PR #10) should use the GHE token.
	assert.Equal(t, "ghe-private-token", tokensByPR[10],
		"ghes.corp.com PR #10 should use ghe-private-token")

	// Token resolution should be called exactly twice — once per
	// (owner, origin) group.
	assert.Equal(t, int32(2), tokenCalls.Load(),
		"TokenResolver should be called once per (owner, origin) group")
}
