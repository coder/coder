package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) DeleteGroupByID(ctx context.Context, id uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) DeleteGroupMember(ctx context.Context, userID uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]database.Group, error) {
	//TODO implement me
	panic("implement me")
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
