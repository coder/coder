package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) DeleteAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetActiveUserCount(ctx context.Context) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetFilteredUserCount(ctx context.Context, arg database.GetFilteredUserCountParams) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetQuotaAllowanceForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetQuotaConsumedForUser(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetUserByEmailOrUsername)(ctx, arg)
}

func (q *AuthzQuerier) GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetUserByID)(ctx, id)
}

func (q *AuthzQuerier) GetUserCount(ctx context.Context) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUsers(ctx context.Context, arg database.GetUsersParams) ([]database.GetUsersRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertUserLink(ctx context.Context, arg database.InsertUserLinkParams) (database.UserLink, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserDeletedByID(ctx context.Context, arg database.UpdateUserDeletedByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserHashedPassword(ctx context.Context, arg database.UpdateUserHashedPasswordParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserLastSeenAt(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserLink(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserProfile(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserRoles(ctx context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserStatus(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAuthorizedUserCount(ctx context.Context, arg database.GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	//TODO implement me
	panic("implement me")
}
