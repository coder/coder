package chatd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	defaultMaxConcurrentRootAgents = int64(5)
	defaultMaxConcurrentSubagents  = int64(10)
)

// AgentCapacityLimiter controls chat admission and reports the current per-pool limits.
type AgentCapacityLimiter interface {
	// Admit runs inside the acquisition transaction so its serialization
	// extends through the ownership write. Refused chats remain unowned.
	Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error)
	Limits() (limits AgentCapacityLimits, capped bool)
}

// AgentCapacityUnlock reports whether the default chat agent caps are disabled.
type AgentCapacityUnlock interface {
	Unlocked() bool
}

// AgentCapacityLimits defines concurrent-agent limits for root and subagent pools.
type AgentCapacityLimits struct {
	Root     int64
	Subagent int64
}

type agentCapacityLimiter struct {
	unlock AgentCapacityUnlock

	staleSeconds     int32
	rootCapacity     int64
	subagentCapacity int64
}

func newAgentCapacityLimiter(unlock AgentCapacityUnlock, staleSeconds int32) *agentCapacityLimiter {
	return &agentCapacityLimiter{
		unlock:           unlock,
		staleSeconds:     staleSeconds,
		rootCapacity:     defaultMaxConcurrentRootAgents,
		subagentCapacity: defaultMaxConcurrentSubagents,
	}
}

func (a *agentCapacityLimiter) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	ctx = dbauthz.AsChatd(ctx)
	if a.unlocked() || chat.Status != database.ChatStatusRunning {
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

func (a *agentCapacityLimiter) Limits() (AgentCapacityLimits, bool) {
	return AgentCapacityLimits{
		Root:     a.rootCapacity,
		Subagent: a.subagentCapacity,
	}, !a.unlocked()
}

func (a *agentCapacityLimiter) unlocked() bool {
	return a.unlock != nil && a.unlock.Unlocked()
}
