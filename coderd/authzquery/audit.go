package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (q *AuthzQuerier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	return insertWithReturn(q.log, q.auth, rbac.ActionCreate, rbac.ResourceAuditLog, q.db.InsertAuditLog)(ctx, arg)
}

func (q *AuthzQuerier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	// To optimize audit logs, we only check the global audit log permission once.
	// This is because we expect a large unbounded set of audit logs, and applying a SQL
	// filter would slow down the query for no benefit.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceAuditLog); err != nil {
		return nil, err
	}
	return q.db.GetAuditLogsOffset(ctx, arg)
}
