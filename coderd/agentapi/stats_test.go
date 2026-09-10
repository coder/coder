package agentapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/workspacestats/workspacestatstest"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateStats(t *testing.T) {
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
			ID:            uuid.New(),
			OwnerID:       user.ID,
			OwnerUsername: user.Username,
			TemplateID:    template.ID,
			Name:          "xyz",
			TemplateName:  template.Name,
		}
		agent = database.WorkspaceAgent{
			ID:   uuid.New(),
			Name: "abc",
		}
		workspaceAsCacheFields = agentapi.CachedWorkspaceFields{}
	)

	workspaceAsCacheFields.UpdateValues(database.Workspace{
		ID:                workspace.ID,
		OwnerID:           workspace.OwnerID,
		OwnerUsername:     workspace.OwnerUsername,
		TemplateID:        workspace.TemplateID,
		Name:              workspace.Name,
		TemplateName:      workspace.TemplateName,
		AutostartSchedule: workspace.AutostartSchedule,
	})

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
			batcher                    = &workspacestatstest.StatsBatcher{}
			updateAgentMetricsFnCalled = false
			tickCh                     = make(chan time.Time)
			flushCh                    = make(chan int, 1)
			wut                        = workspacestats.NewTracker(dbM,
				workspacestats.TrackerWithTickFlush(tickCh, flushCh),
			)

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
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &workspaceAsCacheFields,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				StatsBatcher:          batcher,
				UsageTracker:          wut,
				TemplateScheduleStore: templateScheduleStorePtr(templateScheduleStore),
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
			}),
			AgentStatsRefreshInterval: 10 * time.Second,
			TimeNowFn: func() time.Time {
				return now
			},
		}
		defer wut.Close()

		// We expect an activity bump because ConnectionCount > 0.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: time.Time{}.UTC(),
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs:        []uuid.UUID{workspace.ID},
			LastUsedAt: now,
		}).Return(nil)

		// Ensure that pubsub notifications are sent.
		notifyDescription := make(chan struct{})
		ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
			wspubsub.HandleWorkspaceEvent(
				func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
					if err != nil {
						return
					}
					if e.Kind == wspubsub.WorkspaceEventKindStatsUpdate && e.WorkspaceID == workspace.ID {
						go func() {
							notifyDescription <- struct{}{}
						}()
					}
				}))

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(10 * time.Second),
		}, resp)

		tickCh <- now
		count := <-flushCh
		require.Equal(t, 1, count, "expected one flush with one id")

		batcher.Mu.Lock()
		defer batcher.Mu.Unlock()
		require.Equal(t, int64(1), batcher.Called)
		require.Equal(t, now, batcher.LastTime)
		require.Equal(t, agent.ID, batcher.LastAgentID)
		require.Equal(t, template.ID, batcher.LastTemplateID)
		require.Equal(t, user.ID, batcher.LastUserID)
		require.Equal(t, workspace.ID, batcher.LastWorkspaceID)
		require.Equal(t, req.Stats, batcher.LastStats)
		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-ctx.Done():
			t.Error("timed out while waiting for pubsub notification")
		case <-notifyDescription:
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
			batcher = &workspacestatstest.StatsBatcher{}

			req = &agentproto.UpdateStatsRequest{
				Stats: &agentproto.Stats{
					ConnectionsByProto:        map[string]int64{},
					ConnectionCount:           0,
					ConnectionMedianLatencyMs: 23,
				},
			}
		)
		api := agentapi.StatsAPI{
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &workspaceAsCacheFields,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				UsageTracker:          workspacestats.NewTracker(dbM),
				StatsBatcher:          batcher,
				TemplateScheduleStore: templateScheduleStorePtr(templateScheduleStore),
				// Ignored when nil.
				UpdateAgentMetricsFn: nil,
			}),
			AgentStatsRefreshInterval: 10 * time.Second,
			TimeNowFn: func() time.Time {
				return now
			},
		}

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
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &workspaceAsCacheFields,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				StatsBatcher:          nil, // should not be called
				TemplateScheduleStore: nil, // should not be called
				UpdateAgentMetricsFn:  nil, // should not be called
			}),
			AgentStatsRefreshInterval: 10 * time.Second,
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
			batcher                    = &workspacestatstest.StatsBatcher{}
			updateAgentMetricsFnCalled = false
			tickCh                     = make(chan time.Time)
			flushCh                    = make(chan int, 1)
			wut                        = workspacestats.NewTracker(dbM,
				workspacestats.TrackerWithTickFlush(tickCh, flushCh),
			)

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
		// need to overwrite the cached fields for this test, but the struct has a lock
		ws := agentapi.CachedWorkspaceFields{}
		ws.UpdateValues(workspace)
		// ws.AutostartSchedule = workspace.AutostartSchedule

		api := agentapi.StatsAPI{
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &ws,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				UsageTracker:          wut,
				StatsBatcher:          batcher,
				TemplateScheduleStore: templateScheduleStorePtr(templateScheduleStore),
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
			}),
			AgentStatsRefreshInterval: 15 * time.Second,
			TimeNowFn: func() time.Time {
				return now
			},
		}
		defer wut.Close()

		// We expect an activity bump because ConnectionCount > 0. However, the
		// next autostart time will be set on the bump.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: nextAutostart,
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs:        []uuid.UUID{workspace.ID},
			LastUsedAt: now.UTC(),
		}).Return(nil)

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(15 * time.Second),
		}, resp)

		tickCh <- now
		count := <-flushCh
		require.Equal(t, 1, count, "expected one flush with one id")

		require.True(t, updateAgentMetricsFnCalled)
	})

	t.Run("WorkspaceUsageExperiment", func(t *testing.T) {
		t.Parallel()

		var (
			now = dbtime.Now()
			dbM = dbmock.NewMockStore(gomock.NewController(t))
			ps  = pubsub.NewInMemory()

			templateScheduleStore = schedule.MockTemplateScheduleStore{
				GetFn: func(context.Context, database.Store, uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					t.Fatal("getfn should not be called")
					return schedule.TemplateScheduleOptions{}, nil
				},
				SetFn: func(context.Context, database.Store, database.Template, schedule.TemplateScheduleOptions) (database.Template, error) {
					t.Fatal("setfn not implemented")
					return database.Template{}, nil
				},
			}
			batcher                    = &workspacestatstest.StatsBatcher{}
			updateAgentMetricsFnCalled = false
			tickCh                     = make(chan time.Time)
			flushCh                    = make(chan int, 1)
			wut                        = workspacestats.NewTracker(dbM,
				workspacestats.TrackerWithTickFlush(tickCh, flushCh),
			)

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
		defer wut.Close()
		api := agentapi.StatsAPI{
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &workspaceAsCacheFields,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				StatsBatcher:          batcher,
				UsageTracker:          wut,
				TemplateScheduleStore: templateScheduleStorePtr(templateScheduleStore),
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
			}),
			AgentStatsRefreshInterval: 10 * time.Second,
			TimeNowFn: func() time.Time {
				return now
			},
			Experiments: codersdk.Experiments{
				codersdk.ExperimentWorkspaceUsage,
			},
		}

		// We expect an activity bump because ConnectionCount > 0.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: time.Time{}.UTC(),
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs:        []uuid.UUID{workspace.ID},
			LastUsedAt: now,
		}).Return(nil)

		// Ensure that pubsub notifications are sent.
		notifyDescription := make(chan struct{})
		ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
			wspubsub.HandleWorkspaceEvent(
				func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
					if err != nil {
						return
					}
					if e.Kind == wspubsub.WorkspaceEventKindStatsUpdate && e.WorkspaceID == workspace.ID {
						go func() {
							notifyDescription <- struct{}{}
						}()
					}
				}))

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(10 * time.Second),
		}, resp)

		tickCh <- now
		count := <-flushCh
		require.Equal(t, 1, count, "expected one flush with one id")

		batcher.Mu.Lock()
		defer batcher.Mu.Unlock()
		require.EqualValues(t, 1, batcher.Called)
		require.Empty(t, batcher.LastStats.GetSessionCounts())
		require.EqualValues(t, 0, batcher.LastStats.SessionCountSsh)
		require.EqualValues(t, 0, batcher.LastStats.SessionCountJetbrains)
		require.EqualValues(t, 0, batcher.LastStats.SessionCountVscode)
		require.EqualValues(t, 0, batcher.LastStats.SessionCountReconnectingPty)
		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-ctx.Done():
			t.Error("timed out while waiting for pubsub notification")
		case <-notifyDescription:
		}
		require.True(t, updateAgentMetricsFnCalled)
	})

	t.Run("DropStats", func(t *testing.T) {
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
			updateAgentMetricsFnCalled = false
			tickCh                     = make(chan time.Time)
			flushCh                    = make(chan int, 1)
			wut                        = workspacestats.NewTracker(dbM,
				workspacestats.TrackerWithTickFlush(tickCh, flushCh),
			)

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
			AgentID:   agent.ID,
			AgentName: agent.Name,
			Workspace: &workspaceAsCacheFields,
			Database:  dbM,
			StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
				Database:              dbM,
				Pubsub:                ps,
				StatsBatcher:          nil, // Should not be called.
				UsageTracker:          wut,
				TemplateScheduleStore: templateScheduleStorePtr(templateScheduleStore),
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
				DisableDatabaseInserts: true,
			}),
			AgentStatsRefreshInterval: 10 * time.Second,
			TimeNowFn: func() time.Time {
				return now
			},
		}
		defer wut.Close()

		// We expect an activity bump because ConnectionCount > 0.
		dbM.EXPECT().ActivityBumpWorkspace(gomock.Any(), database.ActivityBumpWorkspaceParams{
			WorkspaceID:   workspace.ID,
			NextAutostart: time.Time{}.UTC(),
		}).Return(nil)

		// Workspace last used at gets bumped.
		dbM.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs:        []uuid.UUID{workspace.ID},
			LastUsedAt: now,
		}).Return(nil)

		// Ensure that pubsub notifications are sent.
		notifyDescription := make(chan struct{})
		ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
			wspubsub.HandleWorkspaceEvent(
				func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
					if err != nil {
						return
					}
					if e.Kind == wspubsub.WorkspaceEventKindStatsUpdate && e.WorkspaceID == workspace.ID {
						go func() {
							notifyDescription <- struct{}{}
						}()
					}
				}))

		resp, err := api.UpdateStats(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.UpdateStatsResponse{
			ReportInterval: durationpb.New(10 * time.Second),
		}, resp)

		tickCh <- now
		count := <-flushCh
		require.Equal(t, 1, count, "expected one flush with one id")

		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-ctx.Done():
			t.Error("timed out while waiting for pubsub notification")
		case <-notifyDescription:
		}
		require.True(t, updateAgentMetricsFnCalled)
	})

	for _, name := range []string{"RankAfterMerging", "AliasesFitWithoutFolding", "NonPositiveSuppressesFallback"} {
		t.Run("SessionCounts/"+name, func(t *testing.T) {
			t.Parallel()

			counts := make(map[string]int64)
			want := make(map[string]int64)
			for i := range 300 {
				counts["VSCode"+strings.Repeat(" ", i)] = 1
			}
			switch name {
			case "RankAfterMerging":
				// Known apps win even with smaller counts. Unknown aliases
				// must merge before ranking, with name breaking count ties.
				counts["SSH"] = 1
				counts["unknown"] = -1_000
				counts[" VSCODE"] = -1_000
				counts["idle"] = 0
				for i := range 300 {
					counts[fmt.Sprintf("app-%03d", i)] = 2
					counts["zzz-busy"+strings.Repeat(" ", i)] = 1
				}
				want = map[string]int64{"vscode": 300, "ssh": 1, "zzz_busy": 300, "unknown": 478}
				for i := range 61 {
					want[fmt.Sprintf("app_%03d", i)] = 2
				}
			case "AliasesFitWithoutFolding":
				want["vscode"] = 300
			case "NonPositiveSuppressesFallback":
				for key := range counts {
					counts[key] = -1
				}
				counts["unknown"] = 0
			}

			ctx := testutil.Context(t, testutil.WaitLong)
			dbM := dbmock.NewMockStore(gomock.NewController(t))
			inserted := make(chan database.InsertWorkspaceAgentStatsParams, 1)
			dbM.EXPECT().InsertWorkspaceAgentStats(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.InsertWorkspaceAgentStatsParams) error {
					inserted <- params
					return nil
				})
			batcher, closeBatcher, err := workspacestats.NewBatcher(ctx,
				workspacestats.BatcherWithStore(dbM),
				workspacestats.BatcherWithLogger(testutil.Logger(t)),
				workspacestats.BatcherWithInterval(time.Hour),
			)
			require.NoError(t, err)
			t.Cleanup(closeBatcher)
			api := agentapi.StatsAPI{
				AgentID:   agent.ID,
				AgentName: agent.Name,
				Workspace: &workspaceAsCacheFields,
				Database:  dbM,
				StatsReporter: workspacestats.NewReporter(workspacestats.ReporterOptions{
					StatsBatcher: batcher,
				}),
				AgentStatsRefreshInterval: 10 * time.Second,
			}
			_, err = api.UpdateStats(ctx, &agentproto.UpdateStatsRequest{Stats: &agentproto.Stats{
				SessionCounts: counts,
				// A present map must suppress deprecated fields even when
				// every reported count is non-positive.
				SessionCountVscode:        9,
				ConnectionsByProto:        map[string]int64{"tcp": 1},
				ConnectionMedianLatencyMs: 23,
				RxPackets:                 120,
				RxBytes:                   1000,
				TxPackets:                 130,
				TxBytes:                   2000,
			}})
			require.NoError(t, err)
			closeBatcher() // Flush synchronously without waiting for a timer.

			select {
			case params := <-inserted:
				var got []map[string]int64
				require.NoError(t, json.Unmarshal(params.SessionCounts, &got))
				require.Equal(t, []map[string]int64{want}, got)
				// Capping sessions must not change the rest of the report.
				require.JSONEq(t, `[{"tcp":1}]`, string(params.ConnectionsByProto))
				require.Equal(t, []int64{0}, params.ConnectionCount)
				require.Equal(t, []float64{23}, params.ConnectionMedianLatencyMS)
				require.Equal(t, []int64{120}, params.RxPackets)
				require.Equal(t, []int64{1000}, params.RxBytes)
				require.Equal(t, []int64{130}, params.TxPackets)
				require.Equal(t, []int64{2000}, params.TxBytes)
			case <-ctx.Done():
				t.Fatal("timed out waiting for stats insertion")
			}
		})
	}
}

func templateScheduleStorePtr(store schedule.TemplateScheduleStore) *atomic.Pointer[schedule.TemplateScheduleStore] {
	var ptr atomic.Pointer[schedule.TemplateScheduleStore]
	ptr.Store(&store)
	return &ptr
}
