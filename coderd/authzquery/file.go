package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
)

func (q *AuthzQuerier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetFileByHashAndCreator)(ctx, arg)
}

func (q *AuthzQuerier) GetFileByID(ctx context.Context, id uuid.UUID) (database.File, error) {
	return authorizedFetch(q.authorizer, rbac.ActionRead, q.database.GetFileByID)(ctx, id)
}

func (q *AuthzQuerier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	//TODO implement me
	panic("implement me")
}
