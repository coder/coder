package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
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

// noteCapacityRefused remembers when a chat was first refused a
// capacity slot. Only the acquisition loop touches the map, so it
// needs no lock.
func (w *chatWorker) noteCapacityRefused(chatID uuid.UUID) {
	if _, ok := w.capacityWaitSince[chatID]; ok {
		return
	}
	w.capacityWaitSince[chatID] = time.Now()
}

// recordCapacityWait emits the capacity_wait stage for a chat that is
// being acquired after at least one capacity refusal, measured from
// the first refusal this worker saw. Chats admitted on their first
// attempt record nothing. The acquisition pass runs before the turn
// span exists, so the turn scope is stated explicitly.
func (w *chatWorker) recordCapacityWait(ctx context.Context, chat database.Chat) {
	since, waited := w.capacityWaitSince[chat.ID]
	if !waited {
		return
	}
	delete(w.capacityWaitSince, chat.ID)
	w.server.stages.RecordAs(ctx, chatloop.StageCapacityWait, chatloop.ScopeTurn, chatloop.StageModel{},
		since, time.Now(), nil,
		attribute.String(chatloop.AttrChatID, chat.ID.String()),
		attribute.String(chatloop.AttrChatKind, chatKindAttr(chat)),
	)
}

// pruneCapacityWaits drops wait starts for chats that are no longer
// acquisition candidates, which happens when they are archived,
// deleted, or picked up by another worker.
func (w *chatWorker) pruneCapacityWaits(candidates []database.GetChatWorkerAcquisitionCandidatesRow) {
	if len(w.capacityWaitSince) == 0 {
		return
	}
	stillCandidate := make(map[uuid.UUID]struct{}, len(candidates))
	for _, row := range candidates {
		stillCandidate[row.ID] = struct{}{}
	}
	for chatID := range w.capacityWaitSince {
		if _, ok := stillCandidate[chatID]; !ok {
			delete(w.capacityWaitSince, chatID)
		}
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
