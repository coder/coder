package chatd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entitlements"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// Root and subagent chats use separate pools so a waiting root keeps its slot
// without starving children. Deployments with remaining licensed agent runtime
// hours bypass both caps. Workspace agents use neither pool.
const (
	maxConcurrentRootAgents = int64(5)
	maxConcurrentSubagents  = int64(10)
)

// NewAgentCapacityLimiterFactory builds limiters that evaluate entitlements
// and pool capacity for each acquisition.
func NewAgentCapacityLimiterFactory(set *entitlements.Set) osschatd.AgentCapacityLimiterFactory {
	return func(heartbeatStaleSeconds int32) osschatd.AgentCapacityLimiter {
		return newAdmission(set, heartbeatStaleSeconds)
	}
}

type admission struct {
	entitlements     *entitlements.Set
	staleSeconds     int32
	rootCapacity     int64
	subagentCapacity int64
}

func newAdmission(set *entitlements.Set, staleSeconds int32) *admission {
	if set == nil {
		set = entitlements.New()
	}
	return &admission{
		entitlements:     set,
		staleSeconds:     staleSeconds,
		rootCapacity:     maxConcurrentRootAgents,
		subagentCapacity: maxConcurrentSubagents,
	}
}

// Admit caps only running chats. Other runnable states remain acquirable for
// interrupt and timeout handling.
func (a *admission) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if a.uncapped() {
		return true, nil
	}
	if chat.Status != database.ChatStatusRunning {
		return true, nil
	}
	// The transaction lock remains held through the caller's ownership write,
	// preventing replicas from over-admitting the pool.
	if err := store.AcquireLock(ctx, database.LockIDChatCapacityAdmission); err != nil {
		return false, err
	}
	counts, err := store.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{
		ExcludeChatID: chat.ID,
		StaleSeconds:  a.staleSeconds,
	})
	if err != nil {
		return false, err
	}
	used, capacity := counts.ActiveRootCount, a.rootCapacity
	if chat.ParentChatID.Valid {
		used, capacity = counts.ActiveSubagentCount, a.subagentCapacity
	}
	return used < capacity, nil
}

func (a *admission) Limits() (osschatd.AgentCapacityLimits, bool) {
	return osschatd.AgentCapacityLimits{
		Root:     a.rootCapacity,
		Subagent: a.subagentCapacity,
	}, !a.uncapped()
}

// Runtime-hour allocation does not enforce caps; the hard limit does.
func (a *admission) uncapped() bool {
	f, ok := a.entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || !f.Enabled {
		return false
	}
	if f.HardLimit == nil || f.Actual == nil {
		return true
	}
	return *f.Actual < *f.HardLimit
}
