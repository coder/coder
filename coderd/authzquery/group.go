package authzquery

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) DeleteGroupByID(ctx context.Context, id uuid.UUID) error {
	return authorizedDelete(q.logger, q.authorizer, q.database.GetGroupByID, q.database.DeleteGroupByID)(ctx, id)
}

func (q *AuthzQuerier) DeleteGroupMemberFromGroup(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) error {
	// Deleting a group member counts as updating a group.
	fetch := func(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) (database.Group, error) {
		return q.database.GetGroupByID(ctx, arg.GroupID)
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.DeleteGroupMemberFromGroup)(ctx, arg)
}

func (q *AuthzQuerier) InsertUserGroupsByName(ctx context.Context, arg database.InsertUserGroupsByNameParams) error {
	// This will add the user to all named groups. This counts as updating a group.
	// NOTE: instead of checking if the user has permission to update each group, we instead
	// check if the user has permission to update *a* group in the org.
	fetch := func(ctx context.Context, arg database.InsertUserGroupsByNameParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.InsertUserGroupsByName)(ctx, arg)
}

func (q *AuthzQuerier) DeleteGroupMembersByOrgAndUser(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	// This will remove the user from all groups in the org. This counts as updating a group.
	// NOTE: instead of fetching all groups in the org with arg.UserID as a member, we instead
	// check if the caller has permission to update any group in the org.
	fetch := func(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.DeleteGroupMembersByOrgAndUser)(ctx, arg)
}

func (q *AuthzQuerier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetGroupByID)(ctx, id)
}

func (q *AuthzQuerier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetGroupByOrgAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	relatedFunc := func(_ []database.User, groupID uuid.UUID) (database.Group, error) {
		return q.database.GetGroupByID(ctx, groupID)
	}
	return authorizedQueryWithRelated(q.logger, q.authorizer, rbac.ActionRead, relatedFunc, q.database.GetGroupMembers)(ctx, groupID)
}

func (q *AuthzQuerier) InsertAllUsersGroup(ctx context.Context, organizationID uuid.UUID) (database.Group, error) {
	// This method creates a new group.
	return authorizedInsertWithReturn(q.logger, q.authorizer, rbac.ActionCreate, rbac.ResourceGroup.InOrg(organizationID), q.database.InsertAllUsersGroup)(ctx, organizationID)
}

func (q *AuthzQuerier) InsertGroup(ctx context.Context, arg database.InsertGroupParams) (database.Group, error) {
	return authorizedInsertWithReturn(q.logger, q.authorizer, rbac.ActionCreate, rbac.ResourceGroup.InOrg(arg.OrganizationID), q.database.InsertGroup)(ctx, arg)
}

func (q *AuthzQuerier) InsertGroupMember(ctx context.Context, arg database.InsertGroupMemberParams) error {
	fetch := func(ctx context.Context, arg database.InsertGroupMemberParams) (database.Group, error) {
		return q.database.GetGroupByID(ctx, arg.GroupID)
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.InsertGroupMember)(ctx, arg)
}

func (q *AuthzQuerier) UpdateGroupByID(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	fetch := func(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
		return q.database.GetGroupByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.logger, q.authorizer, fetch, q.database.UpdateGroupByID)(ctx, arg)
}
