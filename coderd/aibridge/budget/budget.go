// Package budget resolves the effective AI spend budget for a user. A
// per-user override always wins; otherwise the deployment budget policy selects
// a budget from the groups the user belongs to.
package budget

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// Store is the subset of database.Store needed to resolve a user's effective
// AI budget.
type Store interface {
	GetUserAIBudgetOverride(ctx context.Context, userID uuid.UUID) (database.UserAIBudgetOverride, error)
	GetHighestGroupAIBudgetByUser(ctx context.Context, userID uuid.UUID) (database.GetHighestGroupAIBudgetByUserRow, error)
	GetUserEveryoneFallbackGroup(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// EffectiveGroup is a user's resolved effective group and, when a budget
// applies, its limit. Limit is nil for the Everyone fallback (unlimited).
type EffectiveGroup struct {
	// GroupID is the group the spend is attributed to.
	GroupID uuid.UUID
	// Limit is the resolved spend limit, or nil for the unlimited Everyone
	// fallback.
	Limit *Limit
}

// Limit is an AI spend limit and the source that produced it.
type Limit struct {
	// SpendLimitMicros is the spend limit in micro-units (1 unit = 1,000,000).
	SpendLimitMicros int64
	Source           codersdk.AIBudgetLimitSource
}

// ResolveUserAIBudget returns the effective AI budget group for userID,
// resolved in order:
//  1. A per-user override, if configured.
//  2. Otherwise, a group budget selected by the deployment policy.
//
// The second return value is false when no budget is configured for the user.
// TODO(AIGOV-527): unify effective group resolution in a single place.
func ResolveUserAIBudget(ctx context.Context, db Store, userID uuid.UUID, policy codersdk.AIBudgetPolicy) (EffectiveGroup, bool, error) {
	// A per-user override always wins.
	override, err := db.GetUserAIBudgetOverride(ctx, userID)
	if err == nil {
		return EffectiveGroup{
			GroupID: override.GroupID,
			Limit: &Limit{
				SpendLimitMicros: override.SpendLimitMicros,
				Source:           codersdk.AIBudgetLimitSourceUserOverride,
			},
		}, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return EffectiveGroup{}, false, xerrors.Errorf("get user AI budget override: %w", err)
	}

	// No override: select a group budget according to the deployment policy.
	switch policy {
	case codersdk.AIBudgetPolicyHighest:
		row, err := db.GetHighestGroupAIBudgetByUser(ctx, userID)
		if errors.Is(err, sql.ErrNoRows) {
			return EffectiveGroup{}, false, nil
		}
		if err != nil {
			return EffectiveGroup{}, false, xerrors.Errorf("get highest group AI budget: %w", err)
		}
		return EffectiveGroup{
			GroupID: row.GroupID,
			Limit: &Limit{
				SpendLimitMicros: row.SpendLimitMicros,
				Source:           codersdk.AIBudgetLimitSourceGroup,
			},
		}, true, nil
	default:
		return EffectiveGroup{}, false, xerrors.Errorf("unsupported AI budget policy: %q", policy)
	}
}

// ResolveUserEffectiveGroup resolves the user's effective group, falling back to
// the organization's Everyone group when no override or group budget applies.
// The second return value is false when no effective group was found for the
// user.
func ResolveUserEffectiveGroup(ctx context.Context, db Store, userID uuid.UUID, policy codersdk.AIBudgetPolicy) (EffectiveGroup, bool, error) {
	group, ok, err := ResolveUserAIBudget(ctx, db, userID, policy)
	if err != nil {
		return EffectiveGroup{}, false, err
	}
	if ok {
		return group, true, nil
	}

	// No override or group budget: fall back to the Everyone group (unlimited).
	groupID, err := db.GetUserEveryoneFallbackGroup(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		// This should not happen, as a user should always be a member of an
		// organization and its associated Everyone group.
		return EffectiveGroup{}, false, nil
	}
	if err != nil {
		return EffectiveGroup{}, false, xerrors.Errorf("get everyone fallback group: %w", err)
	}
	return EffectiveGroup{GroupID: groupID}, true, nil
}
