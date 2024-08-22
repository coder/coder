package idp_sync

import (
	"context"
	"regexp"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

type IDPSync struct {
	*codersdk.Entitlement
}

// SynchronizeGroupsParams
type SynchronizeGroupsParams struct {
	UserID uuid.UUID
	// CoderGroups should be the coder groups to map a user into.
	// TODO: This function should really take the raw IDP groups, but that
	// is handled by the caller of `oauthLogin` atm. Need to plumb
	// that through to here.
	CoderGroups []string
	// TODO: These options will be moved outside of deployment into organization
	// scoped settings. So these parameters will be removed, and instead sourced
	// from some settings object that should have these values cached.
	// At present, these settings apply to the default organization.
	GroupFilter         *regexp.Regexp
	CreateMissingGroups bool
}

// SynchronizeGroups takes a given user, and ensures their group memberships
// within a given organization are correct.
func SynchronizeGroups(ctx context.Context, logger slog.Logger, tx database.Store, params *SynchronizeGroupsParams) error {
	//nolint:gocritic // group sync happens as a system operation.
	ctx = dbauthz.AsSystemRestricted(ctx)

	wantGroups := params.CoderGroups

	// Apply regex filter if applicable
	if params.GroupFilter != nil {
		wantGroups = make([]string, 0, len(params.CoderGroups))
		for _, group := range params.CoderGroups {
			if params.GroupFilter.MatchString(group) {
				wantGroups = append(wantGroups, group)
			}
		}
	}

	// By default, group sync applies only to the default organization.
	defaultOrganization, err := tx.GetDefaultOrganization(ctx)
	if err != nil {
		// If there is no default org, then we can't assign groups.
		// By default, we assume all groups belong to the default org.
		return xerrors.Errorf("get default organization: %w", err)
	}

	memberships, err := tx.OrganizationMembers(dbauthz.AsSystemRestricted(ctx), database.OrganizationMembersParams{
		UserID:         params.UserID,
		OrganizationID: defaultOrganization.ID,
	})
	if err != nil {
		return xerrors.Errorf("get user memberships: %w", err)
	}

	// If the user is not in the default organization, then we can't assign groups.
	// A user cannot be in groups to an org they are not a member of.
	if len(memberships) == 0 {
		return xerrors.Errorf("user %s is not a member of the default organization, cannot assign to groups in the org", params.UserID)
	}

	userGroups, err := tx.GetGroups(ctx, database.GetGroupsParams{
		OrganizationID: defaultOrganization.ID,
		HasMemberID:    params.UserID,
	})

	userGroupNames := db2sdk.List(userGroups, func(g database.Group) string {
		return g.Name
	})

	// Optimize for the case the user is in the correct groups, since
	// group membership is mostly unchanging.
	add, remove := slice.SymmetricDifference(userGroupNames, wantGroups)
	if len(add) == 0 && len(remove) == 0 {
		// Add done, the user is in all the correct groups! Do not waste any more db
		// calls.
		return nil
	}

	// We could only insert the user to missing groups, and remove them from the excess.
	// But that is at minimum 1 db call, and it's only 2 if we delete them from all groups,
	// then re-add them to the correct groups. So we just do the latter.

	// Just remove the user from all their groups, then add them back in.
	err = tx.RemoveUserFromAllGroups(ctx, database.RemoveUserFromAllGroupsParams{
		UserID:         params.UserID,
		OrganizationID: defaultOrganization.ID,
	})
	if err != nil {
		return xerrors.Errorf("delete user groups: %w", err)
	}

	if params.CreateMissingGroups {
		created, err := tx.InsertMissingGroups(ctx, database.InsertMissingGroupsParams{
			OrganizationID: defaultOrganization.ID,
			GroupNames:     wantGroups,
			Source:         database.GroupSourceOidc,
		})
		if err != nil {
			return xerrors.Errorf("insert missing groups: %w", err)
		}
		if len(created) > 0 {
			logger.Debug(ctx, "auto created missing groups",
				slog.F("org_id", defaultOrganization.ID.ID),
				slog.F("created", created),
				slog.F("num", len(created)),
			)
		}
	}

	err = tx.InsertUserGroupsByName(ctx, database.InsertUserGroupsByNameParams{
		UserID:         params.UserID,
		OrganizationID: defaultOrganization.ID,
		GroupNames:     wantGroups,
	})
	if err != nil {
		return xerrors.Errorf("insert user groups: %w", err)
	}

	return nil
}
