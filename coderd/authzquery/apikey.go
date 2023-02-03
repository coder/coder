package authzquery

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return deleteQ(q.log, q.auth, q.db.GetAPIKeyByID, q.db.DeleteAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByID)(ctx, id)
}

func (q *AuthzQuerier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *AuthzQuerier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *AuthzQuerier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return insertWithReturn(q.log, q.auth,
		rbac.ActionCreate,
		rbac.ResourceAPIKey.WithOwner(arg.UserID.String()),
		q.db.InsertAPIKey)(ctx, arg)
}

func (q *AuthzQuerier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.db.GetAPIKeyByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateAPIKeyByID)(ctx, arg)
}
