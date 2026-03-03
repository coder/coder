package chattool //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestWaitForAgentReady(t *testing.T) {
	t.Parallel()

	t.Run("AgentConnectsAndLifecycleReady", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		// Mock returns Ready lifecycle state.
		db.EXPECT().
			GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agentID).
			Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			}, nil)

		// AgentConnFn succeeds immediately.
		connFn := func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		result := waitForAgentReady(context.Background(), db, agentID, connFn)
		require.Empty(t, result)
	})

	t.Run("AgentConnectTimeout", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		// AgentConnFn always fails - context will timeout.
		connFn := func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, nil, context.DeadlineExceeded
		}

		// Use a context that's already canceled to avoid waiting.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := waitForAgentReady(ctx, db, agentID, connFn)
		require.Equal(t, "not_ready", result["agent_status"])
		require.NotEmpty(t, result["agent_error"])
	})

	t.Run("AgentConnectsButStartupFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		// Mock returns StartError lifecycle state.
		db.EXPECT().
			GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agentID).
			Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
				LifecycleState: database.WorkspaceAgentLifecycleStateStartError,
			}, nil)

		connFn := func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		result := waitForAgentReady(context.Background(), db, agentID, connFn)
		require.Equal(t, "startup_scripts_failed", result["startup_scripts"])
		require.Equal(t, "start_error", result["lifecycle_state"])
	})

	t.Run("NilAgentConnFn", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		agentID := uuid.New()

		// Mock returns Ready lifecycle state.
		db.EXPECT().
			GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agentID).
			Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			}, nil)

		result := waitForAgentReady(context.Background(), db, agentID, nil)
		require.Empty(t, result)
	})

	t.Run("NilDB", func(t *testing.T) {
		t.Parallel()

		connFn := func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		result := waitForAgentReady(context.Background(), nil, uuid.New(), connFn)
		require.Empty(t, result)
	})
}
