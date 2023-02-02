package authzquery

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return authorizedDelete(q.logger, q.authorizer, q.database.GetAPIKeyByID, q.database.DeleteAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return authorizedFetch(q.logger, q.authorizer, q.database.GetAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *AuthzQuerier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return authorizedFetchSet(q.authorizer, q.database.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *AuthzQuerier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return authorizedInsertWithReturn(q.logger, q.authorizer,
		rbac.ActionRead,
		rbac.ResourceAPIKey.WithOwner(arg.UserID.String()),
		q.database.InsertAPIKey)(ctx, arg)
}

func (q *AuthzQuerier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.GetAPIKeyByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.logger, q.authorizer, fetch, q.database.UpdateAPIKeyByID)(ctx, arg)
}
