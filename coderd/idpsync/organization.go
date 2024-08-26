package idpsync

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/slice"
)

func (s IDPSync) ParseOrganizationClaims(ctx context.Context, mergedClaims map[string]interface{}) (OrganizationParams, *HttpError) {
	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Copy in the always included static set of organizations.
	userOrganizations := make([]uuid.UUID, len(s.OrganizationAlwaysAssign))
	copy(userOrganizations, s.OrganizationAlwaysAssign)

	// Pull extra organizations from the claims.
	if s.OrganizationField != "" {
		organizationRaw, ok := mergedClaims[s.OrganizationField]
		if ok {
			parsedOrganizations, err := ParseStringSliceClaim(organizationRaw)
			if err != nil {
				return OrganizationParams{}, &HttpError{
					Code:                 http.StatusBadRequest,
					Msg:                  "Failed to sync organizations from the OIDC claims",
					Detail:               err.Error(),
					RenderStaticPage:     false,
					RenderDetailMarkdown: false,
				}
			}

			// Keep track of which claims are not mapped for debugging purposes.
			var ignored []string
			for _, parsedOrg := range parsedOrganizations {
				if mappedOrganization, ok := s.OrganizationMapping[parsedOrg]; ok {
					// parsedOrg is in the mapping, so add the mapped organizations to the
					// user's organizations.
					userOrganizations = append(userOrganizations, mappedOrganization...)
				} else {
					ignored = append(ignored, parsedOrg)
				}
			}

			s.logger.Debug(ctx, "parsed organizations from claim",
				slog.F("len", len(parsedOrganizations)),
				slog.F("ignored", ignored),
				slog.F("organizations", parsedOrganizations),
			)
		}
	}

	return OrganizationParams{
		Organizations: userOrganizations,
	}, nil
}

type OrganizationParams struct {
	// Organizations is the list of organizations the user should be a member of
	// assuming syncing is turned on.
	Organizations []uuid.UUID
}

func (s IDPSync) SyncOrganizations(ctx context.Context, tx database.Store, user database.User, params OrganizationParams) error {
	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	existingOrgs, err := tx.GetOrganizationsByUserID(ctx, user.ID)
	if err != nil {
		return xerrors.Errorf("failed to get user organizations: %w", err)
	}

	existingOrgIDs := db2sdk.List(existingOrgs, func(org database.Organization) uuid.UUID {
		return org.ID
	})

	// Find the difference in the expected and the existing orgs, and
	// correct the set of orgs the user is a member of.
	add, remove := slice.SymmetricDifference(existingOrgIDs, params.Organizations)
	notExists := make([]uuid.UUID, 0)
	for _, orgID := range add {
		//nolint:gocritic // System actor being used to assign orgs
		_, err := tx.InsertOrganizationMember(dbauthz.AsSystemRestricted(ctx), database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{},
		})
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				notExists = append(notExists, orgID)
				continue
			}
			return xerrors.Errorf("add user to organization: %w", err)
		}
	}

	for _, orgID := range remove {
		//nolint:gocritic // System actor being used to assign orgs
		err := tx.DeleteOrganizationMember(dbauthz.AsSystemRestricted(ctx), database.DeleteOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user.ID,
		})
		if err != nil {
			return xerrors.Errorf("remove user from organization: %w", err)
		}
	}

	if len(notExists) > 0 {
		s.logger.Debug(ctx, "organizations do not exist but attempted to use in org sync",
			slog.F("not_found", notExists),
			slog.F("user_id", user.ID),
			slog.F("username", user.Username),
		)
	}
	return nil
}
