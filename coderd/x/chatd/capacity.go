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

// capacityWait is the start of one chat's wait for a capacity slot.
// historyVersion and updatedAt are the chat row's values at the first
// refusal. A refused acquisition rolls back without writing the row, so
// a change in either value means the row was written since that
// refusal, for example by a new prompt or by another replica acquiring
// the chat, and the wait no longer describes the current prompt.
type capacityWait struct {
	since          time.Time
	historyVersion int64
	updatedAt      time.Time
}

// sameRow reports whether the chat row is unchanged since the refusal
// that started w.
func (w capacityWait) sameRow(historyVersion int64, updatedAt time.Time) bool {
	return w.historyVersion == historyVersion && w.updatedAt.Equal(updatedAt)
}

// noteCapacityRefused remembers when a chat was first refused a
// capacity slot. Later refusals of an unchanged row keep the first
// time; a refusal after the row changed restarts the wait.
func (w *chatWorker) noteCapacityRefused(chatID uuid.UUID, historyVersion int64, updatedAt time.Time) {
	if wait, ok := w.capacityWaits[chatID]; ok && wait.sameRow(historyVersion, updatedAt) {
		return
	}
	w.capacityWaits[chatID] = capacityWait{
		since:          w.opts.Clock.Now(),
		historyVersion: historyVersion,
		updatedAt:      updatedAt,
	}
}

// recordCapacityWait emits the capacity_wait stage for a chat that was
// acquired after at least one capacity refusal, measured from the first
// refusal this worker saw. chat is the row as loaded before acquisition.
// Nothing is recorded for a chat admitted on its first attempt, a chat
// that is no longer running, or a chat whose row changed since the
// refusal. No turn span exists at this point, so the turn scope, the
// chat kind, and the organization are stated explicitly.
func (w *chatWorker) recordCapacityWait(ctx context.Context, chat database.Chat) {
	wait, waited := w.capacityWaits[chat.ID]
	if !waited {
		return
	}
	delete(w.capacityWaits, chat.ID)
	if chat.Status != database.ChatStatusRunning || !wait.sameRow(chat.HistoryVersion, chat.UpdatedAt) {
		return
	}
	ctx = chatloop.ContextWithChatKind(ctx, chatKind(chat))
	ctx = chatloop.ContextWithOrganization(ctx, w.server.organizationName(ctx, chat.OrganizationID))
	w.server.stages.RecordAs(ctx, chatloop.StageCapacityWait, chatloop.ScopeTurn, chatloop.StageModel{},
		wait.since, w.opts.Clock.Now(), nil,
		attribute.String(chatloop.AttrChatID, chat.ID.String()),
	)
}

// forgetCapacityWait drops a chat's wait start. A wait that resumes
// later is measured from the next refusal.
func (w *chatWorker) forgetCapacityWait(chatID uuid.UUID) {
	delete(w.capacityWaits, chatID)
}

// pruneCapacityWaits drops wait starts for chats absent from
// candidates. candidates must be the complete candidate set: a chat
// missing from a truncated batch is still waiting, and dropping it
// would restart its clock.
func (w *chatWorker) pruneCapacityWaits(candidates []database.GetChatWorkerAcquisitionCandidatesRow) {
	if len(w.capacityWaits) == 0 {
		return
	}
	stillCandidate := make(map[uuid.UUID]struct{}, len(candidates))
	for _, row := range candidates {
		stillCandidate[row.ID] = struct{}{}
	}
	for chatID := range w.capacityWaits {
		if _, ok := stillCandidate[chatID]; !ok {
			delete(w.capacityWaits, chatID)
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
	metrics.active.WithLabelValues(string(chatloop.ChatKindRoot)).Set(float64(active.ActiveRootCount))
	metrics.active.WithLabelValues(string(chatloop.ChatKindSubagent)).Set(float64(active.ActiveSubagentCount))
	metrics.queued.WithLabelValues(string(chatloop.ChatKindRoot)).Set(float64(queuedRoot))
	metrics.queued.WithLabelValues(string(chatloop.ChatKindSubagent)).Set(float64(queuedSubagent))
}
