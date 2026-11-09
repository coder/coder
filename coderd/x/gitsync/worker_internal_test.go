package gitsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type acquiredChatDiffStatusesStore struct {
	Store
	rows []database.AcquireStaleChatDiffStatusesRow
}

func (s *acquiredChatDiffStatusesStore) AcquireStaleChatDiffStatuses(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
	return s.rows, nil
}

type cooldownBackoffCall struct {
	params       database.BackoffChatDiffStatusParams
	contextError error
	budget       time.Duration
	hasDeadline  bool
}

type cooldownStoreSpy struct {
	Store
	acquired int
	backoffs []cooldownBackoffCall
}

func (s *cooldownStoreSpy) AcquireStaleChatDiffStatuses(ctx context.Context, limit int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
	rows, err := s.Store.AcquireStaleChatDiffStatuses(ctx, limit)
	s.acquired += len(rows)
	return rows, err
}

func (s *cooldownStoreSpy) BackoffChatDiffStatus(ctx context.Context, arg database.BackoffChatDiffStatusParams) error {
	deadline, ok := ctx.Deadline()
	call := cooldownBackoffCall{
		params:       arg,
		contextError: ctx.Err(),
		hasDeadline:  ok,
	}
	if ok {
		call.budget = time.Until(deadline)
	}
	s.backoffs = append(s.backoffs, call)
	return s.Store.BackoffChatDiffStatus(ctx, arg)
}

func newCooldownWorkspace(t *testing.T, db database.Store, ownerID, organizationID uuid.UUID) uuid.UUID {
	t.Helper()
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: organizationID,
		CreatedBy:      ownerID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  organizationID,
		CreatedBy:       ownerID,
		ActiveVersionID: version.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		TemplateID:     template.ID,
	})
	return workspace.ID
}

type branchCallbackProvider struct {
	gitprovider.Provider
	resolve func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error)
}

func (p branchCallbackProvider) ResolveBranchPullRequest(ctx context.Context, token string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
	return p.resolve(ctx, token, ref)
}

func TestWorker_CooldownBackoffSurvivesTickCancellation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	otherOwner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	clock := quartz.NewMock(t)
	clock.Set(time.Now().UTC().Truncate(time.Second)).MustWait(ctx)
	const origin = "https://github.example/owner/repo"
	var limitedChat database.Chat
	for i, userID := range []uuid.UUID{owner.ID, otherOwner.ID} {
		workspaceID := newCooldownWorkspace(t, db, userID, org.ID)
		chat := dbgen.Chat(t, db, database.Chat{
			OwnerID:           userID,
			OrganizationID:    org.ID,
			WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: true},
			LastModelConfigID: model.ID,
		})
		_, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
			ChatID:          chat.ID,
			GitRemoteOrigin: origin,
			GitBranch:       fmt.Sprintf("branch-%d", i),
			StaleAt:         clock.Now().Add(-time.Minute),
		})
		require.NoError(t, err)
		if i == 0 {
			limitedChat = chat
		}
	}

	client := &http.Client{Transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected HTTP request")
		return nil, xerrors.New("unexpected HTTP request")
	})}
	baseProvider, err := gitprovider.New("github", "https://github.example", client, gitprovider.WithClock(clock))
	require.NoError(t, err)
	rateLimitObserved := make(chan struct{})
	slowRequestStarted := make(chan struct{})
	var limitedCalls atomic.Int32
	retryAfter := clock.Now().Add(35 * time.Minute)
	provider := branchCallbackProvider{
		Provider: baseProvider,
		resolve: func(ctx context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if ref.Branch == "branch-0" {
				if limitedCalls.Add(1) == 1 {
					close(rateLimitObserved)
				}
				return nil, &gitprovider.RateLimitError{RetryAfter: retryAfter}
			}
			close(slowRequestStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	newWorker := func(store Store) *Worker {
		refresher := NewRefresher(
			func(context.Context, string) gitprovider.Provider { return provider },
			func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil },
			logger, clock, WithConcurrency(2),
		)
		return NewWorker(store, refresher, nil, clock, logger)
	}
	store := &cooldownStoreSpy{Store: db}
	worker := newWorker(store)
	tickCtx, cancelTick := context.WithCancel(ctx)
	defer cancelTick()
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		worker.tick(tickCtx)
	}()
	testutil.TryReceive(ctx, t, rateLimitObserved)
	testutil.TryReceive(ctx, t, slowRequestStarted)
	cancelTick()
	testutil.TryReceive(ctx, t, tickDone)

	var limitedBackoff *cooldownBackoffCall
	for i := range store.backoffs {
		if store.backoffs[i].params.GitBranch == "branch-0" {
			limitedBackoff = &store.backoffs[i]
		}
	}
	require.NotNil(t, limitedBackoff)
	require.NoError(t, limitedBackoff.contextError, "the shared stale_at backoff must outlive tick cancellation")
	require.True(t, limitedBackoff.hasDeadline, "cooldown backoff must have its own timeout")
	require.Positive(t, limitedBackoff.budget)
	require.LessOrEqual(t, limitedBackoff.budget, 5*time.Second)
	statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{limitedChat.ID})
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, retryAfter, statuses[0].StaleAt.UTC())
	key := rateLimitKey{ownerID: owner.ID, host: "github.example"}
	require.Equal(t, retryAfter, worker.cooldowns[key])

	otherStore := &cooldownStoreSpy{Store: db}
	otherWorker := newWorker(otherStore)
	otherWorker.tick(ctx)
	require.Zero(t, otherStore.acquired, "stale_at must defer the existing row on another replica")
	require.Equal(t, int32(1), limitedCalls.Load())

	worker.MarkStale(ctx, MarkStaleParams{
		WorkspaceID: limitedChat.WorkspaceID.UUID,
		ChatID:      limitedChat.ID,
		Branch:      "branch-0",
		Origin:      origin,
	})
	worker.tick(ctx)
	require.Equal(t, int32(1), limitedCalls.Load(), "the long-lived worker must keep its cooldown after MarkStale")

	otherWorker.MarkStale(ctx, MarkStaleParams{
		WorkspaceID: limitedChat.WorkspaceID.UUID,
		ChatID:      limitedChat.ID,
		Branch:      "branch-0",
		Origin:      origin,
	})
	otherWorker.tick(ctx)
	require.Equal(t, 1, otherStore.acquired)
	require.Equal(t, int32(2), limitedCalls.Load(), "a replica learns a cooldown from its own first response")
	require.Equal(t, retryAfter, otherWorker.cooldowns[key])
}

func TestWorker_CooldownNeverShortens(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	ownerID := uuid.New()
	const origin = "https://github.example/owner/repo"
	longer := clock.Now().Add(30 * time.Minute)
	shorter := clock.Now().Add(15 * time.Minute)
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return([]database.AcquireStaleChatDiffStatusesRow{
		{
			ChatID:          uuid.New(),
			OwnerID:         ownerID,
			GitRemoteOrigin: origin,
			GitBranch:       "longer",
		},
		{
			ChatID:          uuid.New(),
			OwnerID:         ownerID,
			GitRemoteOrigin: origin,
			GitBranch:       "shorter",
		},
	}, nil)
	var backedOff []time.Time
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
		backedOff = append(backedOff, arg.StaleAt)
		return nil
	}).Times(2)
	client := &http.Client{Transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected HTTP request")
		return nil, xerrors.New("unexpected HTTP request")
	})}
	baseProvider, err := gitprovider.New("github", "https://github.example", client, gitprovider.WithClock(clock))
	require.NoError(t, err)
	bothStarted := make(chan struct{})
	var started atomic.Int32
	provider := branchCallbackProvider{
		Provider: baseProvider,
		resolve: func(_ context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if started.Add(1) == 2 {
				close(bothStarted)
			}
			testutil.TryReceive(ctx, t, bothStarted)
			retryAfter := shorter
			if ref.Branch == "longer" {
				retryAfter = longer
			}
			return nil, &gitprovider.RateLimitError{RetryAfter: retryAfter}
		},
	}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(
		func(context.Context, string) gitprovider.Provider { return provider },
		func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil },
		logger, clock, WithConcurrency(2),
	)
	worker := NewWorker(store, refresher, nil, clock, logger)
	worker.tick(ctx)
	require.Equal(t, longer, worker.cooldowns[rateLimitKey{ownerID: ownerID, host: "github.example"}])
	require.Equal(t, []time.Time{longer, longer}, backedOff)
}

func TestWorker_Cooldown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		header     string
		resetAfter time.Duration
		cooldown   time.Duration
	}{
		{
			name:       "PrimaryRateLimit",
			header:     "X-RateLimit-Reset",
			resetAfter: 30 * time.Minute,
			cooldown:   35 * time.Minute,
		},
		{
			name:       "SecondaryRateLimit",
			header:     "Retry-After",
			resetAfter: 15 * time.Minute,
			cooldown:   20 * time.Minute,
		},
		{
			name:       "FarFutureReset",
			header:     "X-RateLimit-Reset",
			resetAfter: 365 * 24 * time.Hour,
			cooldown:   65 * time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, _ := dbtestutil.NewDB(t)
			owner := dbgen.User(t, db, database.User{})
			otherOwner := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.ChatProvider(t, db, database.ChatProvider{})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
			workspaceID := newCooldownWorkspace(t, db, owner.ID, org.ID)
			chat := dbgen.Chat(t, db, database.Chat{
				OwnerID:           owner.ID,
				OrganizationID:    org.ID,
				WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: true},
				LastModelConfigID: model.ID,
			})

			// Keep scheduling timestamps before PostgreSQL NOW() so acquisition
			// stays eligible while the worker clock controls cooldown expiry.
			clock := quartz.NewMock(t)
			clock.Set(time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)).MustWait(ctx)
			start := clock.Now()
			retryAfter := start.Add(tc.cooldown)
			var limitedCalls, otherCalls, enterpriseCalls, gitlabCalls atomic.Int32
			var limited atomic.Bool
			limited.Store(true)

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Host {
				case "ghe.example.com":
					enterpriseCalls.Add(1)
				case "gitlab.com":
					gitlabCalls.Add(1)
				default:
					if r.Header.Get("Authorization") == "Bearer limited-token" {
						limitedCalls.Add(1)
						if limited.Load() {
							value := strconv.FormatInt(start.Add(tc.resetAfter).Unix(), 10)
							remaining := "0"
							if tc.header == "Retry-After" {
								value = strconv.FormatInt(int64(tc.resetAfter/time.Second), 10)
								remaining = "4990"
							}
							w.Header().Set(tc.header, value)
							w.Header().Set("X-RateLimit-Remaining", remaining)
							w.WriteHeader(http.StatusTooManyRequests)
							return
						}
					} else {
						otherCalls.Add(1)
					}
				}
				_, _ = w.Write([]byte(`[]`))
			}))
			t.Cleanup(server.Close)

			// Every provider request goes to the local server, never its API host.
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := server.Client()
			transport := client.Transport
			client.Transport = testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				r = r.Clone(r.Context())
				r.URL.Scheme = target.Scheme
				r.URL.Host = target.Host
				return transport.RoundTrip(r)
			})
			github, err := gitprovider.New("github", "", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			enterprise, err := gitprovider.New("github", "https://ghe.example.com/api/v3", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			gitlab, err := gitprovider.New("gitlab", "https://gitlab.com/api/v4", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			newWorker := func(store Store) *Worker {
				refresher := NewRefresher(
					func(_ context.Context, origin string) gitprovider.Provider {
						switch {
						case strings.Contains(origin, "ghe.example.com"):
							return enterprise
						case strings.Contains(origin, "gitlab.com"):
							return gitlab
						default:
							return github
						}
					},
					func(_ context.Context, userID uuid.UUID, _ string) (*string, error) {
						if userID == owner.ID {
							return new("limited-token"), nil
						}
						return new("other-token"), nil
					},
					logger, clock, WithConcurrency(1),
				)
				return NewWorker(store, refresher, nil, clock, logger)
			}
			addRef := func(chatID uuid.UUID, origin, branch string) {
				_, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
					ChatID:          chatID,
					GitRemoteOrigin: origin,
					GitBranch:       branch,
					StaleAt:         start.Add(-time.Minute),
				})
				require.NoError(t, err)
			}
			for i := range 4 {
				addRef(chat.ID, "https://github.com/owner/repo", fmt.Sprintf("branch-%d", i))
			}
			acquired, err := db.AcquireStaleChatDiffStatuses(ctx, 1)
			require.NoError(t, err)
			require.Len(t, acquired, 1)

			store := &cooldownStoreSpy{Store: db}
			worker := newWorker(store)
			worker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "two rows should be skipped in the same batch")
			key := rateLimitKey{ownerID: owner.ID, host: "github.com"}
			require.Equal(t, retryAfter, worker.cooldowns[key])

			// This replica acquired its row earlier and must learn its own cooldown.
			queuedWorker := newWorker(&acquiredChatDiffStatusesStore{Store: db, rows: acquired})
			queuedWorker.tick(ctx)
			require.Equal(t, int32(2), limitedCalls.Load())
			require.Equal(t, retryAfter, queuedWorker.cooldowns[key])
			statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{chat.ID})
			require.NoError(t, err)
			require.Len(t, statuses, 4)
			for _, status := range statuses {
				assert.Equal(t, retryAfter, status.StaleAt.UTC(), "direct, in-batch and cooldown-skipped rows share the reset")
			}

			worker.MarkStale(ctx, MarkStaleParams{
				WorkspaceID: workspaceID,
				ChatID:      chat.ID,
				Branch:      "branch-0",
				Origin:      "https://github.com/owner/repo",
			})
			statuses, err = db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{chat.ID})
			require.NoError(t, err)
			markedTimes := make(map[string]time.Time)
			for _, status := range statuses {
				markedTimes[status.GitBranch] = status.StaleAt.UTC()
			}
			require.Equal(t, clock.Now().Add(-time.Second), markedTimes["branch-0"], "MarkStale must reschedule the tracked ref")
			addRef(chat.ID, "https://github.com/owner/another-repo", "new-branch")
			addRef(chat.ID, "git@github.com:owner/repo.git", "ssh-branch")
			addRef(chat.ID, "https://ghe.example.com/owner/repo", "enterprise")
			addRef(chat.ID, "https://gitlab.com/owner/repo", "gitlab")
			otherChat := dbgen.Chat(t, db, database.Chat{
				OwnerID:           otherOwner.ID,
				OrganizationID:    org.ID,
				LastModelConfigID: model.ID,
			})
			addRef(otherChat.ID, "https://github.com/owner/repo", "other-owner")

			clock.Set(retryAfter.Add(-time.Second)).MustWait(ctx)
			queuedWorker.tick(ctx)
			worker.tick(ctx)
			require.Equal(t, int32(2), limitedCalls.Load(), "the workers must retain their cooldowns for SSH and new refs")
			require.Equal(t, int32(1), otherCalls.Load())
			require.Equal(t, int32(1), enterpriseCalls.Load(), "Enterprise diff-status refresh should not be in cooldown")
			require.Equal(t, int32(1), gitlabCalls.Load(), "GitLab diff-status refresh should not be in cooldown")
			require.Equal(t, retryAfter, worker.cooldowns[key])

			limited.Store(false)
			clock.Advance(time.Second).MustWait(ctx)
			worker.tick(ctx)
			require.Equal(t, int32(8), limitedCalls.Load(), "all six affected rows should retry at reset")
			require.Empty(t, worker.cooldowns, "expired cooldowns must be removed")
		})
	}
}
