package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

// TODO: We need the idea of a restricted user. Right now we always return a full user,
// which is problematic since we don't want to leak information about users.

func (q *AuthzQuerier) DeleteAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	err := q.authorizeContext(ctx, rbac.ActionUpdate,
		rbac.ResourceUserData.WithOwner(userID.String()).WithID(userID))
	if err != nil {
		return err
	}
	return q.database.DeleteAPIKeysByUserID(ctx, userID)
}

func (q *AuthzQuerier) GetQuotaAllowanceForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.database.GetQuotaAllowanceForUser(ctx, userID)
}

func (q *AuthzQuerier) GetQuotaConsumedForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.database.GetQuotaConsumedForUser(ctx, userID)
}

func (q *AuthzQuerier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	return authorizedFetch(q.authorizer, q.database.GetUserByEmailOrUsername)(ctx, arg)
}

func (q *AuthzQuerier) GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error) {
	return authorizedFetch(q.authorizer, q.database.GetUserByID)(ctx, id)
}

func (q *AuthzQuerier) GetAuthorizedUserCount(ctx context.Context, arg database.GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	return q.database.GetAuthorizedUserCount(ctx, arg, prepared)
}

func (q *AuthzQuerier) GetFilteredUserCount(ctx context.Context, arg database.GetFilteredUserCountParams) (int64, error) {
	prep, err := prepareSQLFilter(ctx, q.authorizer, rbac.ActionRead, rbac.ResourceUser.Type)
	if err != nil {
		return -1, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	// TODO: This should be the only implementation.
	return q.GetAuthorizedUserCount(ctx, arg, prep)
}

func (q *AuthzQuerier) GetUserCount(ctx context.Context) (int64, error) {
	return q.GetFilteredUserCount(ctx, database.GetFilteredUserCountParams{})
}

func (q *AuthzQuerier) GetUsers(ctx context.Context, arg database.GetUsersParams) ([]database.GetUsersRow, error) {
	// TODO: We should use GetUsersWithCount with a better method signature.
	return authorizedFetchSet(q.authorizer, q.database.GetUsers)(ctx, arg)
}

func (q *AuthzQuerier) GetUsersWithCount(ctx context.Context, arg database.GetUsersParams) ([]database.User, int64, error) {
	// TODO Implement this with a SQL filter. The count is incorrect without it.
	rowUsers, err := q.database.GetUsers(ctx, arg)
	if err != nil {
		return nil, -1, err
	}

	if len(rowUsers) == 0 {
		return []database.User{}, 0, nil
	}

	act, ok := actorFromContext(ctx)
	if !ok {
		return nil, -1, xerrors.Errorf("no authorization actor in context")
	}

	// TODO: Is this correct? Should we return a retricted user?
	users := database.ConvertUserRows(rowUsers)
	users, err = rbac.Filter(ctx, q.authorizer, act.ID.String(), act.Roles, act.Scope, act.Groups, rbac.ActionRead, users)
	if err != nil {
		return nil, -1, err
	}

	return users, rowUsers[0].Count, nil
}

func (q *AuthzQuerier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetUsersByIDs)(ctx, ids)
}

func (q *AuthzQuerier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	obj := rbac.ResourceUser
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, obj, q.database.InsertUser)(ctx, arg)
}

func (q *AuthzQuerier) InsertUserLink(ctx context.Context, arg database.InsertUserLinkParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceUser); err != nil {
		return database.UserLink{}, err
	}
	return q.database.InsertUserLink(ctx, arg)
}

func (q *AuthzQuerier) SoftDeleteUserByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.database.UpdateUserDeletedByID(ctx, database.UpdateUserDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	}
	return authorizedDelete(q.authorizer, q.database.GetUserByID, deleteF)(ctx, id)
}

// UpdateUserDeletedByID
// Deprecated: Delete this function in favor of 'SoftDeleteUserByID'. Deletes are
// irreversible.
func (q *AuthzQuerier) UpdateUserDeletedByID(ctx context.Context, arg database.UpdateUserDeletedByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateUserDeletedByIDParams) (database.User, error) {
		return q.database.GetUserByID(ctx, arg.ID)
	}
	// This uses the rbac.ActionDelete action always as this function should always delete.
	// We should delete this function in favor of 'SoftDeleteUserByID'.
	return authorizedDelete(q.authorizer, fetch, q.database.UpdateUserDeletedByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateUserHashedPassword(ctx context.Context, arg database.UpdateUserHashedPasswordParams) error {
	fetch := func(ctx context.Context, arg database.UpdateUserHashedPasswordParams) (database.User, error) {
		return q.database.GetUserByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateUserHashedPassword)(ctx, arg)
}

func (q *AuthzQuerier) UpdateUserLastSeenAt(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
		return q.database.GetUserByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.authorizer, fetch, q.database.UpdateUserLastSeenAt)(ctx, arg)
}

func (q *AuthzQuerier) UpdateUserProfile(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
		return q.GetUserByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.authorizer, fetch, q.database.UpdateUserProfile)(ctx, arg)
}

func (q *AuthzQuerier) UpdateUserStatus(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
		return q.database.GetUserByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.authorizer, fetch, q.database.UpdateUserStatus)(ctx, arg)
}

func (q *AuthzQuerier) DeleteGitSSHKey(ctx context.Context, userID uuid.UUID) error {
	return authorizedDelete(q.authorizer, q.database.GetGitSSHKey, q.database.DeleteGitSSHKey)(ctx, userID)
}

func (q *AuthzQuerier) GetGitSSHKey(ctx context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	return authorizedFetch(q.authorizer, q.database.GetGitSSHKey)(ctx, userID)
}

func (q *AuthzQuerier) InsertGitSSHKey(ctx context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.database.InsertGitSSHKey)(ctx, arg)
}

func (q *AuthzQuerier) UpdateGitSSHKey(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionUpdate, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.database.UpdateGitSSHKey)(ctx, arg)
}

func (q *AuthzQuerier) GetGitAuthLink(ctx context.Context, arg database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	// TODO @emyrk: Which permissions should be checked here? It looks like oauth has
	// unique authz flow like workspace agents. Maybe this resource should have it's
	// own resource type?
	panic("implement me")
}

func (q *AuthzQuerier) InsertGitAuthLink(ctx context.Context, arg database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateUserLink(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
	// TODO implement me
	panic("implement me")
}

// UpdateUserRoles updates the site roles of a user. The validation for this function include more than
// just a basic RBAC check.
func (q *AuthzQuerier) UpdateUserRoles(ctx context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
	actor, ok := actorFromContext(ctx)
	if !ok {
		return database.User{}, xerrors.Errorf("no authorization actor in context")
	}

	// Only site roles can be updated in this function. If an unsupported role is
	// provided, return an error.
	for _, r := range arg.GrantedRoles {
		if _, ok := rbac.IsOrgRole(r); ok {
			return database.User{}, xerrors.Errorf("Must only update site wide roles")
		}
		if _, err := rbac.RoleByName(r); err != nil {
			return database.User{}, xerrors.Errorf("%q is not a supported role", r)
		}
	}

	// We need to fetch the user being updated to identify the change in roles.
	// This requires read access on the user in question, since the user is
	// returned from this function.
	user, err := authorizedFetch(q.authorizer, q.database.GetUserByID)(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}

	// The member role is always implied.
	impliedTypes := append(arg.GrantedRoles, rbac.RoleMember())
	// If the changeset is nothing, less rbac checks need to be done.
	added, removed := rbac.ChangeRoleSet(user.RBACRoles, impliedTypes)

	// Assigning a role requires the create permission.
	if len(added) > 0 && q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceRoleAssignment) != nil {
		return database.User{}, xerrors.Errorf("not authorized to assign roles")
	}

	// Removing a role requires the delete permission.
	if len(removed) > 0 && q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceRoleAssignment) != nil {
		return database.User{}, xerrors.Errorf("not authorized to delete roles")
	}

	// Just treat adding & removing as "assigning" for now.
	for _, roleName := range append(added, removed...) {
		if !rbac.CanAssignRole(actor.Roles, roleName) {
			return database.User{}, xerrors.Errorf("not authorized to assign role %q", roleName)
		}
	}

	return q.UpdateUserRoles(ctx, arg)
}
