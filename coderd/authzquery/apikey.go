package authzquery

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return authorizedDelete(q.authorizer, q.GetAPIKeyByID, q.DeleteAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return authorizedFetch(q.authorizer, q.GetAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return authorizedFetchSet(q.authorizer, q.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *AuthzQuerier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return authorizedFetchSet(q.authorizer, q.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *AuthzQuerier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return authorizedInsertWithReturn(q.authorizer,
		rbac.ActionRead,
		rbac.ResourceAPIKey.WithOwner(arg.UserID.String()),
		q.InsertAPIKey)(ctx, arg)
}

func (q *AuthzQuerier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.GetAPIKeyByID(ctx, arg.ID)
	}
	return authorizedUpdate(q.authorizer, fetch, q.UpdateAPIKeyByID)(ctx, arg)
}
