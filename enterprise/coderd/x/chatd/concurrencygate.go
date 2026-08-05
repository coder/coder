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

// MaxConcurrentAgents is the default cap for chatd generation loops when
// codersdk.FeatureAgentRuntimeHours does not lift it (see gate.uncapped).
// It does not cap workspace agents.
const MaxConcurrentAgents = 5

// Fallback polling covers missed nudges, entitlement changes, and release
// paths without a publisher.
const defaultCapacityPollInterval = 15 * time.Second

const defaultMetricsInterval = 30 * time.Second

// NewAgentConcurrencyGateFactory returns a gate that evaluates the agent
// runtime hours entitlement before each claim.
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
	Entitlements *entitlements.Set
	Store        database.Store
	Pubsub       pubsub.Pubsub
	Clock        quartz.Clock
	Logger       slog.Logger
	Registerer   prometheus.Registerer
	// OnQueued and OnAdmitted observe a chat entering the capacity
	// queue and leaving it. The chat row reflects the committed state.
	OnQueued    func(chat database.Chat)
	OnAdmitted  func(chat database.Chat)
	LifetimeCtx context.Context

	Capacity     int64
	PollInterval time.Duration
}

// gate serializes cross-replica claims with an advisory lock. Claims are
// cleared when a chat leaves the counted states.
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
		if opts.LifetimeCtx != nil {
			go g.refreshGauges(opts.LifetimeCtx)
		}
	}
	return g
}

// uncapped reports whether the license has remaining agent runtime hours.
// Enabled alone only means the license carries a positive allocation; once
// recorded usage (Actual) reaches it, the deployment is capped like
// community. A nil allocation or usage reading fails open to Enabled
// semantics: capping on a missing usage reading is worse than bounded
// over-admission.
func (g *gate) uncapped() bool {
	f, ok := g.entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || !f.Enabled {
		return false
	}
	if f.Limit == nil || f.Actual == nil {
		return true
	}
	return *f.Actual < *f.Limit
}

func (g *gate) Acquire(ctx context.Context, chatID uuid.UUID, runnerID uuid.UUID) error {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if g.uncapped() {
		g.releaseQueuedMarker(ctx, chatID, runnerID)
		return nil
	}
	if admitted := g.claimOnce(ctx, chatID, runnerID); admitted {
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
		if g.uncapped() {
			g.releaseQueuedMarker(ctx, chatID, runnerID)
			return nil
		}
		if admitted := g.claimOnce(ctx, chatID, runnerID); admitted {
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

// Claim failures are logged so Acquire can retry.
func (g *gate) claimOnce(ctx context.Context, chatID uuid.UUID, runnerID uuid.UUID) bool {
	res, err := g.tryClaim(ctx, chatID, runnerID)
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

// Yield releases the slot while wait_agent blocks so subagent chats can run.
func (g *gate) Yield(ctx context.Context, chatID uuid.UUID, runnerID uuid.UUID) error {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if g.uncapped() {
		return nil
	}
	_, err := g.store.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID: chatID,
		ConcurrencyState: database.NullChatConcurrencyState{
			ChatConcurrencyState: database.ChatConcurrencyStateYielded,
			Valid:                true,
		},
		RunnerID: runnerFence(runnerID),
	})
	if errors.Is(err, sql.ErrNoRows) {
		// The chat is no longer eligible, or this runner no longer owns
		// it; either way there is nothing to free.
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
	// justQueued marks a transition into the queued state.
	justQueued  bool
	queuedSince time.Time
	wasQueued   bool
	chat        database.Chat
}

// tryClaim serializes count and write with the advisory lock so replicas
// cannot over-admit.
func (g *gate) tryClaim(ctx context.Context, chatID uuid.UUID, runnerID uuid.UUID) (claimResult, error) {
	var res claimResult
	err := g.store.InTx(func(tx database.Store) error {
		res = claimResult{}
		if err := tx.AcquireLock(ctx, database.LockIDChatConcurrency); err != nil {
			return err
		}
		chat, err := tx.GetChatByID(ctx, chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Deleted chats cannot hold counted slots, so admit without a claim.
				res.admitted = true
				return nil
			}
			return err
		}
		if chat.Archived || (chat.Status != database.ChatStatusRunning && chat.Status != database.ChatStatusInterrupting) {
			// The chat is no longer eligible for a capacity marker, so admit
			// without a claim.
			res.admitted = true
			return nil
		}
		if runnerID != uuid.Nil && (!chat.RunnerID.Valid || chat.RunnerID.UUID != runnerID) {
			// A replacement runner owns the chat. Admit this stale caller
			// without touching the owner's marker; other fences will stop it.
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
			// Queued chats are admitted oldest first before new claims.
			admit := slices.Contains(oldestQueued, chatID) ||
				(!res.wasQueued && int64(len(oldestQueued)) < free)
			if admit {
				updated, err := tx.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
					ID: chatID,
					ConcurrencyState: database.NullChatConcurrencyState{
						ChatConcurrencyState: database.ChatConcurrencyStateActive,
						Valid:                true,
					},
					RunnerID: runnerFence(runnerID),
				})
				if errors.Is(err, sql.ErrNoRows) {
					// The chat is no longer eligible for a capacity marker, so
					// admit without a claim.
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
				RunnerID: runnerFence(runnerID),
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

func (g *gate) finishAdmission(res claimResult) {
	if !res.wasQueued {
		return
	}
	if !res.queuedSince.IsZero() {
		// Queue entry uses the database clock, so clamp the skew-induced
		// negative case.
		g.waitSeconds.Observe(max(0, g.clock.Since(res.queuedSince).Seconds()))
	}
	if g.onAdmitted != nil {
		g.onAdmitted(res.chat)
	}
}

// Entitlement-bypass cleanup is best effort and does not block admission.
// The read keeps entitled claims cheap: unmarked chats skip the write and
// publish nothing.
func (g *gate) releaseQueuedMarker(ctx context.Context, chatID uuid.UUID, runnerID uuid.UUID) {
	chat, err := g.store.GetChatByID(ctx, chatID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			g.logger.Warn(ctx, "read chat for entitlement bypass", slog.F("chat_id", chatID), slog.Error(err))
		}
		return
	}
	if !chat.ConcurrencyState.Valid {
		return
	}
	updated, err := g.store.SetChatConcurrencyState(ctx, database.SetChatConcurrencyStateParams{
		ID:       chatID,
		RunnerID: runnerFence(runnerID),
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

// jitter spreads poll wakeups across waiters to reduce lock contention.
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d/5))) //nolint:gosec // Non-cryptographic wakeup spread.
}

// runnerFence maps uuid.Nil to NULL, which skips the runner ownership
// guard in SetChatConcurrencyState.
func runnerFence(runnerID uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: runnerID, Valid: runnerID != uuid.Nil}
}
