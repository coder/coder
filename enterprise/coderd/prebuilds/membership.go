package prebuilds

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
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

// ReconcileAll compares the current membership of a user to the membership required in order to create prebuilt workspaces.
// If the user in question is not yet a member of an organization that needs prebuilt workspaces, ReconcileAll will create
// the membership required.
//
// This method does not have an opinion on transaction or lock management. These responsibilities are left to the caller.
func (s StoreMembershipReconciler) ReconcileAll(ctx context.Context, userID uuid.UUID, presets []database.GetTemplatePresetsWithPrebuildsRow) error {
	organizationMemberships, err := s.store.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID: userID,
		Deleted: sql.NullBool{
			Bool:  false,
			Valid: true,
		},
	})
	if err != nil {
		return xerrors.Errorf("determine prebuild organization membership: %w", err)
	}

	systemUserMemberships := make(map[uuid.UUID]struct{}, 0)
	defaultOrg, err := s.store.GetDefaultOrganization(ctx)
	if err != nil {
		return xerrors.Errorf("get default organization: %w", err)
	}
	systemUserMemberships[defaultOrg.ID] = struct{}{}
	for _, o := range organizationMemberships {
		systemUserMemberships[o.ID] = struct{}{}
	}

	var membershipInsertionErrors error
	for _, preset := range presets {
		_, alreadyMember := systemUserMemberships[preset.OrganizationID]
		if alreadyMember {
			continue
		}
		// Add the organization to our list of memberships regardless of potential failure below
		// to avoid a retry that will probably be doomed anyway.
		systemUserMemberships[preset.OrganizationID] = struct{}{}

		// Insert the missing membership
		_, err = s.store.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: preset.OrganizationID,
			UserID:         userID,
			CreatedAt:      s.clock.Now(),
			UpdatedAt:      s.clock.Now(),
			Roles:          []string{},
		})
		if err != nil {
			membershipInsertionErrors = errors.Join(membershipInsertionErrors, xerrors.Errorf("insert membership for prebuilt workspaces: %w", err))
			continue
		}
	}
	return membershipInsertionErrors
}
