package dbmetrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmetrics"
)

func TestInTxMetrics(t *testing.T) {
	t.Parallel()

	const inTxMetricName = "coderd_db_tx_duration_seconds"
	t.Run("QueryMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewQueryMetrics(db, reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxMetricName, prometheus.Labels{})
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

	t.Run("DBMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewDBMetrics(db, reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxMetricName, prometheus.Labels{})
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

}
