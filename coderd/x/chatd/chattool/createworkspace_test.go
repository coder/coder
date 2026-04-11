package chattool //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"database/sql"
	"encoding/json"
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

func TestCreateWorkspace_PrefersChatSuffixAgent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	ownerID := uuid.New()
	templateID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()
	fallbackAgentID := uuid.New()
	chatAgentID := uuid.New()

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
		Return("0s", nil)

	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
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
		Return([]database.WorkspaceAgent{
			{ID: fallbackAgentID, Name: "dev", DisplayOrder: 0},
			{ID: chatAgentID, Name: "dev-coderd-chat", DisplayOrder: 1},
		}, nil)
	db.EXPECT().
		GetWorkspaceAgentLifecycleStateByID(gomock.Any(), chatAgentID).
		Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}, nil)

	var connectedAgentID uuid.UUID
	createFn := func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
		return codersdk.Workspace{
			ID:        workspaceID,
			Name:      req.Name,
			OwnerName: "testuser",
			LatestBuild: codersdk.WorkspaceBuild{
				ID: buildID,
			},
		}, nil
	}
	agentConnFn := func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		connectedAgentID = agentID
		return nil, func() {}, nil
	}

	tool := CreateWorkspace(CreateWorkspaceOptions{
		DB:          db,
		OwnerID:     ownerID,
		CreateFn:    createFn,
		AgentConnFn: agentConnFn,
		WorkspaceMu: &sync.Mutex{},
		Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	input := fmt.Sprintf(`{"template_id":%q,"name":"test-chat-agent"}`, templateID.String())
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "create_workspace",
		Input: input,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)
	require.Equal(t, chatAgentID, connectedAgentID)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, buildID.String(), result["build_id"])
}

func TestCreateWorkspace_ReturnsSelectionErrorImmediately(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	ownerID := uuid.New()
	chatID := uuid.New()
	templateID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()

	db.EXPECT().
		GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{ID: chatID}, nil)
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
		Return("0s", nil)
	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
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
		UpdateChatWorkspaceBinding(gomock.Any(), database.UpdateChatWorkspaceBindingParams{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
			BuildID:     uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID:     uuid.NullUUID{},
		}).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)
	db.EXPECT().
		GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{
			{ID: uuid.New(), Name: "alpha-coderd-chat", DisplayOrder: 0},
			{ID: uuid.New(), Name: "beta-coderd-chat", DisplayOrder: 1},
		}, nil)

	tool := CreateWorkspace(CreateWorkspaceOptions{
		DB:      db,
		OwnerID: ownerID,
		ChatID:  chatID,
		CreateFn: func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
			return codersdk.Workspace{
				ID:        workspaceID,
				Name:      req.Name,
				OwnerName: "testuser",
				LatestBuild: codersdk.WorkspaceBuild{
					ID: buildID,
				},
			}, nil
		},
		AgentConnFn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			t.Fatal("AgentConnFn should not be called when agent selection fails")
			return nil, nil, xerrors.New("unexpected agent dial")
		},
		WorkspaceMu: &sync.Mutex{},
		Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	input := fmt.Sprintf(`{"template_id":%q,"name":"test-selection-error"}`, templateID.String())
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "create_workspace",
		Input: input,
	})
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, true, result["created"])
	require.Equal(t, "testuser/test-selection-error", result["workspace_name"])
	require.Equal(t, "selection_error", result["agent_status"])
	require.Contains(t, result["agent_error"], "multiple agents match the chat suffix")
	require.Equal(t, buildID.String(), result["build_id"])
}

func TestCreateWorkspace_PostCreationBuildFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	ownerID := uuid.New()
	templateID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()

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
		Return("0s", nil)

	// waitForBuild fetches the build by ID.
	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
		}, nil)

	// waitForBuild polls the provisioner job. Return Failed.
	db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusFailed,
			Error:     sql.NullString{String: "terraform apply failed", Valid: true},
		}, nil)

	createFn := func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
		return codersdk.Workspace{
			ID:        workspaceID,
			Name:      req.Name,
			OwnerName: "testuser",
			LatestBuild: codersdk.WorkspaceBuild{
				ID: buildID,
			},
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

	input := fmt.Sprintf(`{"template_id":%q,"name":"test-build-fail"}`, templateID.String())
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "create_workspace",
		Input: input,
	})
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Contains(t, result["error"], "workspace build failed")
	require.Equal(t, buildID.String(), result["build_id"])
	require.False(t, resp.IsError,
		"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
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
			buildID := uuid.New()

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
				GetWorkspaceBuildByID(gomock.Any(), buildID).
				Return(database.WorkspaceBuild{
					ID:          buildID,
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
					LatestBuild: codersdk.WorkspaceBuild{
						ID: buildID,
					},
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

			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			require.Equal(t, buildID.String(), result["build_id"])

			if tc.wantTTLMs != nil {
				require.NotNil(t, capturedReq.TTLMillis)
				require.Equal(t, *tc.wantTTLMs, *capturedReq.TTLMillis)
			} else {
				require.Nil(t, capturedReq.TTLMillis)
			}
		})
	}
}

func TestCheckExistingWorkspace_ConnectedAgent(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	agentID := uuid.New()
	now := time.Now().UTC()

	expectExistingWorkspaceLookup(
		db,
		chatID,
		workspaceID,
		jobID,
		"existing-workspace",
		database.ProvisionerJobStatusSucceeded,
		database.WorkspaceTransitionStart,
	)
	db.EXPECT().
		GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{{
			ID:               agentID,
			Name:             "dev",
			CreatedAt:        now.Add(-time.Minute),
			FirstConnectedAt: validNullTime(now.Add(-45 * time.Second)),
			LastConnectedAt:  validNullTime(now.Add(-5 * time.Second)),
		}}, nil)
	db.EXPECT().
		GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agentID).
		Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}, nil)

	connFn := func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		t.Fatalf("unexpected agent dial for connected workspace")
		return nil, nil, xerrors.New("unexpected agent dial")
	}

	options := testCheckExistingWorkspaceOptions(db, chatID, connFn)
	check := options.checkExistingWorkspace(context.Background())
	require.NoError(t, check.Err)
	require.True(t, check.Done)
	require.Equal(t, "already_exists", check.Result["status"])
	require.Equal(t, "existing-workspace", check.Result["workspace_name"])
	require.Equal(t, "workspace is already running and recently connected", check.Result["message"])
}

func TestCheckExistingWorkspace_InProgressBuildReturnsBuildID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()

	// GetChatByID returns a chat linked to a workspace.
	db.EXPECT().
		GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

	// GetWorkspaceByID returns a non-deleted workspace.
	db.EXPECT().
		GetWorkspaceByID(gomock.Any(), workspaceID).
		Return(database.Workspace{
			ID:   workspaceID,
			Name: "building-workspace",
		}, nil)

	// GetLatestWorkspaceBuildByWorkspaceID is called once in
	// checkExistingWorkspace. waitForBuild now uses
	// GetWorkspaceBuildByID to track the specific build.
	db.EXPECT().
		GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
			Transition:  database.WorkspaceTransitionStart,
		}, nil)
	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
			Transition:  database.WorkspaceTransitionStart,
		}, nil)

	// First GetProvisionerJobByID (in checkExistingWorkspace) returns
	// Running, triggering waitForBuild. The second call (waitForBuild's
	// first poll) returns Succeeded so the loop exits immediately.
	firstJob := db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusRunning,
		}, nil)
	db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusSucceeded,
		}, nil).
		After(firstJob)

	// The in-progress path now publishes the build ID before
	// waitForBuild.
	db.EXPECT().
		UpdateChatWorkspaceBinding(gomock.Any(), database.UpdateChatWorkspaceBindingParams{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
			BuildID:     uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID:     uuid.NullUUID{},
		}).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

	// After waitForBuild completes, checkExistingWorkspace fetches
	// agents. Return empty to keep the test focused on build_id.
	db.EXPECT().
		GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{}, nil)

	options := testCheckExistingWorkspaceOptions(db, chatID, nil)
	check := options.checkExistingWorkspace(context.Background())
	require.NoError(t, check.Err)
	require.True(t, check.Done)
	require.Equal(t, false, check.Result["created"])
	require.Equal(t, "already_exists", check.Result["status"])
	require.Equal(t, buildID.String(), check.Result["build_id"])
	require.Equal(t, "building-workspace", check.Result["workspace_name"])
	require.Equal(t, "workspace build completed", check.Result["message"])
}

func TestCheckExistingWorkspace_InProgressBuildFailureReturnsBuildID(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()

	db.EXPECT().
		GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

	db.EXPECT().
		GetWorkspaceByID(gomock.Any(), workspaceID).
		Return(database.Workspace{
			ID:   workspaceID,
			Name: "failing-workspace",
		}, nil)

	db.EXPECT().
		GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
			Transition:  database.WorkspaceTransitionStart,
		}, nil)
	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
			Transition:  database.WorkspaceTransitionStart,
		}, nil)

	// First call returns Running (triggers waitForBuild), second
	// returns Failed so waitForBuild returns an error.
	firstJob := db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusRunning,
		}, nil)
	db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusFailed,
		}, nil).
		After(firstJob)

	// The in-progress path publishes the build ID before
	// waitForBuild.
	db.EXPECT().
		UpdateChatWorkspaceBinding(gomock.Any(), database.UpdateChatWorkspaceBindingParams{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
			BuildID:     uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID:     uuid.NullUUID{},
		}).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

	options := testCheckExistingWorkspaceOptions(db, chatID, nil)
	check := options.checkExistingWorkspace(context.Background())
	require.Error(t, check.Err)
	require.Contains(t, check.Err.Error(), "existing workspace build failed")
	require.Equal(t, buildID, check.FailedBuildID)
}

func TestCheckExistingWorkspace_ConnectingAgentWaits(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	agentID := uuid.New()
	now := time.Now().UTC()
	connectCalls := 0

	expectExistingWorkspaceLookup(
		db,
		chatID,
		workspaceID,
		jobID,
		"existing-workspace",
		database.ProvisionerJobStatusSucceeded,
		database.WorkspaceTransitionStart,
	)
	db.EXPECT().
		GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{{
			ID:                       agentID,
			Name:                     "dev",
			CreatedAt:                now,
			ConnectionTimeoutSeconds: 60,
		}}, nil)
	db.EXPECT().
		GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agentID).
		Return(database.GetWorkspaceAgentLifecycleStateByIDRow{
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}, nil)

	connFn := func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		connectCalls++
		return nil, func() {}, nil
	}

	options := testCheckExistingWorkspaceOptions(db, chatID, connFn)
	check := options.checkExistingWorkspace(context.Background())
	require.NoError(t, check.Err)
	require.True(t, check.Done)
	require.Equal(t, 1, connectCalls)
	require.Equal(t, "already_exists", check.Result["status"])
	require.Equal(t, "workspace exists and the agent is still connecting", check.Result["message"])
}

func TestCheckExistingWorkspace_DeadAgentAllowsCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		agent database.WorkspaceAgent
	}{
		{
			name: "Disconnected",
			agent: database.WorkspaceAgent{
				ID:               uuid.New(),
				Name:             "disconnected",
				CreatedAt:        time.Now().UTC().Add(-2 * time.Minute),
				FirstConnectedAt: validNullTime(time.Now().UTC().Add(-2 * time.Minute)),
				LastConnectedAt:  validNullTime(time.Now().UTC().Add(-time.Minute)),
			},
		},
		{
			name: "TimedOut",
			agent: database.WorkspaceAgent{
				ID:                       uuid.New(),
				Name:                     "timed-out",
				CreatedAt:                time.Now().UTC().Add(-2 * time.Second),
				ConnectionTimeoutSeconds: 1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)

			chatID := uuid.New()
			workspaceID := uuid.New()
			jobID := uuid.New()

			expectExistingWorkspaceLookup(
				db,
				chatID,
				workspaceID,
				jobID,
				"existing-workspace",
				database.ProvisionerJobStatusSucceeded,
				database.WorkspaceTransitionStart,
			)
			db.EXPECT().
				GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{tc.agent}, nil)

			options := testCheckExistingWorkspaceOptions(db, chatID, nil)
			check := options.checkExistingWorkspace(context.Background())
			require.NoError(t, check.Err)
			require.False(t, check.Done)
			require.Nil(t, check.Result)
		})
	}
}

func TestWaitForBuild_CanceledJob(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	ownerID := uuid.New()
	templateID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()
	buildID := uuid.New()

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
		Return("0s", nil)

	// waitForBuild fetches the build by ID.
	db.EXPECT().
		GetWorkspaceBuildByID(gomock.Any(), buildID).
		Return(database.WorkspaceBuild{
			ID:          buildID,
			WorkspaceID: workspaceID,
			JobID:       jobID,
		}, nil)

	// waitForBuild polls the provisioner job. Return Canceled.
	db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: database.ProvisionerJobStatusCanceled,
		}, nil)

	createFn := func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
		return codersdk.Workspace{
			ID:        workspaceID,
			Name:      req.Name,
			OwnerName: "testuser",
			LatestBuild: codersdk.WorkspaceBuild{
				ID: buildID,
			},
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

	input := fmt.Sprintf(`{"template_id":%q,"name":"test-build-cancel"}`, templateID.String())
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "create_workspace",
		Input: input,
	})
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Contains(t, result["error"], "build was canceled")
	require.Equal(t, buildID.String(), result["build_id"])
	require.False(t, resp.IsError,
		"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
}

func TestCheckExistingWorkspace_StoppedWorkspace(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	workspaceID := uuid.New()
	jobID := uuid.New()

	expectExistingWorkspaceLookup(
		db,
		chatID,
		workspaceID,
		jobID,
		"stopped-workspace",
		database.ProvisionerJobStatusSucceeded,
		database.WorkspaceTransitionStop,
	)

	options := testCheckExistingWorkspaceOptions(db, chatID, nil)
	check := options.checkExistingWorkspace(context.Background())
	require.True(t, check.Done)
	require.NoError(t, check.Err)
	require.Equal(t, "stopped", check.Result["status"])
	require.Contains(t, check.Result["message"], "start_workspace")
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

	options := testCheckExistingWorkspaceOptions(db, chatID, nil)
	check := options.checkExistingWorkspace(context.Background())
	require.NoError(t, check.Err)
	require.False(t, check.Done, "should allow creation for deleted workspace")
	require.Nil(t, check.Result)
}

func testCheckExistingWorkspaceOptions(
	db *dbmock.MockStore,
	chatID uuid.UUID,
	agentConnFn AgentConnFunc,
) CreateWorkspaceOptions {
	return CreateWorkspaceOptions{
		DB:                             db,
		ChatID:                         chatID,
		AgentConnFn:                    agentConnFn,
		AgentInactiveDisconnectTimeout: 30 * time.Second,
	}
}

func expectExistingWorkspaceLookup(
	db *dbmock.MockStore,
	chatID uuid.UUID,
	workspaceID uuid.UUID,
	jobID uuid.UUID,
	workspaceName string,
	jobStatus database.ProvisionerJobStatus,
	transition database.WorkspaceTransition,
) {
	db.EXPECT().
		GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{
			ID:          chatID,
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)
	db.EXPECT().
		GetWorkspaceByID(gomock.Any(), workspaceID).
		Return(database.Workspace{
			ID:   workspaceID,
			Name: workspaceName,
		}, nil)
	db.EXPECT().
		GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return(database.WorkspaceBuild{
			WorkspaceID: workspaceID,
			JobID:       jobID,
			Transition:  transition,
		}, nil)
	db.EXPECT().
		GetProvisionerJobByID(gomock.Any(), jobID).
		Return(database.ProvisionerJob{
			ID:        jobID,
			JobStatus: jobStatus,
		}, nil)
}

func validNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}
