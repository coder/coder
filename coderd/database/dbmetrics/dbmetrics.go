package dbmetrics

import (
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"

	"github.com/coder/coder/v2/coderd/database"
)

type metricsStore struct {
	database.Store
	txDuration prometheus.Histogram
}

// NewDBMetrics returns a database.Store that registers metrics for the database
// but does not handle individual queries.
// metricsStore is intended to always be used, because queryMetrics are a bit
// too verbose for many use cases.
func NewDBMetrics(s database.Store, reg prometheus.Registerer) database.Store {
	// Don't double-wrap.
	if slices.Contains(s.Wrappers(), wrapname) {
		return s
	}
	txDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "db",
		Name:      "tx_duration_seconds",
		Help:      "Duration of transactions in seconds.",
		Buckets:   prometheus.DefBuckets,
	})
	reg.MustRegister(txDuration)
	return &metricsStore{
		Store:      s,
		txDuration: txDuration,
	}
}

func (m metricsStore) Wrappers() []string {
	return append(m.Store.Wrappers(), wrapname)
}

func (m metricsStore) InTx(f func(database.Store) error, options *sql.TxOptions) error {
	start := time.Now()
	err := m.Store.InTx(f, options)
	m.txDuration.Observe(time.Since(start).Seconds())
	return err
}
