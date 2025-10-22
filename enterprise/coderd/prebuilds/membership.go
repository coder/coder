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

// ReconcileAll compares the current organization and group memberships of a user to the memberships required
// in order to create prebuilt workspaces. If the user in question is not yet a member of an organization that
// needs prebuilt workspaces, ReconcileAll will create the membership required.
//
// To facilitate quota management, ReconcileAll will ensure:
// * the existence of a group (defined by PrebuiltWorkspacesGroupName) in each organization that needs prebuilt workspaces
// * that the prebuilds system user belongs to the group in each organization that needs prebuilt workspaces
// * that the group has a quota of 0 by default, which users can adjust based on their needs.
//
// ReconcileAll does not have an opinion on transaction or lock management. These responsibilities are left to the caller.
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

	orgMemberships := make(map[uuid.UUID]struct{}, 0)
	defaultOrg, err := s.store.GetDefaultOrganization(ctx)
	if err != nil {
		return xerrors.Errorf("get default organization: %w", err)
	}
	orgMemberships[defaultOrg.ID] = struct{}{}
	for _, o := range organizationMemberships {
		orgMemberships[o.ID] = struct{}{}
	}

	var membershipInsertionErrors error
	for _, preset := range presets {
		_, alreadyOrgMember := orgMemberships[preset.OrganizationID]
		if !alreadyOrgMember {
			// Add the organization to our list of memberships regardless of potential failure below
			// to avoid a retry that will probably be doomed anyway.
			orgMemberships[preset.OrganizationID] = struct{}{}

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

		// determine whether the org already has a prebuilds group
		prebuildsGroupExists := true
		prebuildsGroup, err := s.store.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
			OrganizationID: preset.OrganizationID,
			Name:           PrebuiltWorkspacesGroupName,
		})
		if err != nil {
			if !xerrors.Is(err, sql.ErrNoRows) {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, xerrors.Errorf("get prebuilds group: %w", err))
				continue
			}
			prebuildsGroupExists = false
		}

		// if the prebuilds group does not exist, create it
		if !prebuildsGroupExists {
			// create a "prebuilds" group in the organization and add the system user to it
			// this group will have a quota of 0 by default, which users can adjust based on their needs
			prebuildsGroup, err = s.store.InsertGroup(ctx, database.InsertGroupParams{
				ID:             uuid.New(),
				Name:           PrebuiltWorkspacesGroupName,
				DisplayName:    PrebuiltWorkspacesGroupDisplayName,
				OrganizationID: preset.OrganizationID,
				AvatarURL:      "",
				QuotaAllowance: 0, // Default quota of 0, users should set this based on their needs
			})
			if err != nil {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, xerrors.Errorf("create prebuilds group: %w", err))
				continue
			}
		}

		// add the system user to the prebuilds group
		err = s.store.InsertGroupMember(ctx, database.InsertGroupMemberParams{
			GroupID: prebuildsGroup.ID,
			UserID:  userID,
		})
		if err != nil {
			// ignore unique violation errors as the user might already be in the group
			if !database.IsUniqueViolation(err) {
				membershipInsertionErrors = errors.Join(membershipInsertionErrors, xerrors.Errorf("add system user to prebuilds group: %w", err))
			}
		}
	}
	return membershipInsertionErrors
}
