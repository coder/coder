package authzquery

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) GetAllOrganizationMembers(ctx context.Context, organizationID uuid.UUID) ([]database.User, error) {
	// TODO: @emyrk this is returned by the template ACL api endpoint. These users are full database.Users, which is
	// problematic since it bypasses the rbac.ResourceUser resource. We should probably return a organizationMember or
	// restricted user type here instead.
	//TODO implement me
	panic("implement me")
}

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
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetOrganizationMemberByUserID(ctx context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationMemberByUserID)(ctx, arg)
}

func (q *AuthzQuerier) GetOrganizationMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	// TODO implement me
	panic("implement me")
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
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// TODO implement me
	panic("implement me")
}
