package enidpsync

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v4"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func (e EnterpriseIDPSync) RoleSyncEnabled() bool {
	return e.entitlements.Enabled(codersdk.FeatureUserRoleManagement)
}

func (e EnterpriseIDPSync) ParseRoleClaims(ctx context.Context, mergedClaims jwt.MapClaims) (idpsync.RoleParams, *idpsync.HTTPError) {
	if !e.RoleSyncEnabled() {
		return e.AGPLIDPSync.ParseRoleClaims(ctx, mergedClaims)
	}

	var claimRoles []string
	if e.AGPLIDPSync.SiteRoleField != "" {
		var err error
		// TODO: Smoke test this error for org and site
		claimRoles, err = e.AGPLIDPSync.RolesFromClaim(e.AGPLIDPSync.SiteRoleField, mergedClaims)
		if err != nil {
			rawType := mergedClaims[e.AGPLIDPSync.SiteRoleField]
			e.Logger.Error(ctx, "oidc claims user roles field was an unknown type",
				slog.F("type", fmt.Sprintf("%T", rawType)),
				slog.F("field", e.AGPLIDPSync.SiteRoleField),
				slog.F("raw_value", rawType),
				slog.Error(err),
			)
			// TODO: Deterine a static page or not
			return idpsync.RoleParams{}, &idpsync.HTTPError{
				Code:             http.StatusInternalServerError,
				Msg:              "Login disabled until site wide OIDC config is fixed",
				Detail:           fmt.Sprintf("Roles claim must be an array of strings, type found: %T. Disabling role sync will allow login to proceed.", rawType),
				RenderStaticPage: false,
			}
		}
	}

	siteRoles := append([]string{}, e.SiteDefaultRoles...)
	for _, role := range claimRoles {
		if mappedRoles, ok := e.SiteRoleMapping[role]; ok {
			if len(mappedRoles) == 0 {
				continue
			}
			// Mapped roles are added to the list of roles
			siteRoles = append(siteRoles, mappedRoles...)
			continue
		}
		// Append as is.
		siteRoles = append(siteRoles, role)
	}

	return idpsync.RoleParams{
		SyncEnabled:   e.RoleSyncEnabled(),
		SyncSiteWide:  e.AGPLIDPSync.SiteRoleField != "",
		SiteWideRoles: slice.Unique(siteRoles),
		MergedClaims:  mergedClaims,
	}, nil
}
