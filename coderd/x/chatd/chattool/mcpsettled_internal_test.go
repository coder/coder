package chattool

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

func TestWaitForMCPSettled(t *testing.T) {
	t.Parallel()

	t.Run("AlreadySettled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{
				McpSettled: sql.NullBool{Bool: true, Valid: true},
			}, nil)

		start := time.Now()
		WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
		elapsed := time.Since(start)
		require.Less(t, elapsed, testutil.WaitShort)
	})

	t.Run("Legacy_NullSettled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{
				McpSettled: sql.NullBool{}, // NULL
			}, nil)

		start := time.Now()
		WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
		elapsed := time.Since(start)
		require.Less(t, elapsed, testutil.WaitShort)
	})

	t.Run("NoSnapshot_EventuallyAppears", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		call := 0
		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows
				}
				return database.WorkspaceAgentContextSnapshot{
					McpSettled: sql.NullBool{Bool: true, Valid: true},
				}, nil
			}).AnyTimes()

		start := time.Now()
		WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
		elapsed := time.Since(start)
		require.GreaterOrEqual(t, call, 3)
		require.Less(t, elapsed, testutil.WaitShort)
	})

	t.Run("TransientDBError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, xerrors.New("connection refused"))

		start := time.Now()
		WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
		elapsed := time.Since(start)
		require.Less(t, elapsed, testutil.WaitShort)
	})

	t.Run("WaitsAndResolves", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		call := 0
		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return database.WorkspaceAgentContextSnapshot{
						McpSettled: sql.NullBool{Bool: false, Valid: true},
					}, nil
				}
				return database.WorkspaceAgentContextSnapshot{
					McpSettled: sql.NullBool{Bool: true, Valid: true},
				}, nil
			}).AnyTimes()

		start := time.Now()
		WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
		elapsed := time.Since(start)
		// Should have polled at least twice before settling.
		require.GreaterOrEqual(t, call, 3)
		require.Less(t, elapsed, testutil.WaitShort)
	})

	t.Run("Canceled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{
				McpSettled: sql.NullBool{Bool: false, Valid: true},
			}, nil).AnyTimes()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
		defer cancel()

		start := time.Now()
		WaitForMCPSettled(ctx, db, agentID, time.Time{})
		elapsed := time.Since(start)
		// Should return after the context timeout, not the full
		// mcpSettledTimeout (15s).
		require.Less(t, elapsed, testutil.WaitShort)
	})
}

func TestWaitForMCPSettled_LegacyAgentNoSnapshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	agentID := uuid.New()

	// Legacy agent never pushes a snapshot.
	db.EXPECT().
		GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
		Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows).
		AnyTimes()

	start := time.Now()
	WaitForMCPSettled(context.Background(), db, agentID, time.Time{})
	elapsed := time.Since(start)
	// Should return within mcpNoSnapshotTimeout (3s) + tolerance,
	// NOT the full mcpSettledTimeout (15s).
	require.Less(t, elapsed, testutil.WaitShort)
	require.Greater(t, elapsed, 2*time.Second)
}

func TestWaitForMCPSettled_StaleSnapshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	agentID := uuid.New()

	staleTime := time.Now().Add(-time.Minute)
	freshTime := time.Now()

	call := 0
	db.EXPECT().
		GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
			call++
			if call <= 2 {
				// Stale snapshot from previous agent process.
				return database.WorkspaceAgentContextSnapshot{
					McpSettled: sql.NullBool{Bool: true, Valid: true},
					ReceivedAt: staleTime,
				}, nil
			}
			// Fresh snapshot from current agent process.
			return database.WorkspaceAgentContextSnapshot{
				McpSettled: sql.NullBool{Bool: true, Valid: true},
				ReceivedAt: freshTime,
			}, nil
		}).AnyTimes()

	start := time.Now()
	// notBefore is between stale and fresh.
	WaitForMCPSettled(context.Background(), db, agentID, staleTime.Add(30*time.Second))
	require.GreaterOrEqual(t, call, 3)
	require.Less(t, time.Since(start), testutil.WaitShort)
}

func TestWaitForMCPSettledIfPending(t *testing.T) {
	t.Parallel()

	t.Run("NoSnapshot_ReturnsImmediately", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)

		start := time.Now()
		WaitForMCPSettledIfPending(context.Background(), db, agentID, time.Time{})
		require.Less(t, time.Since(start), testutil.WaitShort)
	})

	t.Run("AlreadySettled_ReturnsImmediately", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{
				McpSettled: sql.NullBool{Bool: true, Valid: true},
			}, nil)

		start := time.Now()
		WaitForMCPSettledIfPending(context.Background(), db, agentID, time.Time{})
		require.Less(t, time.Since(start), testutil.WaitShort)
	})

	t.Run("Pending_DelegatesToFullWait", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		call := 0
		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return database.WorkspaceAgentContextSnapshot{
						McpSettled: sql.NullBool{Bool: false, Valid: true},
					}, nil
				}
				return database.WorkspaceAgentContextSnapshot{
					McpSettled: sql.NullBool{Bool: true, Valid: true},
				}, nil
			}).AnyTimes()

		start := time.Now()
		WaitForMCPSettledIfPending(context.Background(), db, agentID, time.Time{})
		require.GreaterOrEqual(t, call, 3)
		require.Less(t, time.Since(start), testutil.WaitShort)
	})

	t.Run("NoSnapshot_RecentAgent_DelegatesToFullWait", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		call := 0
		db.EXPECT().
			GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows
				}
				return database.WorkspaceAgentContextSnapshot{
					McpSettled: sql.NullBool{Bool: true, Valid: true},
					ReceivedAt: time.Now(),
				}, nil
			}).AnyTimes()

		start := time.Now()
		WaitForMCPSettledIfPending(context.Background(), db, agentID, time.Now())
		elapsed := time.Since(start)
		require.GreaterOrEqual(t, call, 3,
			"should have polled multiple times via WaitForMCPSettled")
		require.Less(t, elapsed, testutil.WaitShort)
	})
}
