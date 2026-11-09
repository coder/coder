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

func TestCooldownKeyFor(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	for _, tc := range []struct {
		name   string
		origin string
		host   string
	}{
		{name: "HTTPS", origin: "https://GitHub.com/o/r.git", host: "github.com"},
		{name: "HTTPDefaultPort", origin: "http://github.example:80/o/r.git", host: "github.example"},
		{name: "HTTPSDefaultPort", origin: "https://github.com:443/o/r.git", host: "github.com"},
		{name: "HTTPSNonDefaultHTTPPort", origin: "https://github.example:80/o/r.git", host: "github.example:80"},
		{name: "SSHDefaultPort", origin: "ssh://git@GitHub.com:22/o/r.git", host: "github.com"},
		{name: "SCP", origin: "git@github.com:o/r.git", host: "github.com"},
		{name: "HTTPNonDefaultPort", origin: "http://gitlab.example:8080/o/r.git", host: "gitlab.example:8080"},
		{name: "HTTPSNonDefaultPort", origin: "https://github.example:8443/o/r.git", host: "github.example:8443"},
		{name: "OtherInstancePort", origin: "https://github.example:9443/o/r.git", host: "github.example:9443"},
		{name: "SSHNonDefaultPort", origin: "ssh://git@github.example:8443/o/r.git", host: "github.example"},
		{name: "GitLabSSHTransportPort", origin: "ssh://git@gitlab.example:2222/group/repo.git", host: "gitlab.example"},
		{name: "IPv6SCP", origin: "git@[::1]:o/r.git", host: "::1"},
		{name: "IPv6DefaultPort", origin: "https://[::1]:443/o/r.git", host: "::1"},
		{name: "IPv6NonDefaultPort", origin: "https://[::1]:8443/o/r.git", host: "[::1]:8443"},
		{name: "NoHost", origin: "github.com:o/r"},
		{name: "PortWithoutHost", origin: "ssh://:8443/o/r"},
		{name: "Malformed", origin: "://"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, cooldownKey{ownerID: ownerID, host: tc.host}, cooldownKeyFor(ownerID, tc.origin, nil))
		})
	}
}

func TestCooldownEndFallback(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		retryAfter time.Time
	}{
		{name: "Zero"},
		{name: "Past", retryAfter: now.Add(-time.Minute)},
		{name: "Now", retryAfter: now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &gitprovider.RateLimitError{RetryAfter: tc.retryAfter}
			require.Equal(t, now.Add(DiffStatusTTL), cooldownEnd(now, err))
		})
	}
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

func newBranchProvider(t *testing.T, clock quartz.Clock) gitprovider.Provider {
	t.Helper()
	client := &http.Client{Transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected HTTP request")
		return nil, xerrors.New("unexpected HTTP request")
	})}
	provider, err := gitprovider.New("github", "https://github.example", client, gitprovider.WithClock(clock))
	require.NoError(t, err)
	return provider
}

func TestWorker_CooldownSharesGitLabAPIInstance(t *testing.T) {
	t.Parallel()

	for _, instance := range []struct {
		name   string
		origin string
	}{
		{name: "DefaultHTTPS", origin: "https://gitlab.example"},
		{name: "CustomHTTPS", origin: "https://gitlab.example:8443"},
	} {
		t.Run(instance.name, func(t *testing.T) {
			t.Parallel()
			for _, order := range []struct {
				name     string
				sshFirst bool
				batch    bool
			}{
				{name: "HTTPSFirst"},
				{name: "SSHFirst", sshFirst: true},
				{name: "SameBatch", batch: true},
			} {
				t.Run(order.name, func(t *testing.T) {
					t.Parallel()
					ctx := testutil.Context(t, testutil.WaitLong)
					clock := quartz.NewMock(t)
					ownerID := uuid.New()
					end := clock.Now().Add(30 * time.Minute)
					origins := []string{instance.origin + "/group/repo", "ssh://git@gitlab.example:2222/group/repo.git"}
					if order.sshFirst {
						origins[0], origins[1] = origins[1], origins[0]
					}
					store := dbmock.NewMockStore(gomock.NewController(t))
					var rows []database.AcquireStaleChatDiffStatusesRow
					for _, origin := range origins {
						rows = append(rows, database.AcquireStaleChatDiffStatusesRow{
							ChatID:          uuid.New(),
							OwnerID:         ownerID,
							GitRemoteOrigin: origin,
							GitBranch:       "limited",
						})
					}
					if order.batch {
						store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return(rows, nil)
					} else {
						var previous *gomock.Call
						for _, row := range rows {
							call := store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return([]database.AcquireStaleChatDiffStatusesRow{row}, nil)
							if previous != nil {
								call.After(previous)
							}
							previous = call
						}
					}
					store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).Return(nil).Times(2)
					client := &http.Client{Transport: testutil.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
						t.Error("unexpected HTTP request")
						return nil, xerrors.New("unexpected HTTP request")
					})}
					base, err := gitprovider.New("gitlab", instance.origin+"/api/v4", client, gitprovider.WithClock(clock))
					require.NoError(t, err)
					var calls atomic.Int32
					provider := branchCallbackProvider{
						Provider: base,
						resolve: func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
							calls.Add(1)
							return nil, &gitprovider.RateLimitError{RetryAfter: end}
						},
					}
					providers := func(context.Context, string) gitprovider.Provider { return provider }
					tokens := func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil }
					logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
					refresher := NewRefresher(providers, tokens, logger, clock, WithConcurrency(1))
					worker := NewWorker(store, refresher, nil, clock, logger)

					worker.tick(ctx)
					if !order.batch {
						worker.tick(ctx)
					}

					require.Equal(t, int32(1), calls.Load(), "HTTPS and a custom SSH transport must share the provider's API cooldown")
				})
			}
		})
	}
}

func TestWorker_CooldownBackoffFailureSkipsProvider(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	ownerID := uuid.New()
	const origin = "https://github.example/owner/repo"
	rows := []database.AcquireStaleChatDiffStatusesRow{
		{
			ChatID:          uuid.New(),
			OwnerID:         ownerID,
			GitRemoteOrigin: origin,
			GitBranch:       "limited",
		},
		{
			ChatID:          uuid.New(),
			OwnerID:         uuid.New(),
			GitRemoteOrigin: origin,
			GitBranch:       "healthy",
		},
	}
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return(rows, nil)
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).Return(xerrors.New("backoff failed"))
	var limitedCalls, healthyCalls atomic.Int32
	provider := branchCallbackProvider{
		Provider: newBranchProvider(t, clock),
		resolve: func(_ context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if ref.Branch == "limited" {
				limitedCalls.Add(1)
			} else {
				healthyCalls.Add(1)
			}
			return nil, nil //nolint:nilnil // No PR for this branch.
		},
	}
	providers := func(context.Context, string) gitprovider.Provider { return provider }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) { return new("test-token"), nil }
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(providers, tokens, logger, clock)
	worker := NewWorker(store, refresher, nil, clock, logger)
	worker.cooldowns[cooldownKey{ownerID: ownerID, host: "github.example"}] = clock.Now().Add(time.Minute)

	worker.tick(ctx)

	require.Zero(t, limitedCalls.Load(), "a failed stale_at write must not bypass the local cooldown")
	require.Equal(t, int32(1), healthyCalls.Load(), "other owners must still receive a diff-status refresh")
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

	successfulFetch := make(chan struct{})
	provider := branchCallbackProvider{
		Provider: newBranchProvider(t, clock),
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

	rateLimitObserved := make(chan struct{})
	slowRequestStarted := make(chan struct{})
	retryAfter := clock.Now().Add(35 * time.Minute)
	provider := branchCallbackProvider{
		Provider: newBranchProvider(t, clock),
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

	bothStarted := make(chan struct{})
	var started atomic.Int32
	provider := branchCallbackProvider{
		Provider: newBranchProvider(t, clock),
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
			cooldown:   30*time.Minute + gitprovider.RateLimitPadding,
		},
		{
			name:       "SecondaryRateLimit",
			header:     "Retry-After",
			retryDelay: 15 * time.Minute,
			cooldown:   15*time.Minute + gitprovider.RateLimitPadding,
		},
		{
			name:       "FarFutureReset",
			header:     "X-RateLimit-Reset",
			retryDelay: 365 * 24 * time.Hour,
			cooldown:   time.Hour + gitprovider.RateLimitPadding,
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
	end := f.clock.Now().Add(30*time.Minute + gitprovider.RateLimitPadding)

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
	end := f.clock.Now().Add(30*time.Minute + gitprovider.RateLimitPadding)

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
	end := f.clock.Now().Add(30*time.Minute + gitprovider.RateLimitPadding)
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
