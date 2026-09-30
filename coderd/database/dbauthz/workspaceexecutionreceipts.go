package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func receiptObject(row database.WorkspaceExecutionReceipt) rbac.Object {
	return rbac.ResourceWorkspaceExecution.WithID(row.SessionID).InOrg(row.OrganizationID).WithOwner(row.OwnerID.String())
}

func (q *querier) GetWorkspaceExecutionReceiptByID(ctx context.Context, id uuid.UUID) (database.WorkspaceExecutionReceipt, error) {
	row, err := q.db.GetWorkspaceExecutionReceiptByID(ctx, id)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, receiptObject(row)); err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	return row, nil
}

func (q *querier) GetWorkspaceExecutionReceiptByRequest(ctx context.Context, arg database.GetWorkspaceExecutionReceiptByRequestParams) (database.WorkspaceExecutionReceipt, error) {
	row, err := q.db.GetWorkspaceExecutionReceiptByRequest(ctx, arg)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, receiptObject(row)); err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	return row, nil
}

func (q *querier) InsertWorkspaceExecutionReceipt(ctx context.Context, arg database.InsertWorkspaceExecutionReceiptParams) (database.WorkspaceExecutionReceipt, error) {
	session, err := q.db.GetWorkspaceExecutionSessionByID(ctx, arg.SessionID)
	if err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, sessionObject(session)); err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	return q.db.InsertWorkspaceExecutionReceipt(ctx, arg)
}

func (q *querier) UpdateWorkspaceExecutionReceipt(ctx context.Context, arg database.UpdateWorkspaceExecutionReceiptParams) (database.WorkspaceExecutionReceipt, error) {
	row, err := q.db.GetWorkspaceExecutionReceiptByID(ctx, arg.ID)
	if err != nil {
		return row, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, receiptObject(row)); err != nil {
		return database.WorkspaceExecutionReceipt{}, err
	}
	return q.db.UpdateWorkspaceExecutionReceipt(ctx, arg)
}

func (q *querier) GetPendingWorkspaceExecutionReceipts(ctx context.Context, sessionID uuid.UUID) ([]database.WorkspaceExecutionReceipt, error) {
	session, err := q.db.GetWorkspaceExecutionSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, sessionObject(session)); err != nil {
		return nil, err
	}
	return q.db.GetPendingWorkspaceExecutionReceipts(ctx, sessionID)
}

func (q *querier) HasPendingWorkspaceExecutionReceipts(ctx context.Context, sessionID uuid.UUID) (bool, error) {
	session, err := q.db.GetWorkspaceExecutionSessionByID(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if err := q.authorizeContext(ctx, policy.ActionSSH, sessionObject(session)); err != nil {
		return false, err
	}
	return q.db.HasPendingWorkspaceExecutionReceipts(ctx, sessionID)
}
