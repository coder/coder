package chatd

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/entitlements"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// MaxConcurrentAgents is the deployment-wide cap on concurrently
// executing chatd agentic loops when the deployment is not entitled to
// codersdk.FeatureAgentRuntimeHours. A chat holds one slot while its
// status is running or interrupting; chats over the limit queue until
// a slot frees. Subagents are ordinary chats and count against the
// cap; a parent yields its slot while blocked in wait_agent so
// children can run. "Agents" here means chatd agentic loops, not
// workspace agents.
const MaxConcurrentAgents = 5

// defaultCapacityPollInterval bounds how long a queued chat waits
// before re-running its claim without a pubsub nudge. It covers
// missed at-most-once pubsub deliveries, entitlement changes, and
// capacity freed by paths that do not publish (archival, deletion).
const defaultCapacityPollInterval = 15 * time.Second

// defaultMetricsInterval is how often the deployment-wide capacity
// gauges are refreshed from the database.
const defaultMetricsInterval = 30 * time.Second

// NewAgentConcurrencyGateFactory returns the factory the chat worker
// uses to construct its concurrent-agent gate. The set is consulted at
// every claim attempt, so entitlement changes apply without
// reconstruction.
func NewAgentConcurrencyGateFactory(set *entitlements.Set) osschatd.AgentConcurrencyGateFactory {
	return func(opts osschatd.AgentConcurrencyGateOptions) osschatd.AgentConcurrencyGate {
		return newGate(gateOptions{
			Entitlements: set,
			Store:        opts.Store,
			Pubsub:       opts.Pubsub,
			Clock:        opts.Clock,
			Logger:       opts.Logger,
			Registerer:   opts.Registerer,
			OnQueued:     opts.OnQueued,
			OnAdmitted:   opts.OnAdmitted,
			LifetimeCtx:  opts.LifetimeCtx,
		})
	}
}

type gateOptions struct {
	// Entitlements is consulted at every claim attempt; nil defaults
	// to an unlicensed set, so the cap applies.
	Entitlements *entitlements.Set
	Store        database.Store
	Pubsub       pubsub.Pubsub
	Clock        quartz.Clock
	Logger       slog.Logger
	// Registerer registers the gate metrics when non-nil.
	Registerer prometheus.Registerer
	// OnQueued and OnAdmitted observe a chat entering the capacity
	// queue and leaving it. The chat row reflects the committed state.
	OnQueued   func(chat database.Chat)
	OnAdmitted func(chat database.Chat)
	// LifetimeCtx bounds the background metrics refresher. Nil
	// disables it.
	LifetimeCtx context.Context

	// Capacity overrides MaxConcurrentAgents; used by tests.
	Capacity int64
	// PollInterval overrides the queued-chat fallback poll cadence;
	// used by tests.
	PollInterval time.Duration
}

// gate admits chatd agentic loops against a deployment-wide capacity
// count stored on the chats table. Claims from every replica
// serialize on a transaction-scoped advisory lock, so the cap holds
// across replicas. Capacity release is implicit: the status-update
// SQL clears a chat's claim whenever it leaves running/interrupting,
// so the gate has no release bookkeeping and cannot leak slots.
type gate struct {
	entitlements *entitlements.Set
	store        database.Store
	pubsub       pubsub.Pubsub
	clock        quartz.Clock
	logger       slog.Logger
	onQueued     func(chat database.Chat)
	onAdmitted   func(chat database.Chat)
	capacity     int64
	pollInterval time.Duration

	activeGauge prometheus.Gauge
	queuedGauge prometheus.Gauge
	queueTotal  prometheus.Counter
	waitSeconds prometheus.Histogram
	metricsOn   bool
}

func newGate(opts gateOptions) *gate {
	if opts.Entitlements == nil {
		opts.Entitlements = entitlements.New()
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Capacity <= 0 {
		opts.Capacity = MaxConcurrentAgents
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultCapacityPollInterval
	}
	g := &gate{
		entitlements: opts.Entitlements,
		store:        opts.Store,
		pubsub:       opts.Pubsub,
		clock:        opts.Clock,
		logger:       opts.Logger,
		onQueued:     opts.OnQueued,
		onAdmitted:   opts.OnAdmitted,
		capacity:     opts.Capacity,
		pollInterval: opts.PollInterval,
	}
	g.activeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agents_active",
		Help:      "Deployment-wide number of chats holding a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
	})
	g.queuedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agents_queued_for_capacity",
		Help:      "Deployment-wide number of chats waiting for a concurrent-agent capacity slot. Every replica reports the same database-derived value; aggregate with max, not sum.",
	})
	g.queueTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agent_capacity_queue_total",
		Help:      "Total number of times a chat on this replica entered the concurrent-agent capacity queue.",
	})
	g.waitSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "chatd",
		Name:      "agent_capacity_wait_seconds",
		Help:      "Time chats spent queued for a concurrent-agent capacity slot, observed at admission.",
		Buckets:   []float64{1, 5, 15, 60, 300, 900, 3600},
	})
	if opts.Registerer != nil {
		opts.Registerer.MustRegister(g.activeGauge, g.queuedGauge, g.queueTotal, g.waitSeconds)
		g.metricsOn = true
	}
	if opts.LifetimeCtx != nil && g.metricsOn {
		go g.refreshGauges(opts.LifetimeCtx)
	}
	return g
}

func (g *gate) entitled() bool {
	return g.entitlements.Enabled(codersdk.FeatureAgentRuntimeHours)
}

// Acquire blocks until the chat holds a capacity slot or ctx is
// canceled. It is idempotent: a chat already holding a slot is
// re-admitted immediately. Transient database errors are retried on
// the poll cadence rather than surfaced, so the only error returned
// is ctx.Err().
func (g *gate) Acquire(ctx context.Context, chatID uuid.UUID) error {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if g.entitled() {
		return nil
	}
	if admitted := g.claimOnce(ctx, chatID); admitted {
		return nil
	}

	// Subscribe before re-checking so a capacity nudge landing between
	// the claim attempt and the wait is never lost.
	nudges := make(chan struct{}, 1)
	unsubscribe, err := g.pubsub.Subscribe(coderdpubsub.ChatCapacityChannel, func(_ context.Context, _ []byte) {
		select {
		case nudges <- struct{}{}:
		default:
		}
	})
	if err != nil {
		g.logger.Warn(ctx, "chat concurrency capacity subscribe failed; falling back to polling", slog.F("chat_id", chatID), slog.Error(err))
		nudges = nil
	} else {
		defer unsubscribe()
	}

	for {
		if g.entitled() {
			g.releaseQueuedMarker(ctx, chatID)
			return nil
		}
		if admitted := g.claimOnce(ctx, chatID); admitted {
			return nil
		}

		timer := g.clock.NewTimer(jitter(g.pollInterval), "chatd", "capacity_poll")
		select {
		case <-nudges:
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		timer.Stop()
	}
}

// claimOnce runs one claim attempt, records queue metrics, and
// invokes the queue/admission observers. Errors are logged and
// treated as a failed attempt so the caller's wait loop retries.
func (g *gate) claimOnce(ctx context.Context, chatID uuid.UUID) bool {
	res, err := g.tryClaim(ctx, chatID)
	if err != nil {
		g.logger.Warn(ctx, "chat concurrency claim failed; retrying", slog.F("chat_id", chatID), slog.Error(err))
		return false
	}
	if res.admitted {
		g.finishAdmission(res)
		return true
	}
	if res.justQueued {
		g.queueTotal.Inc()
		if g.onQueued != nil {
			g.onQueued(res.chat)
		}
	}
	return false
}

// Yield releases the chat's capacity slot while its runner blocks on
// external completion (wait_agent), so subagent children can run.
// Resuming is a plain Acquire.
func (g *gate) Yield(ctx context.Context, chatID uuid.UUID) error {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if g.entitled() {
		return nil
	}
	_, err := g.store.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chatID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateYielded,
			Valid:                true,
		},
	})
	if errors.Is(err, sql.ErrNoRows) {
		// The chat already left the counted statuses; nothing to free.
		return nil
	}
	if err != nil {
		return err
	}
	if err := g.pubsub.Publish(coderdpubsub.ChatCapacityChannel, []byte("{}")); err != nil {
		g.logger.Warn(ctx, "publish capacity nudge after yield", slog.F("chat_id", chatID), slog.Error(err))
	}
	return nil
}

type claimResult struct {
	admitted bool
	// justQueued is true when this attempt newly wrote the queued
	// marker (as opposed to finding it already set).
	justQueued bool
	// queuedSince carries the queue entry time for wait metrics when
	// an admitted chat had been queued.
	queuedSince time.Time
	wasQueued   bool
	chat        database.Chat
}

// tryClaim runs one serialized claim attempt. All replicas' claims
// serialize on the advisory lock, so the count-then-write sequence
// cannot over-admit.
func (g *gate) tryClaim(ctx context.Context, chatID uuid.UUID) (claimResult, error) {
	var res claimResult
	err := g.store.InTx(func(tx database.Store) error {
		res = claimResult{}
		if err := tx.AcquireLock(ctx, database.LockIDChatConcurrency); err != nil {
			return err
		}
		chat, err := tx.GetChatByID(ctx, chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Deleted chat: admit without a claim; the task is
				// exiting anyway and nothing is counted.
				res.admitted = true
				return nil
			}
			return err
		}
		if chat.Archived || (chat.Status != database.ChatStatusRunning && chat.Status != database.ChatStatusInterrupting) {
			// The chat left the counted statuses; a marker written now
			// would go stale. Admit without a claim.
			res.admitted = true
			return nil
		}
		if chat.ConcurrencyState.Valid && chat.ConcurrencyState.ChatConcurrencyState == database.ChatConcurrencyStateActive {
			res.admitted = true
			return nil
		}
		res.wasQueued = chat.ConcurrencyState.Valid && chat.ConcurrencyState.ChatConcurrencyState == database.ChatConcurrencyStateQueued
		if res.wasQueued {
			res.queuedSince = chat.ConcurrencyQueuedAt.Time
		}

		active, err := tx.CountActiveConcurrencyChats(ctx)
		if err != nil {
			return err
		}
		free := g.capacity - active
		if free > 0 {
			oldestQueued, err := tx.GetOldestQueuedConcurrencyChats(ctx, free)
			if err != nil {
				return err
			}
			// Oldest-first fairness: a queued chat is admitted when it
			// is within the free slots' head of the queue; a chat that
			// never queued only claims a slot the queue head leaves
			// over.
			admit := slices.Contains(oldestQueued, chatID) ||
				(!res.wasQueued && int64(len(oldestQueued)) < free)
			if admit {
				updated, err := tx.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
					ID: chatID,
					ConcurrencyState: database.NullChatConcurrencyState{
						ChatConcurrencyState: database.ChatConcurrencyStateActive,
						Valid:                true,
					},
				})
				if errors.Is(err, sql.ErrNoRows) {
					// A concurrent transition moved the chat out of the
					// counted statuses after our read; admit without a
					// claim, matching the status check above.
					res = claimResult{admitted: true}
					return nil
				}
				if err != nil {
					return err
				}
				res.admitted = true
				res.chat = updated
				return nil
			}
		}
		if !res.wasQueued {
			updated, err := tx.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
				ID: chatID,
				ConcurrencyState: database.NullChatConcurrencyState{
					ChatConcurrencyState: database.ChatConcurrencyStateQueued,
					Valid:                true,
				},
				ConcurrencyQueuedAt: sql.NullTime{Time: g.clock.Now(), Valid: true},
			})
			if errors.Is(err, sql.ErrNoRows) {
				res = claimResult{admitted: true}
				return nil
			}
			if err != nil {
				return err
			}
			res.justQueued = true
			res.chat = updated
		}
		return nil
	}, nil)
	if err != nil {
		return claimResult{}, err
	}
	return res, nil
}

// finishAdmission records wait metrics and notifies observers when an
// admitted chat had been queued.
func (g *gate) finishAdmission(res claimResult) {
	if !res.wasQueued {
		return
	}
	if !res.queuedSince.IsZero() {
		g.waitSeconds.Observe(g.clock.Since(res.queuedSince).Seconds())
	}
	if g.onAdmitted != nil {
		g.onAdmitted(res.chat)
	}
}

// releaseQueuedMarker clears a queued marker after an entitlement
// bypass admits the chat mid-wait, so the queued state does not stay
// visible for the rest of the turn. Best effort: enforcement must not
// block an entitled deployment.
func (g *gate) releaseQueuedMarker(ctx context.Context, chatID uuid.UUID) {
	updated, err := g.store.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chatID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			g.logger.Warn(ctx, "clear queued marker after entitlement bypass", slog.F("chat_id", chatID), slog.Error(err))
		}
		return
	}
	if g.onAdmitted != nil {
		g.onAdmitted(updated)
	}
}

// refreshGauges keeps the deployment-wide capacity gauges current.
func (g *gate) refreshGauges(ctx context.Context) {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	ticker := g.clock.NewTicker(defaultMetricsInterval, "chatd", "capacity_metrics")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		active, err := g.store.CountActiveConcurrencyChats(ctx)
		if err != nil {
			g.logger.Warn(ctx, "count active concurrency chats", slog.Error(err))
			continue
		}
		queued, err := g.store.CountQueuedConcurrencyChats(ctx)
		if err != nil {
			g.logger.Warn(ctx, "count queued concurrency chats", slog.Error(err))
			continue
		}
		g.activeGauge.Set(float64(active))
		g.queuedGauge.Set(float64(queued))
	}
}

// jitter spreads poll wakeups across waiters by up to 20 percent so
// queued chats do not stampede the advisory lock in lockstep.
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d/5))) //nolint:gosec // Non-cryptographic wakeup spread.
}
