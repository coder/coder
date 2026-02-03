package boundaryusage_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestTracker_New(t *testing.T) {
	t.Parallel()

	tracker := boundaryusage.NewTracker()
	require.NotNil(t, tracker)
}

func TestTracker_Track_Single(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	workspaceID := uuid.New()
	ownerID := uuid.New()
	replicaID := uuid.New()

	tracker.Track(workspaceID, ownerID, 5, 2)

	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Verify the data was written correctly.
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.UniqueUsers)
	require.Equal(t, int64(5), summary.AllowedRequests)
	require.Equal(t, int64(2), summary.DeniedRequests)
}

func TestTracker_Track_DuplicateWorkspaceUser(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	workspaceID := uuid.New()
	ownerID := uuid.New()
	replicaID := uuid.New()

	// Track same workspace/user multiple times.
	tracker.Track(workspaceID, ownerID, 3, 1)
	tracker.Track(workspaceID, ownerID, 4, 2)
	tracker.Track(workspaceID, ownerID, 2, 0)

	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces, "should be 1 unique workspace")
	require.Equal(t, int64(1), summary.UniqueUsers, "should be 1 unique user")
	require.Equal(t, int64(9), summary.AllowedRequests, "should accumulate: 3+4+2=9")
	require.Equal(t, int64(3), summary.DeniedRequests, "should accumulate: 1+2+0=3")
}

func TestTracker_Track_MultipleWorkspacesUsers(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	// Track 3 different workspaces with 2 different users.
	workspace1, workspace2, workspace3 := uuid.New(), uuid.New(), uuid.New()
	user1, user2 := uuid.New(), uuid.New()

	tracker.Track(workspace1, user1, 1, 0)
	tracker.Track(workspace2, user1, 2, 1)
	tracker.Track(workspace3, user2, 3, 2)

	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(3), summary.UniqueWorkspaces)
	require.Equal(t, int64(2), summary.UniqueUsers)
	require.Equal(t, int64(6), summary.AllowedRequests)
	require.Equal(t, int64(3), summary.DeniedRequests)
}

func TestTracker_Track_Concurrent(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	const numGoroutines = 100
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workspaceID := uuid.New()
			ownerID := uuid.New()
			for j := 0; j < requestsPerGoroutine; j++ {
				tracker.Track(workspaceID, ownerID, 1, 1)
			}
		}()
	}
	wg.Wait()

	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(numGoroutines), summary.UniqueWorkspaces)
	require.Equal(t, int64(numGoroutines), summary.UniqueUsers)
	require.Equal(t, int64(numGoroutines*requestsPerGoroutine), summary.AllowedRequests)
	require.Equal(t, int64(numGoroutines*requestsPerGoroutine), summary.DeniedRequests)
}

func TestTracker_FlushToDB_Accumulates(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()
	workspaceID := uuid.New()
	ownerID := uuid.New()

	// First flush is an insert, resets unique counts (new period).
	tracker.Track(workspaceID, ownerID, 5, 3)
	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Track & flush more data. Same workspace/user, so unique counts stay at 1.
	tracker.Track(workspaceID, ownerID, 2, 1)
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Track & flush even more data to continue accumulation.
	tracker.Track(workspaceID, ownerID, 3, 2)
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.UniqueUsers)
	require.Equal(t, int64(5+2+3), summary.AllowedRequests)
	require.Equal(t, int64(3+1+2), summary.DeniedRequests)
}

func TestTracker_FlushToDB_NewPeriod(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()
	workspaceID := uuid.New()
	ownerID := uuid.New()

	tracker.Track(workspaceID, ownerID, 10, 5)

	// First flush.
	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Simulate telemetry reset (new period).
	err = db.ResetBoundaryUsageStats(boundaryCtx)
	require.NoError(t, err)

	// Track new data.
	workspace2 := uuid.New()
	owner2 := uuid.New()
	tracker.Track(workspace2, owner2, 3, 1)

	// Flushing again should detect new period and reset in-memory stats.
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// The summary should only contain the new data after reset.
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces, "should only count new workspace")
	require.Equal(t, int64(1), summary.UniqueUsers, "should only count new user")
	require.Equal(t, int64(3), summary.AllowedRequests, "should only count new requests")
	require.Equal(t, int64(1), summary.DeniedRequests, "should only count new requests")
}

func TestTracker_FlushToDB_NoActivity(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Verify nothing was written to DB.
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.UniqueWorkspaces)
	require.Equal(t, int64(0), summary.AllowedRequests)
}

func TestUpsertBoundaryUsageStats_Insert(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	replicaID := uuid.New()

	// Set different values for delta vs cumulative to verify INSERT uses delta.
	newPeriod, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesDelta: 5,
		UniqueUsersDelta:      3,
		UniqueWorkspacesCount: 999, // should be ignored on INSERT
		UniqueUsersCount:      999, // should be ignored on INSERT
		AllowedRequests:       100,
		DeniedRequests:        10,
	})
	require.NoError(t, err)
	require.True(t, newPeriod, "should return true for insert")

	// Verify INSERT used the delta values, not cumulative.
	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(5), summary.UniqueWorkspaces)
	require.Equal(t, int64(3), summary.UniqueUsers)
}

func TestUpsertBoundaryUsageStats_Update(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	replicaID := uuid.New()

	// First insert uses delta fields.
	_, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesDelta: 5,
		UniqueUsersDelta:      3,
		AllowedRequests:       100,
		DeniedRequests:        10,
	})
	require.NoError(t, err)

	// Second upsert (update). Set different delta vs cumulative to verify UPDATE uses cumulative.
	newPeriod, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesCount: 8, // cumulative, should be used
		UniqueUsersCount:      5, // cumulative, should be used
		AllowedRequests:       200,
		DeniedRequests:        20,
	})
	require.NoError(t, err)
	require.False(t, newPeriod, "should return false for update")

	// Verify UPDATE used cumulative values.
	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(8), summary.UniqueWorkspaces)
	require.Equal(t, int64(5), summary.UniqueUsers)
	require.Equal(t, int64(100+200), summary.AllowedRequests)
	require.Equal(t, int64(10+20), summary.DeniedRequests)
}

func TestGetBoundaryUsageSummary_MultipleReplicas(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	replica1 := uuid.New()
	replica2 := uuid.New()
	replica3 := uuid.New()

	// Insert stats for 3 replicas. Delta fields are used for INSERT.
	_, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replica1,
		UniqueWorkspacesDelta: 10,
		UniqueUsersDelta:      5,
		AllowedRequests:       100,
		DeniedRequests:        10,
	})
	require.NoError(t, err)

	_, err = db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replica2,
		UniqueWorkspacesDelta: 15,
		UniqueUsersDelta:      8,
		AllowedRequests:       150,
		DeniedRequests:        15,
	})
	require.NoError(t, err)

	_, err = db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replica3,
		UniqueWorkspacesDelta: 20,
		UniqueUsersDelta:      12,
		AllowedRequests:       200,
		DeniedRequests:        20,
	})
	require.NoError(t, err)

	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)

	// Verify aggregation (SUM of all replicas).
	require.Equal(t, int64(45), summary.UniqueWorkspaces) // 10 + 15 + 20
	require.Equal(t, int64(25), summary.UniqueUsers)      // 5 + 8 + 12
	require.Equal(t, int64(450), summary.AllowedRequests) // 100 + 150 + 200
	require.Equal(t, int64(45), summary.DeniedRequests)   // 10 + 15 + 20
}

func TestGetBoundaryUsageSummary_Empty(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)

	// COALESCE should return 0 for all columns.
	require.Equal(t, int64(0), summary.UniqueWorkspaces)
	require.Equal(t, int64(0), summary.UniqueUsers)
	require.Equal(t, int64(0), summary.AllowedRequests)
	require.Equal(t, int64(0), summary.DeniedRequests)
}

func TestResetBoundaryUsageStats(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	// Insert stats for multiple replicas. Delta fields are used for INSERT.
	for i := 0; i < 5; i++ {
		_, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             uuid.New(),
			UniqueWorkspacesDelta: int64(i + 1),
			UniqueUsersDelta:      int64(i + 1),
			AllowedRequests:       int64((i + 1) * 10),
			DeniedRequests:        int64(i + 1),
		})
		require.NoError(t, err)
	}

	// Verify data exists.
	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)
	require.Greater(t, summary.AllowedRequests, int64(0))

	// Reset.
	err = db.ResetBoundaryUsageStats(ctx)
	require.NoError(t, err)

	// Verify all data is gone.
	summary, err = db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.UniqueWorkspaces)
	require.Equal(t, int64(0), summary.AllowedRequests)
}

func TestDeleteBoundaryUsageStatsByReplicaID(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsBoundaryUsageTracker(context.Background())

	replica1 := uuid.New()
	replica2 := uuid.New()

	// Insert stats for 2 replicas. Delta fields are used for INSERT.
	_, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replica1,
		UniqueWorkspacesDelta: 10,
		UniqueUsersDelta:      5,
		AllowedRequests:       100,
		DeniedRequests:        10,
	})
	require.NoError(t, err)

	_, err = db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replica2,
		UniqueWorkspacesDelta: 20,
		UniqueUsersDelta:      10,
		AllowedRequests:       200,
		DeniedRequests:        20,
	})
	require.NoError(t, err)

	// Delete replica1's stats.
	err = db.DeleteBoundaryUsageStatsByReplicaID(ctx, replica1)
	require.NoError(t, err)

	// Verify only replica2's stats remain.
	summary, err := db.GetBoundaryUsageSummary(ctx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(20), summary.UniqueWorkspaces)
	require.Equal(t, int64(200), summary.AllowedRequests)
}

func TestTracker_TelemetryCycle(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)

	// Simulate 3 replicas.
	tracker1 := boundaryusage.NewTracker()
	tracker2 := boundaryusage.NewTracker()
	tracker3 := boundaryusage.NewTracker()

	replica1 := uuid.New()
	replica2 := uuid.New()
	replica3 := uuid.New()

	// Each tracker records different workspaces/users.
	tracker1.Track(uuid.New(), uuid.New(), 10, 1)
	tracker1.Track(uuid.New(), uuid.New(), 15, 2)

	tracker2.Track(uuid.New(), uuid.New(), 20, 3)
	tracker2.Track(uuid.New(), uuid.New(), 25, 4)
	tracker2.Track(uuid.New(), uuid.New(), 30, 5)

	tracker3.Track(uuid.New(), uuid.New(), 5, 0)

	// All replicas flush to database.
	require.NoError(t, tracker1.FlushToDB(ctx, db, replica1))
	require.NoError(t, tracker2.FlushToDB(ctx, db, replica2))
	require.NoError(t, tracker3.FlushToDB(ctx, db, replica3))

	// Telemetry aggregates.
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)

	// Verify aggregation.
	require.Equal(t, int64(6), summary.UniqueWorkspaces)  // 2 + 3 + 1
	require.Equal(t, int64(6), summary.UniqueUsers)       // 2 + 3 + 1
	require.Equal(t, int64(105), summary.AllowedRequests) // 25 + 75 + 5
	require.Equal(t, int64(15), summary.DeniedRequests)   // 3 + 12 + 0

	// Telemetry resets stats (simulating telemetry report sent).
	require.NoError(t, db.ResetBoundaryUsageStats(boundaryCtx))

	// Next flush from trackers should detect new period.
	tracker1.Track(uuid.New(), uuid.New(), 1, 0)
	require.NoError(t, tracker1.FlushToDB(ctx, db, replica1))

	// Verify trackers reset their in-memory state.
	summary, err = db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.AllowedRequests)
}

func TestTracker_FlushToDB_NoStaleDataAfterReset(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()
	workspaceID := uuid.New()
	ownerID := uuid.New()

	// Track some data, flush, and verify.
	tracker.Track(workspaceID, ownerID, 10, 5)
	err := tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(10), summary.AllowedRequests)

	// Simulate telemetry reset (new period).
	err = db.ResetBoundaryUsageStats(boundaryCtx)
	require.NoError(t, err)
	summary, err = db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.AllowedRequests)

	// Flush again without any new Track() calls. This should not write stale
	// data back to the DB.
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	// Summary should be empty (no stale data written).
	summary, err = db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.UniqueWorkspaces)
	require.Equal(t, int64(0), summary.UniqueUsers)
	require.Equal(t, int64(0), summary.AllowedRequests)
	require.Equal(t, int64(0), summary.DeniedRequests)
}

func TestTracker_ConcurrentFlushAndTrack(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	const numOperations = 50

	var wg sync.WaitGroup

	// Goroutine 1: Continuously track.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			tracker.Track(uuid.New(), uuid.New(), 1, 1)
		}
	}()

	// Goroutine 2: Continuously flush.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			_ = tracker.FlushToDB(ctx, db, replicaID)
		}
	}()

	wg.Wait()

	// Final flush to capture any remaining data.
	require.NoError(t, tracker.FlushToDB(ctx, db, replicaID))

	// Verify stats are non-negative.
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.GreaterOrEqual(t, summary.AllowedRequests, int64(0))
	require.GreaterOrEqual(t, summary.DeniedRequests, int64(0))
}

// trackDuringUpsertDB wraps a database.Store to call Track() during the
// UpsertBoundaryUsageStats operation, simulating a concurrent Track() call.
type trackDuringUpsertDB struct {
	database.Store
	tracker     *boundaryusage.Tracker
	workspaceID uuid.UUID
	userID      uuid.UUID
}

func (s *trackDuringUpsertDB) UpsertBoundaryUsageStats(ctx context.Context, arg database.UpsertBoundaryUsageStatsParams) (bool, error) {
	s.tracker.Track(s.workspaceID, s.userID, 20, 10)
	return s.Store.UpsertBoundaryUsageStats(ctx, arg)
}

func TestTracker_TrackDuringFlush(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	// Track some initial data.
	tracker.Track(uuid.New(), uuid.New(), 10, 5)

	trackingDB := &trackDuringUpsertDB{
		Store:       db,
		tracker:     tracker,
		workspaceID: uuid.New(),
		userID:      uuid.New(),
	}

	// Flush will call Track() during the DB operation.
	err := tracker.FlushToDB(ctx, trackingDB, replicaID)
	require.NoError(t, err)

	// Verify first flush only wrote the initial data.
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(10), summary.AllowedRequests)

	// The second flush should include the Track() call that happened during the
	// first flush's DB operation.
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	summary, err = db.GetBoundaryUsageSummary(boundaryCtx, 60000)
	require.NoError(t, err)
	require.Equal(t, int64(10+20), summary.AllowedRequests)
	require.Equal(t, int64(5+10), summary.DeniedRequests)
}
