package agentapi_test

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestBatchUpdateAppHealths(t *testing.T) {
	t.Parallel()

	var (
		agent = database.WorkspaceAgent{
			ID: uuid.New(),
		}
		app1 = database.WorkspaceApp{
			ID:             uuid.New(),
			AgentID:        agent.ID,
			Slug:           "code-server-1",
			DisplayName:    "code-server 1",
			HealthcheckUrl: "http://localhost:3000",
			Health:         database.WorkspaceAppHealthInitializing,
			OpenIn:         database.WorkspaceAppOpenInSlimWindow,
		}
		app2 = database.WorkspaceApp{
			ID:             uuid.New(),
			AgentID:        agent.ID,
			Slug:           "code-server-2",
			DisplayName:    "code-server 2",
			HealthcheckUrl: "http://localhost:3001",
			Health:         database.WorkspaceAppHealthHealthy,
			OpenIn:         database.WorkspaceAppOpenInSlimWindow,
		}
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return([]database.WorkspaceApp{app1, app2}, nil)
		dbM.EXPECT().UpdateWorkspaceAppHealthByID(gomock.Any(), database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app1.ID,
			Health: database.WorkspaceAppHealthHealthy,
		}).Return(nil)
		dbM.EXPECT().UpdateWorkspaceAppHealthByID(gomock.Any(), database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app2.ID,
			Health: database.WorkspaceAppHealthUnhealthy,
		}).Return(nil)

		publishCalled := false
		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
				publishCalled = true
				return nil
			},
		}

		// Set one to healthy, set another to unhealthy.
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{
				{
					Id:     app1.ID[:],
					Health: agentproto.AppHealth_HEALTHY,
				},
				{
					Id:     app2.ID[:],
					Health: agentproto.AppHealth_UNHEALTHY,
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateAppHealthResponse{}, resp)

		require.True(t, publishCalled)
	})

	t.Run("Unchanged", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return([]database.WorkspaceApp{app1, app2}, nil)

		publishCalled := false
		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
				publishCalled = true
				return nil
			},
		}

		// Set both to their current status, neither should be updated in the
		// DB.
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{
				{
					Id:     app1.ID[:],
					Health: agentproto.AppHealth_INITIALIZING,
				},
				{
					Id:     app2.ID[:],
					Health: agentproto.AppHealth_HEALTHY,
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateAppHealthResponse{}, resp)

		require.False(t, publishCalled)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		// No DB queries are made if there are no updates to process.
		dbM := dbmock.NewMockStore(gomock.NewController(t))

		publishCalled := false
		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
				publishCalled = true
				return nil
			},
		}

		// Do nothing.
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{},
		})
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateAppHealthResponse{}, resp)

		require.False(t, publishCalled)
	})

	t.Run("AppNoHealthcheck", func(t *testing.T) {
		t.Parallel()

		app3 := database.WorkspaceApp{
			ID:          uuid.New(),
			AgentID:     agent.ID,
			Slug:        "code-server-3",
			DisplayName: "code-server 3",
			OpenIn:      database.WorkspaceAppOpenInSlimWindow,
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return([]database.WorkspaceApp{app3}, nil)

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                 dbM,
			Log:                      testutil.Logger(t),
			PublishWorkspaceUpdateFn: nil,
		}

		// Set app3 to healthy, should error.
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{
				{
					Id:     app3.ID[:],
					Health: agentproto.AppHealth_HEALTHY,
				},
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "does not have healthchecks enabled")
		require.Nil(t, resp)
	})

	t.Run("UnknownApp", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return([]database.WorkspaceApp{app1, app2}, nil)

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                 dbM,
			Log:                      testutil.Logger(t),
			PublishWorkspaceUpdateFn: nil,
		}

		// Set an unknown app to healthy, should error.
		id := uuid.New()
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{
				{
					Id:     id[:],
					Health: agentproto.AppHealth_HEALTHY,
				},
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "not found")
		require.Nil(t, resp)
	})

	t.Run("InvalidHealth", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return([]database.WorkspaceApp{app1, app2}, nil)

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                 dbM,
			Log:                      testutil.Logger(t),
			PublishWorkspaceUpdateFn: nil,
		}

		// Set an unknown app to healthy, should error.
		resp, err := api.BatchUpdateAppHealths(context.Background(), &agentproto.BatchUpdateAppHealthRequest{
			Updates: []*agentproto.BatchUpdateAppHealthRequest_HealthUpdate{
				{
					Id:     app1.ID[:],
					Health: -999,
				},
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "unknown health status")
		require.Nil(t, resp)
	})
}

func TestWorkspaceAgentAppStatus(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		fEnq := &notificationstest.FakeEnqueuer{}
		mClock := quartz.NewMock(t)
		agent := database.WorkspaceAgent{
			ID:             uuid.UUID{2},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}
		workspaceUpdates := make(chan wspubsub.WorkspaceEventKind, 100)

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: mDB,
			Log:      testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(_ context.Context, agnt *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
				assert.Equal(t, *agnt, agent)
				testutil.AssertSend(ctx, t, workspaceUpdates, kind)
				return nil
			},
			NotificationsEnqueuer: fEnq,
			Clock:                 mClock,
		}

		app := database.WorkspaceApp{
			ID: uuid.UUID{8},
		}
		mDB.EXPECT().GetWorkspaceAppByAgentIDAndSlug(gomock.Any(), database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: agent.ID,
			Slug:    "vscode",
		}).Times(1).Return(app, nil)
		task := database.Task{
			ID: uuid.UUID{7},
			WorkspaceAppID: uuid.NullUUID{
				Valid: true,
				UUID:  app.ID,
			},
		}
		mDB.EXPECT().GetTaskByID(gomock.Any(), task.ID).Times(1).Return(task, nil)
		workspace := database.Workspace{
			ID: uuid.UUID{9},
			TaskID: uuid.NullUUID{
				Valid: true,
				UUID:  task.ID,
			},
		}
		mDB.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Times(1).Return(workspace, nil)
		appStatus := database.WorkspaceAppStatus{
			ID: uuid.UUID{6},
		}
		mDB.EXPECT().GetLatestWorkspaceAppStatusByAppID(gomock.Any(), app.ID).Times(1).Return(appStatus, nil)
		mDB.EXPECT().InsertWorkspaceAppStatus(
			gomock.Any(),
			gomock.Cond(func(params database.InsertWorkspaceAppStatusParams) bool {
				if params.AgentID == agent.ID && params.AppID == app.ID {
					assert.Equal(t, "testing", params.Message)
					assert.Equal(t, database.WorkspaceAppStatusStateComplete, params.State)
					assert.True(t, params.Uri.Valid)
					assert.Equal(t, "https://example.com", params.Uri.String)
					return true
				}
				return false
			})).Times(1).Return(database.WorkspaceAppStatus{}, nil)

		_, err := api.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
			Slug:    "vscode",
			Message: "testing",
			Uri:     "https://example.com",
			State:   agentproto.UpdateAppStatusRequest_COMPLETE,
		})
		require.NoError(t, err)

		kind := testutil.RequireReceive(ctx, t, workspaceUpdates)
		require.Equal(t, wspubsub.WorkspaceEventKindAgentAppStatusUpdate, kind)
		sent := fEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateTaskCompleted))
		require.Len(t, sent, 1)
	})

	t.Run("FailUnknownApp", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		agent := database.WorkspaceAgent{
			ID:             uuid.UUID{2},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}

		mDB.EXPECT().GetWorkspaceAppByAgentIDAndSlug(gomock.Any(), gomock.Any()).
			Times(1).
			Return(database.WorkspaceApp{}, sql.ErrNoRows)

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: mDB,
			Log:      testutil.Logger(t),
		}
		_, err := api.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
			Slug:    "unknown",
			Message: "testing",
			Uri:     "https://example.com",
			State:   agentproto.UpdateAppStatusRequest_COMPLETE,
		})
		require.ErrorContains(t, err, "No app found with slug")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("FailUnknownState", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		agent := database.WorkspaceAgent{
			ID:             uuid.UUID{2},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: mDB,
			Log:      testutil.Logger(t),
		}

		_, err := api.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
			Slug:    "vscode",
			Message: "testing",
			Uri:     "https://example.com",
			State:   77,
		})
		require.ErrorContains(t, err, "Invalid state")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("FailTooLong", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		agent := database.WorkspaceAgent{
			ID:             uuid.UUID{2},
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		}

		api := &agentapi.AppsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: mDB,
			Log:      testutil.Logger(t),
		}

		_, err := api.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
			Slug:    "vscode",
			Message: strings.Repeat("a", 161),
			Uri:     "https://example.com",
			State:   agentproto.UpdateAppStatusRequest_COMPLETE,
		})
		require.ErrorContains(t, err, "Message is too long")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
}
