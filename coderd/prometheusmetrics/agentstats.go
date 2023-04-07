package prometheusmetrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
)

func AgentStats(ctx context.Context, logger slog.Logger, registerer prometheus.Registerer, db database.Store, duration time.Duration) (context.CancelFunc, error) {
	if duration == 0 {
		duration = 1 * time.Minute
	}

	metricsCollectorAgentStats := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "prometheusmetrics",
		Name:      "agentstats_execution_seconds",
		Help:      "Histogram for duration of agent stats metrics collection in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.500, 1, 5, 10, 30},
	})
	err := registerer.Register(metricsCollectorAgentStats)
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(dbauthz.AsSystemRestricted(ctx))
	ticker := time.NewTicker(duration)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			logger.Debug(ctx, "Agent metrics collection is starting")
			timer := prometheus.NewTimer(metricsCollectorAgentStats)

			logger.Debug(ctx, "Agent metrics collection is done")
			metricsCollectorAgentStats.Observe(timer.ObserveDuration().Seconds())
		}
	}()
	return cancelFunc, nil

}
