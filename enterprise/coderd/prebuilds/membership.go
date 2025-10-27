package prebuilds

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

const (
	PrebuiltWorkspacesGroupName        = "coderprebuiltworkspaces"
	PrebuiltWorkspacesGroupDisplayName = "Prebuilt Workspaces"
)

// StoreMembershipReconciler encapsulates the responsibility of ensuring that the prebuilds system user is a member of all
// organizations for which prebuilt workspaces are requested. This is necessary because our data model requires that such
// prebuilt workspaces belong to a member of the organization of their eventual claimant.
type StoreMembershipReconciler struct {
	store database.Store
	clock quartz.Clock
}

func NewStoreMembershipReconciler(store database.Store, clock quartz.Clock) StoreMembershipReconciler {
	return StoreMembershipReconciler{
		store: store,
		clock: clock,
	}
}

// ReconcileAll ensures the prebuilds system user has the necessary memberships to create prebuilt workspaces.
// For each organization with prebuilds configured, it ensures:
// * The user is a member of the organization
// * A group exists with quota 0
// * The user is a member of that group
//
// Unique constraint violations are safely ignored (concurrent creation).
//
// ReconcileAll does not have an opinion on transaction or lock management. These responsibilities are left to the caller.
func (s StoreMembershipReconciler) ReconcileAll(ctx context.Context, userID uuid.UUID, groupName string) error {
	orgStatuses, err := s.store.GetOrganizationsWithPrebuildStatus(ctx, database.GetOrganizationsWithPrebuildStatusParams{
		UserID:    userID,
		GroupName: groupName,
	})
	if err != nil {
		return xerrors.Errorf("get organizations with prebuild status: %w", err)
	}

	var membershipInsertionErrors error
	for _, orgStatus := range orgStatuses {
		// Add user to org if needed
		if !orgStatus.HasPrebuildUser {
			_, err = s.store.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
				OrganizationID: orgStatus.OrganizationID,
				UserID:         userID,
				CreatedAt:      s.clock.Now(),
				UpdatedAt:      s.clock.Now(),
				Roles:          []string{},
			})
			// Unique violation means membership was created after status check, safe to ignore.
			if err != nil && !database.IsUniqueViolation(err) {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, err)
				continue
			}
		}

		// Create group if it doesn't exist
		var groupID uuid.UUID
		if !orgStatus.PrebuildsGroupID.Valid {
			// Group doesn't exist, create it
			group, err := s.store.InsertGroup(ctx, database.InsertGroupParams{
				ID:             uuid.New(),
				Name:           PrebuiltWorkspacesGroupName,
				DisplayName:    PrebuiltWorkspacesGroupDisplayName,
				OrganizationID: orgStatus.OrganizationID,
				AvatarURL:      "",
				QuotaAllowance: 0,
			})
			// Unique violation means membership was created after status check, safe to ignore.
			if err != nil && !database.IsUniqueViolation(err) {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, err)
				continue
			}
			groupID = group.ID
		} else {
			// Group exists
			groupID = orgStatus.PrebuildsGroupID.UUID
		}

		// Add user to group if needed
		if !orgStatus.HasPrebuildUserInGroup {
			err = s.store.InsertGroupMember(ctx, database.InsertGroupMemberParams{
				GroupID: groupID,
				UserID:  userID,
			})
			// Unique violation means membership was created after status check, safe to ignore.
			if err != nil && !database.IsUniqueViolation(err) {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, err)
				continue
			}
		}
	}

	return membershipInsertionErrors
}
