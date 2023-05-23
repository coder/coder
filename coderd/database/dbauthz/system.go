package dbauthz

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *querier) GetFileTemplates(ctx context.Context, fileID uuid.UUID) ([]database.GetFileTemplatesRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFileTemplates(ctx, fileID)
}

// GetWorkspaceAppsByAgentIDs
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

// GetWorkspaceAgentsByResourceIDs
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
}

// GetWorkspaceResourceMetadataByResourceIDs is only used for build data.
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

// TODO: we need to add a provisioner job resource
func (q *querier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
	// 	return nil, err
	// }
	return q.db.GetProvisionerJobsByIDs(ctx, ids)
}

// GetTemplateVersionsByIDs is only used for workspace build data.
// The workspace is already fetched.
func (q *querier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionsByIDs(ctx, ids)
}

// GetWorkspaceResourcesByJobIDs is only used for workspace build data.
// The workspace is already fetched.
// TODO: Find a way to replace this with proper authz.
func (q *querier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *querier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.UpdateUserLinkedID(ctx, arg)
}

func (q *querier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByLinkedID(ctx, linkedID)
}

func (q *querier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByUserIDLoginType(ctx, arg)
}

func (q *querier) GetLatestWorkspaceBuilds(ctx context.Context) ([]database.WorkspaceBuild, error) {
	// This function is a system function until we implement a join for workspace builds.
	// This is because we need to query for all related workspaces to the returned builds.
	// This is a very inefficient method of fetching the latest workspace builds.
	// We should just join the rbac properties.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetLatestWorkspaceBuilds(ctx)
}

// GetWorkspaceAgentByAuthToken is used in http middleware to get the workspace agent.
// This should only be used by a system user in that middleware.
func (q *querier) GetWorkspaceAgentByAuthToken(ctx context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.GetWorkspaceAgentByAuthToken(ctx, authToken)
}

func (q *querier) GetActiveUserCount(ctx context.Context) (int64, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetActiveUserCount(ctx)
}

func (q *querier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetUnexpiredLicenses(ctx)
}

func (q *querier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetAuthorizationUserRolesRow{}, err
	}
	return q.db.GetAuthorizationUserRoles(ctx, userID)
}

func (q *querier) GetDERPMeshKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetDERPMeshKey(ctx)
}

func (q *querier) InsertDERPMeshKey(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDERPMeshKey(ctx, value)
}

func (q *querier) InsertDeploymentID(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDeploymentID(ctx, value)
}

func (q *querier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.InsertReplica(ctx, arg)
}

func (q *querier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.UpdateReplica(ctx, arg)
}

func (q *querier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *querier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetReplicasUpdatedAfter(ctx, updatedAt)
}

func (q *querier) GetUserCount(ctx context.Context) (int64, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetUserCount(ctx)
}

func (q *querier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplates(ctx)
}

// Only used by metrics cache.
func (q *querier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetTemplateAverageBuildTimeRow{}, err
	}
	return q.db.GetTemplateAverageBuildTime(ctx, arg)
}

// Only used by metrics cache.
func (q *querier) GetTemplateDAUs(ctx context.Context, arg database.GetTemplateDAUsParams) ([]database.GetTemplateDAUsRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplateDAUs(ctx, arg)
}

// Only used by metrics cache.
func (q *querier) GetDeploymentDAUs(ctx context.Context, tzOffset int32) ([]database.GetDeploymentDAUsRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetDeploymentDAUs(ctx, tzOffset)
}

// UpdateWorkspaceBuildCostByID is used by the provisioning system to update the cost of a workspace build.
func (q *querier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.UpdateWorkspaceBuildCostByID(ctx, arg)
}

func (q *querier) UpsertLastUpdateCheck(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertLastUpdateCheck(ctx, value)
}

func (q *querier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetLastUpdateCheck(ctx)
}

// Telemetry related functions. These functions are system functions for returning
// telemetry data. Never called by a user.

func (q *querier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourceMetadataCreatedAfter(ctx, createdAt)
}

func (q *querier) DeleteOldWorkspaceAgentStats(ctx context.Context) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentStats(ctx)
}

func (q *querier) DeleteOldWorkspaceAgentStartupLogs(ctx context.Context) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentStartupLogs(ctx)
}

func (q *querier) GetDeploymentWorkspaceAgentStats(ctx context.Context, createdAfter time.Time) (database.GetDeploymentWorkspaceAgentStatsRow, error) {
	return q.db.GetDeploymentWorkspaceAgentStats(ctx, createdAfter)
}

func (q *querier) GetWorkspaceAgentStats(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsRow, error) {
	return q.db.GetWorkspaceAgentStats(ctx, createdAfter)
}

func (q *querier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	return q.db.GetWorkspaceAgentStatsAndLabels(ctx, createdAfter)
}

func (q *querier) GetDeploymentWorkspaceStats(ctx context.Context) (database.GetDeploymentWorkspaceStatsRow, error) {
	return q.db.GetDeploymentWorkspaceStats(ctx)
}

func (q *querier) GetWorkspacesEligibleForAutoStartStop(ctx context.Context, now time.Time) ([]database.Workspace, error) {
	return q.db.GetWorkspacesEligibleForAutoStartStop(ctx, now)
}

func (q *querier) GetParameterSchemasCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ParameterSchema, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetParameterSchemasCreatedAfter(ctx, createdAt)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.GetProvisionerJobsCreatedAfter(ctx, createdAt)
}

// Provisionerd server functions

func (q *querier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.InsertWorkspaceAgent(ctx, arg)
}

func (q *querier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceApp{}, err
	}
	return q.db.InsertWorkspaceApp(ctx, arg)
}

func (q *querier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.InsertWorkspaceResourceMetadata(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAgentConnectionByID(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.AcquireProvisionerJob(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) UpdateProvisionerJobWithCompleteByID(ctx context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return err
	// }
	return q.db.UpdateProvisionerJobWithCompleteByID(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return err
	// }
	return q.db.UpdateProvisionerJobByID(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.InsertProvisionerJob(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.InsertProvisionerJobLogs(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentStartupLogs(ctx context.Context, arg database.InsertWorkspaceAgentStartupLogsParams) ([]database.WorkspaceAgentStartupLog, error) {
	return q.db.InsertWorkspaceAgentStartupLogs(ctx, arg)
}

// TODO: We need to create a ProvisionerDaemon resource type
func (q *querier) InsertProvisionerDaemon(ctx context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerDaemon{}, err
	// }
	return q.db.InsertProvisionerDaemon(ctx, arg)
}

func (q *querier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionParameter{}, err
	}
	return q.db.InsertTemplateVersionParameter(ctx, arg)
}

func (q *querier) InsertTemplateVersionVariable(ctx context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionVariable{}, err
	}
	return q.db.InsertTemplateVersionVariable(ctx, arg)
}

func (q *querier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceResource{}, err
	}
	return q.db.InsertWorkspaceResource(ctx, arg)
}

func (q *querier) InsertParameterSchema(ctx context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.ParameterSchema{}, err
	}
	return q.db.InsertParameterSchema(ctx, arg)
}

func (q *querier) GetWorkspaceProxyByHostname(ctx context.Context, params database.GetWorkspaceProxyByHostnameParams) (database.WorkspaceProxy, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.WorkspaceProxy{}, err
	}
	return q.db.GetWorkspaceProxyByHostname(ctx, params)
}
