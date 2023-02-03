package authzquery

// This file contains uncategorized methods.

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.ProvisionerDaemon, error) {
		return q.db.GetProvisionerDaemons(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *AuthzQuerier) GetDeploymentDAUs(ctx context.Context) ([]database.GetDeploymentDAUsRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.All()); err != nil {
		return nil, err
	}
	return q.db.GetDeploymentDAUs(ctx)
}
