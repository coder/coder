package chatd

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

// AgentAdmission gates chat ownership within the acquisition transaction,
// before ownership is written. Implementations must support concurrent workers
// and replicas. A nil AgentAdmission admits every chat.
type AgentAdmission interface {
	// Admit reports whether the worker may acquire the chat. Refused chats
	// remain unowned and are retried from the capacity queue.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
}

// AgentAdmissionOptions configures an AgentAdmissionFactory.
type AgentAdmissionOptions struct {
	Store      database.Store
	Logger     slog.Logger
	Clock      quartz.Clock
	Registerer prometheus.Registerer
	// LifetimeCtx is canceled when chatd closes.
	LifetimeCtx context.Context
	// HeartbeatStaleSeconds must match the worker's threshold so admission
	// counts and ownership checks agree which runners are alive.
	HeartbeatStaleSeconds int32
}

// AgentAdmissionFactory builds an admission gate. A nil factory leaves capacity uncapped.
type AgentAdmissionFactory func(AgentAdmissionOptions) AgentAdmission
