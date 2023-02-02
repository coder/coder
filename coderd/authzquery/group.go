package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
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
	panic("not implemented")
}

func (q *AuthzQuerier) DeleteGroupMembersByOrgAndUser(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	panic("not implemented")
}

func (q *AuthzQuerier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetGroupByID)(ctx, id)
}

func (q *AuthzQuerier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetGroupByOrgAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	group, err := q.database.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, group); err != nil {
		return nil, err
	}

	return q.database.GetGroupMembers(ctx, groupID)
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
