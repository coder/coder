package authzquery

// This file contains uncategorized methods.

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.ProvisionerDaemon, error) {
		return q.database.GetProvisionerDaemons(ctx)
	}
	return authorizedFetchSet(q.authorizer, fetch)(ctx, nil)
}

func (q *AuthzQuerier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// TODO: This is missing authorization and is incorrect. This call is used by telemetry, and by 1 http route.
	// That http handler should find a better way to fetch these jobs with easier rbac authz.
	return q.database.GetProvisionerJobsByIDs(ctx, ids)
}

func (q *AuthzQuerier) GetProvisionerLogsByIDBetween(ctx context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	// Authorized read on job lets the actor also read the logs.
	_, err := q.GetProvisionerJobByID(ctx, arg.JobID)
	if err != nil {
		return nil, err
	}
	return q.database.GetProvisionerLogsByIDBetween(ctx, arg)
}
