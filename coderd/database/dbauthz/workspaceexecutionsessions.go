package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (q *querier) HasClosedWorkspaceExecutionAdmission(ctx context.Context, workspaceID uuid.NullUUID) (bool, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return false, err
	}
	return q.db.HasClosedWorkspaceExecutionAdmission(ctx, workspaceID)
}

func (q *querier) HasProtectedWorkspaceExecutionSession(ctx context.Context, arg database.HasProtectedWorkspaceExecutionSessionParams) (bool, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return false, err
	}
	return q.db.HasProtectedWorkspaceExecutionSession(ctx, arg)
}

func sessionObject(row database.WorkspaceExecutionSession) rbac.Object {
	return rbac.ResourceWorkspaceExecution.WithID(row.ID).InOrg(row.OrganizationID).WithOwner(row.OwnerID.String())
}

func (q *querier) GetWorkspaceExecutionSessionByID(ctx context.Context, id uuid.UUID) (database.WorkspaceExecutionSession, error) {
	row, err := q.db.GetWorkspaceExecutionSessionByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, sessionObject(row)); err != nil {
		return database.WorkspaceExecutionSession{}, err
	}
	return row, nil
}

func (q *querier) GetWorkspaceExecutionSessionByRequest(ctx context.Context, arg database.GetWorkspaceExecutionSessionByRequestParams) (database.WorkspaceExecutionSession, error) {
	row, err := q.db.GetWorkspaceExecutionSessionByRequest(ctx, arg)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, sessionObject(row)); err != nil {
		return database.WorkspaceExecutionSession{}, err
	}
	return row, nil
}

func (q *querier) InsertWorkspaceExecutionSession(ctx context.Context, arg database.InsertWorkspaceExecutionSessionParams) (database.WorkspaceExecutionSession, error) {
	object := rbac.ResourceWorkspaceExecution.InOrg(arg.OrganizationID).WithOwner(arg.OwnerID.String())
	if err := q.authorizeContext(ctx, policy.ActionCreate, object); err != nil {
		return database.WorkspaceExecutionSession{}, err
	}
	return q.db.InsertWorkspaceExecutionSession(ctx, arg)
}
