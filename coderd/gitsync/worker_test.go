package gitsync_test

import (
	"context"
	"database/sql"
	"fmt"
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
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// mockStore implements gitsync.Store with function fields.
type mockStore struct {
	acquireStaleChatDiffStatuses func(ctx context.Context, limitVal int32) ([]database.AcquireStaleChatDiffStatusesRow, error)
	backoffChatDiffStatus        func(ctx context.Context, arg database.BackoffChatDiffStatusParams) error
	upsertChatDiffStatus         func(ctx context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error)
	upsertChatDiffStatusRef      func(ctx context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error)
	getChatsByOwnerID            func(ctx context.Context, arg database.GetChatsByOwnerIDParams) ([]database.Chat, error)
}

func (m *mockStore) AcquireStaleChatDiffStatuses(ctx context.Context, limitVal int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
	return m.acquireStaleChatDiffStatuses(ctx, limitVal)
}

func (m *mockStore) BackoffChatDiffStatus(ctx context.Context, arg database.BackoffChatDiffStatusParams) error {
	return m.backoffChatDiffStatus(ctx, arg)
}

func (m *mockStore) UpsertChatDiffStatus(ctx context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
	return m.upsertChatDiffStatus(ctx, arg)
}

func (m *mockStore) UpsertChatDiffStatusReference(ctx context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
	return m.upsertChatDiffStatusRef(ctx, arg)
}

func (m *mockStore) GetChatsByOwnerID(ctx context.Context, arg database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
	return m.getChatsByOwnerID(ctx, arg)
}

// mockPublisher implements gitsync.EventPublisher.
type mockPublisher struct {
	publishDiffStatusChange func(ctx context.Context, chatID uuid.UUID) error
}

func (m *mockPublisher) PublishDiffStatusChange(ctx context.Context, chatID uuid.UUID) error {
	return m.publishDiffStatusChange(ctx, chatID)
}

// testRefresherCfg configures newTestRefresher.
type testRefresherCfg struct {
	resolveBranchPR func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error)
	fetchPRStatus   func(context.Context, string, gitprovider.PRRef) (*gitprovider.PRStatus, error)
}

type testRefresherOpt func(*testRefresherCfg)

func withResolveBranchPR(f func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error)) testRefresherOpt {
	return func(c *testRefresherCfg) { c.resolveBranchPR = f }
}

// newTestRefresher creates a Refresher backed by mock
// provider/token resolvers. The provider recognises any origin,
// resolves branches to a canned PR, and returns a canned PRStatus.
func newTestRefresher(t *testing.T, clk quartz.Clock, opts ...testRefresherOpt) *gitsync.Refresher {
	t.Helper()

	cfg := testRefresherCfg{
		resolveBranchPR: func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return &gitprovider.PRRef{Owner: "o", Repo: "r", Number: 1}, nil
		},
		fetchPRStatus: func(context.Context, string, gitprovider.PRRef) (*gitprovider.PRStatus, error) {
			return &gitprovider.PRStatus{
				State: gitprovider.PRStateOpen,
				DiffStats: gitprovider.DiffStats{
					Additions:    10,
					Deletions:    3,
					ChangedFiles: 2,
				},
			}, nil
		},
	}
	for _, o := range opts {
		o(&cfg)
	}

	prov := &mockProvider{
		parseRepositoryOrigin: func(string) (string, string, string, bool) {
			return "owner", "repo", "https://github.com/owner/repo", true
		},
		parsePullRequestURL: func(raw string) (gitprovider.PRRef, bool) {
			return gitprovider.PRRef{Owner: "owner", Repo: "repo", Number: 1}, raw != ""
		},
		resolveBranchPR:        cfg.resolveBranchPR,
		fetchPullRequestStatus: cfg.fetchPRStatus,
		buildPullRequestURL: func(ref gitprovider.PRRef) string {
			return fmt.Sprintf("https://github.com/%s/%s/pull/%d", ref.Owner, ref.Repo, ref.Number)
		},
	}

	providers := func(string) gitprovider.Provider { return prov }
	tokens := func(context.Context, uuid.UUID, string) (string, error) {
		return "tok", nil
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	return gitsync.NewRefresher(providers, tokens, logger, clk)
}

// makeAcquiredRow returns an AcquireStaleChatDiffStatusesRow with
// a non-empty branch/origin so the Refresher goes through the
// branch-resolution path.
func makeAcquiredRow(chatID, ownerID uuid.UUID) database.AcquireStaleChatDiffStatusesRow {
	return database.AcquireStaleChatDiffStatusesRow{
		ChatID:          chatID,
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/owner/repo",
		StaleAt:         time.Now().Add(-time.Minute),
		OwnerID:         ownerID,
	}
}

// tickOnce traps the worker's NewTicker call, starts the worker,
// fires one tick, waits for it to finish by observing the given
// tickDone channel, then shuts the worker down. The tickDone
// channel must be closed when the last expected operation in the
// tick completes. For tests where the tick does nothing (e.g. 0
// stale rows or store error), tickDone should be closed inside
// acquireStaleChatDiffStatuses.
func tickOnce(
	ctx context.Context,
	t *testing.T,
	mClock *quartz.Mock,
	worker *gitsync.Worker,
	tickDone <-chan struct{},
) {
	t.Helper()

	trap := mClock.Trap().NewTicker("gitsync", "worker")
	defer trap.Close()

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go worker.Start(workerCtx)

	// Wait for the worker to create its ticker.
	trap.MustWait(ctx).MustRelease(ctx)

	// Fire one tick. The waiter resolves when the channel receive
	// completes, not when w.tick() returns, so we use tickDone to
	// know when to proceed.
	_, w := mClock.AdvanceNext()
	w.MustWait(ctx)

	// Wait for the tick's business logic to finish.
	select {
	case <-tickDone:
	case <-ctx.Done():
		t.Fatal("timed out waiting for tick to complete")
	}

	cancel()
	<-worker.Done()
}

func TestWorker_PicksUpStaleRows(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chat1 := uuid.New()
	chat2 := uuid.New()
	ownerID := uuid.New()

	var upsertCount atomic.Int32
	var publishCount atomic.Int32
	tickDone := make(chan struct{})

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(_ context.Context, _ int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return []database.AcquireStaleChatDiffStatusesRow{
				makeAcquiredRow(chat1, ownerID),
				makeAcquiredRow(chat2, ownerID),
			}, nil
		},
		upsertChatDiffStatus: func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(_ context.Context, _ uuid.UUID) error {
			if publishCount.Add(1) == 2 {
				close(tickDone)
			}
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(2), upsertCount.Load())
	assert.Equal(t, int32(2), publishCount.Load())
}

func TestWorker_SkipsFreshRows(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var upsertCount atomic.Int32
	var publishCount atomic.Int32
	tickDone := make(chan struct{})

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			// No stale rows — tick returns immediately.
			close(tickDone)
			return nil, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(0), upsertCount.Load())
	assert.Equal(t, int32(0), publishCount.Load())
}

func TestWorker_LimitsToNRows(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var capturedLimit atomic.Int32
	var upsertCount atomic.Int32
	ownerID := uuid.New()
	const numRows = 5
	tickDone := make(chan struct{})

	rows := make([]database.AcquireStaleChatDiffStatusesRow, numRows)
	for i := range rows {
		rows[i] = makeAcquiredRow(uuid.New(), ownerID)
	}

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(_ context.Context, limitVal int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			capturedLimit.Store(limitVal)
			return rows, nil
		},
		upsertChatDiffStatus: func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			if upsertCount.Load() == numRows {
				close(tickDone)
			}
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	// The default batch size is 50.
	assert.Equal(t, int32(50), capturedLimit.Load())
	assert.Equal(t, int32(numRows), upsertCount.Load())
}

func TestWorker_RefresherReturnsNilNil_SkipsUpsert(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()
	var upsertCount atomic.Int32
	var publishCount atomic.Int32

	// When the Refresher returns (nil, nil) the worker skips the
	// upsert and publish. We signal tickDone from the refresher
	// mock since that is the last operation before the tick
	// returns.
	tickDone := make(chan struct{})

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return []database.AcquireStaleChatDiffStatusesRow{makeAcquiredRow(chatID, ownerID)}, nil
		},
		backoffChatDiffStatus: func(context.Context, database.BackoffChatDiffStatusParams) error {
			t.Fatal("unexpected backoff call")
			return nil
		},
		upsertChatDiffStatus: func(context.Context, database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// ResolveBranchPullRequest returns nil → Refresher returns
	// (nil, nil).
	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			close(tickDone)
			return nil, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(0), upsertCount.Load())
	assert.Equal(t, int32(0), publishCount.Load())
}

func TestWorker_RefresherError_BacksOffRow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chat1 := uuid.New()
	chat2 := uuid.New()
	ownerID := uuid.New()

	var upsertCount atomic.Int32
	var publishCount atomic.Int32
	var backoffCount atomic.Int32
	var mu sync.Mutex
	var backoffArgs []database.BackoffChatDiffStatusParams
	tickDone := make(chan struct{})
	var closeOnce sync.Once

	// Two rows processed: one fails (backoff), one succeeds
	// (upsert+publish). Both must finish before we close tickDone.
	var terminalOps atomic.Int32
	signalIfDone := func() {
		if terminalOps.Add(1) == 2 {
			closeOnce.Do(func() { close(tickDone) })
		}
	}

	mClock := quartz.NewMock(t)

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return []database.AcquireStaleChatDiffStatusesRow{
				makeAcquiredRow(chat1, ownerID),
				makeAcquiredRow(chat2, ownerID),
			}, nil
		},
		backoffChatDiffStatus: func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
			backoffCount.Add(1)
			mu.Lock()
			backoffArgs = append(backoffArgs, arg)
			mu.Unlock()
			signalIfDone()
			return nil
		},
		upsertChatDiffStatus: func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			// Only the successful row publishes.
			publishCount.Add(1)
			signalIfDone()
			return nil
		},
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Fail ResolveBranchPullRequest for the first call, succeed
	// for the second.
	var callCount atomic.Int32
	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, fmt.Errorf("simulated provider error")
			}
			return &gitprovider.PRRef{Owner: "o", Repo: "r", Number: 1}, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	// BackoffChatDiffStatus was called for the failed row.
	assert.Equal(t, int32(1), backoffCount.Load())
	mu.Lock()
	require.Len(t, backoffArgs, 1)
	assert.Equal(t, chat1, backoffArgs[0].ChatID)
	// stale_at should be approximately clock.Now() + DiffStatusTTL (120s).
	expectedStaleAt := mClock.Now().UTC().Add(gitsync.DiffStatusTTL)
	assert.WithinDuration(t, expectedStaleAt, backoffArgs[0].StaleAt, time.Second)
	mu.Unlock()

	// UpsertChatDiffStatus was called for the successful row.
	assert.Equal(t, int32(1), upsertCount.Load())
	// PublishDiffStatusChange was called only for the successful row.
	assert.Equal(t, int32(1), publishCount.Load())
}

func TestWorker_UpsertError_ContinuesNextRow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chat1 := uuid.New()
	chat2 := uuid.New()
	ownerID := uuid.New()

	var publishCount atomic.Int32
	tickDone := make(chan struct{})
	var closeOnce sync.Once
	var mu sync.Mutex
	upsertedChatIDs := make(map[uuid.UUID]struct{})

	// We have 2 rows. The upsert for chat1 fails; the upsert
	// for chat2 succeeds and publishes. Because goroutines run
	// concurrently we don't know which finishes last, so we
	// track the total number of "terminal" events (upsert error
	// + publish success) and close tickDone when both have
	// occurred.
	var terminalOps atomic.Int32
	signalIfDone := func() {
		if terminalOps.Add(1) == 2 {
			closeOnce.Do(func() { close(tickDone) })
		}
	}

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return []database.AcquireStaleChatDiffStatusesRow{
				makeAcquiredRow(chat1, ownerID),
				makeAcquiredRow(chat2, ownerID),
			}, nil
		},
		upsertChatDiffStatus: func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			if arg.ChatID == chat1 {
				// Terminal event for the failing row.
				signalIfDone()
				return database.ChatDiffStatus{}, fmt.Errorf("db write error")
			}
			mu.Lock()
			upsertedChatIDs[arg.ChatID] = struct{}{}
			mu.Unlock()
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			// Terminal event for the successful row.
			signalIfDone()
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	mu.Lock()
	_, gotChat2 := upsertedChatIDs[chat2]
	mu.Unlock()
	assert.True(t, gotChat2, "chat2 should have been upserted")
	assert.Equal(t, int32(1), publishCount.Load())
}

func TestWorker_RespectsShutdown(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return nil, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error { return nil },
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	trap := mClock.Trap().NewTicker("gitsync", "worker")
	defer trap.Close()

	workerCtx, cancel := context.WithCancel(ctx)
	go worker.Start(workerCtx)

	// Wait for ticker creation so the worker is running.
	trap.MustWait(ctx).MustRelease(ctx)

	// Cancel immediately.
	cancel()

	select {
	case <-worker.Done():
		// Success — worker shut down.
	case <-ctx.Done():
		t.Fatal("timed out waiting for worker to shut down")
	}
}

func TestWorker_BatchRefreshAllRows(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	ownerID := uuid.New()
	const numRows = 5
	rows := make([]database.AcquireStaleChatDiffStatusesRow, numRows)
	for i := range rows {
		rows[i] = makeAcquiredRow(uuid.New(), ownerID)
	}

	var upsertCount atomic.Int32
	var publishCount atomic.Int32
	tickDone := make(chan struct{})

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			return rows, nil
		},
		upsertChatDiffStatus: func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			if publishCount.Add(1) == numRows {
				close(tickDone)
			}
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(numRows), upsertCount.Load())
	assert.Equal(t, int32(numRows), publishCount.Load())
}

// --- MarkStale tests ---

func TestWorker_MarkStale_UpsertAndPublish(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()
	chat1 := uuid.New()
	chat2 := uuid.New()
	chatOther := uuid.New()

	var mu sync.Mutex
	var upsertRefCalls []database.UpsertChatDiffStatusReferenceParams
	var publishedIDs []uuid.UUID

	store := &mockStore{
		getChatsByOwnerID: func(_ context.Context, arg database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
			require.Equal(t, ownerID, arg.OwnerID)
			return []database.Chat{
				{ID: chat1, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
				{ID: chat2, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
				{ID: chatOther, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}},
			}, nil
		},
		upsertChatDiffStatusRef: func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
			mu.Lock()
			upsertRefCalls = append(upsertRefCalls, arg)
			mu.Unlock()
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(_ context.Context, chatID uuid.UUID) error {
			mu.Lock()
			publishedIDs = append(publishedIDs, chatID)
			mu.Unlock()
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	now := mClock.Now()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, workspaceID, ownerID, "feature", "https://github.com/owner/repo")

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, upsertRefCalls, 2)
	for _, call := range upsertRefCalls {
		assert.Equal(t, "feature", call.GitBranch)
		assert.Equal(t, "https://github.com/owner/repo", call.GitRemoteOrigin)
		assert.True(t, call.StaleAt.Before(now),
			"stale_at should be in the past, got %v vs now %v", call.StaleAt, now)
		assert.Equal(t, sql.NullString{}, call.Url)
	}

	require.Len(t, publishedIDs, 2)
	assert.ElementsMatch(t, []uuid.UUID{chat1, chat2}, publishedIDs)
}

func TestWorker_MarkStale_NoMatchingChats(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()

	var upsertRefCount atomic.Int32
	var publishCount atomic.Int32

	store := &mockStore{
		getChatsByOwnerID: func(context.Context, database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
			return []database.Chat{
				{ID: uuid.New(), OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}},
				{ID: uuid.New(), OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}},
			}, nil
		},
		upsertChatDiffStatusRef: func(context.Context, database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
			upsertRefCount.Add(1)
			return database.ChatDiffStatus{}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, workspaceID, ownerID, "main", "https://github.com/x/y")

	assert.Equal(t, int32(0), upsertRefCount.Load())
	assert.Equal(t, int32(0), publishCount.Load())
}

func TestWorker_MarkStale_UpsertFails_ContinuesNext(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()
	chat1 := uuid.New()
	chat2 := uuid.New()

	var publishCount atomic.Int32

	store := &mockStore{
		getChatsByOwnerID: func(context.Context, database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
			return []database.Chat{
				{ID: chat1, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
				{ID: chat2, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
			}, nil
		},
		upsertChatDiffStatusRef: func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
			if arg.ChatID == chat1 {
				return database.ChatDiffStatus{}, fmt.Errorf("upsert ref error")
			}
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, workspaceID, ownerID, "dev", "https://github.com/a/b")

	assert.Equal(t, int32(1), publishCount.Load())
}

func TestWorker_MarkStale_GetChatsFails(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var upsertRefCount atomic.Int32

	store := &mockStore{
		getChatsByOwnerID: func(context.Context, database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
			return nil, fmt.Errorf("db error")
		},
		upsertChatDiffStatusRef: func(context.Context, database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
			upsertRefCount.Add(1)
			return database.ChatDiffStatus{}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error { return nil },
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, uuid.New(), uuid.New(), "main", "https://github.com/x/y")

	assert.Equal(t, int32(0), upsertRefCount.Load())
}

func TestWorker_TickStoreError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var upsertCount atomic.Int32
	var publishCount atomic.Int32
	tickDone := make(chan struct{})

	store := &mockStore{
		acquireStaleChatDiffStatuses: func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			close(tickDone)
			return nil, fmt.Errorf("database unavailable")
		},
		upsertChatDiffStatus: func(context.Context, database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{}, nil
		},
	}
	pub := &mockPublisher{
		publishDiffStatusChange: func(context.Context, uuid.UUID) error {
			publishCount.Add(1)
			return nil
		},
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(0), upsertCount.Load())
	assert.Equal(t, int32(0), publishCount.Load())
}

func TestWorker_MarkStale_EmptyBranchOrOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
		origin string
	}{
		{"both empty", "", ""},
		{"branch empty", "", "https://github.com/x/y"},
		{"origin empty", "main", ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			var getChatsCalled atomic.Bool

			store := &mockStore{
				getChatsByOwnerID: func(context.Context, database.GetChatsByOwnerIDParams) ([]database.Chat, error) {
					getChatsCalled.Store(true)
					return nil, nil
				},
			}
			pub := &mockPublisher{
				publishDiffStatusChange: func(context.Context, uuid.UUID) error { return nil },
			}

			mClock := quartz.NewMock(t)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			refresher := newTestRefresher(t, mClock)
			worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

			worker.MarkStale(ctx, uuid.New(), uuid.New(), tc.branch, tc.origin)
			assert.False(t, getChatsCalled.Load(),
				"GetChatsByOwnerID should not be called with branch=%q origin=%q",
				tc.branch, tc.origin)
		})
	}
}
