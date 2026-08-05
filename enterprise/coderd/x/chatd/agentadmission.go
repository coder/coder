package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entitlements"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// Root and subagent chats use separate pools so a waiting root keeps its slot
// without starving children. Deployments with remaining licensed agent runtime
// hours bypass both caps. Workspace agents use neither pool.
const (
	maxConcurrentRootAgents = int64(5)
	maxConcurrentSubagents  = int64(10)
)

const defaultMetricsInterval = 30 * time.Second

// NewAgentAdmissionFactory builds a gate that evaluates current entitlements
// and pool capacity for each acquisition.
func NewAgentAdmissionFactory(set *entitlements.Set) osschatd.AgentAdmissionFactory {
	return func(opts osschatd.AgentAdmissionOptions) osschatd.AgentAdmission {
		return newAdmission(admissionOptions{
			Entitlements:          set,
			Store:                 opts.Store,
			Logger:                opts.Logger,
			Clock:                 opts.Clock,
			Registerer:            opts.Registerer,
			LifetimeCtx:           opts.LifetimeCtx,
			HeartbeatStaleSeconds: opts.HeartbeatStaleSeconds,
		})
	}
}

type admissionOptions struct {
	Entitlements          *entitlements.Set
	Store                 database.Store
	Logger                slog.Logger
	Clock                 quartz.Clock
	Registerer            prometheus.Registerer
	LifetimeCtx           context.Context
	HeartbeatStaleSeconds int32

	RootCapacity     int64
	SubagentCapacity int64
	MetricsInterval  time.Duration
}

// The admission lock remains held through ownership acquisition, serializing
// pool counts across replicas.
type admission struct {
	entitlements     *entitlements.Set
	logger           slog.Logger
	clock            quartz.Clock
	staleSeconds     int32
	rootCapacity     int64
	subagentCapacity int64
	metricsInterval  time.Duration

	activeGauge *prometheus.GaugeVec
	queuedGauge *prometheus.GaugeVec
	queueTotal  prometheus.Counter
	waitSeconds prometheus.Histogram
}

func newAdmission(opts admissionOptions) *admission {
	if opts.Entitlements == nil {
		opts.Entitlements = entitlements.New()
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.RootCapacity <= 0 {
		opts.RootCapacity = maxConcurrentRootAgents
	}
	if opts.SubagentCapacity <= 0 {
		opts.SubagentCapacity = maxConcurrentSubagents
	}
	if opts.MetricsInterval <= 0 {
		opts.MetricsInterval = defaultMetricsInterval
	}
	a := &admission{
		entitlements:     opts.Entitlements,
		logger:           opts.Logger,
		clock:            opts.Clock,
		staleSeconds:     opts.HeartbeatStaleSeconds,
		rootCapacity:     opts.RootCapacity,
		subagentCapacity: opts.SubagentCapacity,
		metricsInterval:  opts.MetricsInterval,
	}
	a.activeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agents_active",
		Help:      "Deployment-wide number of chats holding a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
	}, []string{"pool"})
	a.queuedGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agents_queued_for_capacity",
		Help:      "Deployment-wide number of chats waiting for a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
	}, []string{"pool"})
	a.queueTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agent_capacity_queue_total",
		Help:      "Total number of times a chat on this replica entered the concurrent-agent capacity queue.",
	})
	a.waitSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agent_capacity_wait_seconds",
		Help:      "Time chats spent queued for a concurrent-agent capacity slot, observed at admission.",
		Buckets:   []float64{1, 5, 15, 60, 300, 900, 3600},
	})
	if opts.Registerer != nil {
		opts.Registerer.MustRegister(a.activeGauge, a.queuedGauge, a.queueTotal, a.waitSeconds)
		if opts.LifetimeCtx != nil {
			go a.refreshGauges(opts.LifetimeCtx, opts.Store)
		}
	}
	return a
}

// Admit applies capacity only to running chats. Interrupting chats remain
// acquirable for stop requests, while requires_action chats are idle.
func (a *admission) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if a.uncapped() {
		a.observeAdmission(chat)
		return true, nil
	}
	if chat.Status != database.ChatStatusRunning {
		a.observeAdmission(chat)
		return true, nil
	}
	if err := store.AcquireLock(ctx, database.LockIDChatCapacityAdmission); err != nil {
		return false, err
	}
	counts, err := store.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{
		ExcludeChatID: chat.ID,
		StaleSeconds:  a.staleSeconds,
	})
	if err != nil {
		return false, err
	}
	used, capacity := counts.RootCount, a.rootCapacity
	if chat.ParentChatID.Valid {
		used, capacity = counts.SubagentCount, a.subagentCapacity
	}
	if used >= capacity {
		if !chat.CapacityQueuedAt.Valid {
			a.queueTotal.Inc()
		}
		return false, nil
	}
	a.observeAdmission(chat)
	return true, nil
}

// uncapped returns true for an enabled entitlement with remaining hours.
// Missing limit or usage data also fails open to avoid capping on incomplete
// entitlement data. Entitlements do not yet populate Actual for this feature
// (usage accrues externally via usage events), so today every enabled
// runtime-hours license is uncapped; the exhaustion branch activates when
// usage wiring lands.
func (a *admission) uncapped() bool {
	f, ok := a.entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || !f.Enabled {
		return false
	}
	if f.Limit == nil || f.Actual == nil {
		return true
	}
	return *f.Actual < *f.Limit
}

func (a *admission) observeAdmission(chat database.Chat) {
	if !chat.CapacityQueuedAt.Valid {
		return
	}
	// The database sets queue time, so clamp negative clock skew.
	a.waitSeconds.Observe(max(0, a.clock.Since(chat.CapacityQueuedAt.Time).Seconds()))
}

func (a *admission) refreshGauges(ctx context.Context, store database.Store) {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	ticker := a.clock.NewTicker(a.metricsInterval, "chatd", "capacity_metrics")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		active, err := store.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{
			ExcludeChatID: uuid.Nil,
			StaleSeconds:  a.staleSeconds,
		})
		if err != nil {
			a.logger.Warn(ctx, "count active capacity chats", slog.Error(err))
			continue
		}
		queued, err := store.CountChatCapacityQueuedByPool(ctx)
		if err != nil {
			a.logger.Warn(ctx, "count queued capacity chats", slog.Error(err))
			continue
		}
		a.activeGauge.WithLabelValues("root").Set(float64(active.RootCount))
		a.activeGauge.WithLabelValues("subagent").Set(float64(active.SubagentCount))
		a.queuedGauge.WithLabelValues("root").Set(float64(queued.RootCount))
		a.queuedGauge.WithLabelValues("subagent").Set(float64(queued.SubagentCount))
	}
}
