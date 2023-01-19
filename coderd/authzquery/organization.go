package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) GetAllOrganizationMembers(ctx context.Context, organizationID uuid.UUID) ([]database.User, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetGroupsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]database.Group, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetOrganizationByID(ctx context.Context, id uuid.UUID) (database.Organization, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationByID)(ctx, id)
}

func (q *AuthzQuerier) GetOrganizationByName(ctx context.Context, name string) (database.Organization, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationByName)(ctx, name)
}

func (q *AuthzQuerier) GetOrganizationIDsByMemberIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetOrganizationMemberByUserID(ctx context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	return authorizedFetch(q.authorizer, q.database.GetOrganizationMemberByUserID)(ctx, arg)
}

func (q *AuthzQuerier) GetOrganizationMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetOrganizations(ctx context.Context) ([]database.Organization, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]database.Organization, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertOrganization(ctx context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertOrganizationMember(ctx context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	//TODO implement me
	panic("implement me")
}
