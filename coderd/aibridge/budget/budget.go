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
	GetUserAIBudgetOverrideWithOrganization(ctx context.Context, arg database.GetUserAIBudgetOverrideWithOrganizationParams) (database.GetUserAIBudgetOverrideWithOrganizationRow, error)
	GetHighestGroupAIBudgetByUser(ctx context.Context, arg database.GetHighestGroupAIBudgetByUserParams) (database.GetHighestGroupAIBudgetByUserRow, error)
	GetUserEveryoneFallbackGroup(ctx context.Context, arg database.GetUserEveryoneFallbackGroupParams) (uuid.UUID, error)
}

// EffectiveGroup is a user's resolved effective group and, when a budget
// applies, its limit. Limit is nil for the Everyone fallback (unlimited).
type EffectiveGroup struct {
	// GroupID is the group the spend is attributed to.
	GroupID uuid.UUID
	// OrganizationID is the organization GroupID belongs to. It is uuid.Nil
	// when no effective group was resolved.
	OrganizationID uuid.UUID
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
// When organizationID is set, only that organization's override, group budgets
// and Everyone group count, and the organization must not be deleted. When it
// is null, groups from all of the user's organizations count.
//
// The second return value is false when no budget is configured for the user.
// TODO(AIGOV-527): unify effective group resolution in a single place.
func ResolveUserAIBudget(ctx context.Context, db Store, userID uuid.UUID, organizationID uuid.NullUUID, policy codersdk.AIBudgetPolicy) (EffectiveGroup, bool, error) {
	// A per-user override always wins.
	override, err := db.GetUserAIBudgetOverrideWithOrganization(ctx, database.GetUserAIBudgetOverrideWithOrganizationParams{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if err == nil {
		return EffectiveGroup{
			GroupID:        override.GroupID,
			OrganizationID: override.OrganizationID,
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
		row, err := db.GetHighestGroupAIBudgetByUser(ctx, database.GetHighestGroupAIBudgetByUserParams{
			UserID:         userID,
			OrganizationID: organizationID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return EffectiveGroup{}, false, nil
		}
		if err != nil {
			return EffectiveGroup{}, false, xerrors.Errorf("get highest group AI budget: %w", err)
		}
		return EffectiveGroup{
			GroupID:        row.GroupID,
			OrganizationID: row.OrganizationID,
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
// organizationID limits resolution as described on ResolveUserAIBudget. The
// second return value is false when no effective group was found for the user,
// including when the user is not a member of organizationID.
func ResolveUserEffectiveGroup(ctx context.Context, db Store, userID uuid.UUID, organizationID uuid.NullUUID, policy codersdk.AIBudgetPolicy) (EffectiveGroup, bool, error) {
	group, ok, err := ResolveUserAIBudget(ctx, db, userID, organizationID, policy)
	if err != nil {
		return EffectiveGroup{}, false, err
	}
	if ok {
		return group, true, nil
	}

	// No override or group budget: fall back to the Everyone group (unlimited).
	groupID, err := db.GetUserEveryoneFallbackGroup(ctx, database.GetUserEveryoneFallbackGroupParams{
		UserID:         userID,
		OrganizationID: organizationID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// With a null organizationID this should not happen, as a user should
		// always be a member of an organization and its associated Everyone
		// group. With an organizationID, the user is not a member of it.
		return EffectiveGroup{}, false, nil
	}
	if err != nil {
		return EffectiveGroup{}, false, xerrors.Errorf("get everyone fallback group: %w", err)
	}
	// The Everyone group's ID equals its organization's ID.
	return EffectiveGroup{GroupID: groupID, OrganizationID: groupID}, true, nil
}
