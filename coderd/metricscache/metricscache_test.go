package metricscache_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/metricscache"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func newMetricsCache(t *testing.T, log slog.Logger, clock quartz.Clock, intervals metricscache.Intervals, usage bool) (*metricscache.Cache, database.Store) {
	t.Helper()

	accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControlStore.Store(&acs)

	var (
		auth   = rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
		db, _  = dbtestutil.NewDB(t)
		dbauth = dbauthz.New(db, auth, log, accessControlStore)
		cache  = metricscache.New(dbauth, log, clock, intervals, usage)
	)

	t.Cleanup(func() { cache.Close() })

	return cache, db
}

func TestCache_TemplateWorkspaceOwners(t *testing.T) {
	t.Parallel()

	var (
		ctx   = testutil.Context(t, testutil.WaitShort)
		log   = testutil.Logger(t)
		clock = quartz.NewMock(t)
	)

	trapTickerFunc := clock.Trap().TickerFunc("metricscache")
	defer trapTickerFunc.Close()

	cache, db := newMetricsCache(t, log, clock, metricscache.Intervals{
		TemplateBuildTimes: time.Minute,
	}, false)

	org := dbgen.Organization(t, db, database.Organization{})
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		CreatedBy:      user1.ID,
	})

	// Wait for both ticker functions to be created (template build times and deployment stats)
	trapTickerFunc.MustWait(ctx).MustRelease(ctx)
	trapTickerFunc.MustWait(ctx).MustRelease(ctx)

	clock.Advance(time.Minute).MustWait(ctx)

	count, ok := cache.TemplateWorkspaceOwners(template.ID)
	require.True(t, ok, "TemplateWorkspaceOwners should be populated")
	require.Equal(t, 0, count, "should have 0 owners initially")

	dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user1.ID,
	})

	clock.Advance(time.Minute).MustWait(ctx)

	count, _ = cache.TemplateWorkspaceOwners(template.ID)
	require.Equal(t, 1, count, "should have 1 owner after adding workspace")

	workspace2 := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user2.ID,
	})

	clock.Advance(time.Minute).MustWait(ctx)

	count, _ = cache.TemplateWorkspaceOwners(template.ID)
	require.Equal(t, 2, count, "should have 2 owners after adding second workspace")

	// 3rd workspace should not be counted since we have the same owner as workspace2.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user1.ID,
	})

	db.UpdateWorkspaceDeletedByID(context.Background(), database.UpdateWorkspaceDeletedByIDParams{
		ID:      workspace2.ID,
		Deleted: true,
	})

	clock.Advance(time.Minute).MustWait(ctx)

	count, _ = cache.TemplateWorkspaceOwners(template.ID)
	require.Equal(t, 1, count, "should have 1 owner after deleting workspace")
}

func clockTime(t time.Time, hour, minute, sec int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), hour, minute, sec, t.Nanosecond(), t.Location())
}

func requireBuildTimeStatsEmpty(t *testing.T, stats codersdk.TemplateBuildTimeStats) {
	require.Empty(t, stats[codersdk.WorkspaceTransitionStart])
	require.Empty(t, stats[codersdk.WorkspaceTransitionStop])
	require.Empty(t, stats[codersdk.WorkspaceTransitionDelete])
}

func TestCache_BuildTime(t *testing.T) {
	t.Parallel()

	someDay := date(2022, 10, 1)

	type jobParams struct {
		startedAt   time.Time
		completedAt time.Time
	}

	type args struct {
		rows       []jobParams
		transition database.WorkspaceTransition
	}
	type want struct {
		buildTimeMs int64
		loads       bool
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{"empty", args{}, want{-1, false}},
		{
			"one/start", args{
				rows: []jobParams{
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 10),
					},
				},
				transition: database.WorkspaceTransitionStart,
			}, want{10 * 1000, true},
		},
		{
			"two/stop", args{
				rows: []jobParams{
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 10),
					},
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 50),
					},
				},
				transition: database.WorkspaceTransitionStop,
			}, want{10 * 1000, true},
		},
		{
			"three/delete", args{
				rows: []jobParams{
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 10),
					},
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 50),
					},
					{
						startedAt:   clockTime(someDay, 10, 1, 0),
						completedAt: clockTime(someDay, 10, 1, 20),
					},
				},
				transition: database.WorkspaceTransitionDelete,
			}, want{20 * 1000, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				ctx   = testutil.Context(t, testutil.WaitShort)
				log   = testutil.Logger(t)
				clock = quartz.NewMock(t)
			)

			clock.Set(someDay)

			trapTickerFunc := clock.Trap().TickerFunc("metricscache")

			defer trapTickerFunc.Close()
			cache, db := newMetricsCache(t, log, clock, metricscache.Intervals{
				TemplateBuildTimes: time.Minute,
			}, false)

			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})

			template := dbgen.Template(t, db, database.Template{
				CreatedBy:      user.ID,
				OrganizationID: org.ID,
			})

			templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: org.ID,
				CreatedBy:      user.ID,
				TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
			})

			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				TemplateID:     template.ID,
			})

			gotStats := cache.TemplateBuildTimeStats(template.ID)
			requireBuildTimeStatsEmpty(t, gotStats)

			for buildNumber, row := range tt.args.rows {
				job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					OrganizationID: org.ID,
					InitiatorID:    user.ID,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					StartedAt:      sql.NullTime{Time: row.startedAt, Valid: true},
					CompletedAt:    sql.NullTime{Time: row.completedAt, Valid: true},
				})

				dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					BuildNumber:       int32(1 + buildNumber), // nolint:gosec
					WorkspaceID:       workspace.ID,
					InitiatorID:       user.ID,
					TemplateVersionID: templateVersion.ID,
					JobID:             job.ID,
					Transition:        tt.args.transition,
				})
			}

			// Wait for both ticker functions to be created (template build times and deployment stats)
			trapTickerFunc.MustWait(ctx).MustRelease(ctx)
			trapTickerFunc.MustWait(ctx).MustRelease(ctx)

			clock.Advance(time.Minute).MustWait(ctx)

			if tt.want.loads {
				wantTransition := codersdk.WorkspaceTransition(tt.args.transition)
				gotStats := cache.TemplateBuildTimeStats(template.ID)
				ts := gotStats[wantTransition]
				require.NotNil(t, ts.P50, "P50 should be set for %v", wantTransition)
				require.Equal(t, tt.want.buildTimeMs, *ts.P50, "P50 should match expected value for %v", wantTransition)

				for transition, ts := range gotStats {
					if transition == wantTransition {
						// Checked above
						continue
					}
					require.Empty(t, ts, "%v", transition)
				}
			} else {
				stats := cache.TemplateBuildTimeStats(template.ID)
				requireBuildTimeStatsEmpty(t, stats)
			}
		})
	}
}

func TestCache_DeploymentStats(t *testing.T) {
	t.Parallel()
	for _, usage := range []bool{false, true} {
		name := "Stats"
		if usage {
			name = "Usage"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			now := dbtime.Now().Truncate(time.Minute)
			clock.Set(now)
			tickerTrap := clock.Trap().TickerFunc("metricscache")
			defer tickerTrap.Close()
			cache, db := newMetricsCache(t, testutil.Logger(t), clock, metricscache.Intervals{DeploymentStats: time.Minute}, usage)
			counts := map[string]int64{"vscode": 1, "cursor": 2, "zed": 3, "future_ide": 4, "jetbrains": 5, "reconnecting_pty": 6}
			agentStat := dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
				CreatedAt: now.Add(-2 * time.Minute), Usage: usage, RxBytes: 1, TxBytes: 1, ConnectionCount: 1,
				ConnectionMedianLatencyMS: 10, SessionCounts: dbgen.SessionCounts(t, counts),
			})
			tickerTrap.MustWait(ctx).MustRelease(ctx)
			tickerTrap.MustWait(ctx).MustRelease(ctx)
			clock.Advance(time.Minute).MustWait(ctx)
			stat, ok := cache.DeploymentStats()
			require.True(t, ok)
			require.Equal(t, counts, stat.SessionCount.SessionCounts)
			require.Equal(t, int64(3), stat.SessionCount.VSCode)
			require.Equal(t, int64(3), stat.SessionCount.SSH)
			require.Equal(t, int64(5), stat.SessionCount.JetBrains)
			require.Equal(t, int64(6), stat.SessionCount.ReconnectingPTY)
			require.Equal(t, codersdk.SessionCountApp{DisplayName: "Cursor", Icon: "/icon/cursor.svg"}, stat.SessionCount.Apps["cursor"])
			require.NotContains(t, stat.SessionCount.Apps, "future_ide")
			require.Len(t, stat.SessionCount.Apps, 5)

			other, ok := cache.DeploymentStats()
			require.True(t, ok)
			other.SessionCount.SessionCounts["cursor"] = 999
			other.SessionCount.Apps["cursor"] = codersdk.SessionCountApp{DisplayName: "Changed"}
			unchanged, ok := cache.DeploymentStats()
			require.True(t, ok)
			require.Equal(t, int64(2), unchanged.SessionCount.SessionCounts["cursor"])
			require.Equal(t, "Cursor", unchanged.SessionCount.Apps["cursor"].DisplayName)

			dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
				AgentID: agentStat.AgentID, UserID: agentStat.UserID, WorkspaceID: agentStat.WorkspaceID, TemplateID: agentStat.TemplateID,
				CreatedAt: now.Add(-time.Minute), Usage: usage, ConnectionMedianLatencyMS: 10,
				SessionCounts: dbgen.SessionCounts(t, map[string]int64{}),
			})
			clock.Advance(time.Minute).MustWait(ctx)
			empty, ok := cache.DeploymentStats()
			require.True(t, ok)
			require.Equal(t, map[string]int64{}, empty.SessionCount.SessionCounts)
			require.Equal(t, map[string]codersdk.SessionCountApp{}, empty.SessionCount.Apps)
			require.Zero(t, empty.SessionCount.VSCode)
			require.Zero(t, empty.SessionCount.SSH)
			require.Zero(t, empty.SessionCount.JetBrains)
			require.Zero(t, empty.SessionCount.ReconnectingPTY)
			raw, err := json.Marshal(empty.SessionCount)
			require.NoError(t, err)
			require.JSONEq(t, `{"session_counts":{},"apps":{},"vscode":0,"ssh":0,"jetbrains":0,"reconnecting_pty":0}`, string(raw))
			require.Equal(t, counts, stat.SessionCount.SessionCounts, "refresh must not mutate a published map")
			require.Len(t, stat.SessionCount.Apps, 5)
		})
	}
}
