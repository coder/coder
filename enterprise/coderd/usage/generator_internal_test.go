package usage

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	agplusage "github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestGenerateBucketUniqueViolation pins that a unique violation on the
// bucket index resolves the bucket as complete: another writer already
// recorded it. TestGeneratorConcurrentReplicas also reaches this path, but
// only when its goroutines actually interleave; this case cannot pass by
// scheduling accident.
func TestGenerateBucketUniqueViolation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

	mDB.EXPECT().
		GetTotalChatMessageRuntimeMsInRange(gomock.Any(), gomock.Any()).
		Return(int64(1000), nil)
	mDB.EXPECT().
		InsertUsageEvent(gomock.Any(), gomock.Any()).
		Return(&pq.Error{
			Code:       "23505", // unique_violation
			Constraint: string(database.UniqueIndexUsageEventsAgentRuntime),
		})

	bucket := time.Date(2025, 3, 10, 10, 0, 0, 0, time.UTC)
	require.NoError(t, gen.generateBucket(ctx, bucket))
}

func TestGeneratorStartCheckpointFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	mDB.EXPECT().EnsureAgentRuntimeBackfillCheckpoint(gomock.Any()).Return(xerrors.New("boom"))
	gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

	err := gen.Start(ctx)
	require.ErrorContains(t, err, "ensure agent runtime backfill checkpoint")
	require.Equal(t, err, gen.Start(ctx), "subsequent starts return the initialization error")
	require.NoError(t, gen.Close())
}

func TestGenerateHistoricalCatchupBatchLockUnavailable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("agent_runtime_historical_backfill")).
		DoAndReturn(func(f func(database.Store) error, _ *database.TxOptions) error { return f(mDB) })
	mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAgentRuntimeHistoricalBackfill)).Return(false, nil)
	gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

	result, err := gen.generateHistoricalCatchupBatch(ctx)
	require.NoError(t, err)
	require.False(t, result.lockAcquired)
}

func TestGenerateHistoricalCatchupBatchFailureDoesNotAdvance(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	checkpoint, err := agplusage.MarshalAgentRuntimeBackfillState(agplusage.AgentRuntimeBackfillState{
		Version:      agplusage.AgentRuntimeBackfillVersion,
		Status:       agplusage.AgentRuntimeBackfillStatusRunning,
		NextBucket:   &start,
		EndExclusive: &end,
	})
	require.NoError(t, err)

	mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("agent_runtime_historical_backfill")).
		DoAndReturn(func(f func(database.Store) error, _ *database.TxOptions) error { return f(mDB) })
	mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAgentRuntimeHistoricalBackfill)).Return(true, nil)
	mDB.EXPECT().GetAgentRuntimeBackfillCheckpoint(gomock.Any()).Return(database.GetAgentRuntimeBackfillCheckpointRow{
		Value: checkpoint, Present: true,
	}, nil)
	mDB.EXPECT().ListMissingChatMessageRuntimeBuckets(gomock.Any(), database.ListMissingChatMessageRuntimeBucketsParams{
		StartTime: start, EndTime: end,
	}).Return([]database.ListMissingChatMessageRuntimeBucketsRow{{Bucket: start, RuntimeMs: 1000}}, nil)
	mDB.EXPECT().InsertUsageEvent(gomock.Any(), gomock.Any()).Return(xerrors.New("insert failed"))
	// No checkpoint update is expected, so gomock fails if the cursor advances.
	gen := NewGenerator(quartz.NewMock(t), slogtest.Make(t, nil), mDB, NewDBInserter())

	_, err = gen.generateHistoricalCatchupBatch(ctx)
	require.ErrorContains(t, err, "insert historical bucket")
}

func TestUpdateHistoricalCatchupCheckpointRequiresExistingRow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mDB.EXPECT().UpdateAgentRuntimeBackfillCheckpoint(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	err := updateHistoricalCatchupCheckpoint(ctx, mDB, agplusage.AgentRuntimeBackfillState{
		Version:      agplusage.AgentRuntimeBackfillVersion,
		Status:       agplusage.AgentRuntimeBackfillStatusRunning,
		NextBucket:   &start,
		EndExclusive: &end,
	})
	require.ErrorContains(t, err, "expected 1 row, updated 0")
}

func TestGenerateHistoricalCatchupBatchSkipsNegativeBucket(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	clock := quartz.NewMock(t)
	clock.Set(time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC))
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	checkpoint, err := agplusage.MarshalAgentRuntimeBackfillState(agplusage.AgentRuntimeBackfillState{
		Version:      agplusage.AgentRuntimeBackfillVersion,
		Status:       agplusage.AgentRuntimeBackfillStatusRunning,
		NextBucket:   &start,
		EndExclusive: &end,
	})
	require.NoError(t, err)

	mDB.EXPECT().InTx(gomock.Any(), database.DefaultTXOptions().WithID("agent_runtime_historical_backfill")).
		DoAndReturn(func(f func(database.Store) error, _ *database.TxOptions) error { return f(mDB) })
	mDB.EXPECT().TryAcquireLock(gomock.Any(), int64(database.LockIDAgentRuntimeHistoricalBackfill)).Return(true, nil)
	mDB.EXPECT().GetAgentRuntimeBackfillCheckpoint(gomock.Any()).Return(database.GetAgentRuntimeBackfillCheckpointRow{
		Value: checkpoint, Present: true,
	}, nil)
	mDB.EXPECT().ListMissingChatMessageRuntimeBuckets(gomock.Any(), database.ListMissingChatMessageRuntimeBucketsParams{
		StartTime: start, EndTime: end,
	}).Return([]database.ListMissingChatMessageRuntimeBucketsRow{{Bucket: start, RuntimeMs: -1}}, nil)
	mDB.EXPECT().UpdateAgentRuntimeBackfillCheckpoint(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, value string) (int64, error) {
			state, err := agplusage.ParseAgentRuntimeBackfillState(value)
			require.NoError(t, err)
			require.Equal(t, agplusage.AgentRuntimeBackfillStatusComplete, state.Status)
			require.Equal(t, end, *state.NextBucket)
			return 1, nil
		})
	gen := NewGenerator(clock, slogtest.Make(t, nil), mDB, NewDBInserter())

	result, err := gen.generateHistoricalCatchupBatch(ctx)
	require.NoError(t, err)
	require.True(t, result.complete)
	require.Equal(t, []time.Time{start}, result.invalidBuckets)
}
