package dbmetrics_test

import (
	"bytes"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmetrics"
	"github.com/coder/coder/v2/testutil"
)

func TestInTxMetrics(t *testing.T) {
	t.Parallel()

	successLabels := prometheus.Labels{
		"success": "true",
		"tx_id":   "unlabeled",
	}
	const inTxHistMetricName = "coderd_db_tx_duration_seconds"
	const inTxCountMetricName = "coderd_db_tx_executions_count"
	t.Run("QueryMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewQueryMetrics(db, testutil.Logger(t), reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxHistMetricName, successLabels)
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

	t.Run("DBMetrics", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		reg := prometheus.NewRegistry()
		db = dbmetrics.NewDBMetrics(db, testutil.Logger(t), reg)

		err := db.InTx(func(s database.Store) error {
			return nil
		}, nil)
		require.NoError(t, err)

		// Check that the metrics are registered
		inTxMetric := promhelp.HistogramValue(t, reg, inTxHistMetricName, successLabels)
		require.NotNil(t, inTxMetric)
		require.Equal(t, uint64(1), inTxMetric.GetSampleCount())
	})

	// Test log output and metrics on failures
	// Log example:
	//  [erro]  database transaction hit serialization error and had to retry  success=false  executions=2  id=foobar_factory
	t.Run("SerializationError", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&output))

		reg := prometheus.NewRegistry()
		db := dbmetrics.NewDBMetrics(dbmem.New(), logger, reg)
		const id = "foobar_factory"

		txOpts := database.DefaultTXOptions().WithID(id)
		database.IncrementExecutionCount(txOpts) // 2 executions

		err := db.InTx(func(s database.Store) error {
			return xerrors.Errorf("some dumb error")
		}, txOpts)
		require.Error(t, err)

		// Check that the metrics are registered
		inTxHistMetric := promhelp.HistogramValue(t, reg, inTxHistMetricName, prometheus.Labels{
			"success": "false",
			"tx_id":   id,
		})
		require.NotNil(t, inTxHistMetric)
		require.Equal(t, uint64(1), inTxHistMetric.GetSampleCount())

		inTxCountMetric := promhelp.CounterValue(t, reg, inTxCountMetricName, prometheus.Labels{
			"success": "false",
			"retries": "1",
			"tx_id":   id,
		})
		require.NotNil(t, inTxCountMetric)
		require.Equal(t, 1, inTxCountMetric)

		// Also check the logs
		require.Contains(t, output.String(), "some dumb error")
		require.Contains(t, output.String(), "database transaction hit serialization error and had to retry")
		require.Contains(t, output.String(), "success=false")
		require.Contains(t, output.String(), "executions=2")
		require.Contains(t, output.String(), "id="+id)
	})
}
