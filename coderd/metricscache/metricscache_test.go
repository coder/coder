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

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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
	var ()

	var (
		log       = testutil.Logger(t)
		clock     = quartz.NewReal()
		cache, db = newMetricsCache(t, log, clock, metricscache.Intervals{
			TemplateBuildTimes: testutil.IntervalFast,
		}, false)
	)

	org := dbgen.Organization(t, db, database.Organization{})
	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		CreatedBy:      user1.ID,
	})
	require.Eventuallyf(t, func() bool {
		count, ok := cache.TemplateWorkspaceOwners(template.ID)
		return ok && count == 0
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 0 owners",
	)

	dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user1.ID,
	})

	require.Eventuallyf(t, func() bool {
		count, _ := cache.TemplateWorkspaceOwners(template.ID)
		return count == 1
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 1 owner",
	)

	workspace2 := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user2.ID,
	})

	require.Eventuallyf(t, func() bool {
		count, _ := cache.TemplateWorkspaceOwners(template.ID)
		return count == 2
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 2 owners",
	)

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

	require.Eventuallyf(t, func() bool {
		count, _ := cache.TemplateWorkspaceOwners(template.ID)
		return count == 1
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 1 owner after delete",
	)
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				log       = testutil.Logger(t)
				clock     = quartz.NewMock(t)
				cache, db = newMetricsCache(t, log, clock, metricscache.Intervals{
					TemplateBuildTimes: testutil.IntervalFast,
				}, false)
			)

			clock.Set(someDay)

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
					// #nosec G115 - Safe conversion as build number is expected to be within int32 range
					BuildNumber:       int32(1 + buildNumber),
					WorkspaceID:       workspace.ID,
					InitiatorID:       user.ID,
					TemplateVersionID: templateVersion.ID,
					JobID:             job.ID,
					Transition:        tt.args.transition,
				})
			}

			if tt.want.loads {
				wantTransition := codersdk.WorkspaceTransition(tt.args.transition)
				require.Eventuallyf(t, func() bool {
					stats := cache.TemplateBuildTimeStats(template.ID)
					return stats[wantTransition] != codersdk.TransitionStats{}
				}, testutil.WaitLong, testutil.IntervalMedium,
					"BuildTime never populated",
				)

				gotStats = cache.TemplateBuildTimeStats(template.ID)
				for transition, stats := range gotStats {
					if transition == wantTransition {
						require.Equal(t, tt.want.buildTimeMs, *stats.P50)
					} else {
						require.Empty(
							t, stats, "%v", transition,
						)
					}
				}
			} else {
				var stats codersdk.TemplateBuildTimeStats
				require.Never(t, func() bool {
					stats = cache.TemplateBuildTimeStats(template.ID)
					requireBuildTimeStatsEmpty(t, stats)
					return t.Failed()
				}, testutil.WaitShort/2, testutil.IntervalMedium,
					"BuildTimeStats populated", stats,
				)
			}
		})
	}
}

func TestCache_DeploymentStats(t *testing.T) {
	t.Parallel()

	var (
		log       = testutil.Logger(t)
		clock     = quartz.NewMock(t)
		cache, db = newMetricsCache(t, log, clock, metricscache.Intervals{
			DeploymentStats: testutil.IntervalFast,
		}, false)
	)

	err := db.InsertWorkspaceAgentStats(context.Background(), database.InsertWorkspaceAgentStatsParams{
		ID:                 []uuid.UUID{uuid.New()},
		CreatedAt:          []time.Time{clock.Now()},
		WorkspaceID:        []uuid.UUID{uuid.New()},
		UserID:             []uuid.UUID{uuid.New()},
		TemplateID:         []uuid.UUID{uuid.New()},
		AgentID:            []uuid.UUID{uuid.New()},
		ConnectionsByProto: json.RawMessage(`[{}]`),

		RxPackets:                   []int64{0},
		RxBytes:                     []int64{1},
		TxPackets:                   []int64{0},
		TxBytes:                     []int64{1},
		ConnectionCount:             []int64{1},
		SessionCountVSCode:          []int64{1},
		SessionCountJetBrains:       []int64{0},
		SessionCountReconnectingPTY: []int64{0},
		SessionCountSSH:             []int64{0},
		ConnectionMedianLatencyMS:   []float64{10},
		Usage:                       []bool{false},
	})
	require.NoError(t, err)

	var stat codersdk.DeploymentStats
	require.Eventually(t, func() bool {
		var ok bool
		stat, ok = cache.DeploymentStats()
		return ok
	}, testutil.WaitLong, testutil.IntervalMedium)
	require.Equal(t, int64(1), stat.SessionCount.VSCode)
}
