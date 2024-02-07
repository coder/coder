package metricscache_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/metricscache"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func dateH(year, month, day, hour int) time.Time {
	return time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC)
}

func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestCache_TemplateUsers(t *testing.T) {
	t.Parallel()
	statRow := func(user uuid.UUID, date time.Time) database.InsertWorkspaceAgentStatParams {
		return database.InsertWorkspaceAgentStatParams{
			CreatedAt: date,
			UserID:    user,
		}
	}

	var (
		zebra = uuid.UUID{1}
		tiger = uuid.UUID{2}
	)

	type args struct {
		rows []database.InsertWorkspaceAgentStatParams
	}
	type want struct {
		entries     []codersdk.DAUEntry
		uniqueUsers int
	}
	tests := []struct {
		name    string
		args    args
		tplWant want
		// dauWant is optional
		dauWant  []codersdk.DAUEntry
		tzOffset int
	}{
		{name: "empty", args: args{}, tplWant: want{nil, 0}},
		{
			name: "one hole",
			args: args{
				rows: []database.InsertWorkspaceAgentStatParams{
					statRow(zebra, dateH(2022, 8, 27, 0)),
					statRow(zebra, dateH(2022, 8, 30, 0)),
				},
			},
			tplWant: want{[]codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 27)),
					Amount: 1,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 28)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 29)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 30)),
					Amount: 1,
				},
			}, 1},
		},
		{
			name: "no holes",
			args: args{
				rows: []database.InsertWorkspaceAgentStatParams{
					statRow(zebra, dateH(2022, 8, 27, 0)),
					statRow(zebra, dateH(2022, 8, 28, 0)),
					statRow(zebra, dateH(2022, 8, 29, 0)),
				},
			},
			tplWant: want{[]codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 27)),
					Amount: 1,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 28)),
					Amount: 1,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 8, 29)),
					Amount: 1,
				},
			}, 1},
		},
		{
			name: "holes",
			args: args{
				rows: []database.InsertWorkspaceAgentStatParams{
					statRow(zebra, dateH(2022, 1, 1, 0)),
					statRow(tiger, dateH(2022, 1, 1, 0)),
					statRow(zebra, dateH(2022, 1, 4, 0)),
					statRow(zebra, dateH(2022, 1, 7, 0)),
					statRow(tiger, dateH(2022, 1, 7, 0)),
				},
			},
			tplWant: want{[]codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 1)),
					Amount: 2,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 2)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 3)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 4)),
					Amount: 1,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 5)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 6)),
					Amount: 0,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 7)),
					Amount: 2,
				},
			}, 2},
		},
		{
			name:     "tzOffset",
			tzOffset: 3,
			args: args{
				rows: []database.InsertWorkspaceAgentStatParams{
					statRow(zebra, dateH(2022, 1, 2, 3)),
					statRow(tiger, dateH(2022, 1, 2, 3)),
					// With offset these should be in the previous day
					statRow(zebra, dateH(2022, 1, 2, 0)),
					statRow(tiger, dateH(2022, 1, 2, 0)),
				},
			},
			tplWant: want{[]codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 2)),
					Amount: 2,
				},
			}, 2},
			dauWant: []codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 1)),
					Amount: 2,
				},
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 2)),
					Amount: 2,
				},
			},
		},
		{
			name:     "tzOffsetPreviousDay",
			tzOffset: 6,
			args: args{
				rows: []database.InsertWorkspaceAgentStatParams{
					statRow(zebra, dateH(2022, 1, 2, 1)),
					statRow(tiger, dateH(2022, 1, 2, 1)),
					statRow(zebra, dateH(2022, 1, 2, 0)),
					statRow(tiger, dateH(2022, 1, 2, 0)),
				},
			},
			dauWant: []codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 1)),
					Amount: 2,
				},
			},
			tplWant: want{[]codersdk.DAUEntry{
				{
					Date:   metricscache.OnlyDate(date(2022, 1, 2)),
					Amount: 2,
				},
			}, 2},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				db    = dbmem.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), metricscache.Intervals{
					TemplateDAUs: testutil.IntervalFast,
				})
			)

			defer cache.Close()

			template := dbgen.Template(t, db, database.Template{
				Provisioner: database.ProvisionerTypeEcho,
			})

			for _, row := range tt.args.rows {
				row.TemplateID = template.ID
				row.ConnectionCount = 1
				db.InsertWorkspaceAgentStat(context.Background(), row)
			}

			require.Eventuallyf(t, func() bool {
				_, _, ok := cache.TemplateDAUs(template.ID, tt.tzOffset)
				return ok
			}, testutil.WaitShort, testutil.IntervalMedium,
				"TemplateDAUs never populated",
			)

			gotUniqueUsers, ok := cache.TemplateUniqueUsers(template.ID)
			require.True(t, ok)

			if tt.dauWant != nil {
				_, dauResponse, ok := cache.DeploymentDAUs(tt.tzOffset)
				require.True(t, ok)
				require.Equal(t, tt.dauWant, dauResponse.Entries)
			}

			offset, gotEntries, ok := cache.TemplateDAUs(template.ID, tt.tzOffset)
			require.True(t, ok)
			// Template only supports 0 offset.
			require.Equal(t, 0, offset)
			require.Equal(t, tt.tplWant.entries, gotEntries.Entries)
			require.Equal(t, tt.tplWant.uniqueUsers, gotUniqueUsers)
		})
	}
}

func TestCache_TemplateWorkspaceOwners(t *testing.T) {
	t.Parallel()
	var ()

	var (
		db    = dbmem.New()
		cache = metricscache.New(db, slogtest.Make(t, nil), metricscache.Intervals{
			TemplateDAUs: testutil.IntervalFast,
		})
	)

	defer cache.Close()

	user1 := dbgen.User(t, db, database.User{})
	user2 := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		Provisioner: database.ProvisionerTypeEcho,
	})
	require.Eventuallyf(t, func() bool {
		count, ok := cache.TemplateWorkspaceOwners(template.ID)
		return ok && count == 0
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 0 owners",
	)

	dbgen.Workspace(t, db, database.Workspace{
		TemplateID: template.ID,
		OwnerID:    user1.ID,
	})

	require.Eventuallyf(t, func() bool {
		count, _ := cache.TemplateWorkspaceOwners(template.ID)
		return count == 1
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 1 owner",
	)

	workspace2 := dbgen.Workspace(t, db, database.Workspace{
		TemplateID: template.ID,
		OwnerID:    user2.ID,
	})

	require.Eventuallyf(t, func() bool {
		count, _ := cache.TemplateWorkspaceOwners(template.ID)
		return count == 2
	}, testutil.WaitShort, testutil.IntervalMedium,
		"TemplateWorkspaceOwners never populated 2 owners",
	)

	// 3rd workspace should not be counted since we have the same owner as workspace2.
	dbgen.Workspace(t, db, database.Workspace{
		TemplateID: template.ID,
		OwnerID:    user1.ID,
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
			}, want{50 * 1000, true},
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
			ctx := context.Background()

			var (
				db    = dbmem.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), metricscache.Intervals{
					TemplateDAUs: testutil.IntervalFast,
				})
			)

			defer cache.Close()

			id := uuid.New()
			err := db.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:                  id,
				Provisioner:         database.ProvisionerTypeEcho,
				MaxPortSharingLevel: database.AppSharingLevelOwner,
			})
			require.NoError(t, err)
			template, err := db.GetTemplateByID(ctx, id)
			require.NoError(t, err)

			templateVersionID := uuid.New()
			err = db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID:         templateVersionID,
				TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true},
			})
			require.NoError(t, err)

			gotStats := cache.TemplateBuildTimeStats(template.ID)
			requireBuildTimeStatsEmpty(t, gotStats)

			for _, row := range tt.args.rows {
				_, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
					ID:            uuid.New(),
					Provisioner:   database.ProvisionerTypeEcho,
					StorageMethod: database.ProvisionerStorageMethodFile,
					Type:          database.ProvisionerJobTypeWorkspaceBuild,
				})
				require.NoError(t, err)

				job, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
					StartedAt: sql.NullTime{Time: row.startedAt, Valid: true},
					Types: []database.ProvisionerType{
						database.ProvisionerTypeEcho,
					},
				})
				require.NoError(t, err)

				err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
					TemplateVersionID: templateVersionID,
					JobID:             job.ID,
					Transition:        tt.args.transition,
					Reason:            database.BuildReasonInitiator,
				})
				require.NoError(t, err)

				err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
					ID:          job.ID,
					CompletedAt: sql.NullTime{Time: row.completedAt, Valid: true},
				})
				require.NoError(t, err)
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
	db := dbmem.New()
	cache := metricscache.New(db, slogtest.Make(t, nil), metricscache.Intervals{
		DeploymentStats: testutil.IntervalFast,
	})
	defer cache.Close()

	_, err := db.InsertWorkspaceAgentStat(context.Background(), database.InsertWorkspaceAgentStatParams{
		ID:                 uuid.New(),
		AgentID:            uuid.New(),
		CreatedAt:          dbtime.Now(),
		ConnectionCount:    1,
		RxBytes:            1,
		TxBytes:            1,
		SessionCountVSCode: 1,
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
