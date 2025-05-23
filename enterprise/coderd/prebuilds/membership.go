package prebuilds

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/quartz"
)

// StoreMembershipReconciler encapsulates the responsibility of ensuring that the prebuilds system user is a member of all
// organizations for which prebuilt workspaces are requested. This is necessary because our data model requires that such
// prebuilt workspaces belong to a member of the organization of their eventual claimant.
type StoreMembershipReconciler struct {
	store    database.Store
	clock    quartz.Clock
	snapshot *prebuilds.GlobalSnapshot
	userID   uuid.UUID
}

// ReconcileAll compares the current membership of a user to the membership required in order to create prebuilt workspaces.
// If the user in question is not yet a member of an organization that needs prebuilt workspaces, ReconcileAll will create
// the membership required.
//
// This method does not have an opinion on transaction or lock management. These responsibilities are left to the caller.
func (s StoreMembershipReconciler) ReconcileAll(ctx context.Context) error {
	membershipsByUserID, err := s.store.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{s.userID})
	if err != nil {
		return xerrors.Errorf("determine prebuild organization membership: %w", err)
	}

	systemUserMemberships := []uuid.UUID{}
	if len(membershipsByUserID) == 1 {
		systemUserMemberships = membershipsByUserID[0].OrganizationIDs
	}
	addedMemberships := []uuid.UUID{}

	var membershipInsertionErrors error
	for _, preset := range s.snapshot.Presets {
		systemUserNeedsMembership := true
		for _, systemUserMembership := range append(systemUserMemberships, addedMemberships...) {
			if systemUserMembership == preset.OrganizationID {
				systemUserNeedsMembership = false
				break
			}
		}
		if systemUserNeedsMembership {
			_, err = s.store.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
				OrganizationID: preset.OrganizationID,
				UserID:         s.userID,
				CreatedAt:      s.clock.Now(),
				UpdatedAt:      s.clock.Now(),
				Roles:          []string{
					// TODO: what roles do we need
				},
			})
			if err != nil {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, xerrors.Errorf("insert membership for prebuilt workspaces: %w", err))
				continue
			}
			addedMemberships = append(addedMemberships, preset.OrganizationID)
		}
	}
	return membershipInsertionErrors
}
