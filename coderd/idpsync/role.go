package idpsync

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

type RoleParams struct {
	// SyncEntitled if false will skip syncing the user's roles at
	// all levels.
	SyncEntitled  bool
	SyncSiteWide  bool
	SiteWideRoles []string
	// MergedClaims are passed to the organization level for syncing
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) RoleSyncEntitled() bool {
	// AGPL does not support syncing groups.
	return false
}

func (AGPLIDPSync) OrganizationRoleSyncEnabled(_ context.Context, _ database.Store, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (AGPLIDPSync) SiteRoleSyncEnabled() bool {
	return false
}

func (s AGPLIDPSync) UpdateRoleSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store, settings RoleSyncSettings) error {
	orgResolver := s.Manager.OrganizationResolver(db, orgID)
	err := s.SyncSettings.Role.SetRuntimeValue(ctx, orgResolver, &settings)
	if err != nil {
		return xerrors.Errorf("update role sync settings: %w", err)
	}

	return nil
}

func (s AGPLIDPSync) RoleSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store) (*RoleSyncSettings, error) {
	rlv := s.Manager.OrganizationResolver(db, orgID)
	settings, err := s.Role.Resolve(ctx, rlv)
	if err != nil {
		if !errors.Is(err, runtimeconfig.ErrEntryNotFound) {
			return nil, xerrors.Errorf("resolve role sync settings: %w", err)
		}
		return &RoleSyncSettings{}, nil
	}
	return settings, nil
}

func (s AGPLIDPSync) ParseRoleClaims(_ context.Context, _ jwt.MapClaims) (RoleParams, *HTTPError) {
	return RoleParams{
		SyncEntitled: s.RoleSyncEntitled(),
		SyncSiteWide: s.SiteRoleSyncEnabled(),
	}, nil
}

func (s AGPLIDPSync) SyncRoles(ctx context.Context, db database.Store, user database.User, params RoleParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEntitled {
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
			OrganizationID: uuid.Nil,
			UserID:         user.ID,
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
			settings, err := s.RoleSyncSettings(ctx, orgID, tx)
			if err != nil {
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

				// TODO: If rolesync fails, we might want to reset a user's
				// roles to prevent stale roles from existing.
				// Eg: `expectedRoles[orgID] = []rbac.RoleIdentifier{}`
				// However, implementing this could lock an org admin out
				// of fixing their configuration.
				// There is also no current method to notify an org admin of
				// a configuration issue.
				// So until org admins can be notified of configuration issues,
				// and they will not be locked out, this code will do nothing to
				// the user's roles.

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
			// Ignore the implied member role
			validExpected = slices.DeleteFunc(validExpected, func(s string) bool {
				return s == rbac.RoleOrgMember()
			})

			existingFound := existingRoles[orgID]
			existingFound = slices.DeleteFunc(existingFound, func(s string) bool {
				return s == rbac.RoleOrgMember()
			})

			// Only care about unique roles. So remove all duplicates
			existingFound = slice.Unique(existingFound)
			validExpected = slice.Unique(validExpected)
			// A sort is required for the equality check
			slices.Sort(existingFound)
			slices.Sort(validExpected)
			// Is there a difference between the expected roles and the existing roles?
			if !slices.Equal(existingFound, validExpected) {
				// TODO: Write a unit test to verify we do no db call on no diff
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

func (s AGPLIDPSync) syncSiteWideRoles(ctx context.Context, tx database.Store, user database.User, params RoleParams) error {
	// Apply site wide roles to a user.
	// ignored is the list of roles that are not valid Coder roles and will
	// be skipped.
	ignored := make([]string, 0)
	filtered := make([]string, 0, len(params.SiteWideRoles))
	for _, role := range params.SiteWideRoles {
		// Because we are only syncing site wide roles, we intentionally will always
		// omit 'OrganizationID' from the RoleIdentifier.
		// TODO: If custom site wide roles are introduced, this needs to use the
		// database to verify the role exists.
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

	filtered = slice.Unique(filtered)
	slices.Sort(filtered)

	existing := slice.Unique(user.RBACRoles)
	slices.Sort(existing)
	if !slices.Equal(existing, filtered) {
		_, err := tx.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
			GrantedRoles: filtered,
			ID:           user.ID,
		})
		if err != nil {
			return xerrors.Errorf("set site wide roles: %w", err)
		}
	}
	return nil
}

func (AGPLIDPSync) RolesFromClaim(field string, claims jwt.MapClaims) ([]string, error) {
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

type RoleSyncSettings codersdk.RoleSyncSettings

func (s *RoleSyncSettings) Set(v string) error {
	return json.Unmarshal([]byte(v), s)
}

func (s *RoleSyncSettings) String() string {
	return runtimeconfig.JSONString(s)
}
