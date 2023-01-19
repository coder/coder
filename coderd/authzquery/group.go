package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) DeleteGroupByID(ctx context.Context, id uuid.UUID) error {
	return authorizedDelete(q.authorizer, q.database.GetGroupByID, q.database.DeleteGroupByID)(ctx, id)
}

func (q *AuthzQuerier) DeleteGroupMember(ctx context.Context, userID uuid.UUID) error {
	// Deleting a group member counts as updating a group.
	return authorizedUpdate(q.authorizer, q.database.GetGroupByID, q.database.DeleteGroupMember)(ctx, userID)
}

func (q *AuthzQuerier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return authorizedFetch(q.authorizer, q.database.GetGroupByID)(ctx, id)
}

func (q *AuthzQuerier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return authorizedFetch(q.authorizer, q.database.GetGroupByOrgAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	// TODO: @emyrk feels like there should be a better way to do this.

	// Get the group using the AuthzQuerier to check read access. If it works, we
	// can fetch the members.
	_, err := q.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	return q.database.GetGroupMembers(ctx, groupID)
}

func (q *AuthzQuerier) InsertAllUsersGroup(ctx context.Context, organizationID uuid.UUID) (database.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertGroup(ctx context.Context, arg database.InsertGroupParams) (database.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertGroupMember(ctx context.Context, arg database.InsertGroupMemberParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateGroupByID(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	//TODO implement me
	panic("implement me")
}
