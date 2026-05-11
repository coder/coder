package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// createExternalAuthHeaderUser provisions a Coder user asserted by the
// Coder-Authorization header (see httpmw.ExternalAuthHeaderConfig).
// It is wired as the CreateUser callback in coderd.New so the httpmw
// package can request a user create without importing coderd.
//
// The flow mirrors the OIDC auto-create path in oauthLogin: look up
// the default organization, validate every role, then defer to
// (*API).CreateUser which handles InTx semantics, SSH key generation,
// org membership, and admin notifications.
//
// SECURITY: callers must have already validated that the asserting
// request came from a trusted origin. This method does not re-check
// that.
func (api *API) createExternalAuthHeaderUser(ctx context.Context, params httpmw.ExternalAuthHeaderCreateUserParams) (database.User, error) {
	if params.Email == "" {
		return database.User{}, xerrors.New("UserEmail is required to auto-create users")
	}
	if params.Username == "" {
		return database.User{}, xerrors.New("Username could not be derived; supply Username= in the header or use an email with a valid local part")
	}

	roles, err := validateExternalAuthHeaderRoles(params.Roles)
	if err != nil {
		return database.User{}, err
	}

	//nolint:gocritic // System needs to read the default organization
	// for any unauthenticated identity assertion.
	defaultOrg, err := api.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return database.User{}, xerrors.Errorf("fetch default organization: %w", err)
	}

	//nolint:gocritic // System creates the user on behalf of the gateway.
	user, err := api.CreateUser(dbauthz.AsSystemRestricted(ctx), api.Database, CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Email:           params.Email,
			Username:        params.Username,
			Name:            params.Name,
			OrganizationIDs: []uuid.UUID{defaultOrg.ID},
			UserStatus:      ptr.Ref(codersdk.UserStatusActive),
		},
		LoginType:          database.LoginTypeNone,
		accountCreatorName: "external-auth-header",
		RBACRoles:          roles,
	})
	if err != nil {
		return database.User{}, xerrors.Errorf("create user: %w", err)
	}
	api.Logger.Info(ctx, "auto-created user via external authentication header",
		slog.F("user_id", user.ID),
		slog.F("username", user.Username),
		slog.F("email", user.Email),
		slog.F("roles", roles),
	)
	return user, nil
}

// validateExternalAuthHeaderRoles ensures every role supplied by the
// gateway (either via the header's Roles= field or the deployment's
// default-role list) is a real built-in role. Custom roles are
// rejected because they may not exist until the operator creates
// them; we prefer a clear error over silently dropping unrecognized
// names.
func validateExternalAuthHeaderRoles(roles []string) ([]string, error) {
	if len(roles) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if _, err := rbac.RoleByName(rbac.RoleIdentifier{Name: r}); err != nil {
			return nil, xerrors.Errorf("invalid role %q: %w", r, err)
		}
		out = append(out, r)
	}
	return out, nil
}
