package authzquery

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) GetGroupsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]database.Group, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetGroupsByOrganizationID)(ctx, organizationID)
}

func (q *AuthzQuerier) GetOrganizationByID(ctx context.Context, id uuid.UUID) (database.Organization, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationByID)(ctx, id)
}

func (q *AuthzQuerier) GetOrganizationByName(ctx context.Context, name string) (database.Organization, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationByName)(ctx, name)
}

func (q *AuthzQuerier) GetOrganizationIDsByMemberIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	// TODO: This should be rewritten to return a list of database.OrganizationMember for consistent RBAC objects.
	// Currently this row returns a list of org ids per user, which is challenging to check against the RBAC system.
	return authorizedFetchSet(q.authorizer, q.database.GetOrganizationIDsByMemberIDs)(ctx, ids)
}

func (q *AuthzQuerier) GetOrganizationMemberByUserID(ctx context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationMemberByUserID)(ctx, arg)
}

func (q *AuthzQuerier) GetOrganizationMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetOrganizationMembershipsByUserID)(ctx, userID)
}

func (q *AuthzQuerier) GetOrganizations(ctx context.Context) ([]database.Organization, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.Organization, error) {
		return q.database.GetOrganizations(ctx)
	}
	return authorizedFetchSet(q.authorizer, fetch)(ctx, nil)
}

func (q *AuthzQuerier) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]database.Organization, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetOrganizationsByUserID)(ctx, userID)
}

func (q *AuthzQuerier) InsertOrganization(ctx context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, rbac.ResourceOrganization, q.database.InsertOrganization)(ctx, arg)
}

func (q *AuthzQuerier) InsertOrganizationMember(ctx context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	// All roles are added roles. Org member is always implied.
	addedRoles := append(arg.Roles, rbac.RoleOrgMember(arg.OrganizationID))
	err := q.canAssignRoles(ctx, &arg.OrganizationID, addedRoles, []string{})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	obj := rbac.ResourceOrganizationMember.InOrg(arg.OrganizationID).WithID(arg.UserID)
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, obj, q.database.InsertOrganizationMember)(ctx, arg)
}

func (q *AuthzQuerier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// Authorized fetch will check that the actor has read access to the org member since the org member is returned.
	member, err := q.GetOrganizationMemberByUserID(ctx, database.GetOrganizationMemberByUserIDParams{
		OrganizationID: arg.OrgID,
		UserID:         arg.UserID,
	})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	// The org member role is always implied.
	impliedTypes := append(arg.GrantedRoles, rbac.RoleOrgMember(arg.OrgID))
	added, removed := rbac.ChangeRoleSet(member.Roles, impliedTypes)
	err = q.canAssignRoles(ctx, &arg.OrgID, added, removed)
	if err != nil {
		return database.OrganizationMember{}, err
	}

	return q.database.UpdateMemberRoles(ctx, arg)
}

func (q *AuthzQuerier) canAssignRoles(ctx context.Context, orgID *uuid.UUID, added, removed []string) error {
	actor, ok := actorFromContext(ctx)
	if !ok {
		return xerrors.Errorf("no authorization actor in context")
	}

	roleAssign := rbac.ResourceRoleAssignment
	shouldBeOrgRoles := false
	if orgID != nil {
		roleAssign = roleAssign.InOrg(*orgID)
		shouldBeOrgRoles = true
	}

	grantedRoles := append(added, removed...)
	// Validate that the roles being assigned are valid.
	for _, r := range grantedRoles {
		_, isOrgRole := rbac.IsOrgRole(r)
		if shouldBeOrgRoles && !isOrgRole {
			return xerrors.Errorf("Must only update org roles")
		}
		if !shouldBeOrgRoles && isOrgRole {
			return xerrors.Errorf("Must only update site wide roles")
		}

		// All roles should be valid roles
		if _, err := rbac.RoleByName(r); err != nil {
			return xerrors.Errorf("%q is not a supported role", r)
		}
	}

	if len(added) > 0 && q.authorizeContext(ctx, rbac.ActionCreate, roleAssign) != nil {
		return xerrors.Errorf("not authorized to assign roles")
	}

	if len(removed) > 0 && q.authorizeContext(ctx, rbac.ActionDelete, roleAssign) != nil {
		return xerrors.Errorf("not authorized to delete roles")
	}

	for _, roleName := range grantedRoles {
		if !rbac.CanAssignRole(actor.Roles, roleName) {
			return xerrors.Errorf("not authorized to assign role %q", roleName)
		}
	}

	return nil
}
