package agentapi_test

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestUpdateLifecycle(t *testing.T) {
	t.Parallel()

	someTime, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	require.NoError(t, err)
	someTime = dbtime.Time(someTime)
	now := dbtime.Now()

	var (
		workspaceID  = uuid.New()
		agentCreated = database.WorkspaceAgent{
			ID:             uuid.New(),
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
			StartedAt:      sql.NullTime{Valid: false},
			ReadyAt:        sql.NullTime{Valid: false},
		}
		agentStarting = database.WorkspaceAgent{
			ID:             uuid.New(),
			LifecycleState: database.WorkspaceAgentLifecycleStateStarting,
			StartedAt:      sql.NullTime{Valid: true, Time: someTime},
			ReadyAt:        sql.NullTime{Valid: false},
		}
	)

	t.Run("OKStarting", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_STARTING,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agentCreated.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateStarting,
			StartedAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			ReadyAt: sql.NullTime{Valid: false},
		}).Return(nil)

		publishCalled := false
		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent) error {
				publishCalled = true
				return nil
			},
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)
		require.True(t, publishCalled)
	})

	t.Run("OKReadying", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_READY,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agentStarting.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			StartedAt:      agentStarting.StartedAt,
			ReadyAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		}).Return(nil)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentStarting, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			// Test that nil publish fn works.
			PublishWorkspaceUpdateFn: nil,
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)
	})

	// This test jumps from CREATING to READY, skipping STARTED. Both the
	// StartedAt and ReadyAt fields should be set.
	t.Run("OKStraightToReady", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_READY,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agentCreated.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			StartedAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			ReadyAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		}).Return(nil)

		publishCalled := false
		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent) error {
				publishCalled = true
				return nil
			},
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)
		require.True(t, publishCalled)
	})

	t.Run("NoTimeSpecified", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State: agentproto.Lifecycle_READY,
			// Zero time
			ChangedAt: timestamppb.New(time.Time{}),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		now := dbtime.Now()
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agentCreated.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			StartedAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			ReadyAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		})

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database:                 dbM,
			Log:                      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: nil,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)
	})

	t.Run("AllStates", func(t *testing.T) {
		t.Parallel()

		agent := database.WorkspaceAgent{
			ID:             uuid.New(),
			LifecycleState: database.WorkspaceAgentLifecycleState(""),
			StartedAt:      sql.NullTime{Valid: false},
			ReadyAt:        sql.NullTime{Valid: false},
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		var publishCalled int64
		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent) error {
				atomic.AddInt64(&publishCalled, 1)
				return nil
			},
		}

		states := []agentproto.Lifecycle_State{
			agentproto.Lifecycle_CREATED,
			agentproto.Lifecycle_STARTING,
			agentproto.Lifecycle_START_TIMEOUT,
			agentproto.Lifecycle_START_ERROR,
			agentproto.Lifecycle_READY,
			agentproto.Lifecycle_SHUTTING_DOWN,
			agentproto.Lifecycle_SHUTDOWN_TIMEOUT,
			agentproto.Lifecycle_SHUTDOWN_ERROR,
			agentproto.Lifecycle_OFF,
		}
		for i, state := range states {
			t.Log("state", state)
			// Use a time after the last state change to ensure ordering.
			stateNow := now.Add(time.Hour * time.Duration(i))
			lifecycle := &agentproto.Lifecycle{
				State:     state,
				ChangedAt: timestamppb.New(stateNow),
			}

			expectedStartedAt := agent.StartedAt
			expectedReadyAt := agent.ReadyAt
			if state == agentproto.Lifecycle_STARTING {
				expectedStartedAt = sql.NullTime{Valid: true, Time: stateNow}
			}
			if state == agentproto.Lifecycle_READY || state == agentproto.Lifecycle_START_ERROR {
				expectedReadyAt = sql.NullTime{Valid: true, Time: stateNow}
			}

			dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.ID,
				LifecycleState: database.WorkspaceAgentLifecycleState(strings.ToLower(state.String())),
				StartedAt:      expectedStartedAt,
				ReadyAt:        expectedReadyAt,
			}).Times(1).Return(nil)

			resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
				Lifecycle: lifecycle,
			})
			require.NoError(t, err)
			require.Equal(t, lifecycle, resp)
			require.Equal(t, int64(i+1), atomic.LoadInt64(&publishCalled))

			// For future iterations:
			agent.StartedAt = expectedStartedAt
			agent.ReadyAt = expectedReadyAt
		}
	})

	t.Run("UnknownLifecycleState", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     -999,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		publishCalled := false
		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent) error {
				publishCalled = true
				return nil
			},
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "unknown lifecycle state")
		require.Nil(t, resp)
		require.False(t, publishCalled)
	})
}

func TestUpdateStartup(t *testing.T) {
	t.Parallel()

	var (
		workspaceID = uuid.New()
		agent       = database.WorkspaceAgent{
			ID: uuid.New(),
		}
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			// Not used by UpdateStartup.
			PublishWorkspaceUpdateFn: nil,
		}

		startup := &agentproto.Startup{
			Version:           "v1.2.3",
			ExpandedDirectory: "/path/to/expanded/dir",
			Subsystems: []agentproto.Startup_Subsystem{
				agentproto.Startup_ENVBOX,
				agentproto.Startup_ENVBUILDER,
				agentproto.Startup_EXECTRACE,
			},
		}

		dbM.EXPECT().UpdateWorkspaceAgentStartupByID(gomock.Any(), database.UpdateWorkspaceAgentStartupByIDParams{
			ID:                agent.ID,
			Version:           startup.Version,
			ExpandedDirectory: startup.ExpandedDirectory,
			Subsystems: []database.WorkspaceAgentSubsystem{
				database.WorkspaceAgentSubsystemEnvbox,
				database.WorkspaceAgentSubsystemEnvbuilder,
				database.WorkspaceAgentSubsystemExectrace,
			},
			APIVersion: agentapi.AgentAPIVersionDRPC,
		}).Return(nil)

		resp, err := api.UpdateStartup(context.Background(), &agentproto.UpdateStartupRequest{
			Startup: startup,
		})
		require.NoError(t, err)
		require.Equal(t, startup, resp)
	})

	t.Run("BadVersion", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			// Not used by UpdateStartup.
			PublishWorkspaceUpdateFn: nil,
		}

		startup := &agentproto.Startup{
			Version:           "asdf",
			ExpandedDirectory: "/path/to/expanded/dir",
			Subsystems:        []agentproto.Startup_Subsystem{},
		}

		resp, err := api.UpdateStartup(context.Background(), &agentproto.UpdateStartupRequest{
			Startup: startup,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid agent semver version")
		require.Nil(t, resp)
	})

	t.Run("BadSubsystem", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceIDFn: func(ctx context.Context, agent *database.WorkspaceAgent) (uuid.UUID, error) {
				return workspaceID, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			// Not used by UpdateStartup.
			PublishWorkspaceUpdateFn: nil,
		}

		startup := &agentproto.Startup{
			Version:           "v1.2.3",
			ExpandedDirectory: "/path/to/expanded/dir",
			Subsystems: []agentproto.Startup_Subsystem{
				agentproto.Startup_ENVBOX,
				-999,
			},
		}

		resp, err := api.UpdateStartup(context.Background(), &agentproto.UpdateStartupRequest{
			Startup: startup,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid agent subsystem")
		require.Nil(t, resp)
	})
}
