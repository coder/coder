package dbmetrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmetrics"
)

func TestInTxMetrics(t *testing.T) {
	t.Parallel()

	successLabels := prometheus.Labels{
		"success":    "true",
		"executions": "1",
		"id":         "",
	}
	const inTxMetricName = "coderd_db_tx_duration_seconds"
	t.Run("QueryMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewQueryMetrics(db, slogtest.Make(t, nil), reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxMetricName, successLabels)
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

	t.Run("DBMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewDBMetrics(db, slogtest.Make(t, nil), reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxMetricName, successLabels)
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

}
