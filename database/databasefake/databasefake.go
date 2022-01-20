package databasefake

import (
	"context"
	"database/sql"

	"github.com/coder/coder/database"
)

// New returns an in-memory fake of the database.
func New() database.Store {
	return &fakeQuerier{
		apiKeys: make([]database.APIKey, 0),
		users:   make([]database.User, 0),
	}
}

// fakeQuerier replicates database functionality to enable quick testing.
type fakeQuerier struct {
	apiKeys []database.APIKey
	users   []database.User
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *fakeQuerier) InTx(ctx context.Context, fn func(database.Store) error) error {
	return fn(q)
}

func (q *fakeQuerier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	for _, apiKey := range q.apiKeys {
		if apiKey.ID == id {
			return apiKey, nil
		}
	}
	return database.APIKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	for _, user := range q.users {
		if user.Email == arg.Email || user.Username == arg.Username {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByID(ctx context.Context, id string) (database.User, error) {
	for _, user := range q.users {
		if user.ID == id {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserCount(ctx context.Context) (int64, error) {
	return int64(len(q.users)), nil
}

func (q *fakeQuerier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	key := database.APIKey{
		ID:               arg.ID,
		HashedSecret:     arg.HashedSecret,
		UserID:           arg.UserID,
		Application:      arg.Application,
		Name:             arg.Name,
		LastUsed:         arg.LastUsed,
		ExpiresAt:        arg.ExpiresAt,
		CreatedAt:        arg.CreatedAt,
		UpdatedAt:        arg.UpdatedAt,
		LoginType:        arg.LoginType,
		OIDCAccessToken:  arg.OIDCAccessToken,
		OIDCRefreshToken: arg.OIDCRefreshToken,
		OIDCIDToken:      arg.OIDCIDToken,
		OIDCExpiry:       arg.OIDCExpiry,
		DevurlToken:      arg.DevurlToken,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *fakeQuerier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	user := database.User{
		ID:             arg.ID,
		Email:          arg.Email,
		Name:           arg.Name,
		LoginType:      arg.LoginType,
		HashedPassword: arg.HashedPassword,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Username:       arg.Username,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *fakeQuerier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	for index, apiKey := range q.apiKeys {
		if apiKey.ID != arg.ID {
			continue
		}
		apiKey.LastUsed = arg.LastUsed
		apiKey.ExpiresAt = arg.ExpiresAt
		apiKey.OIDCAccessToken = arg.OIDCAccessToken
		apiKey.OIDCRefreshToken = arg.OIDCRefreshToken
		apiKey.OIDCExpiry = arg.OIDCExpiry
		q.apiKeys[index] = apiKey
		return nil
	}
	return sql.ErrNoRows
}
