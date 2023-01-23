package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	// TODO Delete this function, all GetWorkspaces should be authorized. For now just call GetWorkspaces on the authz querier.
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
	fetch := func(_ database.WorkspaceBuild, workspaceID uuid.UUID) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, workspaceID)
	}
	return authorizedQueryWithRelated(
		q.authorizer,
		rbac.ActionRead,
		fetch,
		q.database.GetLatestWorkspaceBuildByWorkspaceID)(ctx, workspaceID)
}

func (q *AuthzQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This is not ideal as not all builds will be returned if the workspace cannot be read.
	// This should probably be handled differently? Maybe join workspace builds with workspace
	// ownership properties and filter on that.
	workspaces, err := q.GetWorkspaces(ctx, database.GetWorkspacesParams{WorkspaceIds: ids})
	if err != nil {
		return nil, err
	}

	allowedIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, workspace := range workspaces {
		allowedIDs = append(allowedIDs, workspace.ID)
	}

	return q.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, allowedIDs)
}

func (q *AuthzQuerier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	fetch := func(agent database.WorkspaceAgent, _ uuid.UUID) (database.Workspace, error) {
		return q.database.GetWorkspaceByAgentID(ctx, agent.ID)
	}
	// Curently agent resource is just the related workspace resource.
	return authorizedQueryWithRelated(q.authorizer, rbac.ActionRead, fetch, q.database.GetWorkspaceAgentByID)(ctx, id)
}

// GetWorkspaceAgentByInstanceID might want to be a system call? Unsure exactly,
// but this will fail. Need to figure out what AuthInstanceID is, and if it
// is essentially an auth token. But the caller using this function is not
// an authenticated user. So this authz check will fail.
func (q *AuthzQuerier) GetWorkspaceAgentByInstanceID(ctx context.Context, authInstanceID string) (database.WorkspaceAgent, error) {
	fetch := func(agent database.WorkspaceAgent, _ string) (database.Workspace, error) {
		return q.database.GetWorkspaceByAgentID(ctx, agent.ID)
	}
	return authorizedQueryWithRelated(q.authorizer, rbac.ActionRead, fetch, q.database.GetWorkspaceAgentByInstanceID)(ctx, authInstanceID)
}

func (q *AuthzQuerier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceAppsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceApp, error) {
	fetch := func(_ []database.WorkspaceApp, agentID uuid.UUID) (database.Workspace, error) {
		return q.database.GetWorkspaceByAgentID(ctx, agentID)
	}

	return authorizedQueryWithRelated(q.authorizer, rbac.ActionRead, fetch, q.database.GetWorkspaceAppsByAgentID)(ctx, agentID)
}

func (q *AuthzQuerier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	// TODO: This should be rewritten to support workspace ids, rather than agent ids imo.
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	fetch := func(build database.WorkspaceBuild, _ uuid.UUID) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, build.WorkspaceID)
	}
	return authorizedQueryWithRelated(
		q.authorizer,
		rbac.ActionRead,
		fetch,
		q.database.GetWorkspaceBuildByID)(ctx, id)
}

func (q *AuthzQuerier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	fetch := func(_ database.WorkspaceBuild, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return authorizedQueryWithRelated(q.authorizer, rbac.ActionRead, fetch, q.database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceBuildParameters(ctx context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	fetch := func(_ []database.WorkspaceBuild, arg database.GetWorkspaceBuildsByWorkspaceIDParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return authorizedQueryWithRelated(q.authorizer, rbac.ActionRead, fetch, q.database.GetWorkspaceBuildsByWorkspaceID)(ctx, arg)
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

func (q *AuthzQuerier) GetWorkspaceOwnerCountsByTemplateIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetWorkspaceOwnerCountsByTemplateIDsRow, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourceByID(ctx context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, obj, q.database.InsertWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	fetch := func(_ database.WorkspaceBuild, arg database.InsertWorkspaceBuildParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return authorizedQueryWithRelated(q.authorizer, rbac.ActionUpdate, fetch, q.database.InsertWorkspaceBuild)(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceBuildParameters(ctx context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	// TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	return authorizedUpdateWithReturn(q.authorizer, fetch, q.database.UpdateWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByAgentID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateWorkspaceAgentConnectionByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAgentVersionByID(ctx context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByAgentID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateWorkspaceAgentVersionByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	workspace, err := q.database.GetWorkspaceByWorkspaceAppID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.database.UpdateWorkspaceAppHealthByID(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.database.UpdateWorkspaceAutostart)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceBuildByID(ctx context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	build, err := q.database.GetWorkspaceBuildByID(ctx, arg.ID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	workspace, err := q.database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	return q.UpdateWorkspaceBuildByID(ctx, arg)
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
	// TODO delete me, placeholder for database.Store
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) (database.Workspace, error) {
		return q.database.GetWorkspaceByID(ctx, arg.ID)
	}
	// This function is always used to delete.
	return authorizedDelete(q.authorizer, fetch, q.database.UpdateWorkspaceDeletedByID)(ctx, arg)
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

func (q *AuthzQuerier) GetWorkspaceByWorkspaceAppID(ctx context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	return authorizedFetch(q.authorizer, q.database.GetWorkspaceByWorkspaceAppID)(ctx, workspaceAppID)
}
