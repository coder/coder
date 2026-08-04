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

// AgentConcurrencyGate admits chatd agentic loops against a
// deployment-wide capacity cap. This package only defines the
// contract and invokes it at generation boundaries; the enforcing
// implementation lives in the enterprise-licensed
// enterprise/coderd/x/chatd package, where the license forbids
// modification.
type AgentConcurrencyGate interface {
	// Acquire blocks until the chat holds a capacity slot or ctx is
	// canceled. Idempotent: a chat already holding a slot is
	// re-admitted immediately. Capacity release is implicit in chat
	// status transitions, so there is no matching release call.
	Acquire(ctx context.Context, chatID uuid.UUID) error
	// Yield releases the chat's slot while its generation blocks on
	// external completion (wait_agent). Resuming is a plain Acquire.
	Yield(ctx context.Context, chatID uuid.UUID) error
}

// AgentConcurrencyGateOptions carries chatd-owned infrastructure into
// an AgentConcurrencyGateFactory.
type AgentConcurrencyGateOptions struct {
	Store      database.Store
	Pubsub     pubsub.Pubsub
	Clock      quartz.Clock
	Logger     slog.Logger
	Registerer prometheus.Registerer
	// OnQueued and OnAdmitted observe a chat entering the capacity
	// queue and leaving it; chatd publishes the capacity_change watch
	// event from them.
	OnQueued   func(chat database.Chat)
	OnAdmitted func(chat database.Chat)
	// LifetimeCtx bounds gate background work; canceled when the
	// chatd server closes.
	LifetimeCtx context.Context
}

// AgentConcurrencyGateFactory constructs the chat worker's
// concurrent-agent gate. Nil leaves agentic loops uncapped.
type AgentConcurrencyGateFactory func(AgentConcurrencyGateOptions) AgentConcurrencyGate

// agentSlotLease brokers one runner's participation in the gate. It
// binds the chat ID and reference-counts wait_agent pauses so
// parallel wait_agent tool calls share the chat's single slot. A nil
// gate makes every method a no-op (AGPL builds).
type agentSlotLease struct {
	gate   AgentConcurrencyGate
	chatID uuid.UUID
	logger slog.Logger

	// mu serializes gate transitions. Holding it across a blocking
	// Acquire is safe: within a runner, a resume that re-acquires
	// only happens while its generation step waits on that tool, so
	// no other lease operation can run concurrently.
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

// Pause yields the slot while the holder blocks on external
// completion (wait_agent), freeing capacity for subagent children.
// Reference counted: parallel wait_agent calls share the chat's slot.
// Best effort: a failed yield leaves the slot held, which only delays
// children behind the cap.
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
