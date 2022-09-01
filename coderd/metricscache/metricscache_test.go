package metricscache_test

import (
	"context"
	"reflect"
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
		zebra = uuid.New()
		tiger = uuid.New()
	)

	type args struct {
		rows []database.InsertAgentStatParams
	}
	tests := []struct {
		name string
		args args
		want []codersdk.DAUEntry
	}{
		{"empty", args{}, nil},
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
		}, []codersdk.DAUEntry{
			{
				Date: date(2022, 8, 27),
				DAUs: 1,
			},
			{
				Date: date(2022, 8, 28),
				DAUs: 0,
			},
			{
				Date: date(2022, 8, 29),
				DAUs: 0,
			},
			{
				Date: date(2022, 8, 30),
				DAUs: 1,
			},
		}},
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
		}, []codersdk.DAUEntry{
			{
				Date: date(2022, 8, 27),
				DAUs: 1,
			},
			{
				Date: date(2022, 8, 28),
				DAUs: 1,
			},
			{
				Date: date(2022, 8, 29),
				DAUs: 1,
			},
		}},
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
		}, []codersdk.DAUEntry{
			{
				Date: date(2022, 1, 1),
				DAUs: 2,
			},
			{
				Date: date(2022, 1, 2),
				DAUs: 0,
			},
			{
				Date: date(2022, 1, 3),
				DAUs: 0,
			},
			{
				Date: date(2022, 1, 4),
				DAUs: 1,
			},
			{
				Date: date(2022, 1, 5),
				DAUs: 0,
			},
			{
				Date: date(2022, 1, 6),
				DAUs: 0,
			},
			{
				Date: date(2022, 1, 7),
				DAUs: 2,
			},
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				db    = databasefake.New()
				cache = metricscache.New(db, slogtest.Make(t, nil), time.Millisecond*100)
			)

			defer cache.Close()

			for _, row := range tt.args.rows {
				db.InsertAgentStat(context.Background(), row)
			}

			var got codersdk.DAUsResponse

			require.Eventuallyf(t, func() bool {
				got = cache.DAUs()
				return reflect.DeepEqual(got.Entries, tt.want)
			}, testutil.WaitShort, testutil.IntervalFast,
				"GetDAUs() = %v, want %v", got, tt.want,
			)
		})
	}
}
