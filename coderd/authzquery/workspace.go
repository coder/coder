package authzquery

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	//TODO Delete this function, all GetWorkspaces should be authorized. For now just call GetWorkspaces on the authz querier.
	return q.GetWorkspaces(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	prep, err := prepareSQLFilter(ctx, q.authorizer, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.database.GetAuthorizedWorkspaces(ctx, arg, prep)
}

func (q *AuthzQuerier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLatestWorkspaceBuilds(ctx context.Context) ([]database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAgentByAuthToken(ctx context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAgentByInstanceID(ctx context.Context, authInstanceID string) (database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceApp, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildParameters(ctx context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error) {
	return authorizedFetch(q.authorizer, q.database.GetWorkspaceByAgentID)(ctx, agentID)
}

func (q *AuthzQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	return authorizedFetch(q.authorizer, q.database.GetWorkspaceByID)(ctx, id)
}

func (q *AuthzQuerier) GetWorkspaceByOwnerIDAndName(ctx context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	return authorizedFetch(q.authorizer, q.database.GetWorkspaceByOwnerIDAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceCountByUserID(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceOwnerCountsByTemplateIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetWorkspaceOwnerCountsByTemplateIDsRow, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourceByID(ctx context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	return authorizedInsert(q.authorizer, rbac.ActionCreate, obj, q.database.InsertWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceBuildParameters(ctx context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.authorizer, fetch, q.database.UpdateWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceAgentVersionByID(ctx context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceBuildByID(ctx context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) SoftDeleteWorkspaceByID(ctx context.Context, id uuid.UUID) error {
	return authorizedDelete(q.authorizer, q.database.GetWorkspaceByID, func(ctx context.Context, id uuid.UUID) error {
		return q.database.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	})(ctx, id)
}

// Deprecated: Use SoftDeleteWorkspaceByID
func (q *AuthzQuerier) UpdateWorkspaceDeletedByID(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	//TODO delete me, placeholder for database.Store
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspaceLastUsedAt(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateWorkspaceLastUsedAt)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceTTL(ctx context.Context, arg database.UpdateWorkspaceTTLParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceTTLParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateWorkspaceTTL)(ctx, arg)
}
