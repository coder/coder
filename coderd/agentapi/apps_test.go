package agentapi_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/testutil"
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
