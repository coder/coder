package chattool

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestResolveChatAgent(t *testing.T) {
	t.Parallel()

	type result struct {
		resolved ResolvedChatAgent
		err      error
	}

	newParent := func(state database.WorkspaceAgentLifecycleState) database.WorkspaceAgent {
		return database.WorkspaceAgent{ID: uuid.New(), Name: "dev", LifecycleState: state}
	}
	withState := func(agent database.WorkspaceAgent, state database.WorkspaceAgentLifecycleState) database.WorkspaceAgent {
		agent.LifecycleState = state
		return agent
	}
	newChild := func(parent database.WorkspaceAgent) database.WorkspaceAgent {
		return database.WorkspaceAgent{
			ID:       uuid.New(),
			Name:     "devcontainer",
			ParentID: uuid.NullUUID{UUID: parent.ID, Valid: true},
		}
	}
	declared := []database.WorkspaceAgentDevcontainer{{ID: uuid.New()}}

	// resolveAsync runs ResolveChatAgent in a goroutine and waits for it to
	// create its poll ticker, so callers can drive the mock clock.
	resolveAsync := func(ctx context.Context, t *testing.T, db database.Store, clock *quartz.Mock, workspaceID uuid.UUID) <-chan result {
		t.Helper()
		trap := clock.Trap().NewTicker("chattool", "devcontainer_agent_poll")
		defer trap.Close()

		done := make(chan result, 1)
		go func() {
			resolved, err := ResolveChatAgent(ctx, db, clock, workspaceID)
			done <- result{resolved: resolved, err: err}
		}()
		trap.MustWait(ctx).MustRelease(ctx)
		return done
	}

	t.Run("NoDevcontainers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent}, nil)
		db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(nil, nil)

		resolved, err := ResolveChatAgent(ctx, db, quartz.NewMock(t), workspaceID)
		require.NoError(t, err)
		require.Equal(t, ResolvedChatAgent{Agent: parent}, resolved)
	})

	t.Run("DevcontainerAgentAlreadyRegistered", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)
		child := newChild(parent)

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent, child}, nil)

		resolved, err := ResolveChatAgent(ctx, db, quartz.NewMock(t), workspaceID)
		require.NoError(t, err)
		require.Equal(t, ResolvedChatAgent{Agent: child}, resolved)
	})

	t.Run("WaitsForDevcontainerAgent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)
		child := newChild(parent)

		gomock.InOrder(
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{parent}, nil),
			db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil),
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{parent, child}, nil),
		)

		done := resolveAsync(ctx, t, db, clock, workspaceID)
		clock.Advance(startupScriptPollInterval).MustWait(ctx)

		got := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, got.err)
		require.Equal(t, ResolvedChatAgent{Agent: child}, got.resolved)
	})

	t.Run("ParentReadyWithoutDevcontainerAgent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateReady)

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent}, nil)
		db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil)

		resolved, err := ResolveChatAgent(ctx, db, quartz.NewMock(t), workspaceID)
		require.NoError(t, err)
		require.Equal(t, ResolvedChatAgent{Agent: parent, DevcontainerUnavailable: true}, resolved)
	})

	t.Run("ParentNeverConnected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateCreated)
		parent.CreatedAt = clock.Now().Add(-time.Hour)
		parent.ConnectionTimeoutSeconds = 120

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent}, nil)
		db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil)

		resolved, err := ResolveChatAgent(ctx, db, clock, workspaceID)
		require.NoError(t, err)
		require.Equal(t, ResolvedChatAgent{Agent: parent, DevcontainerUnavailable: true}, resolved)
	})

	t.Run("ParentStartErrorWhileWaiting", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)
		failed := withState(parent, database.WorkspaceAgentLifecycleStateStartError)

		gomock.InOrder(
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{parent}, nil),
			db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil),
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{failed}, nil),
		)

		done := resolveAsync(ctx, t, db, clock, workspaceID)
		clock.Advance(startupScriptPollInterval).MustWait(ctx)

		got := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, got.err)
		require.Equal(t, ResolvedChatAgent{Agent: failed, DevcontainerUnavailable: true}, got.resolved)
	})

	t.Run("ReplacedParentWithoutDevcontainers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)
		replacement := newParent(database.WorkspaceAgentLifecycleStateStarting)

		gomock.InOrder(
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{parent}, nil),
			db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil),
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{replacement}, nil),
			db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), replacement.ID).Return(nil, nil),
		)

		done := resolveAsync(ctx, t, db, clock, workspaceID)
		clock.Advance(startupScriptPollInterval).MustWait(ctx)

		got := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, got.err)
		require.Equal(t, ResolvedChatAgent{Agent: replacement}, got.resolved)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent}, nil).AnyTimes()
		db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil)

		done := resolveAsync(ctx, t, db, clock, workspaceID)
		for range startupScriptTimeout / startupScriptPollInterval {
			_, waiter := clock.AdvanceNext()
			waiter.MustWait(ctx)
		}

		got := testutil.RequireReceive(ctx, t, done)
		require.NoError(t, got.err)
		require.Equal(t, ResolvedChatAgent{Agent: parent, DevcontainerUnavailable: true}, got.resolved)
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		clock := quartz.NewMock(t)
		workspaceID := uuid.New()
		parent := newParent(database.WorkspaceAgentLifecycleStateStarting)

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{parent}, nil)
		db.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), parent.ID).Return(declared, nil)

		resolveCtx, cancel := context.WithCancel(ctx)
		done := resolveAsync(resolveCtx, t, db, clock, workspaceID)
		cancel()

		got := testutil.RequireReceive(ctx, t, done)
		require.ErrorIs(t, got.err, context.Canceled)
	})

	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		workspaceID := uuid.New()

		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return(nil, nil)

		_, err := ResolveChatAgent(ctx, db, quartz.NewMock(t), workspaceID)
		require.ErrorIs(t, err, ErrNoWorkspaceAgent)
	})
}
