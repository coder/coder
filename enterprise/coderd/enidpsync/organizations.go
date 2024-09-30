package enidpsync

import (
	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func (e EnterpriseIDPSync) OrganizationSyncEnabled() bool {
	return e.entitlements.Enabled(codersdk.FeatureMultipleOrganizations) && e.OrganizationField != ""
}

func (e EnterpriseIDPSync) ParseOrganizationClaims(ctx context.Context, mergedClaims jwt.MapClaims) (idpsync.OrganizationParams, *idpsync.HTTPError) {
	if !e.OrganizationSyncEnabled() {
		// Default to agpl if multi-org is not enabled
		return e.AGPLIDPSync.ParseOrganizationClaims(ctx, mergedClaims)
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)
	userOrganizations := make([]uuid.UUID, 0)

	// Pull extra organizations from the claims.
	if e.OrganizationField != "" {
		organizationRaw, ok := mergedClaims[e.OrganizationField]
		if ok {
			parsedOrganizations, err := idpsync.ParseStringSliceClaim(organizationRaw)
			if err != nil {
				return idpsync.OrganizationParams{}, &idpsync.HTTPError{
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
				if mappedOrganization, ok := e.OrganizationMapping[parsedOrg]; ok {
					// parsedOrg is in the mapping, so add the mapped organizations to the
					// user's organizations.
					userOrganizations = append(userOrganizations, mappedOrganization...)
				} else {
					ignored = append(ignored, parsedOrg)
				}
			}

			e.Logger.Debug(ctx, "parsed organizations from claim",
				slog.F("len", len(parsedOrganizations)),
				slog.F("ignored", ignored),
				slog.F("organizations", parsedOrganizations),
			)
		}
	}

	return idpsync.OrganizationParams{
		// If the field is not set, then sync is not enabled.
		SyncEnabled:    e.OrganizationField != "",
		IncludeDefault: e.OrganizationAssignDefault,
		// Do not return duplicates
		Organizations: slice.Unique(userOrganizations),
	}, nil
}
