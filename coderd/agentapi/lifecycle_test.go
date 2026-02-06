package agentapi_test

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/testutil"
)

// fullMetricName is the fully-qualified Prometheus metric name
// (namespace + name) used for gathering in tests.
const fullMetricName = "coderd_" + agentapi.BuildDurationMetricName

// newBuildDurationHistogram creates a fresh histogram for testing, registered
// with the given registry so metrics can be gathered and validated.
func newBuildDurationHistogram(t *testing.T, reg *prometheus.Registry) *prometheus.HistogramVec {
	t.Helper()
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Name:      agentapi.BuildDurationMetricName,
		Help:      "Duration from workspace build creation to agent ready, by template.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"template_name", "organization_name", "transition", "status", "is_prebuild"})
	reg.MustRegister(h)
	return h
}

func TestUpdateLifecycle(t *testing.T) {
	t.Parallel()

	someTime, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	require.NoError(t, err)
	someTime = dbtime.Time(someTime)
	now := dbtime.Now()

	// Fixed times for build duration metric assertions.
	// The expected duration is exactly 90 seconds.
	buildCreatedAt := dbtime.Time(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	agentReadyAt := dbtime.Time(time.Date(2025, 1, 1, 0, 1, 30, 0, time.UTC))
	expectedDuration := agentReadyAt.Sub(buildCreatedAt).Seconds() // 90.0

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
			WorkspaceID: workspaceID,
			Database:    dbM,
			Log:         testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
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
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentStarting.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        buildCreatedAt,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       false,
			AllAgentsReady:   true,
			LastAgentReadyAt: agentReadyAt,
			WorstStatus:      "success",
		}, nil)

		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentStarting, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			// Test that nil publish fn works.
			PublishWorkspaceUpdateFn: nil,
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)

		got := promhelp.HistogramValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "success",
			"is_prebuild":       "false",
		})
		require.Equal(t, uint64(1), got.GetSampleCount())
		require.Equal(t, expectedDuration, got.GetSampleSum())
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
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentCreated.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        buildCreatedAt,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       false,
			AllAgentsReady:   true,
			LastAgentReadyAt: agentReadyAt,
			WorstStatus:      "success",
		}, nil)

		publishCalled := false
		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
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

		got := promhelp.HistogramValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "success",
			"is_prebuild":       "false",
		})
		require.Equal(t, uint64(1), got.GetSampleCount())
		require.Equal(t, expectedDuration, got.GetSampleSum())
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
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentCreated.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        buildCreatedAt,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       false,
			AllAgentsReady:   true,
			LastAgentReadyAt: agentReadyAt,
			WorstStatus:      "success",
		}, nil)

		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentCreated, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn:        nil,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)

		got := promhelp.HistogramValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "success",
			"is_prebuild":       "false",
		})
		require.Equal(t, uint64(1), got.GetSampleCount())
		require.Equal(t, expectedDuration, got.GetSampleSum())
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
		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
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
			if state == agentproto.Lifecycle_READY || state == agentproto.Lifecycle_START_TIMEOUT || state == agentproto.Lifecycle_START_ERROR {
				expectedReadyAt = sql.NullTime{Valid: true, Time: stateNow}
			}

			dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.ID,
				LifecycleState: database.WorkspaceAgentLifecycleState(strings.ToLower(state.String())),
				StartedAt:      expectedStartedAt,
				ReadyAt:        expectedReadyAt,
			}).Times(1).Return(nil)

			// The first ready state triggers the build duration metric query.
			if state == agentproto.Lifecycle_READY || state == agentproto.Lifecycle_START_TIMEOUT || state == agentproto.Lifecycle_START_ERROR {
				dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agent.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
					CreatedAt:        someTime,
					Transition:       database.WorkspaceTransitionStart,
					TemplateName:     "test-template",
					OrganizationName: "test-org",
					IsPrebuild:       false,
					AllAgentsReady:   true,
					LastAgentReadyAt: stateNow,
					WorstStatus:      "success",
				}, nil).MaxTimes(1)
			}

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
			WorkspaceID: workspaceID,
			Database:    dbM,
			Log:         testutil.Logger(t),
			PublishWorkspaceUpdateFn: func(ctx context.Context, agent *database.WorkspaceAgent, kind wspubsub.WorkspaceEventKind) error {
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

	// Test that metric is NOT emitted when not all agents are ready (multi-agent case).
	t.Run("MetricNotEmittedWhenNotAllAgentsReady", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_READY,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), gomock.Any()).Return(nil)
		// Return AllAgentsReady = false to simulate multi-agent case where not all are ready.
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentStarting.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        someTime,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       false,
			AllAgentsReady:   false,       // Not all agents ready yet
			LastAgentReadyAt: time.Time{}, // No ready time yet
			WorstStatus:      "success",
		}, nil)

		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentStarting, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn:        nil,
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)

		require.Nil(t, promhelp.MetricValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "success",
			"is_prebuild":       "false",
		}), "metric should not be emitted when not all agents are ready")
	})

	// Test that prebuild label is "true" when owner is prebuild system user.
	t.Run("PrebuildLabelTrue", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_READY,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), gomock.Any()).Return(nil)
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentStarting.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        buildCreatedAt,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       true, // Prebuild workspace
			AllAgentsReady:   true,
			LastAgentReadyAt: agentReadyAt,
			WorstStatus:      "success",
		}, nil)

		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentStarting, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn:        nil,
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)

		got := promhelp.HistogramValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "success",
			"is_prebuild":       "true",
		})
		require.Equal(t, uint64(1), got.GetSampleCount())
		require.Equal(t, expectedDuration, got.GetSampleSum())
	})

	// Test worst status is used when one agent has an error.
	t.Run("WorstStatusError", func(t *testing.T) {
		t.Parallel()

		lifecycle := &agentproto.Lifecycle{
			State:     agentproto.Lifecycle_READY,
			ChangedAt: timestamppb.New(now),
		}

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), gomock.Any()).Return(nil)
		dbM.EXPECT().GetWorkspaceBuildMetricsByResourceID(gomock.Any(), agentStarting.ResourceID).Return(database.GetWorkspaceBuildMetricsByResourceIDRow{
			CreatedAt:        buildCreatedAt,
			Transition:       database.WorkspaceTransitionStart,
			TemplateName:     "test-template",
			OrganizationName: "test-org",
			IsPrebuild:       false,
			AllAgentsReady:   true,
			LastAgentReadyAt: agentReadyAt,
			WorstStatus:      "error", // One agent had an error
		}, nil)

		reg := prometheus.NewRegistry()
		hist := newBuildDurationHistogram(t, reg)

		api := &agentapi.LifecycleAPI{
			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agentStarting, nil
			},
			WorkspaceID:                     workspaceID,
			Database:                        dbM,
			Log:                             testutil.Logger(t),
			WorkspaceBuildDurationHistogram: hist,
			PublishWorkspaceUpdateFn:        nil,
		}

		resp, err := api.UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{
			Lifecycle: lifecycle,
		})
		require.NoError(t, err)
		require.Equal(t, lifecycle, resp)

		got := promhelp.HistogramValue(t, reg, fullMetricName, prometheus.Labels{
			"template_name":     "test-template",
			"organization_name": "test-org",
			"transition":        "start",
			"status":            "error",
			"is_prebuild":       "false",
		})
		require.Equal(t, uint64(1), got.GetSampleCount())
		require.Equal(t, expectedDuration, got.GetSampleSum())
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
			WorkspaceID: workspaceID,
			Database:    dbM,
			Log:         testutil.Logger(t),
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
			APIVersion: "2.0",
		}).Return(nil)

		ctx := agentapi.WithAPIVersion(context.Background(), "2.0")
		resp, err := api.UpdateStartup(ctx, &agentproto.UpdateStartupRequest{
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
			WorkspaceID: workspaceID,
			Database:    dbM,
			Log:         testutil.Logger(t),
			// Not used by UpdateStartup.
			PublishWorkspaceUpdateFn: nil,
		}

		startup := &agentproto.Startup{
			Version:           "asdf",
			ExpandedDirectory: "/path/to/expanded/dir",
			Subsystems:        []agentproto.Startup_Subsystem{},
		}

		ctx := agentapi.WithAPIVersion(context.Background(), "2.0")
		resp, err := api.UpdateStartup(ctx, &agentproto.UpdateStartupRequest{
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
			WorkspaceID: workspaceID,
			Database:    dbM,
			Log:         testutil.Logger(t),
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

		ctx := agentapi.WithAPIVersion(context.Background(), "2.0")
		resp, err := api.UpdateStartup(ctx, &agentproto.UpdateStartupRequest{
			Startup: startup,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid agent subsystem")
		require.Nil(t, resp)
	})
}
