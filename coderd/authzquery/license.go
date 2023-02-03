package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetLicenses(ctx context.Context) ([]database.License, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.License, error) {
		return q.db.GetLicenses(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *AuthzQuerier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	return insertWithReturn(q.log, q.auth, rbac.ResourceLicense, q.db.InsertLicense)(ctx, arg)
}

func (q *AuthzQuerier) InsertOrUpdateLogoURL(ctx context.Context, value string) error {
	return insert(q.log, q.auth, rbac.ResourceDeploymentConfig, q.db.InsertOrUpdateLogoURL)(ctx, value)
}

func (q *AuthzQuerier) InsertOrUpdateServiceBanner(ctx context.Context, value string) error {
	return insert(q.log, q.auth, rbac.ResourceDeploymentConfig, q.db.InsertOrUpdateServiceBanner)(ctx, value)
}

func (q *AuthzQuerier) GetLicenseByID(ctx context.Context, id int32) (database.License, error) {
	return fetch(q.log, q.auth, q.db.GetLicenseByID)(ctx, id)
}

func (q *AuthzQuerier) DeleteLicense(ctx context.Context, id int32) (int32, error) {
	err := deleteQ(q.log, q.auth, q.db.GetLicenseByID, func(ctx context.Context, id int32) error {
		_, err := q.db.DeleteLicense(ctx, id)
		return err
	})(ctx, id)
	if err != nil {
		return -1, err
	}
	return id, nil
}

func (q *AuthzQuerier) GetDeploymentID(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetDeploymentID(ctx)
}

func (q *AuthzQuerier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetLogoURL(ctx)
}

func (q *AuthzQuerier) GetServiceBanner(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetServiceBanner(ctx)
}
