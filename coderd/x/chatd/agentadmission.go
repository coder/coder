package chatd

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

// AgentAdmission decides at ownership-acquisition time whether a chat
// may start generating. Implementations run inside the acquisition
// transaction, after the runnable and ownership checks and before
// ownership is written, and must be safe for concurrent use across
// workers and replicas.
//
// A nil AgentAdmission admits everything; the enterprise
// implementation enforces deployment-wide concurrency pools.
type AgentAdmission interface {
	// Admit reports whether the chat may be acquired now. Refused
	// chats stay unowned; the worker marks them capacity-queued and
	// retries on later acquisition passes.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
}

// AgentAdmissionOptions configures an AgentAdmissionFactory.
type AgentAdmissionOptions struct {
	Store      database.Store
	Logger     slog.Logger
	Clock      quartz.Clock
	Registerer prometheus.Registerer
	// LifetimeCtx bounds background work and is canceled when chatd
	// closes.
	LifetimeCtx context.Context
	// HeartbeatStaleSeconds is the ownership staleness threshold the
	// worker uses; admission counting must use the same threshold so
	// both sides agree on which runners are alive.
	HeartbeatStaleSeconds int32
}

// AgentAdmissionFactory builds a chat worker admission gate. A nil
// factory leaves capacity uncapped.
type AgentAdmissionFactory func(AgentAdmissionOptions) AgentAdmission
