package gitsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
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
	timeLeft     time.Duration
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
		call.timeLeft = time.Until(deadline)
	}
	s.backoffs = append(s.backoffs, call)
	return s.Store.BackoffChatDiffStatus(ctx, arg)
}

func (s *cooldownStoreSpy) backoff(t *testing.T, branch string) cooldownBackoffCall {
	t.Helper()
	for _, call := range s.backoffs {
		if call.params.GitBranch == branch {
			return call
		}
	}
	t.Fatalf("no backoff for branch %q", branch)
	return cooldownBackoffCall{}
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
	fetch   func(context.Context, string, gitprovider.PRRef) (*gitprovider.PRStatus, error)
}

func (p branchCallbackProvider) ResolveBranchPullRequest(ctx context.Context, token string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
	return p.resolve(ctx, token, ref)
}

func (p branchCallbackProvider) FetchPullRequestStatus(ctx context.Context, token string, ref gitprovider.PRRef) (*gitprovider.PRStatus, error) {
	if p.fetch != nil {
		return p.fetch(ctx, token, ref)
	}
	return p.Provider.FetchPullRequestStatus(ctx, token, ref)
}

func TestWorker_CooldownBackoffPrecedesOrdinaryResults(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	ownerID := uuid.New()
	const origin = "https://github.example/owner/repo"
	end := clock.Now().Add(35 * time.Minute)
	acquiredRows := []database.AcquireStaleChatDiffStatusesRow{
		{
			ChatID:          uuid.New(),
			OwnerID:         ownerID,
			GitRemoteOrigin: origin,
			GitBranch:       "successful",
		},
		{
			ChatID:          uuid.New(),
			OwnerID:         ownerID,
			GitRemoteOrigin: origin,
			GitBranch:       "limited",
		},
	}
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return(acquiredRows, nil)

	cooldownWritten := make(chan struct{})
	var savedBackoff database.BackoffChatDiffStatusParams
	recordBackoff := func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
		savedBackoff = arg
		close(cooldownWritten)
		return nil
	}
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).DoAndReturn(recordBackoff)

	ordinaryStarted := make(chan struct{})
	releaseOrdinary := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseOrdinary) })
	upsert := func(ctx context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
		close(ordinaryStarted)
		select {
		case <-releaseOrdinary:
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		case <-ctx.Done():
			return database.ChatDiffStatus{}, ctx.Err()
		}
	}
	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).DoAndReturn(upsert)

	client := &http.Client{Transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, xerrors.New("unexpected HTTP request")
	})}
	baseProvider, err := gitprovider.New("github", "https://github.example", client, gitprovider.WithClock(clock))
	require.NoError(t, err)
	successfulFetch := make(chan struct{})
	provider := branchCallbackProvider{
		Provider: baseProvider,
		resolve: func(ctx context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if ref.Branch == "successful" {
				return &gitprovider.PRRef{Owner: "owner", Repo: "repo", Number: 1}, nil
			}
			select {
			case <-successfulFetch:
				return nil, &gitprovider.RateLimitError{RetryAfter: end}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		fetch: func(context.Context, string, gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			close(successfulFetch)
			return &gitprovider.PRStatus{State: gitprovider.PRStateOpen, PRNumber: 1}, nil
		},
	}
	providers := func(context.Context, string) gitprovider.Provider { return provider }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil }
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(
		providers,
		tokens,
		logger,
		clock,
		WithConcurrency(2),
	)
	worker := NewWorker(store, refresher, nil, clock, logger)
	tickDone := make(chan struct{})

	go func() {
		defer close(tickDone)
		worker.tick(ctx)
	}()
	defer func() {
		release()
		testutil.TryReceive(ctx, t, tickDone)
	}()

	testutil.TryReceive(ctx, t, ordinaryStarted)
	select {
	case <-cooldownWritten:
	default:
		t.Fatal("cooldown backoff must finish before slow ordinary result processing")
	}
	require.Equal(t, acquiredRows[1].ChatID, savedBackoff.ChatID)
	require.Equal(t, end, savedBackoff.StaleAt)
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
	for branch, userID := range map[string]uuid.UUID{
		"limited": owner.ID,
		"blocked": otherOwner.ID,
	} {
		chat := dbgen.Chat(t, db, database.Chat{
			OwnerID:           userID,
			OrganizationID:    org.ID,
			LastModelConfigID: model.ID,
		})
		_, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
			ChatID:          chat.ID,
			GitRemoteOrigin: origin,
			GitBranch:       branch,
			StaleAt:         clock.Now().Add(-time.Minute),
		})
		require.NoError(t, err)
		if branch == "limited" {
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
	retryAfter := clock.Now().Add(35 * time.Minute)
	provider := branchCallbackProvider{
		Provider: baseProvider,
		resolve: func(ctx context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if ref.Branch == "limited" {
				close(rateLimitObserved)
				return nil, &gitprovider.RateLimitError{RetryAfter: retryAfter}
			}
			close(slowRequestStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	providers := func(context.Context, string) gitprovider.Provider { return provider }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil }
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(
		providers,
		tokens,
		logger,
		clock,
		WithConcurrency(2),
	)
	store := &cooldownStoreSpy{Store: db}
	worker := NewWorker(store, refresher, nil, clock, logger)
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

	limitedBackoff := store.backoff(t, "limited")
	require.NoError(t, limitedBackoff.contextError, "the stale_at write must outlive tick cancellation")
	require.True(t, limitedBackoff.hasDeadline, "cooldown backoff must have its own timeout")
	require.Positive(t, limitedBackoff.timeLeft)
	require.LessOrEqual(t, limitedBackoff.timeLeft, cooldownBackoffTimeout)

	statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{limitedChat.ID})
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, retryAfter, statuses[0].StaleAt.UTC())
	key := cooldownKey{ownerID: owner.ID, host: "github.example"}
	require.Equal(t, retryAfter, worker.cooldowns[key])
}

func TestWorker_CooldownNeverShortens(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	ownerID := uuid.New()
	const origin = "https://github.example/owner/repo"
	longer := clock.Now().Add(30 * time.Minute)
	shorter := clock.Now().Add(15 * time.Minute)
	acquiredRows := []database.AcquireStaleChatDiffStatusesRow{
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
	}
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return(acquiredRows, nil)

	var backedOff []time.Time
	recordBackoff := func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
		backedOff = append(backedOff, arg.StaleAt)
		return nil
	}
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).DoAndReturn(recordBackoff).Times(2)

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
		resolve: func(ctx context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if started.Add(1) == 2 {
				close(bothStarted)
			}

			// Both rows must be in flight before either can trigger a skip.
			select {
			case <-bothStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			retryAfter := shorter
			if ref.Branch == "longer" {
				retryAfter = longer
			}
			return nil, &gitprovider.RateLimitError{RetryAfter: retryAfter}
		},
	}
	providers := func(context.Context, string) gitprovider.Provider { return provider }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil }
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(
		providers,
		tokens,
		logger,
		clock,
		WithConcurrency(2),
	)
	worker := NewWorker(store, refresher, nil, clock, logger)

	worker.tick(ctx)

	require.Equal(t, longer, worker.cooldowns[cooldownKey{ownerID: ownerID, host: "github.example"}])
	require.Equal(t, []time.Time{longer, longer}, backedOff)
}

const cooldownOrigin = "https://github.com/owner/repo"

type cooldownFixture struct {
	t               *testing.T
	ctx             context.Context
	db              database.Store
	clock           *quartz.Mock
	ownerID         uuid.UUID
	organizationID  uuid.UUID
	modelID         uuid.UUID
	chat            database.Chat
	limited         atomic.Bool
	limitedCalls    atomic.Int32
	otherCalls      atomic.Int32
	enterpriseCalls atomic.Int32
	gitlabCalls     atomic.Int32
	newWorker       func(Store) *Worker
}

func newCooldownFixture(t *testing.T, now time.Time, retryHeader string, retryDelay time.Duration) *cooldownFixture {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	f := &cooldownFixture{
		t:              t,
		ctx:            testutil.Context(t, testutil.WaitLong),
		db:             db,
		clock:          quartz.NewMock(t),
		ownerID:        owner.ID,
		organizationID: org.ID,
		modelID:        model.ID,
	}
	f.clock.Set(now.UTC().Truncate(time.Second)).MustWait(f.ctx)
	f.chat = f.newChat(owner.ID)
	f.limited.Store(true)

	retryHeaders := make(http.Header)
	retryHeaders.Set(retryHeader, strconv.FormatInt(f.clock.Now().Add(retryDelay).Unix(), 10))
	retryHeaders.Set("X-RateLimit-Remaining", "0")
	if retryHeader == "Retry-After" {
		retryHeaders.Set(retryHeader, strconv.FormatInt(int64(retryDelay/time.Second), 10))
		// Secondary limits apply even when primary requests remain.
		retryHeaders.Set("X-RateLimit-Remaining", "4990")
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Host == "ghe.example.com":
			f.enterpriseCalls.Add(1)
		case r.Host == "gitlab.com":
			f.gitlabCalls.Add(1)
		case r.Header.Get("Authorization") != "Bearer limited-token":
			f.otherCalls.Add(1)
		default:
			f.limitedCalls.Add(1)
			if f.limited.Load() {
				for name, values := range retryHeaders {
					w.Header()[name] = values
				}
				w.WriteHeader(http.StatusTooManyRequests)
				return
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
	github, err := gitprovider.New("github", "", client, gitprovider.WithClock(f.clock))
	require.NoError(t, err)
	enterprise, err := gitprovider.New("github", "https://ghe.example.com/api/v3", client, gitprovider.WithClock(f.clock))
	require.NoError(t, err)
	gitlab, err := gitprovider.New("gitlab", "https://gitlab.com/api/v4", client, gitprovider.WithClock(f.clock))
	require.NoError(t, err)
	providers := func(_ context.Context, origin string) gitprovider.Provider {
		switch {
		case strings.Contains(origin, "ghe.example.com"):
			return enterprise
		case strings.Contains(origin, "gitlab.com"):
			return gitlab
		default:
			return github
		}
	}
	tokens := func(_ context.Context, ownerID uuid.UUID, _ string) (*string, error) {
		if ownerID == f.ownerID {
			return new("limited-token"), nil
		}
		return new("other-token"), nil
	}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(
		providers,
		tokens,
		logger,
		f.clock,
		WithConcurrency(1),
	)
	f.newWorker = func(store Store) *Worker {
		return NewWorker(store, refresher, nil, f.clock, logger)
	}
	return f
}

func (f *cooldownFixture) newChat(ownerID uuid.UUID) database.Chat {
	f.t.Helper()
	workspaceID := newCooldownWorkspace(f.t, f.db, ownerID, f.organizationID)
	return dbgen.Chat(f.t, f.db, database.Chat{
		OwnerID:           ownerID,
		OrganizationID:    f.organizationID,
		WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: true},
		LastModelConfigID: f.modelID,
	})
}

func (f *cooldownFixture) addRef(chatID uuid.UUID, origin, branch string) {
	f.t.Helper()
	_, err := f.db.UpsertChatDiffStatusReference(f.ctx, database.UpsertChatDiffStatusReferenceParams{
		ChatID:          chatID,
		GitRemoteOrigin: origin,
		GitBranch:       branch,
		StaleAt:         f.clock.Now().Add(-time.Minute),
	})
	require.NoError(f.t, err)
}

func (f *cooldownFixture) staleAt(chatID uuid.UUID, branch string) time.Time {
	f.t.Helper()
	statuses, err := f.db.GetChatDiffStatusesByChatIDs(f.ctx, []uuid.UUID{chatID})
	require.NoError(f.t, err)
	for _, status := range statuses {
		if status.GitBranch == branch {
			return status.StaleAt.UTC()
		}
	}
	f.t.Fatalf("no diff-status row for chat %s and branch %q", chatID, branch)
	return time.Time{}
}

func (f *cooldownFixture) markStale(worker *Worker) {
	f.t.Helper()
	worker.MarkStale(f.ctx, MarkStaleParams{
		WorkspaceID: f.chat.WorkspaceID.UUID,
		ChatID:      f.chat.ID,
		Branch:      "limited",
		Origin:      cooldownOrigin,
	})
	require.Equal(f.t, f.clock.Now().Add(-time.Second), f.staleAt(f.chat.ID, "limited"))
}

func TestWorker_CooldownRetriesWhenEnded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		header     string
		retryDelay time.Duration
		cooldown   time.Duration
	}{
		{
			name:       "PrimaryRateLimit",
			header:     "X-RateLimit-Reset",
			retryDelay: 30 * time.Minute,
			cooldown:   35 * time.Minute,
		},
		{
			name:       "SecondaryRateLimit",
			header:     "Retry-After",
			retryDelay: 15 * time.Minute,
			cooldown:   20 * time.Minute,
		},
		{
			name:       "FarFutureReset",
			header:     "X-RateLimit-Reset",
			retryDelay: 365 * 24 * time.Hour,
			cooldown:   65 * time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// All written stale_at values are before PostgreSQL NOW(), so
			// only the worker's mock clock decides when cooldowns end.
			f := newCooldownFixture(t, time.Now().Add(-24*time.Hour), tc.header, tc.retryDelay)
			end := f.clock.Now().Add(tc.cooldown)
			const rowCount = 3
			for i := range rowCount {
				f.addRef(f.chat.ID, cooldownOrigin, fmt.Sprintf("limited-%d", i))
			}
			worker := f.newWorker(f.db)

			worker.tick(f.ctx)

			require.Equal(t, int32(1), f.limitedCalls.Load(), "two rows must be skipped in the same batch")
			for i := range rowCount {
				require.Equal(t, end, f.staleAt(f.chat.ID, fmt.Sprintf("limited-%d", i)),
					"rate-limited and in-batch skipped rows must share the cooldown end")
			}

			f.clock.Set(end.Add(-time.Second)).MustWait(f.ctx)
			worker.tick(f.ctx)
			require.Equal(t, int32(1), f.limitedCalls.Load())

			f.limited.Store(false)
			f.clock.Advance(time.Second).MustWait(f.ctx)
			worker.tick(f.ctx)
			require.Equal(t, int32(1+rowCount), f.limitedCalls.Load(), "all rows must retry when the cooldown ends")
			require.Empty(t, worker.cooldowns, "expired cooldowns must be removed")
		})
	}
}

func TestWorker_CooldownPerOwnerAndHost(t *testing.T) {
	t.Parallel()
	f := newCooldownFixture(t, time.Now().Add(-24*time.Hour), "X-RateLimit-Reset", 30*time.Minute)
	f.addRef(f.chat.ID, cooldownOrigin, "limited")
	worker := f.newWorker(f.db)
	worker.tick(f.ctx)
	require.Equal(t, int32(1), f.limitedCalls.Load())
	end := f.clock.Now().Add(35 * time.Minute)

	f.addRef(f.chat.ID, "https://github.com/owner/another-repo", "new-row")
	f.addRef(f.chat.ID, "git@github.com:owner/repo.git", "ssh-origin")
	f.addRef(f.chat.ID, "https://ghe.example.com/owner/repo", "enterprise")
	f.addRef(f.chat.ID, "https://gitlab.com/owner/repo", "gitlab")
	otherOwner := dbgen.User(t, f.db, database.User{})
	otherChat := f.newChat(otherOwner.ID)
	f.addRef(otherChat.ID, cooldownOrigin, "other-owner")

	worker.tick(f.ctx)

	require.Equal(t, int32(1), f.limitedCalls.Load(), "the SSH origin and new diff-status row must share the cooldown")
	require.Equal(t, end, f.staleAt(f.chat.ID, "new-row"))
	require.Equal(t, end, f.staleAt(f.chat.ID, "ssh-origin"))
	require.Equal(t, int32(1), f.otherCalls.Load())
	require.Equal(t, int32(1), f.enterpriseCalls.Load(), "GitHub Enterprise must remain outside this host's cooldown")
	require.Equal(t, int32(1), f.gitlabCalls.Load(), "GitLab must remain outside this host's cooldown")
}

func TestWorker_CooldownSurvivesMarkStale(t *testing.T) {
	t.Parallel()
	f := newCooldownFixture(t, time.Now(), "X-RateLimit-Reset", 30*time.Minute)
	f.addRef(f.chat.ID, cooldownOrigin, "limited")
	store := &cooldownStoreSpy{Store: f.db}
	worker := f.newWorker(store)
	worker.tick(f.ctx)
	end := f.clock.Now().Add(35 * time.Minute)

	f.markStale(worker)
	worker.tick(f.ctx)

	require.Equal(t, 2, store.acquired, "MarkStale must cause the row to be acquired again")
	require.Equal(t, int32(1), f.limitedCalls.Load(), "MarkStale must not clear this worker's cooldown")
	require.Equal(t, end, f.staleAt(f.chat.ID, "limited"))
}

func TestWorker_CooldownDefersRowsAcrossReplicas(t *testing.T) {
	t.Parallel()
	f := newCooldownFixture(t, time.Now(), "X-RateLimit-Reset", 30*time.Minute)
	f.addRef(f.chat.ID, cooldownOrigin, "limited")
	worker := f.newWorker(f.db)
	worker.tick(f.ctx)
	end := f.clock.Now().Add(35 * time.Minute)
	require.Equal(t, end, f.staleAt(f.chat.ID, "limited"))

	replicaStore := &cooldownStoreSpy{Store: f.db}
	replica := f.newWorker(replicaStore)
	replica.tick(f.ctx)
	require.Zero(t, replicaStore.acquired, "stale_at must defer the row on every replica")
	require.Equal(t, int32(1), f.limitedCalls.Load())

	f.markStale(replica)
	replica.tick(f.ctx)
	require.Equal(t, 1, replicaStore.acquired)
	require.Equal(t, int32(2), f.limitedCalls.Load(), "a replica without a cooldown must get its own rate-limit response")
	require.Equal(t, end, f.staleAt(f.chat.ID, "limited"))
	require.Equal(t, end, replica.cooldowns[cooldownKey{ownerID: f.ownerID, host: "github.com"}])
}

func TestWorker_CooldownOnAlreadyAcquiredReplicaRow(t *testing.T) {
	t.Parallel()
	f := newCooldownFixture(t, time.Now(), "X-RateLimit-Reset", 30*time.Minute)
	f.addRef(f.chat.ID, cooldownOrigin, "queued")
	f.addRef(f.chat.ID, cooldownOrigin, "limited")

	// This replica acquired a row before the other worker's backoff. It has
	// no cooldown, so its already-acquired row must call the provider once.
	acquiredRows, err := f.db.AcquireStaleChatDiffStatuses(f.ctx, 1)
	require.NoError(t, err)
	require.Len(t, acquiredRows, 1)
	replica := f.newWorker(&acquiredChatDiffStatusesStore{Store: f.db, rows: acquiredRows})
	worker := f.newWorker(f.db)

	worker.tick(f.ctx)
	replica.tick(f.ctx)

	end := f.clock.Now().Add(35 * time.Minute)
	require.Equal(t, int32(2), f.limitedCalls.Load())
	require.Equal(t, end, f.staleAt(f.chat.ID, "queued"))
	require.Equal(t, end, f.staleAt(f.chat.ID, "limited"))
	require.Equal(t, end, replica.cooldowns[cooldownKey{ownerID: f.ownerID, host: "github.com"}])
}
