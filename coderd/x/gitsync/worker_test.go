package gitsync_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/gitsync"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// testRefresherCfg configures newTestRefresher.
type testRefresherCfg struct {
	resolveBranchPR func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error)
	fetchPRStatus   func(context.Context, string, gitprovider.PRRef) (*gitprovider.PRStatus, error)
	refresherOpts   []gitsync.RefresherOption
}

type testRefresherOpt func(*testRefresherCfg)

func withResolveBranchPR(f func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error)) testRefresherOpt {
	return func(c *testRefresherCfg) { c.resolveBranchPR = f }
}

func withRefresherOpts(opts ...gitsync.RefresherOption) testRefresherOpt {
	return func(c *testRefresherCfg) { c.refresherOpts = opts }
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
	tokens := func(context.Context, uuid.UUID, string) (*string, error) {
		return ptr.Ref("tok"), nil
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	return gitsync.NewRefresher(providers, tokens, logger, clk, cfg.refresherOpts...)
}

// makeAcquiredRowWithBranch returns an AcquireStaleChatDiffStatusesRow with
// the given branch and a non-empty origin so the Refresher goes through the
// branch-resolution path.
func makeAcquiredRowWithBranch(chatID, ownerID uuid.UUID, branch string) database.AcquireStaleChatDiffStatusesRow {
	return database.AcquireStaleChatDiffStatusesRow{
		ChatID:          chatID,
		GitBranch:       branch,
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

func TestWorker_SkipsFreshRows(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	tickDone := make(chan struct{})

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			// No stale rows — tick returns immediately.
			close(tickDone)
			return nil, nil
		})

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)
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
		rows[i] = makeAcquiredRowWithBranch(uuid.New(), ownerID, "feature")
	}

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, limitVal int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			capturedLimit.Store(limitVal)
			return rows, nil
		})
	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		}).Times(numRows)

	pub := func(_ context.Context, _ uuid.UUID) error {
		if upsertCount.Load() == numRows {
			close(tickDone)
		}
		return nil
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

func TestWorker_NoPR_RecentMarkStale_BacksOffShort(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	// When the Refresher returns (nil, nil) AND the row was
	// recently marked stale (updated_at within NoPRRetryWindow),
	// the worker should call BackoffChatDiffStatus with NoPRBackoff
	// so the row is retried quickly.
	tickDone := make(chan struct{})

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	mClock := quartz.NewMock(t)

	row := makeAcquiredRowWithBranch(chatID, ownerID, "feature")
	row.UpdatedAt = mClock.Now() // recently marked stale

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{row}, nil)
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
			assert.Equal(t, chatID, arg.ChatID)
			expected := mClock.Now().UTC().Add(gitsync.NoPRBackoff)
			assert.WithinDuration(t, expected, arg.StaleAt, time.Second,
				"stale_at should be NoPRBackoff from now")
			close(tickDone)
			return nil
		})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// ResolveBranchPullRequest returns nil → Refresher returns
	// (nil, nil).
	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return nil, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)
}

func TestWorker_NoPR_OldRow_Skips(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	// When the Refresher returns (nil, nil) but the row's
	// updated_at is outside the NoPRRetryWindow, the worker should
	// skip the row entirely (no backoff call) and let the 5-minute
	// acquisition lock serve as the natural retry interval.
	tickDone := make(chan struct{})

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	mClock := quartz.NewMock(t)

	row := makeAcquiredRowWithBranch(chatID, ownerID, "feature")
	row.UpdatedAt = mClock.Now().Add(-5 * time.Minute) // old row

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{row}, nil)
	// BackoffChatDiffStatus should NOT be called.

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			close(tickDone)
			return nil, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)
}

func TestWorker_NoPR_BoundaryExactWindow_Skips(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	// When updated_at is exactly NoPRRetryWindow ago, the strict
	// "<" comparison means the row should be skipped (no backoff).
	// This pins the boundary so an accidental change to "<=" is
	// caught.
	tickDone := make(chan struct{})

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	mClock := quartz.NewMock(t)

	row := makeAcquiredRowWithBranch(chatID, ownerID, "feature")
	row.UpdatedAt = mClock.Now().Add(-gitsync.NoPRRetryWindow) // exactly at boundary

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{row}, nil)
	// BackoffChatDiffStatus should NOT be called.

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			close(tickDone)
			return nil, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)
}

func TestWorker_NoPR_BackoffError_ContinuesNextRow(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chat1 := uuid.New()
	chat2 := uuid.New()
	ownerID := uuid.New()

	// Two recent rows, both with no PR. BackoffChatDiffStatus
	// fails for the first row but the second row should still
	// be processed (backoff succeeds).
	var backoffCount atomic.Int32
	tickDone := make(chan struct{})
	var closeOnce sync.Once

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	mClock := quartz.NewMock(t)

	row1 := makeAcquiredRowWithBranch(chat1, ownerID, "no-pr-1")
	row1.UpdatedAt = mClock.Now()
	row2 := makeAcquiredRowWithBranch(chat2, ownerID, "no-pr-2")
	row2.UpdatedAt = mClock.Now()

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{row1, row2}, nil)
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
			n := backoffCount.Add(1)
			if arg.ChatID == chat1 {
				return fmt.Errorf("simulated backoff error")
			}
			// Second call succeeds; both rows processed.
			if n >= 2 {
				closeOnce.Do(func() { close(tickDone) })
			}
			return nil
		}).Times(2)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return nil, nil
		},
	))

	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	assert.Equal(t, int32(2), backoffCount.Load(),
		"both rows should have attempted backoff")
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

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{
			makeAcquiredRowWithBranch(chat1, ownerID, "fail-branch"),
			makeAcquiredRowWithBranch(chat2, ownerID, "success-branch"),
		}, nil)
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
			backoffCount.Add(1)
			mu.Lock()
			backoffArgs = append(backoffArgs, arg)
			mu.Unlock()
			signalIfDone()
			return nil
		})
	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			upsertCount.Add(1)
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		})

	pub := func(_ context.Context, _ uuid.UUID) error {
		// Only the successful row publishes.
		publishCount.Add(1)
		signalIfDone()
		return nil
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Fail ResolveBranchPullRequest based on the branch name
	// so the behavior is deterministic regardless of execution
	// order.
	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(_ context.Context, _ string, ref gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			if ref.Branch == "fail-branch" {
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

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{
			makeAcquiredRowWithBranch(chat1, ownerID, "feature"),
			makeAcquiredRowWithBranch(chat2, ownerID, "feature"),
		}, nil)
	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			if arg.ChatID == chat1 {
				// Terminal event for the failing row.
				signalIfDone()
				return database.ChatDiffStatus{}, fmt.Errorf("db write error")
			}
			mu.Lock()
			upsertedChatIDs[arg.ChatID] = struct{}{}
			mu.Unlock()
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		}).Times(2)

	pub := func(_ context.Context, _ uuid.UUID) error {
		publishCount.Add(1)
		// Terminal event for the successful row.
		signalIfDone()
		return nil
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

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

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

func TestWorker_MarkStale_UpsertAndPublish(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()
	chat1 := uuid.New()
	chat2 := uuid.New()

	var mu sync.Mutex
	var upsertRefCalls []database.UpsertChatDiffStatusReferenceParams
	var publishedIDs []uuid.UUID

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ids []uuid.UUID) ([]database.Chat, error) {
			require.Equal(t, []uuid.UUID{workspaceID}, ids)
			return []database.Chat{
				{ID: chat1, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
				{ID: chat2, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
			}, nil
		})
	store.EXPECT().UpsertChatDiffStatusReference(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
		mu.Lock()
		upsertRefCalls = append(upsertRefCalls, arg)
		mu.Unlock()
		return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
	}).Times(2)

	pub := func(_ context.Context, chatID uuid.UUID) error {
		mu.Lock()
		publishedIDs = append(publishedIDs, chatID)
		mu.Unlock()
		return nil
	}

	mClock := quartz.NewMock(t)
	now := mClock.Now()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: workspaceID,
		Branch:      "feature",
		Origin:      "https://github.com/owner/repo",
	})

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

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: workspaceID,
		Branch:      "main",
		Origin:      "https://github.com/x/y",
	})
}

func TestWorker_MarkStale_UpsertFails_ContinuesNext(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()
	chat1 := uuid.New()
	chat2 := uuid.New()

	var publishCount atomic.Int32

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		Return([]database.Chat{
			{ID: chat1, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
			{ID: chat2, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
		}, nil)
	store.EXPECT().UpsertChatDiffStatusReference(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
			if arg.ChatID == chat1 {
				return database.ChatDiffStatus{}, fmt.Errorf("upsert ref error")
			}
			return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
		}).Times(2)

	pub := func(_ context.Context, _ uuid.UUID) error {
		publishCount.Add(1)
		return nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: workspaceID,
		Branch:      "dev",
		Origin:      "https://github.com/a/b",
	})

	assert.Equal(t, int32(1), publishCount.Load())
}

func TestWorker_MarkStale_GetChatsByWorkspaceIDsFails(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("db error"))

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: uuid.New(),
		Branch:      "main",
		Origin:      "https://github.com/x/y",
	})
}

func TestWorker_TickStoreError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	tickDone := make(chan struct{})

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
			close(tickDone)
			return nil, fmt.Errorf("database unavailable")
		})

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			ctrl := gomock.NewController(t)
			store := dbmock.NewMockStore(ctrl)

			mClock := quartz.NewMock(t)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			refresher := newTestRefresher(t, mClock)
			worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

			worker.MarkStale(ctx, gitsync.MarkStaleParams{
				WorkspaceID: uuid.New(),
				Branch:      tc.branch,
				Origin:      tc.origin,
			})
		})
	}
}

func TestWorker_MarkStale_WithChatID(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	targetChat := uuid.New()

	var mu sync.Mutex
	var upsertRefCalls []database.UpsertChatDiffStatusReferenceParams
	var publishedIDs []uuid.UUID

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	// GetChatsByWorkspaceIDs should NOT be called when a specific chat ID is provided.
	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).Times(0)
	store.EXPECT().UpsertChatDiffStatusReference(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
		mu.Lock()
		upsertRefCalls = append(upsertRefCalls, arg)
		mu.Unlock()
		return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
	}).Times(1)

	pub := func(_ context.Context, chatID uuid.UUID) error {
		mu.Lock()
		publishedIDs = append(publishedIDs, chatID)
		mu.Unlock()
		return nil
	}

	mClock := quartz.NewMock(t)
	now := mClock.Now()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: uuid.New(),
		Branch:      "my-branch",
		Origin:      "https://github.com/org/repo",
		ChatID:      targetChat,
	})

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, upsertRefCalls, 1)
	assert.Equal(t, targetChat, upsertRefCalls[0].ChatID)
	assert.Equal(t, "my-branch", upsertRefCalls[0].GitBranch)
	assert.Equal(t, "https://github.com/org/repo", upsertRefCalls[0].GitRemoteOrigin)
	assert.True(t, upsertRefCalls[0].StaleAt.Before(now),
		"stale_at should be in the past, got %v vs now %v", upsertRefCalls[0].StaleAt, now)

	require.Len(t, publishedIDs, 1)
	assert.Equal(t, targetChat, publishedIDs[0])
}

func TestWorker_MarkStale_NilChatID_Broadcasts(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	workspaceID := uuid.New()
	ownerID := uuid.New()
	chat1 := uuid.New()

	var mu sync.Mutex
	var upsertRefCalls []database.UpsertChatDiffStatusReferenceParams
	var publishedIDs []uuid.UUID

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	// Broadcast path: GetChatsByWorkspaceIDs scopes the query to
	// the workspace directly; no post-filtering needed.
	store.EXPECT().GetChatsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ids []uuid.UUID) ([]database.Chat, error) {
			require.Equal(t, []uuid.UUID{workspaceID}, ids)
			return []database.Chat{
				{ID: chat1, OwnerID: ownerID, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}},
			}, nil
		})
	store.EXPECT().UpsertChatDiffStatusReference(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusReferenceParams) (database.ChatDiffStatus, error) {
		mu.Lock()
		upsertRefCalls = append(upsertRefCalls, arg)
		mu.Unlock()
		return database.ChatDiffStatus{ChatID: arg.ChatID}, nil
	}).Times(1)

	pub := func(_ context.Context, chatID uuid.UUID) error {
		mu.Lock()
		publishedIDs = append(publishedIDs, chatID)
		mu.Unlock()
		return nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	// Zero-value ChatID (uuid.Nil) triggers broadcast.
	worker.MarkStale(ctx, gitsync.MarkStaleParams{
		WorkspaceID: workspaceID,
		Branch:      "main",
		Origin:      "https://github.com/org/repo",
	})

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, upsertRefCalls, 1)
	assert.Equal(t, chat1, upsertRefCalls[0].ChatID)
	assert.Equal(t, "main", upsertRefCalls[0].GitBranch)

	require.Len(t, publishedIDs, 1)
	assert.Equal(t, chat1, publishedIDs[0])
}

// TestWorker exercises the worker tick against a
// real PostgreSQL database to verify that the SQL queries, foreign key
// constraints, and upsert logic work end-to-end.
func TestWorker(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	// 1. Real database store.
	db, _ := dbtestutil.NewDB(t)

	// 2. Create a user and an organization (FKs for chats).
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	// 3. Set up FK chain: chat_providers -> chat_model_configs -> chats.
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	modelCfg, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "test-model",
		DisplayName:          "Test Model",
		Enabled:              true,
		ContextLimit:         100000,
		CompressionThreshold: 70,
		Options:              json.RawMessage("{}"),
	})
	require.NoError(t, err)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "integration-test",
	})
	require.NoError(t, err)

	// 4. Seed a stale diff status row so the worker picks it up.
	_, err = db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
		ChatID:          chat.ID,
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/o/r",
		StaleAt:         time.Now().Add(-time.Minute),
		Url:             sql.NullString{},
	})
	require.NoError(t, err)

	// 5. Mock refresher returns a canned PR status.
	mClock := quartz.NewMock(t)
	refresher := newTestRefresher(t, mClock)

	// 6. Track publish calls.
	var publishCount atomic.Int32
	tickDone := make(chan struct{})
	pub := func(_ context.Context, chatID uuid.UUID) error {
		assert.Equal(t, chat.ID, chatID)
		if publishCount.Add(1) == 1 {
			close(tickDone)
		}
		return nil
	}

	// 7. Create and run the worker for one tick.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	worker := gitsync.NewWorker(db, refresher, pub, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	// 8. Assert publisher was called.
	require.Equal(t, int32(1), publishCount.Load())

	// 9. Read back and verify persisted fields.
	status, err := db.GetChatDiffStatusByChatID(ctx, chat.ID)
	require.NoError(t, err)

	// The mock resolveBranchPR returns PRRef{Owner: "o", Repo: "r", Number: 1}
	// and buildPullRequestURL formats it as https://github.com/o/r/pull/1.
	assert.Equal(t, "https://github.com/o/r/pull/1", status.Url.String)
	assert.True(t, status.Url.Valid)
	assert.Equal(t, string(gitprovider.PRStateOpen), status.PullRequestState.String)
	assert.True(t, status.PullRequestState.Valid)
	assert.Equal(t, int32(10), status.Additions)
	assert.Equal(t, int32(3), status.Deletions)
	assert.Equal(t, int32(2), status.ChangedFiles)
	assert.True(t, status.RefreshedAt.Valid, "refreshed_at should be set")
	// The mock clock's Now() + DiffStatusTTL determines stale_at.
	expectedStaleAt := mClock.Now().Add(gitsync.DiffStatusTTL)
	assert.WithinDuration(t, expectedStaleAt, status.StaleAt, time.Second)
}

func TestRefreshChat_Success(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	row := database.ChatDiffStatus{
		ChatID:          chatID,
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/owner/repo",
	}

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	upsertedStatus := database.ChatDiffStatus{
		ChatID:       chatID,
		Url:          sql.NullString{String: "https://github.com/o/r/pull/1", Valid: true},
		Additions:    10,
		Deletions:    3,
		ChangedFiles: 2,
	}
	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatDiffStatusParams) (database.ChatDiffStatus, error) {
			assert.Equal(t, chatID, arg.ChatID)
			return upsertedStatus, nil
		})

	var publishCalled atomic.Bool
	pub := func(_ context.Context, id uuid.UUID) error {
		assert.Equal(t, chatID, id)
		publishCalled.Store(true)
		return nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	result, err := worker.RefreshChat(ctx, row, ownerID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, chatID, result.ChatID)
	assert.Equal(t, upsertedStatus.Url, result.Url)
	assert.True(t, publishCalled.Load(), "publish should have been called")
}

func TestRefreshChat_NoPR(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	row := database.ChatDiffStatus{
		ChatID:          chatID,
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/owner/repo",
	}

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	// UpsertChatDiffStatus should NOT be called.

	var publishCalled atomic.Bool
	pub := func(_ context.Context, _ uuid.UUID) error {
		publishCalled.Store(true)
		return nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// ResolveBranchPullRequest returns nil → no PR exists yet.
	refresher := newTestRefresher(t, mClock, withResolveBranchPR(
		func(context.Context, string, gitprovider.BranchRef) (*gitprovider.PRRef, error) {
			return nil, nil
		},
	))
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	result, err := worker.RefreshChat(ctx, row, ownerID)
	require.NoError(t, err)
	assert.Nil(t, result, "result should be nil when no PR exists")
	assert.False(t, publishCalled.Load(), "publish should not be called when no PR exists")
}

func TestRefreshChat_RefreshError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	row := database.ChatDiffStatus{
		ChatID:          chatID,
		Url:             sql.NullString{String: "https://github.com/org/repo/pull/1", Valid: true},
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/owner/repo",
	}

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	// UpsertChatDiffStatus should NOT be called.

	// Provider resolver returns nil → "no provider" error.
	providers := func(string) gitprovider.Provider { return nil }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) {
		return ptr.Ref("tok"), nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := gitsync.NewRefresher(providers, tokens, logger, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	result, err := worker.RefreshChat(ctx, row, ownerID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no provider")
	assert.Nil(t, result)
}

func TestRefreshChat_UpsertError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	row := database.ChatDiffStatus{
		ChatID:          chatID,
		GitBranch:       "feature",
		GitRemoteOrigin: "https://github.com/owner/repo",
	}

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().UpsertChatDiffStatus(gomock.Any(), gomock.Any()).
		Return(database.ChatDiffStatus{}, fmt.Errorf("db write error"))

	var publishCalled atomic.Bool
	pub := func(_ context.Context, _ uuid.UUID) error {
		publishCalled.Store(true)
		return nil
	}

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := newTestRefresher(t, mClock)
	worker := gitsync.NewWorker(store, refresher, pub, mClock, logger)

	result, err := worker.RefreshChat(ctx, row, ownerID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert chat diff status")
	assert.Nil(t, result)
	assert.False(t, publishCalled.Load(), "publish should not be called when upsert fails")
}

func TestWorker_NoTokenBackoff(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	chatID := uuid.New()
	ownerID := uuid.New()

	var mu sync.Mutex
	var backoffArgs []database.BackoffChatDiffStatusParams
	tickDone := make(chan struct{})

	mClock := quartz.NewMock(t)

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)

	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).
		Return([]database.AcquireStaleChatDiffStatusesRow{
			makeAcquiredRowWithBranch(chatID, ownerID, "feature"),
		}, nil)
	store.EXPECT().BackoffChatDiffStatus(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.BackoffChatDiffStatusParams) error {
			mu.Lock()
			backoffArgs = append(backoffArgs, arg)
			mu.Unlock()
			close(tickDone)
			return nil
		})

	// Token resolver returns empty token → ErrNoTokenAvailable.
	// Provider methods should never be called.
	prov := &mockProvider{}
	providers := func(string) gitprovider.Provider { return prov }
	tokens := func(context.Context, uuid.UUID, string) (*string, error) {
		return ptr.Ref(""), nil
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := gitsync.NewRefresher(providers, tokens, logger, mClock)
	worker := gitsync.NewWorker(store, refresher, nil, mClock, logger)

	tickOnce(ctx, t, mClock, worker, tickDone)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, backoffArgs, 1)
	assert.Equal(t, chatID, backoffArgs[0].ChatID)

	// The backoff should use NoTokenBackoff (10min), not
	// DiffStatusTTL (2min).
	expectedStaleAt := mClock.Now().UTC().Add(gitsync.NoTokenBackoff)
	assert.WithinDuration(t, expectedStaleAt, backoffArgs[0].StaleAt, time.Second)
}
