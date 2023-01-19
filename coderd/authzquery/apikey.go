package authzquery

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/database"
)

func (q *AuthzQuerier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	//TODO implement me
	panic("implement me")
}

func (q *AuthzQuerier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	//TODO implement me
	panic("implement me")
}
