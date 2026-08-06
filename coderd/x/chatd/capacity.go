package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database"
)

// capacityMetrics observes the concurrent-agent limiter from the worker,
// which sees refusals and admissions. Constructed only when an admission
// factory is configured.
type capacityMetrics struct {
	active      *prometheus.GaugeVec
	queued      *prometheus.GaugeVec
	waitSeconds prometheus.Histogram
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
		waitSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "agent_capacity_wait_seconds",
			Help:      "Time chats spent waiting for a concurrent-agent capacity slot, measured from this replica's first observed refusal to admission.",
			Buckets:   []float64{1, 5, 15, 60, 300, 900, 3600},
		}),
	}
	registerer.MustRegister(m.active, m.queued, m.waitSeconds)
	return m
}

// enterCapacityQueue records a capacity refusal and reports whether the chat
// was newly queued on this replica. Only the first local entry publishes the
// queued event; other replicas refusing the chat publish their own.
func (w *chatWorker) enterCapacityQueue(ctx context.Context, chatID uuid.UUID) bool {
	w.capacityMu.Lock()
	_, exists := w.capacityQueue[chatID]
	if !exists {
		w.capacityQueue[chatID] = w.opts.Clock.Now()
	}
	w.capacityMu.Unlock()
	if exists {
		return false
	}
	w.publishCapacityChange(ctx, chatID, true)
	return true
}

func (w *chatWorker) dropCapacityQueue(chatID uuid.UUID) (time.Time, bool) {
	w.capacityMu.Lock()
	defer w.capacityMu.Unlock()
	first, ok := w.capacityQueue[chatID]
	if ok {
		delete(w.capacityQueue, chatID)
	}
	return first, ok
}

// pruneCapacityQueue drops entries for chats that stopped being acquisition
// candidates without a local admission: archived, deleted, owned by another
// replica, or no longer runnable. seen must cover a complete candidate scan.
func (w *chatWorker) pruneCapacityQueue(seen map[uuid.UUID]struct{}) {
	w.capacityMu.Lock()
	defer w.capacityMu.Unlock()
	for id := range w.capacityQueue {
		if _, ok := seen[id]; !ok {
			delete(w.capacityQueue, id)
		}
	}
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
	unowned, err := w.opts.Store.CountChatCapacityUnownedByPool(ctx, w.opts.HeartbeatStaleSeconds)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker count unowned capacity chats failed", slogError(err))
		}
		return
	}
	metrics := w.opts.CapacityMetrics
	metrics.active.WithLabelValues("root").Set(float64(active.RootCount))
	metrics.active.WithLabelValues("subagent").Set(float64(active.SubagentCount))
	// Unowned running chats count as queued only when their pool is full;
	// otherwise they are ordinary pickups the next acquisition pass owns.
	limits := w.opts.AgentCapacityPolicy.CurrentLimits()
	var queuedRoot, queuedSubagent int64
	if limits.Capped && active.RootCount >= limits.Root {
		queuedRoot = unowned.RootCount
	}
	if limits.Capped && active.SubagentCount >= limits.Subagent {
		queuedSubagent = unowned.SubagentCount
	}
	metrics.queued.WithLabelValues("root").Set(float64(queuedRoot))
	metrics.queued.WithLabelValues("subagent").Set(float64(queuedSubagent))
}
