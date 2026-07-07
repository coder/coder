package license

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
func CountWorkspaceCapableUsers(ctx context.Context, db database.Store, authorizer rbac.Authorizer) (int64, error) {
	//nolint:gocritic // Counting licensed seats is a system function.
	rows, err := db.GetActiveUsersAuthorizationRoles(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return 0, xerrors.Errorf("get active users authorization roles: %w", err)
	}

	// Users with identical role and group sets always produce the same
	// authorization outcome because the subject ID and the object owner
	// are the same user in every check. Deduplicate on that signature so
	// evaluation cost scales with the number of unique role sets, not
	// the number of users.
	capableBySignature := make(map[string]bool)
	var count int64
	for _, row := range rows {
		sig := authorizationSignature(row)
		capable, ok := capableBySignature[sig]
		if !ok {
			capable, err = canCreateWorkspace(ctx, db, authorizer, row)
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
func canCreateWorkspace(ctx context.Context, db database.Store, authorizer rbac.Authorizer, row database.GetActiveUsersAuthorizationRolesRow) (bool, error) {
	roleNames, err := row.RoleNames()
	if err != nil {
		return false, xerrors.Errorf("expand role names: %w", err)
	}

	//nolint:gocritic // Expanding custom roles requires system access.
	roles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, roleNames)
	if err != nil {
		return false, xerrors.Errorf("expand roles: %w", err)
	}

	subject := rbac.Subject{
		Type:   rbac.SubjectTypeUser,
		ID:     row.ID.String(),
		Roles:  roles,
		Groups: row.Groups,
		Scope:  rbac.ScopeAll,
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

// authorizationSignature returns a canonical key for the user's role and
// group sets. Two users with equal signatures are interchangeable for
// workspace-create evaluation.
func authorizationSignature(row database.GetActiveUsersAuthorizationRolesRow) string {
	roles := make([]string, len(row.Roles))
	copy(roles, row.Roles)
	sort.Strings(roles)
	groups := make([]string, len(row.Groups))
	copy(groups, row.Groups)
	sort.Strings(groups)
	return strings.Join(roles, "\x00") + "\x01" + strings.Join(groups, "\x00")
}
