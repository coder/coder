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
	w.capacityMu.Unlock()
	if exists {
		return false
	}
	// A candidate can become owned after the batch query. Validate and
	// publish one snapshot, so a newly owned chat never publishes and a
	// concurrent acquisition's event carries the pre-acquisition updated_at.
	chat, err := w.opts.Store.GetChatByID(ctx, chatID)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker load chat for capacity queue failed", slogError(err))
		}
		return false
	}
	if chat.Archived || chat.Status != database.ChatStatusRunning {
		return false
	}
	if chat.WorkerID.Valid && chat.RunnerID.Valid {
		stale, err := w.opts.Store.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
			ChatID:       chat.ID,
			RunnerID:     chat.RunnerID.UUID,
			StaleSeconds: w.opts.HeartbeatStaleSeconds,
		})
		if err != nil {
			if ctx.Err() == nil {
				w.opts.Logger.Warn(ctx, "chatworker check heartbeat for capacity queue failed", slogError(err))
			}
			return false
		}
		if !stale {
			return false
		}
	}
	w.capacityMu.Lock()
	w.capacityQueue[chatID] = w.opts.Clock.Now()
	w.capacityMu.Unlock()
	w.server.publishChatCapacityChange(chat, true)
	return true
}

// reconcileCapacityQueue reconciles the local queue against one listing of
// capacity-wait candidates: entries absent from the listing departed, and
// candidates in pools this pass proved full enter the queue.
func (w *chatWorker) reconcileCapacityQueue(ctx context.Context, refusedPools map[bool]bool) {
	waiting, err := w.opts.Store.ListChatCapacityWaiting(ctx, w.opts.HeartbeatStaleSeconds)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker reconcile capacity queue failed", slogError(err))
		}
		return
	}
	waitingIDs := make(map[uuid.UUID]struct{}, len(waiting))
	for _, row := range waiting {
		waitingIDs[row.ID] = struct{}{}
	}
	arrivals := make([]uuid.UUID, 0)
	w.capacityMu.Lock()
	for id := range w.capacityQueue {
		if _, ok := waitingIDs[id]; !ok {
			delete(w.capacityQueue, id)
		}
	}
	for _, row := range waiting {
		if _, ok := w.capacityQueue[row.ID]; ok {
			continue
		}
		if refusedPools[row.Subagent] {
			arrivals = append(arrivals, row.ID)
		}
	}
	w.capacityMu.Unlock()
	for _, id := range arrivals {
		w.enterCapacityQueue(ctx, id)
	}
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

// The noop limiter never refuses, so no queued event can exist and clears
// would be pure noise.
func (w *chatWorker) capacityEventsEnabled() bool {
	_, noop := w.opts.AgentCapacityLimiter.(noopAgentCapacityLimiter)
	return !noop
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
	counts, err := w.opts.Store.CountChatCapacityByPool(ctx, database.CountChatCapacityByPoolParams{
		ExcludeChatID: uuid.Nil,
		StaleSeconds:  w.opts.HeartbeatStaleSeconds,
	})
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker count capacity chats failed", slogError(err))
		}
		return
	}
	metrics := w.opts.CapacityMetrics
	metrics.active.WithLabelValues("root").Set(float64(counts.ActiveRootCount))
	metrics.active.WithLabelValues("subagent").Set(float64(counts.ActiveSubagentCount))
	// Only unowned running chats in full pools are capacity-queued; the rest
	// remain ordinary worker pickups.
	limits, capped := w.opts.AgentCapacityLimiter.Limits()
	var queuedRoot, queuedSubagent int64
	if capped && counts.ActiveRootCount >= limits.Root {
		queuedRoot = counts.UnownedRootCount
	}
	if capped && counts.ActiveSubagentCount >= limits.Subagent {
		queuedSubagent = counts.UnownedSubagentCount
	}
	metrics.queued.WithLabelValues("root").Set(float64(queuedRoot))
	metrics.queued.WithLabelValues("subagent").Set(float64(queuedSubagent))
}
