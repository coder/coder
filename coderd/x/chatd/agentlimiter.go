package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// AgentConcurrencyGate is invoked at generation boundaries to enforce
// deployment-wide chatd agent capacity. The enterprise gate implements
// this contract.
type AgentConcurrencyGate interface {
	// Acquire blocks until the chat holds a capacity slot or ctx is
	// canceled. It is idempotent; status transitions release the slot
	// implicitly.
	Acquire(ctx context.Context, chatID uuid.UUID) error
	// Yield releases the chat's slot while its generation blocks on
	// external completion (wait_agent). Resuming is a plain Acquire.
	Yield(ctx context.Context, chatID uuid.UUID) error
}

// AgentConcurrencyGateOptions configures an AgentConcurrencyGateFactory.
type AgentConcurrencyGateOptions struct {
	Store      database.Store
	Pubsub     pubsub.Pubsub
	Clock      quartz.Clock
	Logger     slog.Logger
	Registerer prometheus.Registerer
	// OnQueued and OnAdmitted observe committed queue transitions used for
	// capacity_change watch events.
	OnQueued   func(chat database.Chat)
	OnAdmitted func(chat database.Chat)
	// LifetimeCtx bounds gate background work; canceled when the
	// chatd server closes.
	LifetimeCtx context.Context
}

// AgentConcurrencyGateFactory constructs the chat worker's gate. Nil leaves
// agentic loops uncapped.
type AgentConcurrencyGateFactory func(AgentConcurrencyGateOptions) AgentConcurrencyGate

// agentSlotLease binds one runner to the gate and reference-counts
// concurrent wait_agent pauses.
type agentSlotLease struct {
	gate   AgentConcurrencyGate
	chatID uuid.UUID
	logger slog.Logger

	// mu protects pauseRefs and serializes Yield with re-acquisition.
	mu        sync.Mutex
	pauseRefs int
}

func newAgentSlotLease(gate AgentConcurrencyGate, chatID uuid.UUID, logger slog.Logger) *agentSlotLease {
	return &agentSlotLease{gate: gate, chatID: chatID, logger: logger}
}

// EnsureHeld acquires the chat's capacity slot, blocking until one
// frees or ctx is canceled. Idempotent at step boundaries and across
// task retries.
func (l *agentSlotLease) EnsureHeld(ctx context.Context) error {
	if l.gate == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.gate.Acquire(ctx, l.chatID)
}

// Pause yields the slot while wait_agent blocks. Only the first concurrent
// pause yields; a failed yield leaves the claim held.
func (l *agentSlotLease) Pause(ctx context.Context) {
	if l.gate == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pauseRefs++
	if l.pauseRefs != 1 {
		return
	}
	if err := l.gate.Yield(ctx, l.chatID); err != nil {
		l.logger.Warn(ctx, "yield agent capacity slot", slog.F("chat_id", l.chatID), slog.Error(err))
	}
}

// Resume undoes one Pause; the last Resume re-acquires the slot,
// blocking until one frees. A canceled Resume returns the context
// error; the next generation attempt re-acquires through EnsureHeld.
func (l *agentSlotLease) Resume(ctx context.Context) error {
	if l.gate == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pauseRefs > 0 {
		l.pauseRefs--
	}
	if l.pauseRefs != 0 {
		return nil
	}
	return l.gate.Acquire(ctx, l.chatID)
}

// AttachToContext injects the lease into a generation task context so
// wait_agent can pause it. Returns ctx unchanged for the no-op lease.
func (l *agentSlotLease) AttachToContext(ctx context.Context) context.Context {
	if l.gate == nil {
		return ctx
	}
	return context.WithValue(ctx, agentSlotLeaseCtxKey{}, l)
}

type agentSlotLeaseCtxKey struct{}

// agentSlotLeaseFromContext returns the lease injected by the runner
// into generation task contexts. Absent on uncapped deployments and
// for non-generation callers; callers treat absence as a no-op.
func agentSlotLeaseFromContext(ctx context.Context) (*agentSlotLease, bool) {
	lease, ok := ctx.Value(agentSlotLeaseCtxKey{}).(*agentSlotLease)
	return lease, ok
}

func agentGateFromFactory(lifetimeCtx context.Context, p *Server, cfg Config, clk quartz.Clock, ps pubsub.Pubsub) AgentConcurrencyGate {
	if cfg.AgentConcurrencyGateFactory == nil {
		return nil
	}
	return cfg.AgentConcurrencyGateFactory(AgentConcurrencyGateOptions{
		Store:      cfg.Database,
		Pubsub:     ps,
		Clock:      clk,
		Logger:     cfg.Logger.Named("chatworker"),
		Registerer: cfg.PrometheusRegistry,
		OnQueued: func(chat database.Chat) {
			p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindCapacityChange, nil)
		},
		OnAdmitted: func(chat database.Chat) {
			p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindCapacityChange, nil)
		},
		LifetimeCtx: lifetimeCtx,
	})
}
