package chattool

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWaitForMCPDiscovery(t *testing.T) {
	t.Parallel()

	agentRow := func(runID string) database.WorkspaceAgent {
		return database.WorkspaceAgent{AgentRunID: runID}
	}
	snapshot := func(runID string, phase database.WorkspaceAgentMcpDiscoveryPhase) database.WorkspaceAgentContextSnapshot {
		return database.WorkspaceAgentContextSnapshot{AgentRunID: runID, McpDiscoveryPhase: phase}
	}
	// expectSummary is the row read the wait performs once it stops
	// polling; the rows exercise every server and config classification.
	expectSummary := func(db *dbmock.MockStore, agentID uuid.UUID) {
		toolBody, err := json.Marshal(map[string]any{"server_name": "fs", "tools": []map[string]any{{"name": "read"}}})
		require.NoError(t, err)
		db.EXPECT().ListWorkspaceAgentContextResources(gomock.Any(), agentID).Return([]database.WorkspaceAgentContextResource{
			{BodyKind: database.WorkspaceAgentContextBodyKindMcpServer, Status: database.WorkspaceAgentContextResourceStatusOk, Body: toolBody},
			{BodyKind: database.WorkspaceAgentContextBodyKindMcpServer, Status: database.WorkspaceAgentContextResourceStatusOk, Body: []byte(`{"server_name":"empty"}`)},
			{BodyKind: database.WorkspaceAgentContextBodyKindMcpServer, Status: database.WorkspaceAgentContextResourceStatusUnreadable, Body: []byte(`{}`)},
			{BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig, Status: database.WorkspaceAgentContextResourceStatusInvalid, Body: []byte(`{}`)},
			{BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig, Status: database.WorkspaceAgentContextResourceStatusOk, Body: []byte(`{}`)},
			{BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Status: database.WorkspaceAgentContextResourceStatusOk, Body: []byte(`{}`)},
		}, nil).MaxTimes(1)
	}
	requireCounts := func(t *testing.T, got MCPDiscoveryOutcome) {
		t.Helper()
		require.Equal(t, 1, got.ServersOK)
		require.Equal(t, 1, got.ServersEmpty)
		require.Equal(t, 1, got.ServersFailed)
		require.Equal(t, 1, got.ConfigErrors)
	}

	t.Run("AlreadyComplete", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
		require.False(t, got.Stale)
		requireCounts(t, got)
	})

	t.Run("LegacyUnspecifiedReturnsImmediately", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(""), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("", database.WorkspaceAgentMcpDiscoveryPhaseUnspecified), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("NoSnapshotEventuallyAppears", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil).AnyTimes()
		call := 0
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows
				}
				return snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
			}).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.GreaterOrEqual(t, call, 3)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("PendingThenComplete", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil).AnyTimes()
		call := 0
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					return snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil
				}
				return snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
			}).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.GreaterOrEqual(t, call, 3)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("StaleRunWaitsForCurrentRun", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b"), nil).AnyTimes()
		call := 0
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					// A complete snapshot from the previous process never
					// establishes current completeness.
					return snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
				}
				return snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
			}).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.GreaterOrEqual(t, call, 3)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.Stale)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("StaleRunOnlyTimesOutAfterGrace", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b"), nil).AnyTimes()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil).AnyTimes()
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		elapsed := time.Since(start)
		require.Greater(t, elapsed, 2*time.Second, "a previous process snapshot gets the no-snapshot grace")
		require.Less(t, elapsed, testutil.WaitShort)
		require.True(t, got.WaitTimedOut)
		require.True(t, got.Stale)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
	})

	t.Run("LegacySnapshotUnderCurrentRunIsStale", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b"), nil).AnyTimes()
		call := 0
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			DoAndReturn(func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
				call++
				if call <= 2 {
					// A complete snapshot left by a legacy process (no run
					// id) under a current agent is a previous process too.
					return snapshot("", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
				}
				return snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil
			}).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.GreaterOrEqual(t, call, 3, "the wait does not accept the legacy snapshot")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.Stale)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("NoSnapshotTimesOutAfterGrace", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil).AnyTimes()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows).AnyTimes()

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		elapsed := time.Since(start)
		require.Greater(t, elapsed, 2*time.Second)
		require.Less(t, elapsed, testutil.WaitShort)
		require.True(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
	})

	t.Run("CurrentReadsOnce", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b"), nil).Times(3)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		got, complete := CurrentMCPDiscovery(context.Background(), db, agentID)
		require.False(t, complete)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)

		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		got, complete = CurrentMCPDiscovery(context.Background(), db, agentID)
		require.False(t, complete, "a previous process's completion does not count")
		require.True(t, got.Stale)

		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		expectSummary(db, agentID)
		got, complete = CurrentMCPDiscovery(context.Background(), db, agentID)
		require.True(t, complete)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
		requireCounts(t, got)
	})

	t.Run("DBErrorFailsOpen", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil).AnyTimes()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, xerrors.New("connection refused"))

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.False(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
	})

	t.Run("CanceledWhilePending", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a"), nil).AnyTimes()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil).AnyTimes()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
		defer cancel()
		start := time.Now()
		got := WaitForMCPDiscovery(ctx, db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.True(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
	})
}
