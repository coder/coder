package chatd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// AgentAdmission gates chat ownership. Implementations must support
// concurrent workers and replicas.
type AgentAdmission interface {
	// Admit runs inside the acquisition transaction so its serialization
	// extends through the ownership write. Refused chats remain unowned
	// for later acquisition passes.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
}

// AgentCapacityPolicy reports the dynamic per-pool caps. capped is false when
// the deployment currently admits every chat.
type AgentCapacityPolicy interface {
	Limits() (limits AgentCapacityLimits, capped bool)
}

// AgentCapacityLimiter combines admission and capacity policy.
type AgentCapacityLimiter interface {
	AgentAdmission
	AgentCapacityPolicy
}

// AgentCapacityLimits is a point-in-time snapshot of the enforced caps.
type AgentCapacityLimits struct {
	Root     int64
	Subagent int64
}

type noopAgentCapacityLimiter struct{}

func (noopAgentCapacityLimiter) Admit(context.Context, database.Store, database.Chat) (bool, error) {
	return true, nil
}

func (noopAgentCapacityLimiter) Limits() (AgentCapacityLimits, bool) {
	return AgentCapacityLimits{}, false
}

// AgentCapacityLimiterFactory builds the concurrent-agent capacity limiter.
// Its stale threshold must match worker ownership checks. A nil factory or
// limiter leaves capacity uncapped.
type AgentCapacityLimiterFactory func(heartbeatStaleSeconds int32) AgentCapacityLimiter
