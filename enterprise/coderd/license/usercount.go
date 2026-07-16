package license

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

// CountWorkspaceCapableUsers returns the number of active users the RBAC
// engine authorizes to create a workspace, either in one of the
// organizations they belong to or in any organization via a site-wide
// role such as owner. Users without workspace-create capability ("gateway
// accounts") do not consume license seats. System users and service
// accounts are excluded by the underlying query, matching
// GetActiveUserCount.
func CountWorkspaceCapableUsers(ctx context.Context, logger slog.Logger, db database.Store, authorizer rbac.Authorizer) (int64, error) {
	//nolint:gocritic // Counting licensed seats is a system function.
	rows, err := db.GetActiveUsersAuthorizationRoles(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return 0, xerrors.Errorf("get active users authorization roles: %w", err)
	}

	// Users with identical role sets always produce the same authorization
	// outcome: the subject ID and the object owner are the same user in
	// every check, and the objects carry no ACLs, so group membership
	// (which only influences authorization through object ACL matching)
	// cannot change the result. Deduplicate on the role signature so
	// evaluation cost scales with the number of unique role sets, not the
	// number of users. TestWorkspaceCreateIgnoresGroups enforces the
	// group-independence assumption; if it ever breaks, groups must be
	// added back to the subject, the signature, and the query.
	capableBySignature := make(map[string]bool)
	var count int64
	for _, row := range rows {
		sig := authorizationSignature(row)
		capable, ok := capableBySignature[sig]
		if !ok {
			capable, err = canCreateWorkspace(ctx, logger, db, authorizer, row)
			if err != nil {
				return 0, xerrors.Errorf("evaluate workspace-create for user %s: %w", row.ID, err)
			}
			capableBySignature[sig] = capable
		}
		if capable {
			count++
		}
	}
	return count, nil
}

// canCreateWorkspace reports whether the RBAC engine authorizes the user
// to create a workspace they own, checked against every organization the
// user is a member of plus the any-organization form that site-wide roles
// satisfy regardless of org membership.
func canCreateWorkspace(ctx context.Context, logger slog.Logger, db database.Store, authorizer rbac.Authorizer, row database.GetActiveUsersAuthorizationRolesRow) (bool, error) {
	roleNames, err := row.RoleNames()
	if err != nil {
		// A stored role string that fails to parse grants nothing:
		// authorization fails closed on it, so this user cannot create a
		// workspace. Treat the user as not capable rather than returning
		// the error, which would fail the count for every user over one
		// bad row. Role-signature dedupe means this logs once per unique
		// role set, not once per user sharing it.
		logger.Warn(ctx, "user has an unparseable role, counting them as not workspace-capable for license seats",
			slog.F("user_id", row.ID),
			slog.Error(err),
		)
		return false, nil
	}

	//nolint:gocritic // Expanding custom roles requires system access.
	roles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, roleNames)
	if err != nil {
		return false, xerrors.Errorf("expand roles: %w", err)
	}

	subject := rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    row.ID.String(),
		Roles: roles,
		// Groups are deliberately omitted: they only influence
		// authorization through object ACL matching, and the objects
		// below carry no ACLs. Keeping them off the subject keeps the
		// role-only dedupe signature honest.
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	// Site-wide grants (e.g. the owner role) authorize workspace creation
	// in any organization, independent of org membership. This also covers
	// users who belong to zero organizations.
	if authorizer.Authorize(ctx, subject, policy.ActionCreate,
		rbac.ResourceWorkspace.AnyOrganization().WithOwner(subject.ID)) == nil {
		return true, nil
	}

	seen := make(map[uuid.UUID]struct{})
	for _, role := range roleNames {
		orgID := role.OrganizationID
		if orgID == uuid.Nil {
			continue
		}
		if _, ok := seen[orgID]; ok {
			continue
		}
		seen[orgID] = struct{}{}
		if authorizer.Authorize(ctx, subject, policy.ActionCreate,
			rbac.ResourceWorkspace.InOrg(orgID).WithOwner(subject.ID)) == nil {
			return true, nil
		}
	}
	return false, nil
}

// authorizationSignature returns a canonical key for the user's role set.
// Two users with equal signatures are interchangeable for
// workspace-create evaluation.
func authorizationSignature(row database.GetActiveUsersAuthorizationRolesRow) string {
	roles := make([]string, len(row.Roles))
	copy(roles, row.Roles)
	sort.Strings(roles)
	return strings.Join(roles, "\x00")
}
