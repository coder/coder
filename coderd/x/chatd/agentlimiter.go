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

// AgentConcurrencyGate controls deployment-wide capacity for chat generation.
type AgentConcurrencyGate interface {
	// Acquire blocks until the chat holds a slot or ctx is canceled. Calls are
	// idempotent; leaving a counted status releases the claim.
	Acquire(ctx context.Context, chatID uuid.UUID) error
	// Yield releases the slot while wait_agent blocks. Resume by calling Acquire.
	Yield(ctx context.Context, chatID uuid.UUID) error
}

// AgentConcurrencyGateOptions configures an AgentConcurrencyGateFactory.
type AgentConcurrencyGateOptions struct {
	Store      database.Store
	Pubsub     pubsub.Pubsub
	Clock      quartz.Clock
	Logger     slog.Logger
	Registerer prometheus.Registerer
	// OnQueued and OnAdmitted receive committed queue transitions.
	OnQueued   func(chat database.Chat)
	OnAdmitted func(chat database.Chat)
	// LifetimeCtx bounds background work and is canceled when chatd closes.
	LifetimeCtx context.Context
}

// AgentConcurrencyGateFactory builds a chat worker capacity gate. A nil
// factory leaves capacity uncapped.
type AgentConcurrencyGateFactory func(AgentConcurrencyGateOptions) AgentConcurrencyGate

// agentSlotLease shares one gate claim across concurrent wait_agent pauses.
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

// EnsureHeld acquires the slot or waits until ctx is canceled. Calls are
// idempotent.
func (l *agentSlotLease) EnsureHeld(ctx context.Context) error {
	if l.gate == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.gate.Acquire(ctx, l.chatID)
}

// Pause yields on the first concurrent wait_agent pause. A failed yield
// leaves the claim held.
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

// Resume re-acquires on the final matching pause. If canceled, the next
// generation attempt retries through EnsureHeld.
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

// AttachToContext makes the lease available to wait_agent. It leaves ctx
// unchanged when no gate is configured.
func (l *agentSlotLease) AttachToContext(ctx context.Context) context.Context {
	if l.gate == nil {
		return ctx
	}
	return context.WithValue(ctx, agentSlotLeaseCtxKey{}, l)
}

type agentSlotLeaseCtxKey struct{}

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
