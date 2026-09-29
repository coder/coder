package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (q *querier) UpdateWorkspaceExecutionSession(ctx context.Context, arg database.UpdateWorkspaceExecutionSessionParams) (database.WorkspaceExecutionSession, error) {
	row, err := q.db.GetWorkspaceExecutionSessionByID(ctx, arg.ID)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, sessionObject(row)); err != nil {
		return database.WorkspaceExecutionSession{}, err
	}
	return q.db.UpdateWorkspaceExecutionSession(ctx, arg)
}

func (q *querier) GetReconciliableWorkspaceExecutionSessions(ctx context.Context, arg database.GetReconciliableWorkspaceExecutionSessionsParams) ([]database.WorkspaceExecutionSession, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetReconciliableWorkspaceExecutionSessions(ctx, arg)
}

func (q *querier) LockWorkspaceExecutionWorkspace(ctx context.Context, id uuid.UUID) error {
	if err := q.db.LockWorkspaceExecutionWorkspace(ctx, id); err != nil {
		return err
	}
	workspace, err := q.db.GetWorkspaceByID(ctx, id)
	if err != nil {
		return err
	}
	return q.authorizeContext(ctx, policy.ActionRead, workspace)
}

func (q *querier) HasBusyWorkspaceExecutionChats(ctx context.Context, id uuid.NullUUID) (bool, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return false, err
	}
	return q.db.HasBusyWorkspaceExecutionChats(ctx, id)
}

func (q *querier) HasPendingWorkspaceExecutionReceiptsByWorkspaceID(ctx context.Context, id uuid.NullUUID) (bool, error) {
	workspace, err := q.db.GetWorkspaceByID(ctx, id.UUID)
	if err != nil {
		return false, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, workspace); err != nil {
		return false, err
	}
	return q.db.HasPendingWorkspaceExecutionReceiptsByWorkspaceID(ctx, id)
}

func (q *querier) GetOtherWorkspaceExecutionSessionsByWorkspaceID(ctx context.Context, arg database.GetOtherWorkspaceExecutionSessionsByWorkspaceIDParams) ([]database.WorkspaceExecutionSession, error) {
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID.UUID)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, workspace); err != nil {
		return nil, err
	}
	return q.db.GetOtherWorkspaceExecutionSessionsByWorkspaceID(ctx, arg)
}
