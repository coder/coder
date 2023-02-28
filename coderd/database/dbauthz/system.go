package dbauthz

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

// GetWorkspaceAppsByAgentIDs
// The workspace/job is already fetched.
// TODO: This function should be removed/replaced with something with proper auth.
func (q *querier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

// GetWorkspaceAgentsByResourceIDs
// The workspace/job is already fetched.
// TODO: This function should be removed/replaced with something with proper auth.
func (q *querier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	return q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
}

// GetWorkspaceResourceMetadataByResourceIDs is only used for build data.
// The workspace/job is already fetched.
// TODO: This function should be removed/replaced with something with proper auth.
func (q *querier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

// GetUsersByIDs is only used for usernames on workspace return data.
// This function should be replaced by joining this data to the workspace query
// itself.
// TODO: This function should be removed/replaced with something with proper auth.
// A SQL compiled filter is an option.
func (q *querier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	return q.db.GetUsersByIDs(ctx, ids)
}

func (q *querier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// TODO: This is missing authorization and is incorrect. This call is used by telemetry, and by 1 http route.
	// That http handler should find a better way to fetch these jobs with easier rbac authz.
	return q.db.GetProvisionerJobsByIDs(ctx, ids)
}

// GetTemplateVersionsByIDs is only used for workspace build data.
// The workspace is already fetched.
// TODO: Find a way to replace this with proper authz.
func (q *querier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	return q.db.GetTemplateVersionsByIDs(ctx, ids)
}

// GetWorkspaceResourcesByJobIDs is only used for workspace build data.
// The workspace is already fetched.
// TODO: Find a way to replace this with proper authz.
func (q *querier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *querier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	return q.db.UpdateUserLinkedID(ctx, arg)
}

func (q *querier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	return q.db.GetUserLinkByLinkedID(ctx, linkedID)
}

func (q *querier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	return q.db.GetUserLinkByUserIDLoginType(ctx, arg)
}

func (q *querier) GetLatestWorkspaceBuilds(ctx context.Context) ([]database.WorkspaceBuild, error) {
	// This function is a system function until we implement a join for workspace builds.
	// This is because we need to query for all related workspaces to the returned builds.
	// This is a very inefficient method of fetching the latest workspace builds.
	// We should just join the rbac properties.
	return q.db.GetLatestWorkspaceBuilds(ctx)
}

// GetWorkspaceAgentByAuthToken is used in http middleware to get the workspace agent.
// This should only be used by a system user in that middleware.
func (q *querier) GetWorkspaceAgentByAuthToken(ctx context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	return q.db.GetWorkspaceAgentByAuthToken(ctx, authToken)
}

func (q *querier) GetActiveUserCount(ctx context.Context) (int64, error) {
	return q.db.GetActiveUserCount(ctx)
}

func (q *querier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	return q.db.GetUnexpiredLicenses(ctx)
}

func (q *querier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	return q.db.GetAuthorizationUserRoles(ctx, userID)
}

func (q *querier) GetDERPMeshKey(ctx context.Context) (string, error) {
	// TODO Implement authz check for system user.
	return q.db.GetDERPMeshKey(ctx)
}

func (q *querier) InsertDERPMeshKey(ctx context.Context, value string) error {
	// TODO Implement authz check for system user.
	return q.db.InsertDERPMeshKey(ctx, value)
}

func (q *querier) InsertDeploymentID(ctx context.Context, value string) error {
	// TODO Implement authz check for system user.
	return q.db.InsertDeploymentID(ctx, value)
}

func (q *querier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.InsertReplica(ctx, arg)
}

func (q *querier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.UpdateReplica(ctx, arg)
}

func (q *querier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	// TODO Implement authz check for system user.
	return q.db.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *querier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	// TODO Implement authz check for system user.
	return q.db.GetReplicasUpdatedAfter(ctx, updatedAt)
}

func (q *querier) GetUserCount(ctx context.Context) (int64, error) {
	return q.db.GetUserCount(ctx)
}

func (q *querier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	// TODO Implement authz check for system user.
	return q.db.GetTemplates(ctx)
}

// Only used by metrics cache.
func (q *querier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	return q.db.GetTemplateAverageBuildTime(ctx, arg)
}

// Only used by metrics cache.
func (q *querier) GetTemplateDAUs(ctx context.Context, templateID uuid.UUID) ([]database.GetTemplateDAUsRow, error) {
	return q.db.GetTemplateDAUs(ctx, templateID)
}

// Only used by metrics cache.
func (q *querier) GetDeploymentDAUs(ctx context.Context) ([]database.GetDeploymentDAUsRow, error) {
	return q.db.GetDeploymentDAUs(ctx)
}

// UpdateWorkspaceBuildCostByID is used by the provisioning system to update the cost of a workspace build.
func (q *querier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	return q.db.UpdateWorkspaceBuildCostByID(ctx, arg)
}

func (q *querier) InsertOrUpdateLastUpdateCheck(ctx context.Context, value string) error {
	return q.db.InsertOrUpdateLastUpdateCheck(ctx, value)
}

func (q *querier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	return q.db.GetLastUpdateCheck(ctx)
}

// Telemetry related functions. These functions are system functions for returning
// telemetry data. Never called by a user.

func (q *querier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	return q.db.GetWorkspaceBuildsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	return q.db.GetWorkspaceAgentsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	return q.db.GetWorkspaceAppsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	return q.db.GetWorkspaceResourcesCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	return q.db.GetWorkspaceResourceMetadataCreatedAfter(ctx, createdAt)
}

func (q *querier) DeleteOldWorkspaceAgentStats(ctx context.Context) error {
	return q.db.DeleteOldWorkspaceAgentStats(ctx)
}

func (q *querier) GetParameterSchemasCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ParameterSchema, error) {
	return q.db.GetParameterSchemasCreatedAfter(ctx, createdAt)
}

func (q *querier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	return q.db.GetProvisionerJobsCreatedAfter(ctx, createdAt)
}

// Provisionerd server functions

func (q *querier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	return q.db.InsertWorkspaceAgent(ctx, arg)
}

func (q *querier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	return q.db.InsertWorkspaceApp(ctx, arg)
}

func (q *querier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	return q.db.InsertWorkspaceResourceMetadata(ctx, arg)
}

func (q *querier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	return q.db.AcquireProvisionerJob(ctx, arg)
}

func (q *querier) UpdateProvisionerJobWithCompleteByID(ctx context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	return q.db.UpdateProvisionerJobWithCompleteByID(ctx, arg)
}

func (q *querier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	return q.db.UpdateProvisionerJobByID(ctx, arg)
}

func (q *querier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	return q.db.InsertProvisionerJob(ctx, arg)
}

func (q *querier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	return q.db.InsertProvisionerJobLogs(ctx, arg)
}

func (q *querier) InsertProvisionerDaemon(ctx context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	return q.db.InsertProvisionerDaemon(ctx, arg)
}

func (q *querier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	return q.db.InsertTemplateVersionParameter(ctx, arg)
}

func (q *querier) InsertTemplateVersionVariable(ctx context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	return q.db.InsertTemplateVersionVariable(ctx, arg)
}

func (q *querier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	return q.db.InsertWorkspaceResource(ctx, arg)
}

func (q *querier) InsertParameterSchema(ctx context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	return q.db.InsertParameterSchema(ctx, arg)
}
