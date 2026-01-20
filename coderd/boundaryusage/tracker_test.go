package boundaryusage_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestTracker(t *testing.T) {
	t.Parallel()

	t.Run("TrackAndFlush", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		ws1, owner1 := uuid.New(), uuid.New()
		ws2, owner2 := uuid.New(), uuid.New()

		tracker.Track(ws1, owner1, 5, 2)
		tracker.Track(ws2, owner2, 3, 1)

		// Expect upsert with 2 unique workspaces, 2 unique users, 8 allowed, 3 denied.
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 2,
			UniqueUsersCount:      2,
			AllowedRequests:       8,
			DeniedRequests:        3,
		}).Return(false, nil) // false = UPDATE (not new period)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("DeduplicatesSameWorkspaceAndUser", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		ws := uuid.New()
		owner := uuid.New()

		// Track same workspace/owner multiple times.
		tracker.Track(ws, owner, 5, 2)
		tracker.Track(ws, owner, 3, 1)

		// Should have 1 unique workspace, 1 unique user, but accumulated request counts.
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1,
			UniqueUsersCount:      1,
			AllowedRequests:       8,
			DeniedRequests:        3,
		}).Return(false, nil)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("StatsAccumulateAcrossFlushes", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		ws, owner := uuid.New(), uuid.New()

		tracker.Track(ws, owner, 5, 2)

		// First flush.
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1,
			UniqueUsersCount:      1,
			AllowedRequests:       5,
			DeniedRequests:        2,
		}).Return(false, nil)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)

		// Track more requests.
		tracker.Track(ws, owner, 3, 1)

		// Second flush should have accumulated request counts.
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1,
			UniqueUsersCount:      1,
			AllowedRequests:       8, // Accumulated: 5 + 3.
			DeniedRequests:        3, // Accumulated: 2 + 1.
		}).Return(false, nil)

		err = tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("StatsResetOnNewPeriod", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		ws1, owner1 := uuid.New(), uuid.New()

		tracker.Track(ws1, owner1, 1, 0)

		// First flush returns true (INSERT = new period).
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1,
			UniqueUsersCount:      1,
			AllowedRequests:       1,
			DeniedRequests:        0,
		}).Return(true, nil) // true = INSERT (new period)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)

		// Track a different workspace/user.
		ws2, owner2 := uuid.New(), uuid.New()
		tracker.Track(ws2, owner2, 2, 0)

		// Second flush should only have the new stats (all were reset).
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1, // Only ws2, ws1 was reset.
			UniqueUsersCount:      1, // Only owner2, owner1 was reset.
			AllowedRequests:       2, // Only 2, the 1 was reset.
			DeniedRequests:        0,
		}).Return(false, nil)

		err = tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("StatsPreservedOnUpdate", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		ws1, owner1 := uuid.New(), uuid.New()

		tracker.Track(ws1, owner1, 1, 0)

		// First flush returns false (UPDATE = same period).
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 1,
			UniqueUsersCount:      1,
			AllowedRequests:       1,
			DeniedRequests:        0,
		}).Return(false, nil) // false = UPDATE (same period)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)

		// Track a different workspace/user.
		ws2, owner2 := uuid.New(), uuid.New()
		tracker.Track(ws2, owner2, 2, 0)

		// Second flush should have all stats accumulated.
		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: 2, // Both ws1 and ws2.
			UniqueUsersCount:      2, // Both owner1 and owner2.
			AllowedRequests:       3, // Accumulated: 1 + 2.
			DeniedRequests:        0,
		}).Return(false, nil)

		err = tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("NoFlushWhenNoActivity", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		// No Track calls, so no DB call expected.
		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})

	t.Run("ConcurrentTracking", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		var wg sync.WaitGroup
		const numGoroutines = 100

		workspaces := make([]uuid.UUID, numGoroutines)
		owners := make([]uuid.UUID, numGoroutines)
		for i := range numGoroutines {
			workspaces[i] = uuid.New()
			owners[i] = uuid.New()
		}

		for i := range numGoroutines {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tracker.Track(workspaces[idx], owners[idx], 1, 1)
			}(i)
		}

		wg.Wait()

		mockDB.EXPECT().UpsertBoundaryUsageStats(gomock.Any(), database.UpsertBoundaryUsageStatsParams{
			ReplicaID:             replicaID,
			UniqueWorkspacesCount: int64(numGoroutines),
			UniqueUsersCount:      int64(numGoroutines),
			AllowedRequests:       int64(numGoroutines),
			DeniedRequests:        int64(numGoroutines),
		}).Return(false, nil)

		err := tracker.FlushToDB(context.Background(), mockDB, replicaID)
		require.NoError(t, err)
	})
}
