package idpsync

import (
	"context"
	"encoding/json"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

type RoleParams struct {
	// SyncEnabled if false will skip syncing the user's roles
	SyncEnabled   bool
	SyncSiteWide  bool
	SiteWideRoles []string
	// MergedClaims are passed to the organization level for syncing
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
		SyncEnabled:  s.RoleSyncEnabled(),
		SyncSiteWide: false,
	}, nil
}

func (s AGPLIDPSync) SyncRoles(ctx context.Context, db database.Store, user database.User, params RoleParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEnabled {
		return nil
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	err := db.InTx(func(tx database.Store) error {
		if params.SyncSiteWide {
			if err := s.syncSiteWideRoles(ctx, tx, user, params); err != nil {
				return err
			}
		}

		// sync roles per organization
		orgMemberships, err := tx.OrganizationMembers(ctx, database.OrganizationMembersParams{
			UserID: user.ID,
		})
		if err != nil {
			return xerrors.Errorf("get organizations by user id: %w", err)
		}

		// Sync for each organization
		// If a key for a given org exists in the map, the user's roles will be
		// updated to the value of that key.
		expectedRoles := make(map[uuid.UUID][]rbac.RoleIdentifier)
		existingRoles := make(map[uuid.UUID][]string)
		allExpected := make([]rbac.RoleIdentifier, 0)
		for _, member := range orgMemberships {
			orgID := member.OrganizationMember.OrganizationID
			orgResolver := s.Manager.OrganizationResolver(tx, orgID)
			settings, err := s.RoleSyncSettings().Resolve(ctx, orgResolver)
			if err != nil {
				if !xerrors.Is(err, runtimeconfig.ErrEntryNotFound) {
					return xerrors.Errorf("resolve group sync settings: %w", err)
				}
				// No entry means no role syncing for this organization
				continue
			}
			if settings.Field == "" {
				// Explicitly disabled role sync for this organization
				continue
			}

			existingRoles[orgID] = member.OrganizationMember.Roles
			orgRoleClaims, err := s.RolesFromClaim(settings.Field, params.MergedClaims)
			if err != nil {
				s.Logger.Error(ctx, "failed to parse roles from claim",
					slog.F("field", settings.Field),
					slog.F("organization_id", orgID),
					slog.F("user_id", user.ID),
					slog.F("username", user.Username),
					slog.Error(err),
				)

				// Failing role sync should reset a user's roles.
				expectedRoles[orgID] = []rbac.RoleIdentifier{}

				// Do not return an error, because that would prevent a user
				// from logging in. A misconfigured organization should not
				// stop a user from logging into the site.
				continue
			}

			expected := make([]rbac.RoleIdentifier, 0, len(orgRoleClaims))
			for _, role := range orgRoleClaims {
				if mappedRoles, ok := settings.Mapping[role]; ok {
					for _, mappedRole := range mappedRoles {
						expected = append(expected, rbac.RoleIdentifier{OrganizationID: orgID, Name: mappedRole})
					}
					continue
				}
				expected = append(expected, rbac.RoleIdentifier{OrganizationID: orgID, Name: role})
			}

			expectedRoles[orgID] = expected
			allExpected = append(allExpected, expected...)
		}

		// Now mass sync the user's org membership roles.
		validRoles, err := rolestore.Expand(ctx, tx, allExpected)
		if err != nil {
			return xerrors.Errorf("expand roles: %w", err)
		}
		validMap := make(map[string]struct{}, len(validRoles))
		for _, validRole := range validRoles {
			validMap[validRole.Identifier.UniqueName()] = struct{}{}
		}

		// For each org, do the SQL query to update the user's roles.
		// TODO: Would be better to batch all these into a single SQL query.
		for orgID, roles := range expectedRoles {
			validExpected := make([]string, 0, len(roles))
			for _, role := range roles {
				if _, ok := validMap[role.UniqueName()]; ok {
					validExpected = append(validExpected, role.Name)
				}
			}
			// Always add the member role to the user.
			validExpected = append(validExpected, rbac.RoleOrgMember())

			// Is there a difference between the expected roles and the existing roles?
			if !slices.Equal(existingRoles[orgID], validExpected) {
				_, err = tx.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
					GrantedRoles: validExpected,
					UserID:       user.ID,
					OrgID:        orgID,
				})
				if err != nil {
					return xerrors.Errorf("update member roles(%s): %w", user.ID.String(), err)
				}
			}
		}
		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("sync user roles(%s): %w", user.ID.String(), err)
	}

	return nil
}

// resetUserOrgRoles will reset the user's roles for a specific organization.
// It does not remove them as a member from the organization.
func (s AGPLIDPSync) resetUserOrgRoles(ctx context.Context, tx database.Store, member database.OrganizationMembersRow, orgID uuid.UUID) error {
	withoutMember := slices.DeleteFunc(member.OrganizationMember.Roles, func(s string) bool {
		return s == rbac.RoleOrgMember()
	})
	// If the user has no roles, then skip doing any database request.
	if len(withoutMember) == 0 {
		return nil
	}

	_, err := tx.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: []string{},
		UserID:       member.OrganizationMember.UserID,
		OrgID:        orgID,
	})
	if err != nil {
		return xerrors.Errorf("zero out member roles(%s): %w", member.OrganizationMember.UserID.String(), err)
	}
	return nil
}

func (s AGPLIDPSync) syncSiteWideRoles(ctx context.Context, tx database.Store, user database.User, params RoleParams) error {
	// Apply site wide roles to a user.
	// ignored is the list of roles that are not valid Coder roles and will
	// be skipped.
	ignored := make([]string, 0)
	filtered := make([]string, 0, len(params.SiteWideRoles))
	for _, role := range params.SiteWideRoles {
		// Because we are only syncing site wide roles, we intentionally will always
		// omit 'OrganizationID' from the RoleIdentifier.
		if _, err := rbac.RoleByName(rbac.RoleIdentifier{Name: role}); err == nil {
			filtered = append(filtered, role)
		} else {
			ignored = append(ignored, role)
		}
	}
	if len(ignored) > 0 {
		s.Logger.Debug(ctx, "OIDC roles ignored in assignment",
			slog.F("ignored", ignored),
			slog.F("assigned", filtered),
			slog.F("user_id", user.ID),
			slog.F("username", user.Username),
		)
	}

	_, err := tx.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
		GrantedRoles: filtered,
		ID:           user.ID,
	})
	if err != nil {
		return xerrors.Errorf("set site wide roles: %w", err)
	}
	return nil
}

func (s AGPLIDPSync) RolesFromClaim(field string, claims jwt.MapClaims) ([]string, error) {
	rolesRow, ok := claims[field]
	if !ok {
		// If no claim is provided than we can assume the user is just
		// a member. This is because there is no way to tell the difference
		// between []string{} and nil for OIDC claims. IDPs omit claims
		// if they are empty ([]string{}).
		// Use []interface{}{} so the next typecast works.
		rolesRow = []interface{}{}
	}

	parsedRoles, err := ParseStringSliceClaim(rolesRow)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse roles from claim: %w", err)
	}

	return parsedRoles, nil
}

type RoleSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// groups. If the group field is the empty string, then no group updates
	// will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping maps from an OIDC group --> Coder organization role
	Mapping map[string][]string `json:"mapping"`
}

func (s *RoleSyncSettings) Set(v string) error {
	return json.Unmarshal([]byte(v), s)
}

func (s *RoleSyncSettings) String() string {
	return runtimeconfig.JSONString(s)
}
