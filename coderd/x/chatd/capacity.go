package chatd

import (
	"context"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database"
)

type capacityMetrics struct {
	active *prometheus.GaugeVec
	queued *prometheus.GaugeVec
}

func newCapacityMetrics(registerer prometheus.Registerer) *capacityMetrics {
	m := &capacityMetrics{
		active: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "agents_active",
			Help:      "Deployment-wide number of chats holding a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
		}, []string{"pool"}),
		queued: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "agents_queued_for_capacity",
			Help:      "Deployment-wide number of chats waiting for a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
		}, []string{"pool"}),
	}
	registerer.MustRegister(m.active, m.queued)
	return m
}

func (w *chatWorker) capacityMetricsLoop(ctx context.Context) {
	ticker := w.opts.Clock.NewTicker(w.opts.CapacityMetricsInterval, "chatworker", "capacity-metrics")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		w.refreshCapacityMetrics(ctx)
	}
}

func (w *chatWorker) refreshCapacityMetrics(ctx context.Context) {
	active, err := w.opts.Store.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{
		ExcludeChatID: uuid.Nil,
		StaleSeconds:  w.opts.HeartbeatStaleSeconds,
	})
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker count active capacity chats failed", slogError(err))
		}
		return
	}

	limits, capped := w.opts.AgentCapacityLimiter.Limits()
	var queuedRoot, queuedSubagent int64
	if capped && (active.ActiveRootCount >= limits.Root || active.ActiveSubagentCount >= limits.Subagent) {
		queued, err := w.opts.Store.CountChatCapacityQueuedByPool(ctx, w.opts.HeartbeatStaleSeconds)
		if err != nil {
			if ctx.Err() == nil {
				w.opts.Logger.Warn(ctx, "chatworker count queued capacity chats failed", slogError(err))
			}
			return
		}
		if active.ActiveRootCount >= limits.Root {
			queuedRoot = queued.QueuedRootCount
		}
		if active.ActiveSubagentCount >= limits.Subagent {
			queuedSubagent = queued.QueuedSubagentCount
		}
	}

	metrics := w.opts.CapacityMetrics
	metrics.active.WithLabelValues("root").Set(float64(active.ActiveRootCount))
	metrics.active.WithLabelValues("subagent").Set(float64(active.ActiveSubagentCount))
	metrics.queued.WithLabelValues("root").Set(float64(queuedRoot))
	metrics.queued.WithLabelValues("subagent").Set(float64(queuedSubagent))
}
