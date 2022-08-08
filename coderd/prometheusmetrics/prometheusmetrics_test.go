package prometheusmetrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/testutil"
)

func TestActiveUsers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Database func() database.Store
		Count    int
	}{{
		Name: "None",
		Database: func() database.Store {
			return databasefake.New()
		},
		Count: 0,
	}, {
		Name: "One",
		Database: func() database.Store {
			db := databasefake.New()
			_, _ = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
				UserID:   uuid.New(),
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "OneWithExpired",
		Database: func() database.Store {
			db := databasefake.New()
			_, _ = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
				UserID:   uuid.New(),
				LastUsed: database.Now(),
			})
			// Because this API key hasn't been used in the past hour, this shouldn't
			// add to the user count.
			_, _ = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
				UserID:   uuid.New(),
				LastUsed: database.Now().Add(-2 * time.Hour),
			})
			return db
		},
		Count: 1,
	}, {
		Name: "Multiple",
		Database: func() database.Store {
			db := databasefake.New()
			_, _ = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
				UserID:   uuid.New(),
				LastUsed: database.Now(),
			})
			_, _ = db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
				UserID:   uuid.New(),
				LastUsed: database.Now(),
			})
			return db
		},
		Count: 2,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			cancel, err := prometheusmetrics.ActiveUsers(context.Background(), registry, tc.Database(), time.Millisecond)
			require.NoError(t, err)
			t.Cleanup(cancel)

			var result int
			require.Eventually(t, func() bool {
				metrics, err := registry.Gather()
				assert.NoError(t, err)
				result = int(*metrics[0].Metric[0].Gauge.Value)
				return result == tc.Count
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}
