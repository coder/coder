package metricscache_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/metricscache"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestCache_TemplateUsers(t *testing.T) {
	t.Parallel()

	var (
		zebra = uuid.UUID{1}
		tiger = uuid.UUID{2}
	)

	type args struct {
		rows []database.InsertAgentStatParams
	}
	type want struct {
		entries     []codersdk.DAUEntry
		uniqueUsers int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{"empty", args{}, want{nil, 0}},
		{
			"one hole", args{
				rows: []database.InsertAgentStatParams{
					{
						CreatedAt: date(2022, 8, 27),
						UserID:    zebra,
					},
					{
						CreatedAt: date(2022, 8, 30),
						UserID:    zebra,
					},
				},
			}, want{[]codersdk.DAUEntry{
				{
					Date:   date(2022, 8, 27),
					Amount: 1,
				},
				{
					Date:   date(2022, 8, 28),
					Amount: 0,
				},
				{
					Date:   date(2022, 8, 29),
					Amount: 0,
				},
				{
					Date:   date(2022, 8, 30),
					Amount: 1,
				},
			}, 1},
		},
		{"no holes", args{
			rows: []database.InsertAgentStatParams{
				{
					CreatedAt: date(2022, 8, 27),
					UserID:    zebra,
				},
				{
					CreatedAt: date(2022, 8, 28),
					UserID:    zebra,
				},
				{
					CreatedAt: date(2022, 8, 29),
					UserID:    zebra,
				},
			},
		}, want{[]codersdk.DAUEntry{
			{
				Date:   date(2022, 8, 27),
				Amount: 1,
			},
			{
				Date:   date(2022, 8, 28),
				Amount: 1,
			},
			{
				Date:   date(2022, 8, 29),
				Amount: 1,
			},
		}, 1}},
		{"holes", args{
			rows: []database.InsertAgentStatParams{
				{
					CreatedAt: date(2022, 1, 1),
					UserID:    zebra,
				},
				{
					CreatedAt: date(2022, 1, 1),
					UserID:    tiger,
				},
				{
					CreatedAt: date(2022, 1, 4),
					UserID:    zebra,
				},
				{
					CreatedAt: date(2022, 1, 7),
					UserID:    zebra,
				},
				{
					CreatedAt: date(2022, 1, 7),
					UserID:    tiger,
				},
			},
		}, want{[]codersdk.DAUEntry{
			{
				Date:   date(2022, 1, 1),
				Amount: 2,
			},
			{
				Date:   date(2022, 1, 2),
				Amount: 0,
			},
			{
				Date:   date(2022, 1, 3),
				Amount: 0,
			},
			{
				Date:   date(2022, 1, 4),
				Amount: 1,
			},
			{
				Date:   date(2022, 1, 5),
				Amount: 0,
			},
			{
				Date:   date(2022, 1, 6),
				Amount: 0,
			},
			{
				Date:   date(2022, 1, 7),
				Amount: 2,
			},
		}, 2}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				db    = dbfake.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), testutil.IntervalFast)
			)

			defer cache.Close()

			template := dbgen.Template(t, db, database.Template{
				Provisioner: database.ProvisionerTypeEcho,
			})

			gotUniqueUsers, ok := cache.TemplateUniqueUsers(template.ID)
			require.False(t, ok, "template shouldn't have loaded yet")
			require.EqualValues(t, -1, gotUniqueUsers)

			for _, row := range tt.args.rows {
				row.TemplateID = template.ID
				db.InsertAgentStat(context.Background(), row)
			}

			require.Eventuallyf(t, func() bool {
				_, ok := cache.TemplateDAUs(template.ID)
				return ok
			}, testutil.WaitShort, testutil.IntervalMedium,
				"TemplateDAUs never populated",
			)

			gotUniqueUsers, ok = cache.TemplateUniqueUsers(template.ID)
			require.True(t, ok)

			gotEntries, ok := cache.TemplateDAUs(template.ID)
			require.True(t, ok)
			require.Equal(t, tt.want.entries, gotEntries.Entries)
			require.Equal(t, tt.want.uniqueUsers, gotUniqueUsers)
		})
	}
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
				db    = dbfake.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), testutil.IntervalFast)
			)

			defer cache.Close()

			template, err := db.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:          uuid.New(),
				Provisioner: database.ProvisionerTypeEcho,
			})
			require.NoError(t, err)

			templateVersion, err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID:         uuid.New(),
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

				_, err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
					TemplateVersionID: templateVersion.ID,
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
