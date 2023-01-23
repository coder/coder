package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	return authorizedFetch(q.authorizer, q.database.GetFileByHashAndCreator)(ctx, arg)
}

func (q *AuthzQuerier) GetFileByID(ctx context.Context, id uuid.UUID) (database.File, error) {
	return authorizedFetch(q.authorizer, q.database.GetFileByID)(ctx, id)
}

func (q *AuthzQuerier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	return authorizedInsertWithReturn(q.authorizer, rbac.ActionCreate, rbac.ResourceFile.WithOwner(arg.CreatedBy.String()), q.database.InsertFile)(ctx, arg)
}
