package agentapi_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type statsBatcher struct {
	mu sync.Mutex

	called          int64
	lastTime        time.Time
	lastAgentID     uuid.UUID
	lastTemplateID  uuid.UUID
	lastUserID      uuid.UUID
	lastWorkspaceID uuid.UUID
	lastStats       *agentproto.Stats
}

var _ agentapi.StatsBatcher = &statsBatcher{}

func (b *statsBatcher) Add(now time.Time, agentID uuid.UUID, templateID uuid.UUID, userID uuid.UUID, workspaceID uuid.UUID, st *agentproto.Stats) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.called++
	b.lastTime = now
	b.lastAgentID = agentID
	b.lastTemplateID = templateID
	b.lastUserID = userID
	b.lastWorkspaceID = workspaceID
	b.lastStats = st
	return nil
}

func TestUpdateStates(t *testing.T) {
	t.Parallel()

	var (
		user = database.User{
			ID:       uuid.New(),
			Username: "bill",
		}
		template = database.Template{
			ID:   uuid.New(),
			Name: "tpl",
		}
		workspace = database.Workspace{
			ID:         uuid.New(),
			OwnerID:    user.ID,
			TemplateID: template.ID,
			Name:       "xyz",
		}
		agent = database.WorkspaceAgent{
			ID:   uuid.New(),
			Name: "abc",
		}
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			now = dbtime.Now()
			dbM = dbmock.NewMockStore(gomock.NewController(t))
			ps  = pubsub.NewInMemory()

			templateScheduleStore = schedule.MockTemplateScheduleStore{
				GetFn: func(context.Context, database.Store, uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					panic("should not be called")
				},
				SetFn: func(context.Context, database.Store, database.Template, schedule.TemplateScheduleOptions) (database.Template, error) {
					panic("not implemented")
				},
			}
			batcher                    = &statsBatcher{}
			updateAgentMetricsFnCalled = false

			req = &agentproto.UpdateStatsRequest{
				Stats: &agentproto.Stats{
					ConnectionsByProto: map[string]int64{
						"tcp":  1,
						"dean": 2,
					},
					ConnectionCount:             3,
					ConnectionMedianLatencyMs:   23,
					RxPackets:                   120,
					RxBytes:                     1000,
					TxPackets:                   130,
					TxBytes:                     2000,
					SessionCountVscode:          1,
					SessionCountJetbrains:       2,
					SessionCountReconnectingPty: 3,
					SessionCountSsh:             4,
					Metrics: []*agentproto.Stats_Metric{
						{
							Name:  "awesome metric",
							Value: 42,
						},
						{
							Name:  "uncool metric",
							Value: 0,
						},
					},
				},
			}
		)
		api := agentapi.StatsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                  dbM,
			Pubsub:                    ps,
			StatsBatcher:              batcher,
			TemplateScheduleStore:     templateScheduleStorePtr(templateScheduleStore),
			AgentStatsRefreshInterval: 10 * time.Second,
			UpdateAgentMetricsFn: func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric) {
				updateAgentMetricsFnCalled = true
				assert.Equal(t, prometheusmetrics.AgentMetricLabels{
					Username:      user.Username,
					WorkspaceName: workspace.Name,
					AgentName:     agent.Name,
					TemplateName:  template.Name,
				}, labels)
				assert.Equal(t, req.Stats.Metrics, metrics)
			},
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Workspace gets fetched.
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(database.GetWorkspaceByAgentIDRow{
			Workspace:    workspace,
			TemplateName: template.Name,
		}, nil)

		// We expect an activity bump because ConnectionCount > 0.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: time.Time{}.UTC(),
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().UpdateWorkspaceLastUsedAt(gomock.Any(), database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		}).Return(nil)

		// User gets fetched to hit the UpdateAgentMetricsFn.
		dbM.EXPECT().GetUserByID(gomock.Any(), user.ID).Return(user, nil)

		// Ensure that pubsub notifications are sent.
		notifyDescription := make(chan []byte)
		ps.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, description []byte) {
			go func() {
				notifyDescription <- description
			}()
		})

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(10 * time.Second),
		}, resp)

		batcher.mu.Lock()
		defer batcher.mu.Unlock()
		require.Equal(t, int64(1), batcher.called)
		require.Equal(t, now, batcher.lastTime)
		require.Equal(t, agent.ID, batcher.lastAgentID)
		require.Equal(t, template.ID, batcher.lastTemplateID)
		require.Equal(t, user.ID, batcher.lastUserID)
		require.Equal(t, workspace.ID, batcher.lastWorkspaceID)
		require.Equal(t, req.Stats, batcher.lastStats)
		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-ctx.Done():
			t.Error("timed out while waiting for pubsub notification")
		case description := <-notifyDescription:
			require.Equal(t, description, []byte{})
		}
		require.True(t, updateAgentMetricsFnCalled)
	})

	t.Run("ConnectionCountZero", func(t *testing.T) {
		t.Parallel()

		var (
			now                   = dbtime.Now()
			dbM                   = dbmock.NewMockStore(gomock.NewController(t))
			ps                    = pubsub.NewInMemory()
			templateScheduleStore = schedule.MockTemplateScheduleStore{
				GetFn: func(context.Context, database.Store, uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					panic("should not be called")
				},
				SetFn: func(context.Context, database.Store, database.Template, schedule.TemplateScheduleOptions) (database.Template, error) {
					panic("not implemented")
				},
			}
			batcher = &statsBatcher{}

			req = &agentproto.UpdateStatsRequest{
				Stats: &agentproto.Stats{
					ConnectionsByProto:        map[string]int64{},
					ConnectionCount:           0,
					ConnectionMedianLatencyMs: 23,
				},
			}
		)
		api := agentapi.StatsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                  dbM,
			Pubsub:                    ps,
			StatsBatcher:              batcher,
			TemplateScheduleStore:     templateScheduleStorePtr(templateScheduleStore),
			AgentStatsRefreshInterval: 10 * time.Second,
			// Ignored when nil.
			UpdateAgentMetricsFn: nil,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Workspace gets fetched.
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(database.GetWorkspaceByAgentIDRow{
			Workspace:    workspace,
			TemplateName: template.Name,
		}, nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().UpdateWorkspaceLastUsedAt(gomock.Any(), database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		}).Return(nil)

		_, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("NoStats", func(t *testing.T) {
		t.Parallel()

		var (
			dbM = dbmock.NewMockStore(gomock.NewController(t))
			ps  = pubsub.NewInMemory()
			req = &agentproto.UpdateStatsRequest{
				Stats: nil,
			}
		)
		api := agentapi.StatsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                  dbM,
			Pubsub:                    ps,
			StatsBatcher:              nil, // should not be called
			TemplateScheduleStore:     nil, // should not be called
			AgentStatsRefreshInterval: 10 * time.Second,
			UpdateAgentMetricsFn:      nil, // should not be called
			TimeNowFn: func() time.Time {
				panic("should not be called")
			},
		}

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(10 * time.Second),
		}, resp)
	})

	t.Run("AutostartAwareBump", func(t *testing.T) {
		t.Parallel()

		// Use a workspace with an autostart schedule.
		workspace := workspace
		workspace.AutostartSchedule = sql.NullString{
			String: "CRON_TZ=Australia/Sydney 0 8 * * *",
			Valid:  true,
		}

		// Use a custom time for now which would trigger the autostart aware
		// bump.
		now, err := time.Parse("2006-01-02 15:04:05 -0700 MST", "2023-12-19 07:30:00 +1100 AEDT")
		require.NoError(t, err)
		now = dbtime.Time(now)
		nextAutostart := now.Add(30 * time.Minute).UTC() // always sent to DB as UTC

		var (
			dbM = dbmock.NewMockStore(gomock.NewController(t))
			ps  = pubsub.NewInMemory()

			templateScheduleStore = schedule.MockTemplateScheduleStore{
				GetFn: func(context.Context, database.Store, uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostartEnabled: true,
						AutostartRequirement: schedule.TemplateAutostartRequirement{
							DaysOfWeek: 0b01111111, // every day
						},
					}, nil
				},
				SetFn: func(context.Context, database.Store, database.Template, schedule.TemplateScheduleOptions) (database.Template, error) {
					panic("not implemented")
				},
			}
			batcher                    = &statsBatcher{}
			updateAgentMetricsFnCalled = false

			req = &agentproto.UpdateStatsRequest{
				Stats: &agentproto.Stats{
					ConnectionsByProto: map[string]int64{
						"tcp":  1,
						"dean": 2,
					},
					ConnectionCount: 3,
				},
			}
		)
		api := agentapi.StatsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database:                  dbM,
			Pubsub:                    ps,
			StatsBatcher:              batcher,
			TemplateScheduleStore:     templateScheduleStorePtr(templateScheduleStore),
			AgentStatsRefreshInterval: 15 * time.Second,
			UpdateAgentMetricsFn: func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric) {
				updateAgentMetricsFnCalled = true
				assert.Equal(t, prometheusmetrics.AgentMetricLabels{
					Username:      user.Username,
					WorkspaceName: workspace.Name,
					AgentName:     agent.Name,
					TemplateName:  template.Name,
				}, labels)
				assert.Equal(t, req.Stats.Metrics, metrics)
			},
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Workspace gets fetched.
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(database.GetWorkspaceByAgentIDRow{
			Workspace:    workspace,
			TemplateName: template.Name,
		}, nil)

		// We expect an activity bump because ConnectionCount > 0. However, the
		// next autostart time will be set on the bump.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: nextAutostart,
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().UpdateWorkspaceLastUsedAt(gomock.Any(), database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		}).Return(nil)

		// User gets fetched to hit the UpdateAgentMetricsFn.
		dbM.EXPECT().GetUserByID(gomock.Any(), user.ID).Return(user, nil)

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(15 * time.Second),
		}, resp)

		require.True(t, updateAgentMetricsFnCalled)
	})
}

func templateScheduleStorePtr(store schedule.TemplateScheduleStore) *atomic.Pointer[schedule.TemplateScheduleStore] {
	var ptr atomic.Pointer[schedule.TemplateScheduleStore]
	ptr.Store(&store)
	return &ptr
}
