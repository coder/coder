package authzquery

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

// TODO: All these system functions should have rbac objects created to allow
// only system roles to call them. No user roles should ever have the permission
// to these objects. Might need a negative permission on the `Owner` role to
// prevent owners.

func (q *AuthzQuerier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	return q.db.UpdateUserLinkedID(ctx, arg)
}

func (q *AuthzQuerier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	return q.db.GetUserLinkByLinkedID(ctx, linkedID)
}

func (q *AuthzQuerier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	return q.db.GetUserLinkByUserIDLoginType(ctx, arg)
}

func (q *AuthzQuerier) GetLatestWorkspaceBuilds(ctx context.Context) ([]database.WorkspaceBuild, error) {
	// This function is a system function until we implement a join for workspace builds.
	// This is because we need to query for all related workspaces to the returned builds.
	// This is a very inefficient method of fetching the latest workspace builds.
	// We should just join the rbac properties.
	return q.db.GetLatestWorkspaceBuilds(ctx)
}

// GetWorkspaceAgentByAuthToken is used in http middleware to get the workspace agent.
// This should only be used by a system user in that middleware.
func (q *AuthzQuerier) GetWorkspaceAgentByAuthToken(ctx context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	return q.db.GetWorkspaceAgentByAuthToken(ctx, authToken)
}

func (q *AuthzQuerier) GetActiveUserCount(ctx context.Context) (int64, error) {
	return q.db.GetActiveUserCount(ctx)
}

func (q *AuthzQuerier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	return q.db.GetUnexpiredLicenses(ctx)
}

func (q *AuthzQuerier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	return q.db.GetAuthorizationUserRoles(ctx, userID)
}

func (q *AuthzQuerier) GetDERPMeshKey(ctx context.Context) (string, error) {
	// TODO Implement authz check for system user.
	return q.db.GetDERPMeshKey(ctx)
}

func (q *AuthzQuerier) InsertDERPMeshKey(ctx context.Context, value string) error {
	// TODO Implement authz check for system user.
	return q.db.InsertDERPMeshKey(ctx, value)
}

func (q *AuthzQuerier) InsertDeploymentID(ctx context.Context, value string) error {
	// TODO Implement authz check for system user.
	return q.db.InsertDeploymentID(ctx, value)
}

func (q *AuthzQuerier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.InsertReplica(ctx, arg)
}

func (q *AuthzQuerier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.UpdateReplica(ctx, arg)
}

func (q *AuthzQuerier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	// TODO Implement authz check for system user.
	return q.db.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *AuthzQuerier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.GetReplicasUpdatedAfter(ctx, updatedAt)
}

func (q *AuthzQuerier) GetUserCount(ctx context.Context) (int64, error) {
	return q.db.GetUserCount(ctx)
}

func (q *AuthzQuerier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	// TODO Implement authz check for system user.
	return q.db.GetTemplates(ctx)
}

// UpdateWorkspaceBuildCostByID is used by the provisioning system to update the cost of a workspace build.
func (q *AuthzQuerier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	return q.db.UpdateWorkspaceBuildCostByID(ctx, arg)
}

func (q *AuthzQuerier) InsertOrUpdateLastUpdateCheck(ctx context.Context, value string) error {
	return q.db.InsertOrUpdateLastUpdateCheck(ctx, value)
}

func (q *AuthzQuerier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	return q.db.GetLastUpdateCheck(ctx)
}

// Telemetry related functions. These functions are system functions for returning
// telemetry data. Never called by a user.

func (q *AuthzQuerier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	return q.db.GetWorkspaceBuildsCreatedAfter(ctx, createdAt)
}

func (q *AuthzQuerier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	return q.db.GetWorkspaceAgentsCreatedAfter(ctx, createdAt)
}

func (q *AuthzQuerier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	return q.db.GetWorkspaceAppsCreatedAfter(ctx, createdAt)
}

func (q *AuthzQuerier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	return q.db.GetWorkspaceResourcesCreatedAfter(ctx, createdAt)
}

func (q *AuthzQuerier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	return q.db.GetWorkspaceResourceMetadataCreatedAfter(ctx, createdAt)
}

func (q *AuthzQuerier) DeleteOldAgentStats(ctx context.Context) error {
	return q.db.DeleteOldAgentStats(ctx)
}

func (q *AuthzQuerier) GetParameterSchemasCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ParameterSchema, error) {
	return q.db.GetParameterSchemasCreatedAfter(ctx, createdAt)
}
func (q *AuthzQuerier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	return q.db.GetProvisionerJobsCreatedAfter(ctx, createdAt)
}

// Provisionerd server functions

func (q *AuthzQuerier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	return q.db.InsertWorkspaceAgent(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	return q.db.InsertWorkspaceApp(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	return q.db.InsertWorkspaceResourceMetadata(ctx, arg)
}

func (q *AuthzQuerier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	return q.db.AcquireProvisionerJob(ctx, arg)
}

func (q *AuthzQuerier) UpdateProvisionerJobWithCompleteByID(ctx context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	return q.db.UpdateProvisionerJobWithCompleteByID(ctx, arg)
}

func (q *AuthzQuerier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	return q.db.UpdateProvisionerJobByID(ctx, arg)
}

func (q *AuthzQuerier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	return q.db.InsertProvisionerJob(ctx, arg)
}

func (q *AuthzQuerier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	return q.db.InsertProvisionerJobLogs(ctx, arg)
}

func (q *AuthzQuerier) InsertProvisionerDaemon(ctx context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	return q.db.InsertProvisionerDaemon(ctx, arg)
}

func (q *AuthzQuerier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	return q.db.InsertTemplateVersionParameter(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	return q.db.InsertWorkspaceResource(ctx, arg)
}

func (q *AuthzQuerier) InsertParameterSchema(ctx context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	return q.db.InsertParameterSchema(ctx, arg)
}
