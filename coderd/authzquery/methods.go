package authzquery

// This file contains uncategorized methods.

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

var _ database.Store = (*AuthzQuerier)(nil)

func (q *AuthzQuerier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerLogsByIDBetween(ctx context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertAgentStat(ctx context.Context, arg database.InsertAgentStatParams) (database.AgentStat, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateGitAuthLink(ctx context.Context, arg database.UpdateGitAuthLinkParams) error {
	// TODO implement me
	panic("implement me")
}
