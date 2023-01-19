package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetLicenses(ctx context.Context) ([]database.License, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertOrUpdateLogoURL(ctx context.Context, value string) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertOrUpdateServiceBanner(ctx context.Context, value string) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLicenseByID(ctx context.Context, id int32) (database.License, error) {
	return authorizedFetch(q.authorizer, q.database.GetLicenseByID)(ctx, id)
}

func (q *AuthzQuerier) DeleteLicense(ctx context.Context, id int32) (int32, error) {
	err := authorizedDelete(q.authorizer, q.database.GetLicenseByID, func(ctx context.Context, id int32) error {
		_, err := q.database.DeleteLicense(ctx, id)
		return err
	})(ctx, id)
	if err != nil {
		return -1, err
	}
	return id, nil
}

func (q *AuthzQuerier) GetDeploymentID(ctx context.Context) (string, error) {
	// No authz checks
	return q.GetDeploymentID(ctx)
}

func (q *AuthzQuerier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.GetLogoURL(ctx)
}

func (q *AuthzQuerier) GetServiceBanner(ctx context.Context) (string, error) {
	// No authz checks
	return q.GetServiceBanner(ctx)
}
