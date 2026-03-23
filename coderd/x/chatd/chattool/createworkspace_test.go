package chattool //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
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

func TestCreateWorkspace_GlobalTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ttlReturn string
		ttlErr    error
		wantTTLMs *int64
	}{
		{
			name:      "PositiveTTL",
			ttlReturn: "2h",
			wantTTLMs: ptr.Ref(int64(2 * time.Hour / time.Millisecond)),
		},
		{
			name:      "ZeroTTLUsesTemplateDefault",
			ttlReturn: "0s",
			wantTTLMs: nil,
		},
		{
			name:      "DBError_FallsBackToNil",
			ttlReturn: "",
			ttlErr:    xerrors.New("db error"),
			wantTTLMs: nil,
		},
		{
			name:      "InvalidStoredValue_FallsBackToNil",
			ttlReturn: "not-a-duration",
			wantTTLMs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)

			ownerID := uuid.New()
			templateID := uuid.New()
			workspaceID := uuid.New()
			jobID := uuid.New()

			db.EXPECT().
				GetAuthorizationUserRoles(gomock.Any(), ownerID).
				Return(database.GetAuthorizationUserRolesRow{
					ID:     ownerID,
					Roles:  []string{},
					Groups: []string{},
					Status: database.UserStatusActive,
				}, nil)

			db.EXPECT().
				GetChatWorkspaceTTL(gomock.Any()).
				Return(tc.ttlReturn, tc.ttlErr)

			db.EXPECT().
				GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return(database.WorkspaceBuild{
					WorkspaceID: workspaceID,
					JobID:       jobID,
				}, nil)
			db.EXPECT().
				GetProvisionerJobByID(gomock.Any(), jobID).
				Return(database.ProvisionerJob{
					ID:        jobID,
					JobStatus: database.ProvisionerJobStatusSucceeded,
				}, nil)

			db.EXPECT().
				GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{}, nil)

			var capturedReq codersdk.CreateWorkspaceRequest
			createFn := func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
				capturedReq = req
				return codersdk.Workspace{
					ID:        workspaceID,
					Name:      req.Name,
					OwnerName: "testuser",
				}, nil
			}

			tool := CreateWorkspace(CreateWorkspaceOptions{
				DB:          db,
				OwnerID:     ownerID,
				ChatID:      uuid.Nil,
				CreateFn:    createFn,
				WorkspaceMu: &sync.Mutex{},
				Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			})

			input := fmt.Sprintf(`{"template_id":%q,"name":"test-ws-%s"}`, templateID.String(), tc.name)
			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-1",
				Name:  "create_workspace",
				Input: input,
			})
			require.NoError(t, err)
			require.NotEmpty(t, resp.Content)

			if tc.wantTTLMs != nil {
				require.NotNil(t, capturedReq.TTLMillis)
				require.Equal(t, *tc.wantTTLMs, *capturedReq.TTLMillis)
			} else {
				require.Nil(t, capturedReq.TTLMillis)
			}
		})
	}
}

func TestCheckExistingWorkspace_DeletedWorkspace(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()

	// Mock GetChatByID returns a chat linked to a workspace.
	db.EXPECT().
		GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

	// Mock GetWorkspaceByID returns a soft-deleted workspace.
	db.EXPECT().
		GetWorkspaceByID(gomock.Any(), workspaceID).
		Return(database.Workspace{
			ID:      workspaceID,
			Deleted: true,
		}, nil)

	result, done, err := checkExistingWorkspace(
		context.Background(), db, chatID, nil,
	)
	require.NoError(t, err)
	require.False(t, done, "should allow creation for deleted workspace")
	require.Nil(t, result)
}
