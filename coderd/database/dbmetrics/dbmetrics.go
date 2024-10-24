package dbmetrics

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
)

type metricsStore struct {
	database.Store
	logger     slog.Logger
	txDuration *prometheus.HistogramVec
}

// NewDBMetrics returns a database.Store that registers metrics for the database
// but does not handle individual queries.
// metricsStore is intended to always be used, because queryMetrics are a bit
// too verbose for many use cases.
func NewDBMetrics(s database.Store, logger slog.Logger, reg prometheus.Registerer) database.Store {
	// Don't double-wrap.
	if slices.Contains(s.Wrappers(), wrapname) {
		return s
	}
	txDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "db",
		Name:      "tx_duration_seconds",
		Help:      "Duration of transactions in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{
		"success", // Did the InTx function return an error?
		// Number of executions, since we have retry logic on serialization errors.
		"executions",
		// Uniquely naming some transactions can help debug reoccurring errors.
		"id",
	})
	reg.MustRegister(txDuration)
	return &metricsStore{
		Store:      s,
		txDuration: txDuration,
		logger:     logger,
	}
}

func (m metricsStore) Wrappers() []string {
	return append(m.Store.Wrappers(), wrapname)
}

func (m metricsStore) InTx(f func(database.Store) error, options *database.TxOptions) error {
	if options == nil {
		options = database.DefaultTXOptions()
	}

	start := time.Now()
	err := m.Store.InTx(f, options)
	dur := time.Since(start)
	// The number of unique label combinations is
	// 2 x 3 (retry count) x #IDs
	// So IDs should be used sparingly to prevent too much bloat.
	m.txDuration.With(prometheus.Labels{
		"success":    strconv.FormatBool(err == nil),
		"executions": strconv.FormatInt(int64(options.ExecutionCount()), 10),
		"id":         options.TxIdentifier, // Can be empty string for unlabeled
	}).Observe(dur.Seconds())

	// Log all serializable transactions that are retried.
	// This is expected to happen in production, but should be kept
	// to a minimum. If these logs happen frequently, something is wrong.
	if options.ExecutionCount() > 1 {
		l := m.logger.Warn
		if err != nil {
			// Error leve if retries were not enough
			l = m.logger.Error
		}
		// No context is present in this function :(
		l(context.Background(), "database transaction hit serialization error and had to retry",
			slog.F("success", err == nil), // It can succeed on retry
			// Note the error might not be a serialization error. It is possible
			// the first error was a serialization error, and the error on the
			// retry is different. If this is the case, we still want to log it
			// since the first error was a serialization error.
			slog.Error(err), // Might be nil, that is ok!
			slog.F("executions", options.ExecutionCount()),
			slog.F("id", options.TxIdentifier),
			slog.F("duration", dur),
		)
	}
	return err
}
