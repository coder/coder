package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, rbac.ResourceAuditLog, q.InsertAuditLog)(ctx, arg)
}

func (q *AuthzQuerier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	// To optimize audit logs, we only check the global audit log permission once.
	// This is because we expect a large unbounded set of audit logs, and applying a SQL
	// filter would slow down the query for no benefit.
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceAuditLog)
	if err != nil {
		return nil, err
	}
	return q.database.GetAuditLogsOffset(ctx, arg)
}
