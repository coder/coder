package idpsync

import (
	"context"

	"github.com/golang-jwt/jwt/v4"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

type RoleParams struct {
	// SyncEnabled if false will skip syncing the user's roles
	SyncEnabled  bool
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) RoleSyncEnabled() bool {
	// AGPL does not support syncing groups.
	return false
}
func (s AGPLIDPSync) RoleSyncSettings() runtimeconfig.RuntimeEntry[*RoleSyncSettings] {
	return s.Role
}

func (s AGPLIDPSync) ParseRoleClaims(_ context.Context, _ jwt.MapClaims) (RoleParams, *HTTPError) {
	return RoleParams{
		SyncEnabled: s.RoleSyncEnabled(),
	}, nil
}

func (s AGPLIDPSync) SyncRoles(ctx context.Context, db database.Store, user database.User, params RoleParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEnabled {
		return nil
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	return nil
}

type RoleSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// groups. If the group field is the empty string, then no group updates
	// will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping maps from an OIDC group --> Coder organization role
	Mapping map[string][]string `json:"mapping"`
}
