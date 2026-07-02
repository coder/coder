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
}

// EffectiveBudget is the AI budget that applies to a user after override and
// policy resolution.
type EffectiveBudget struct {
	// GroupID is the group the spend is attributed to.
	GroupID uuid.UUID
	// SpendLimitMicros is the effective spend limit in micro-units
	// (1 unit = 1,000,000).
	SpendLimitMicros int64
	Source           codersdk.AIBudgetLimitSource
}

// ResolveUserAIBudget returns the effective AI budget for userID. The second
// return value is false when no budget is configured for the user. A per-user
// override wins unconditionally; otherwise the budget is selected from the
// user's groups according to policy.
func ResolveUserAIBudget(ctx context.Context, db Store, userID uuid.UUID, policy codersdk.AIBudgetPolicy) (EffectiveBudget, bool, error) {
	// A per-user override always wins.
	override, err := db.GetUserAIBudgetOverride(ctx, userID)
	if err == nil {
		return EffectiveBudget{
			GroupID:          override.GroupID,
			SpendLimitMicros: override.SpendLimitMicros,
			Source:           codersdk.AIBudgetLimitSourceUserOverride,
		}, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return EffectiveBudget{}, false, xerrors.Errorf("get user AI budget override: %w", err)
	}

	// No override: select a group budget according to the deployment policy.
	switch policy {
	case codersdk.AIBudgetPolicyHighest:
		row, err := db.GetHighestGroupAIBudgetByUser(ctx, userID)
		if errors.Is(err, sql.ErrNoRows) {
			return EffectiveBudget{}, false, nil
		}
		if err != nil {
			return EffectiveBudget{}, false, xerrors.Errorf("get highest group AI budget: %w", err)
		}
		return EffectiveBudget{
			GroupID:          row.GroupID,
			SpendLimitMicros: row.SpendLimitMicros,
			Source:           codersdk.AIBudgetLimitSourceGroup,
		}, true, nil
	default:
		return EffectiveBudget{}, false, xerrors.Errorf("unsupported AI budget policy: %q", policy)
	}
}
