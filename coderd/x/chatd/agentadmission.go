package chatd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// AgentCapacityLimiter gates chat ownership and exposes the caps it enforces.
// Implementations must support concurrent workers and replicas.
type AgentCapacityLimiter interface {
	// Admit runs inside the acquisition transaction so its serialization
	// extends through the ownership write. Refused chats remain unowned
	// for later acquisition passes.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
	// Limits reports the per-pool caps. capped is false when the deployment
	// currently admits every chat, in which case the caps are meaningless.
	// Limits are dynamic: entitlement changes take effect on the next call.
	Limits() (limits AgentCapacityLimits, capped bool)
}

// AgentCapacityLimits is a point-in-time snapshot of the enforced caps.
type AgentCapacityLimits struct {
	Root     int64
	Subagent int64
}

// noopAgentCapacityLimiter keeps capacity uncapped when no limiter is configured.
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
