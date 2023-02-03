package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	return fetch(q.log, q.auth, q.db.GetFileByHashAndCreator)(ctx, arg)
}

func (q *AuthzQuerier) GetFileByID(ctx context.Context, id uuid.UUID) (database.File, error) {
	return fetch(q.log, q.auth, q.db.GetFileByID)(ctx, id)
}

func (q *AuthzQuerier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	return insertWithReturn(q.log, q.auth, rbac.ActionCreate, rbac.ResourceFile.WithOwner(arg.CreatedBy.String()), q.db.InsertFile)(ctx, arg)
}
