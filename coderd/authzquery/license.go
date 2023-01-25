package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetLicenses(ctx context.Context) ([]database.License, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.License, error) {
		return q.database.GetLicenses(ctx)
	}
	return authorizedFetchSet(q.authorizer, fetch)(ctx, nil)
}

func (q *AuthzQuerier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, rbac.ResourceLicense, q.database.InsertLicense)(ctx, arg)
}

func (q *AuthzQuerier) InsertOrUpdateLogoURL(ctx context.Context, value string) error {
	return authorizedInsert(q.authorizer, rbac.ActionUpdate, rbac.ResourceDeploymentConfig, q.database.InsertOrUpdateLogoURL)(ctx, value)
}

func (q *AuthzQuerier) InsertOrUpdateServiceBanner(ctx context.Context, value string) error {
	return authorizedInsert(q.authorizer, rbac.ActionUpdate, rbac.ResourceDeploymentConfig, q.database.InsertOrUpdateServiceBanner)(ctx, value)
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
	return q.database.GetDeploymentID(ctx)
}

func (q *AuthzQuerier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.database.GetLogoURL(ctx)
}

func (q *AuthzQuerier) GetServiceBanner(ctx context.Context) (string, error) {
	// No authz checks
	return q.database.GetServiceBanner(ctx)
}
