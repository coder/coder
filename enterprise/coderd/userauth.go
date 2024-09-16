package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) setUserSiteRoles(ctx context.Context, logger slog.Logger, db database.Store, userID uuid.UUID, roles []string) error {
	if !api.Entitlements.Enabled(codersdk.FeatureUserRoleManagement) {
		logger.Warn(ctx, "attempted to assign OIDC user roles without enterprise entitlement, roles left unchanged",
			slog.F("user_id", userID), slog.F("roles", roles),
		)
		return nil
	}

	// Should this be feature protected?
	return db.InTx(func(tx database.Store) error {
		_, err := db.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
			GrantedRoles: roles,
			ID:           userID,
		})
		if err != nil {
			return xerrors.Errorf("set user roles(%s): %w", userID.String(), err)
		}

		return nil
	}, nil)
}
