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

	// agentRow returns the agent row with its discovery anchor set to
	// startedAgo before now; a negative value leaves the anchor NULL.
	agentRow := func(runID string, startedAgo time.Duration) database.WorkspaceAgent {
		agent := database.WorkspaceAgent{AgentRunID: runID}
		if startedAgo >= 0 {
			agent.McpDiscoveryStartedAt = sql.NullTime{Time: time.Now().Add(-startedAgo), Valid: true}
		}
		return agent
	}
	snapshot := func(runID string, phase database.WorkspaceAgentMcpDiscoveryPhase) database.WorkspaceAgentContextSnapshot {
		return database.WorkspaceAgentContextSnapshot{AgentRunID: runID, McpDiscoveryPhase: phase}
	}
	// sequence serves first for the first reads calls and then second.
	sequence := func(reads int, first, second database.WorkspaceAgentContextSnapshot, firstErr error) (func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error), *int) {
		calls := 0
		return func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
			calls++
			if calls <= reads {
				return first, firstErr
			}
			return second, nil
		}, &calls
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
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a", 0), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
		require.False(t, got.Stale)
		requireCounts(t, got)
	})

	t.Run("LegacyAgentNeverWaits", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("", 0), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("LegacyAgentWithCurrentRunSnapshotIsStale", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("", 0), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.True(t, got.Stale)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("StartupScriptsRunningReturnsAtOnce", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a", -1), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("NoSnapshotThenComplete", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		read, calls := sequence(2, database.WorkspaceAgentContextSnapshot{}, snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), sql.ErrNoRows)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).AnyTimes()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a", 0), nil).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Equal(t, 3, *calls)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("PendingThenComplete", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		read, calls := sequence(2, snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).AnyTimes()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a", 0), nil).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Equal(t, 3, *calls)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("PreviousRunSnapshotWaitsForCurrentRun", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		// A complete snapshot from the previous process never
		// establishes current completeness.
		read, calls := sequence(2, snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).AnyTimes()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b", 0), nil).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Equal(t, 3, *calls)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.Stale)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("LimitAlreadyPassedReturnsAtOnce", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-b", MCPDiscoveryWaitTimeout+time.Second), nil)
		expectSummary(db, agentID)

		start := time.Now()
		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.True(t, got.WaitTimedOut)
		require.True(t, got.Stale)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		requireCounts(t, got)
	})

	t.Run("LimitReachedWhilePending", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil).AnyTimes()
		// Two seconds of the run's limit remain.
		agent := agentRow("run-a", MCPDiscoveryWaitTimeout-2*time.Second)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agent, nil).AnyTimes()
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(context.Background(), db, agentID)
		require.False(t, time.Now().Before(agent.McpDiscoveryStartedAt.Time.Add(MCPDiscoveryWaitTimeout)),
			"the wait lasts until the run's limit")
		require.Less(t, time.Since(agent.McpDiscoveryStartedAt.Time), MCPDiscoveryWaitTimeout+testutil.WaitShort)
		require.True(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
	})

	t.Run("DBErrorFailsOpen", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
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
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil).AnyTimes()
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow("run-a", 0), nil).AnyTimes()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
		defer cancel()
		start := time.Now()
		got := WaitForMCPDiscovery(ctx, db, agentID)
		require.Less(t, time.Since(start), testutil.WaitShort)
		require.False(t, got.WaitTimedOut, "cancellation is not the run's limit")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
	})
}
