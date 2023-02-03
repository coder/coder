package authzquery

import (
	"context"
	"database/sql"
	"errors"

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
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedWorkspaces(ctx, arg, prep)
}

func (q *AuthzQuerier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	fetch := func(_ database.WorkspaceBuild, workspaceID uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, workspaceID)
	}
	return queryWithRelated(
		q.log,
		q.auth,
		rbac.ActionRead,
		fetch,
		q.db.GetLatestWorkspaceBuildByWorkspaceID)(ctx, workspaceID)
}

func (q *AuthzQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This is not ideal as not all builds will be returned if the workspace cannot be read.
	// This should probably be handled differently? Maybe join workspace builds with workspace
	// ownership properties and filter on that.
	for _, id := range ids {
		_, err := q.GetWorkspaceByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, ids)
}

func (q *AuthzQuerier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	fetch := func(agent database.WorkspaceAgent, _ uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	}
	// Currently agent resource is just the related workspace resource.
	return queryWithRelated(q.log, q.auth, rbac.ActionRead, fetch, q.db.GetWorkspaceAgentByID)(ctx, id)
}

// GetWorkspaceAgentByInstanceID might want to be a system call? Unsure exactly,
// but this will fail. Need to figure out what AuthInstanceID is, and if it
// is essentially an auth token. But the caller using this function is not
// an authenticated user. So this authz check will fail.
func (q *AuthzQuerier) GetWorkspaceAgentByInstanceID(ctx context.Context, authInstanceID string) (database.WorkspaceAgent, error) {
	fetch := func(agent database.WorkspaceAgent, _ string) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	}
	return queryWithRelated(q.log, q.auth, rbac.ActionRead, fetch, q.db.GetWorkspaceAgentByInstanceID)(ctx, authInstanceID)
}

// GetWorkspaceAgentsByResourceIDs is an all or nothing call. If the user cannot read
// a single agent, the entire call will fail.
func (q *AuthzQuerier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	// TODO: Make this more efficient. This is annoying because all these resources should be owned by the same workspace.
	// So the authz check should just be 1 check, but we cannot do that easily here. We should see if all callers can
	// instead do something like GetWorkspaceAgentsByWorkspaceID.
	agents, err := q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, a := range agents {
		// Check if we can fetch the workspace by the agent ID.
		_, err := q.GetWorkspaceByAgentID(ctx, a.ID)
		if err == nil {
			continue
		}
		if errors.Is(err, sql.ErrNoRows) {
			// The agent is not tied to a workspace, likely from an orphaned template version.
			// Just return it.
			continue
		}
		// Otherwise, we cannot read the workspace, so we cannot read the agent.
		return nil, err
	}
	return agents, nil
}

func (q *AuthzQuerier) UpdateWorkspaceAgentLifecycleStateByID(ctx context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	// If we can fetch the workspace, we can fetch the apps. Use the authorized call.
	_, err := q.GetWorkspaceByAgentID(ctx, arg.AgentID)
	if err != nil {
		return database.WorkspaceApp{}, err
	}

	return q.db.GetWorkspaceAppByAgentIDAndSlug(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceAppsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceApp, error) {
	fetch := func(_ []database.WorkspaceApp, agentID uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, agentID)
	}

	return queryWithRelated(q.log, q.auth, rbac.ActionRead, fetch, q.db.GetWorkspaceAppsByAgentID)(ctx, agentID)
}

// GetWorkspaceAppsByAgentIDs is an all or nothing call. If the user cannot read a single app, the entire call will fail.
func (q *AuthzQuerier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	// TODO: This should be reworked. All these apps are likely owned by the same workspace, so we should be able to
	// do 1 authz call. We should refactor this to be GetWorkspaceAppsByWorkspaceID.
	for _, id := range ids {
		_, err := q.GetWorkspaceAgentByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

func (q *AuthzQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	fetch := func(build database.WorkspaceBuild, _ uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	}
	return queryWithRelated(
		q.log,
		q.auth,
		rbac.ActionRead,
		fetch,
		q.db.GetWorkspaceBuildByID)(ctx, id)
}

func (q *AuthzQuerier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	// Authorized fetch
	_, err = q.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	return build, nil
}

func (q *AuthzQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	fetch := func(_ database.WorkspaceBuild, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return queryWithRelated(q.log, q.auth, rbac.ActionRead, fetch, q.db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceBuildParameters(ctx context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	// Authorized call to get the workspace build. If we can read the build,
	// we can read the params.
	_, err := q.GetWorkspaceBuildByID(ctx, workspaceBuildID)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceBuildParameters(ctx, workspaceBuildID)
}

func (q *AuthzQuerier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	fetch := func(_ []database.WorkspaceBuild, arg database.GetWorkspaceBuildsByWorkspaceIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return queryWithRelated(q.log, q.auth, rbac.ActionRead, fetch, q.db.GetWorkspaceBuildsByWorkspaceID)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByAgentID)(ctx, agentID)
}

func (q *AuthzQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByID)(ctx, id)
}

func (q *AuthzQuerier) GetWorkspaceByOwnerIDAndName(ctx context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByOwnerIDAndName)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceResourceByID(ctx context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	// TODO: Optimize this
	resource, err := q.db.GetWorkspaceResourceByID(ctx, id)
	if err != nil {
		return database.WorkspaceResource{}, err
	}

	build, err := q.db.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		return database.WorkspaceResource{}, nil
	}

	// If the workspace can be read, then the resource can be read.
	_, err = fetch(q.log, q.auth, q.db.GetWorkspaceByID)(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceResource{}, nil
	}
	return resource, err
}

// GetWorkspaceResourceMetadataByResourceIDs is an all or nothing call. If a single resource is not authorized, then
// an error is returned.
func (q *AuthzQuerier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	// TODO: This is very inefficient. Since all these resources are likely asscoiated with the same workspace.
	for _, id := range ids {
		// If we can read the resource, we can read the metadata.
		_, err := q.GetWorkspaceResourceByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

func (q *AuthzQuerier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	job, err := q.db.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var obj rbac.Objecter
	switch job.Type {
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// We don't need to do an authorized check, but this helper function
		// handles the job type for us.
		// TODO: Do not duplicate auth checks.
		tv, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return nil, err
		}
		if !tv.TemplateID.Valid {
			// Orphaned template version
			obj = tv.RBACObjectNoTemplate()
		} else {
			template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
			if err != nil {
				return nil, err
			}
			obj = template.RBACObject()
		}
	case database.ProvisionerJobTypeWorkspaceBuild:
		build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
		if err != nil {
			return nil, err
		}
		workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return nil, err
		}
		obj = workspace
	default:
		return nil, xerrors.Errorf("unknown job type: %s", job.Type)
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, obj); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobID(ctx, jobID)
}

// GetWorkspaceResourcesByJobIDs is an all or nothing call. If a single resource is not authorized, then
// an error is returned.
func (q *AuthzQuerier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	// TODO: This is very inefficient. Since all these resources are likely asscoiated with the same workspace.
	for _, id := range ids {
		// If we can read the resource, we can read the metadata.
		_, err := q.GetProvisionerJobByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *AuthzQuerier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	return insertWithReturn(q.log, q.auth, rbac.ActionCreate, obj, q.db.InsertWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	fetch := func(build database.WorkspaceBuild, arg database.InsertWorkspaceBuildParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}

	var action rbac.Action = rbac.ActionUpdate
	if arg.Transition == database.WorkspaceTransitionDelete {
		action = rbac.ActionDelete
	}
	return queryWithRelated(q.log, q.auth, action, fetch, q.db.InsertWorkspaceBuild)(ctx, arg)
}

func (q *AuthzQuerier) InsertWorkspaceBuildParameters(ctx context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	// TODO: Optimize this. We always have the workspace and build already fetched.
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.WorkspaceBuildID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return err
	}

	return q.db.InsertWorkspaceBuildParameters(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspace)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAgentConnectionByID)(ctx, arg)
}

func (q *AuthzQuerier) InsertAgentStat(ctx context.Context, arg database.InsertAgentStatParams) (database.AgentStat, error) {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	// Not really sure what this is for.
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.AgentStat{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return database.AgentStat{}, err
	}
	return q.db.InsertAgentStat(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAgentVersionByID(ctx context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAgentVersionByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	workspace, err := q.db.GetWorkspaceByWorkspaceAppID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAppHealthByID(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAutostart)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceBuildByID(ctx context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.ID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	return q.db.UpdateWorkspaceBuildByID(ctx, arg)
}

func (q *AuthzQuerier) SoftDeleteWorkspaceByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetWorkspaceByID, func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	})(ctx, id)
}

// Deprecated: Use SoftDeleteWorkspaceByID
func (q *AuthzQuerier) UpdateWorkspaceDeletedByID(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	// TODO deleteQ me, placeholder for database.Store
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	// This function is always used to deleteQ.
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateWorkspaceDeletedByID)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceLastUsedAt(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceLastUsedAt)(ctx, arg)
}

func (q *AuthzQuerier) UpdateWorkspaceTTL(ctx context.Context, arg database.UpdateWorkspaceTTLParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceTTLParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceTTL)(ctx, arg)
}

func (q *AuthzQuerier) GetWorkspaceByWorkspaceAppID(ctx context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByWorkspaceAppID)(ctx, workspaceAppID)
}
