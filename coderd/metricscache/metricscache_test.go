package metricscache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/metricscache"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestCache(t *testing.T) {
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
		{"one hole", args{
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
				db    = databasefake.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), testutil.IntervalFast)
			)

			defer cache.Close()

			templateID := uuid.New()
			db.InsertTemplate(context.Background(), database.InsertTemplateParams{
				ID: templateID,
			})

			gotUniqueUsers, ok := cache.TemplateUniqueUsers(templateID)
			require.False(t, ok, "template shouldn't have loaded yet")
			require.EqualValues(t, -1, gotUniqueUsers)

			for _, row := range tt.args.rows {
				row.TemplateID = templateID
				db.InsertAgentStat(context.Background(), row)
			}

			require.Eventuallyf(t, func() bool {
				_, ok := cache.TemplateDAUs(templateID)
				return ok
			}, testutil.WaitShort, testutil.IntervalMedium,
				"TemplateDAUs never populated",
			)

			gotUniqueUsers, ok = cache.TemplateUniqueUsers(templateID)
			require.True(t, ok)

			gotEntries, ok := cache.TemplateDAUs(templateID)
			require.True(t, ok)
			require.Equal(t, tt.want.entries, gotEntries.Entries)
			require.Equal(t, tt.want.uniqueUsers, gotUniqueUsers)
		})
	}
}
