package authzquery

// This file contains uncategorized methods.

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

var _ database.Store = (*AuthzQuerier)(nil)

func (q *AuthzQuerier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) DeleteOldAgentStats(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) DeleteParameterValueByID(ctx context.Context, id uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetDeploymentID(ctx context.Context) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLatestAgentStat(ctx context.Context, agentID uuid.UUID) (database.AgentStat, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterSchemasByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterSchemasCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ParameterSchema, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetParameterValueByScopeAndName(ctx context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerDaemonByID(ctx context.Context, id uuid.UUID) (database.ProvisionerDaemon, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetProvisionerLogsByIDBetween(ctx context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertAgentStat(ctx context.Context, arg database.InsertAgentStatParams) (database.AgentStat, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertOrUpdateLastUpdateCheck(ctx context.Context, value string) error {
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

func (q *AuthzQuerier) InsertParameterSchema(ctx context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertParameterValue(ctx context.Context, arg database.InsertParameterValueParams) (database.ParameterValue, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertProvisionerDaemon(ctx context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) ParameterValue(ctx context.Context, id uuid.UUID) (database.ParameterValue, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) ParameterValues(ctx context.Context, arg database.ParameterValuesParams) ([]database.ParameterValue, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateGitAuthLink(ctx context.Context, arg database.UpdateGitAuthLinkParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateGitSSHKey(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateProvisionerDaemonByID(ctx context.Context, arg database.UpdateProvisionerDaemonByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateProvisionerJobWithCancelByID(ctx context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateProvisionerJobWithCompleteByID(ctx context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	//TODO implement me
	panic("implement me")
}
