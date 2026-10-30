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
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// listenerPubsub hands the waiter's listener to the test, which can then
// deliver listener errors that the in-memory pubsub never produces.
type listenerPubsub struct {
	pubsub.Pubsub
	subscribeErr error
	listeners    chan pubsub.ListenerWithErr
}

func (p *listenerPubsub) SubscribeWithErr(_ string, listener pubsub.ListenerWithErr) (func(), error) {
	if p.subscribeErr != nil {
		return nil, p.subscribeErr
	}
	p.listeners <- listener
	return func() {}, nil
}

func TestWaitForMCPDiscovery(t *testing.T) {
	t.Parallel()

	// agentRow returns the agent row with its discovery anchor set to
	// startedAgo before the clock's now; a negative value leaves the
	// anchor NULL.
	agentRow := func(clock quartz.Clock, runID string, startedAgo time.Duration) database.WorkspaceAgent {
		agent := database.WorkspaceAgent{AgentRunID: runID}
		if startedAgo >= 0 {
			agent.McpDiscoveryStartedAt = sql.NullTime{Time: clock.Now().Add(-startedAgo), Valid: true}
		}
		return agent
	}
	snapshot := func(runID string, phase database.WorkspaceAgentMcpDiscoveryPhase) database.WorkspaceAgentContextSnapshot {
		return database.WorkspaceAgentContextSnapshot{AgentRunID: runID, McpDiscoveryPhase: phase}
	}
	// sequence serves first on the first read, signaling firstRead, and
	// second on every later read.
	sequence := func(first, second database.WorkspaceAgentContextSnapshot, firstErr error) (read func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error), calls *int, firstRead chan struct{}) {
		n := 0
		firstRead = make(chan struct{})
		return func(context.Context, uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
			n++
			if n == 1 {
				close(firstRead)
				return first, firstErr
			}
			return second, nil
		}, &n, firstRead
	}
	// expectSummary is the row read the wait performs once it stops
	// waiting; the rows exercise every server and config classification.
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
	// waitAsync runs the wait in the background. The mock clock never
	// advances on its own, so a wait that should return at once and does
	// not instead blocks until ctx expires.
	waitAsync := func(ctx context.Context, db database.Store, ps pubsub.Pubsub, clock quartz.Clock, agentID uuid.UUID) <-chan MCPDiscoveryOutcome {
		done := make(chan MCPDiscoveryOutcome, 1)
		go func() { done <- WaitForMCPDiscovery(ctx, db, ps, clock, agentID) }()
		return done
	}
	publish := func(t *testing.T, ps pubsub.Pubsub, agentID uuid.UUID) {
		t.Helper()
		require.NoError(t, ps.Publish(coderdpubsub.WorkspaceAgentMCPDiscoveryChannel(agentID), nil))
	}

	t.Run("AlreadyComplete", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "run-a", 0), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), clock, agentID)
		require.NoError(t, ctx.Err(), "a complete snapshot must not wait")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
		require.False(t, got.Stale)
		requireCounts(t, got)
	})

	t.Run("LegacyAgentNeverWaits", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "", 0), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), clock, agentID)
		require.NoError(t, ctx.Err(), "a legacy agent must not wait")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("LegacyAgentWithCurrentRunSnapshotIsStale", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "", 0), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), clock, agentID)
		require.NoError(t, ctx.Err(), "a legacy agent must not wait")
		require.True(t, got.Stale)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("StartupScriptsRunningReturnsAtOnce", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "run-a", -1), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), clock, agentID)
		require.NoError(t, ctx.Err(), "no discovery anchor means no limit to wait against")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	// The waits below end on a pubsub message: each reads exactly twice,
	// once before waiting and once after the message.
	for _, tc := range []struct {
		name          string
		first, second database.WorkspaceAgentContextSnapshot
		firstErr      error
		agentRunID    string
	}{
		{
			name:       "NoSnapshotThenComplete",
			second:     snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete),
			firstErr:   sql.ErrNoRows,
			agentRunID: "run-a",
		},
		{
			name:       "PendingThenComplete",
			first:      snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending),
			second:     snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete),
			agentRunID: "run-a",
		},
		{
			// A complete snapshot from the previous process never
			// establishes current completeness.
			name:       "PreviousRunSnapshotWaitsForCurrentRun",
			first:      snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete),
			second:     snapshot("run-b", database.WorkspaceAgentMcpDiscoveryPhaseComplete),
			agentRunID: "run-b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			ps := pubsub.NewInMemory()
			db := dbmock.NewMockStore(gomock.NewController(t))
			agentID := uuid.New()
			read, calls, firstRead := sequence(tc.first, tc.second, tc.firstErr)
			db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).Times(2)
			db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, tc.agentRunID, 0), nil).Times(2)
			expectSummary(db, agentID)

			done := waitAsync(ctx, db, ps, clock, agentID)
			// The waiter subscribes before its first read, so this
			// message cannot be lost.
			testutil.TryReceive(ctx, t, firstRead)
			publish(t, ps, agentID)
			got := testutil.TryReceive(ctx, t, done)
			require.Equal(t, 2, *calls)
			require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
			require.False(t, got.Stale)
			require.False(t, got.WaitTimedOut)
		})
	}

	t.Run("DroppedMessagesReadAgain", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		ps := &listenerPubsub{listeners: make(chan pubsub.ListenerWithErr, 1)}
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		read, calls, firstRead := sequence(
			snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending),
			snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).Times(2)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "run-a", 0), nil).Times(2)
		expectSummary(db, agentID)

		done := waitAsync(ctx, db, ps, clock, agentID)
		listener := testutil.TryReceive(ctx, t, ps.listeners)
		testutil.TryReceive(ctx, t, firstRead)
		listener(ctx, nil, pubsub.ErrDroppedMessages)
		got := testutil.TryReceive(ctx, t, done)
		require.Equal(t, 2, *calls)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		require.False(t, got.WaitTimedOut)
	})

	t.Run("SubscribeErrorFailsOpen", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		// No database expectations: the wait ends before any read.
		db := dbmock.NewMockStore(gomock.NewController(t))
		ps := &listenerPubsub{subscribeErr: xerrors.New("pubsub closed")}

		got := WaitForMCPDiscovery(ctx, db, ps, quartz.NewMock(t), uuid.New())
		require.NoError(t, ctx.Err())
		require.False(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
	})

	t.Run("LimitAlreadyPassedReturnsAtOnce", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhaseComplete), nil)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "run-b", MCPDiscoveryWaitTimeout+time.Second), nil)
		expectSummary(db, agentID)

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), clock, agentID)
		require.NoError(t, ctx.Err(), "a passed limit must not wait")
		require.True(t, got.WaitTimedOut)
		require.True(t, got.Stale)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseComplete, got.Phase)
		requireCounts(t, got)
	})

	t.Run("LimitReachedWhilePending", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		deadlineTimer := clock.Trap().NewTimer("mcpDiscovery", "deadline")
		defer deadlineTimer.Close()
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil).Times(2)
		// Two seconds of the run's limit remain.
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
			Return(agentRow(clock, "run-a", MCPDiscoveryWaitTimeout-2*time.Second), nil).Times(2)
		expectSummary(db, agentID)

		done := waitAsync(ctx, db, pubsub.NewInMemory(), clock, agentID)
		call := deadlineTimer.MustWait(ctx)
		require.Equal(t, 2*time.Second, call.Duration, "the wait lasts until the run's limit")
		call.MustRelease(ctx)
		clock.Advance(2 * time.Second).MustWait(ctx)
		got := testutil.TryReceive(ctx, t, done)
		require.True(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
	})

	t.Run("DBErrorFailsOpen", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, xerrors.New("connection refused"))

		got := WaitForMCPDiscovery(ctx, db, pubsub.NewInMemory(), quartz.NewMock(t), agentID)
		require.NoError(t, ctx.Err())
		require.False(t, got.WaitTimedOut)
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhaseUnknown, got.Phase)
	})

	t.Run("CanceledWhilePending", func(t *testing.T) {
		t.Parallel()
		testCtx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		db := dbmock.NewMockStore(gomock.NewController(t))
		agentID := uuid.New()
		read, _, firstRead := sequence(
			snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending),
			snapshot("run-a", database.WorkspaceAgentMcpDiscoveryPhasePending), nil)
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).DoAndReturn(read).Times(1)
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(agentRow(clock, "run-a", 0), nil).Times(1)

		ctx, cancel := context.WithCancel(testCtx)
		defer cancel()
		done := waitAsync(ctx, db, pubsub.NewInMemory(), clock, agentID)
		testutil.TryReceive(testCtx, t, firstRead)
		cancel()
		got := testutil.TryReceive(testCtx, t, done)
		require.False(t, got.WaitTimedOut, "cancellation is not the run's limit")
		require.Equal(t, codersdk.ChatContextMCPDiscoveryPhasePending, got.Phase)
	})
}
