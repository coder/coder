package idpsync

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type OrganizationParams struct {
	// SyncEntitled if false will skip syncing the user's organizations.
	SyncEntitled bool
	// MergedClaims are passed to the organization level for syncing
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) OrganizationSyncEntitled() bool {
	// AGPL does not support syncing organizations.
	return false
}

func (AGPLIDPSync) OrganizationSyncEnabled(_ context.Context, _ database.Store) bool {
	return false
}

func (s AGPLIDPSync) UpdateOrganizationSyncSettings(ctx context.Context, db database.Store, settings OrganizationSyncSettings) error {
	rlv := s.Manager.Resolver(db)
	err := s.SyncSettings.Organization.SetRuntimeValue(ctx, rlv, &settings)
	if err != nil {
		return xerrors.Errorf("update organization sync settings: %w", err)
	}

	return nil
}

func (s AGPLIDPSync) OrganizationSyncSettings(ctx context.Context, db database.Store) (*OrganizationSyncSettings, error) {
	// If this logic is ever updated, make sure to update the corresponding
	// checkIDPOrgSync in coderd/telemetry/telemetry.go.
	rlv := s.Manager.Resolver(db)
	orgSettings, err := s.SyncSettings.Organization.Resolve(ctx, rlv)
	if err != nil {
		if !xerrors.Is(err, runtimeconfig.ErrEntryNotFound) {
			return nil, xerrors.Errorf("resolve org sync settings: %w", err)
		}

		// Default to the statically assigned settings if they exist.
		orgSettings = &OrganizationSyncSettings{
			Field:         s.DeploymentSyncSettings.OrganizationField,
			Mapping:       s.DeploymentSyncSettings.OrganizationMapping,
			AssignDefault: s.DeploymentSyncSettings.OrganizationAssignDefault,
		}
	}
	return orgSettings, nil
}

func (s AGPLIDPSync) ParseOrganizationClaims(_ context.Context, claims jwt.MapClaims) (OrganizationParams, *HTTPError) {
	// For AGPL we only sync the default organization.
	return OrganizationParams{
		SyncEntitled: s.OrganizationSyncEntitled(),
		MergedClaims: claims,
	}, nil
}

// SyncOrganizations if enabled will ensure the user is a member of the provided
// organizations. It will add and remove their membership to match the expected set.
func (s AGPLIDPSync) SyncOrganizations(ctx context.Context, tx database.Store, user database.User, params OrganizationParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEntitled {
		return nil
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	orgSettings, err := s.OrganizationSyncSettings(ctx, tx)
	if err != nil {
		return xerrors.Errorf("failed to get org sync settings: %w", err)
	}

	if orgSettings.Field == "" {
		return nil // No sync configured, nothing to do
	}

	expectedOrgIDs, err := orgSettings.ParseClaims(ctx, tx, params.MergedClaims)
	if err != nil {
		return xerrors.Errorf("organization claims: %w", err)
	}

	// Fetch all organizations, even deleted ones. This is to remove a user
	// from any deleted organizations they may be in.
	existingOrgs, err := tx.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  user.ID,
		Deleted: sql.NullBool{},
	})
	if err != nil {
		return xerrors.Errorf("failed to get user organizations: %w", err)
	}

	existingOrgIDs := db2sdk.List(existingOrgs, func(org database.Organization) uuid.UUID {
		return org.ID
	})

	// finalExpected is the final set of org ids the user is expected to be in.
	// Deleted orgs are omitted from this set.
	finalExpected := expectedOrgIDs
	if len(expectedOrgIDs) > 0 {
		// If you pass in an empty slice to the db arg, you get all orgs. So the slice
		// has to be non-empty to get the expected set. Logically it also does not make
		// sense to fetch an empty set from the db.
		expectedOrganizations, err := tx.GetOrganizations(ctx, database.GetOrganizationsParams{
			IDs: expectedOrgIDs,
			// Do not include deleted organizations. Omitting deleted orgs will remove the
			// user from any deleted organizations they are a member of.
			Deleted: false,
		})
		if err != nil {
			return xerrors.Errorf("failed to get expected organizations: %w", err)
		}
		finalExpected = db2sdk.List(expectedOrganizations, func(org database.Organization) uuid.UUID {
			return org.ID
		})
	}

	// Find the difference in the expected and the existing orgs, and
	// correct the set of orgs the user is a member of.
	add, remove := slice.SymmetricDifference(existingOrgIDs, finalExpected)
	// notExists is purely for debugging. It logs when the settings want
	// a user in an organization, but the organization does not exist.
	notExists := slice.DifferenceFunc(expectedOrgIDs, finalExpected, func(a, b uuid.UUID) bool {
		return a == b
	})
	for _, orgID := range add {
		_, err := tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{},
		})
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				// This should not happen because we check the org existence
				// beforehand.
				notExists = append(notExists, orgID)
				continue
			}

			if database.IsUniqueViolation(err, database.UniqueOrganizationMembersPkey) {
				// If we hit this error we have a bug. The user already exists in the
				// organization, but was not detected to be at the start of this function.
				// Instead of failing the function, an error will be logged. This is to not bring
				// down the entire syncing behavior from a single failed org. Failing this can
				// prevent user logins, so only fatal non-recoverable errors should be returned.
				//
				// Inserting a user is privilege escalation. So skipping this instead of failing
				// leaves the user with fewer permissions. So this is safe from a security
				// perspective to continue.
				s.Logger.Error(ctx, "syncing user to organization failed as they are already a member, please report this failure to Coder",
					slog.F("user_id", user.ID),
					slog.F("username", user.Username),
					slog.F("organization_id", orgID),
					slog.Error(err),
				)
				continue
			}
			return xerrors.Errorf("add user to organization: %w", err)
		}
	}

	for _, orgID := range remove {
		err := tx.DeleteOrganizationMember(ctx, database.DeleteOrganizationMemberParams{
			OrganizationID: orgID,
			UserID:         user.ID,
		})
		if err != nil {
			return xerrors.Errorf("remove user from organization: %w", err)
		}
	}

	if len(notExists) > 0 {
		notExists = slice.Unique(notExists) // Remove dupes
		s.Logger.Debug(ctx, "organizations do not exist but attempted to use in org sync",
			slog.F("not_found", notExists),
			slog.F("user_id", user.ID),
			slog.F("username", user.Username),
		)
	}
	return nil
}

type OrganizationSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// organizations. If the field is the empty string, then no organization updates
	// will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping controls how organizations returned by the OIDC provider get mapped
	Mapping map[string][]uuid.UUID `json:"mapping"`
	// AssignDefault will ensure all users that authenticate will be
	// placed into the default organization. This is mostly a hack to support
	// legacy deployments.
	AssignDefault bool `json:"assign_default"`
}

func (s *OrganizationSyncSettings) Set(v string) error {
	return json.Unmarshal([]byte(v), s)
}

func (s *OrganizationSyncSettings) String() string {
	return runtimeconfig.JSONString(s)
}

// ParseClaims will parse the claims and return the list of organizations the user
// should sync to.
func (s *OrganizationSyncSettings) ParseClaims(ctx context.Context, db database.Store, mergedClaims jwt.MapClaims) ([]uuid.UUID, error) {
	userOrganizations := make([]uuid.UUID, 0)

	if s.AssignDefault {
		// This is a bit hacky, but if AssignDefault is included, then always
		// make sure to include the default org in the list of expected.
		defaultOrg, err := db.GetDefaultOrganization(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed to get default organization: %w", err)
		}

		// Always include default org.
		userOrganizations = append(userOrganizations, defaultOrg.ID)
	}

	organizationRaw, ok := mergedClaims[s.Field]
	if !ok {
		return userOrganizations, nil
	}

	parsedOrganizations, err := ParseStringSliceClaim(organizationRaw)
	if err != nil {
		return userOrganizations, xerrors.Errorf("failed to parese organizations OIDC claims: %w", err)
	}

	// add any mapped organizations
	for _, parsedOrg := range parsedOrganizations {
		if mappedOrganization, ok := s.Mapping[parsedOrg]; ok {
			// parsedOrg is in the mapping, so add the mapped organizations to the
			// user's organizations.
			userOrganizations = append(userOrganizations, mappedOrganization...)
		}
	}

	// Deduplicate the organizations
	return slice.Unique(userOrganizations), nil
}
