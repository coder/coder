package authzquery

// This file contains uncategorized methods.

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

var _ database.Store = (*AuthzQuerier)(nil)

func (q *AuthzQuerier) DeleteParameterValueByID(ctx context.Context, id uuid.UUID) error {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLatestAgentStat(ctx context.Context, agentID uuid.UUID) (database.AgentStat, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterSchemasByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterSchemasCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ParameterSchema, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterValueByScopeAndName(ctx context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerDaemonByID(ctx context.Context, id uuid.UUID) (database.ProvisionerDaemon, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
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

func (q *AuthzQuerier) InsertParameterSchema(ctx context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertParameterValue(ctx context.Context, arg database.InsertParameterValueParams) (database.ParameterValue, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) ParameterValue(ctx context.Context, id uuid.UUID) (database.ParameterValue, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) ParameterValues(ctx context.Context, arg database.ParameterValuesParams) ([]database.ParameterValue, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateGitAuthLink(ctx context.Context, arg database.UpdateGitAuthLinkParams) error {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateProvisionerJobWithCancelByID(ctx context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
	// TODO implement me
	panic("implement me")
}
