package chatd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// AgentAdmission gates chat ownership within the acquisition transaction,
// before ownership is written. Implementations must support concurrent workers
// and replicas. A nil AgentAdmission admits every chat.
type AgentAdmission interface {
	// Admit runs inside the acquisition transaction so its serialization
	// extends through the ownership write. Refused chats remain unowned
	// for later acquisition passes.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
}

// AgentCapacityPolicy exposes the current capacity limits for read-side
// derivation of queued state (API responses and metrics). It is split from
// AgentAdmission so admission stays a single-method gate.
type AgentCapacityPolicy interface {
	// CurrentLimits reports whether the deployment currently caps concurrent
	// chat agents and the per-pool caps. Limits are dynamic: entitlement
	// changes take effect on the next call.
	CurrentLimits() AgentCapacityLimits
}

// AgentCapacityLimits is a point-in-time snapshot of the capacity policy.
type AgentCapacityLimits struct {
	// Capped is false when the deployment currently admits every chat, in
	// which case the caps are meaningless.
	Capped   bool
	Root     int64
	Subagent int64
}

// AgentAdmissionFactory builds capacity admission and its read-side policy.
// heartbeatStaleSeconds must match the worker's threshold so admission counts
// and ownership checks agree which runners are alive. A nil factory leaves
// capacity uncapped.
type AgentAdmissionFactory func(heartbeatStaleSeconds int32) (AgentAdmission, AgentCapacityPolicy)
