package enidpsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func (e EnterpriseIDPSync) RoleSyncEntitled() bool {
	return e.entitlements.Enabled(codersdk.FeatureUserRoleManagement)
}

func (e EnterpriseIDPSync) OrganizationRoleSyncEnabled(ctx context.Context, db database.Store, orgID uuid.UUID) (bool, error) {
	if !e.RoleSyncEntitled() {
		return false, nil
	}
	roleSyncSettings, err := e.Role.Resolve(ctx, e.Manager.OrganizationResolver(db, orgID))
	if err != nil {
		if errors.Is(err, runtimeconfig.ErrEntryNotFound) {
			return false, nil
		}
		return false, err
	}
	return roleSyncSettings.Field != "", nil
}

func (e EnterpriseIDPSync) SiteRoleSyncEnabled() bool {
	if !e.RoleSyncEntitled() {
		return false
	}
	return e.AGPLIDPSync.SiteRoleField != ""
}

func (e EnterpriseIDPSync) ParseRoleClaims(ctx context.Context, mergedClaims jwt.MapClaims) (idpsync.RoleParams, *idpsync.HTTPError) {
	if !e.RoleSyncEntitled() {
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
			// TODO: Determine a static page or not
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
		SyncEntitled:  e.RoleSyncEntitled(),
		SyncSiteWide:  e.SiteRoleSyncEnabled(),
		SiteWideRoles: slice.Unique(siteRoles),
		MergedClaims:  mergedClaims,
	}, nil
}
