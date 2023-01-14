package database

import (
	"context"

	"github.com/coder/coder/coderd/rbac"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (Workspace, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetWorkspaceByID)(ctx, id)
}

func (q *AuthzQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (Template, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetTemplateByID)(ctx, id)
}
