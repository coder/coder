package session

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/cryptorand"
)

func TestUserActor(t *testing.T) {
	t.Parallel()

	t.Run("NoCookie", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		// If there's no cookie, the user actor function should return nil and
		// true (i.e. it shouldn't respond) so that other handlers can run
		// afterwards.
		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.True(t, ok)
		require.Nil(t, act)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: "test-wow-hello",
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("InvalidIDLength", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: "test-wow",
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: "testtestid-wow",
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		// Use a random API key.
		id, secret, _ := randomAPIKey(t)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: fmt.Sprintf("%s-%s", id, secret),
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db        = databasefake.New()
			u         = newUser(t, db)
			apiKey, _ = newAPIKey(t, db, u, time.Time{}, time.Time{})
			r         = httptest.NewRequest("GET", "/", nil)
			rw        = httptest.NewRecorder()
		)

		// Use a random secret in the request so they don't match.
		_, secret, _ := randomAPIKey(t)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: fmt.Sprintf("%s-%s", apiKey.ID, secret),
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		var (
			db       = databasefake.New()
			u        = newUser(t, db)
			now      = database.Now()
			_, token = newAPIKey(t, db, u, now, now.Add(-time.Hour))
			r        = httptest.NewRequest("GET", "/", nil)
			rw       = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: token,
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			db            = databasefake.New()
			u             = newUser(t, db)
			now           = database.Now()
			apiKey, token = newAPIKey(t, db, u, now, now.Add(12*time.Hour))
			r             = httptest.NewRequest("GET", "/", nil)
			rw            = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: token,
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.True(t, ok)

		require.NotNil(t, act)
		require.Equal(t, ActorTypeUser, act.Type())
		require.Equal(t, u.ID.String(), act.ID())
		require.Equal(t, u.Username, act.Name())
		require.Equal(t, u, *act.User())
		require.Equal(t, apiKey, *act.APIKey())

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), apiKey.ID)
		require.NoError(t, err)

		assertTimesEqual(t, apiKey.LastUsed, gotAPIKey.LastUsed)
		assertTimesNotEqual(t, apiKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidUpdateLastUsed", func(t *testing.T) {
		t.Parallel()
		var (
			db            = databasefake.New()
			u             = newUser(t, db)
			now           = database.Now()
			apiKey, token = newAPIKey(t, db, u, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
			r             = httptest.NewRequest("GET", "/", nil)
			rw            = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: token,
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.True(t, ok)
		require.NotNil(t, act)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), apiKey.ID)
		require.NoError(t, err)

		assertTimesNotEqual(t, apiKey.LastUsed, gotAPIKey.LastUsed)
		assertTimesEqual(t, apiKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})

	t.Run("ValidUpdateExpiry", func(t *testing.T) {
		t.Parallel()
		var (
			db            = databasefake.New()
			u             = newUser(t, db)
			now           = database.Now()
			apiKey, token = newAPIKey(t, db, u, now, now.Add(time.Minute))
			r             = httptest.NewRequest("GET", "/", nil)
			rw            = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  AuthCookie,
			Value: token,
		})

		act, ok := UserActorFromRequest(context.Background(), db, rw, r)
		require.True(t, ok)
		require.NotNil(t, act)

		gotAPIKey, err := db.GetAPIKeyByID(r.Context(), apiKey.ID)
		require.NoError(t, err)

		assertTimesEqual(t, apiKey.LastUsed, gotAPIKey.LastUsed)
		assertTimesNotEqual(t, apiKey.ExpiresAt, gotAPIKey.ExpiresAt)
	})
}

func newUser(t *testing.T, db database.Store) database.User {
	t.Helper()

	id, err := uuid.NewRandom()
	require.NoError(t, err, "generate random user ID")

	now := database.Now()
	user, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:             id,
		Email:          fmt.Sprintf("test+%s@coder.com", id),
		Name:           "Test User",
		LoginType:      database.LoginTypeBuiltIn,
		HashedPassword: nil,
		CreatedAt:      now,
		UpdatedAt:      now,
		Username:       id.String(),
	})
	require.NoError(t, err, "insert user")

	return user
}

func randomAPIKey(t *testing.T) (keyID string, keySecret string, secretHashed []byte) {
	t.Helper()

	id, err := cryptorand.String(10)
	require.NoError(t, err, "generate random API key ID")
	secret, err := cryptorand.String(22)
	require.NoError(t, err, "generate random API key secret")
	hashed := sha256.Sum256([]byte(secret))

	return id, secret, hashed[:]
}

func newAPIKey(t *testing.T, db database.Store, user database.User, lastUsed, expiresAt time.Time) (database.APIKey, string) {
	t.Helper()

	var (
		id, secret, hashed = randomAPIKey(t)
		now                = database.Now()
	)
	if lastUsed.IsZero() {
		lastUsed = now
	}
	if expiresAt.IsZero() {
		expiresAt = now.Add(10 * time.Minute)
	}

	apiKey, err := db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
		ID:           id,
		HashedSecret: hashed[:],
		UserID:       user.ID,
		Application:  false,
		Name:         "test-key-" + id,
		LastUsed:     lastUsed,
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
		LoginType:    database.LoginTypeBuiltIn,
	})
	require.NoError(t, err, "insert API key")

	return apiKey, fmt.Sprintf("%v-%v", id, secret)
}

func assertTimesEqual(t *testing.T, a, b time.Time) {
	t.Helper()
	require.Equal(t, a.Truncate(time.Second), b.Truncate(time.Second))
}

func assertTimesNotEqual(t *testing.T, a, b time.Time) {
	t.Helper()
	require.NotEqual(t, a.Truncate(time.Second), b.Truncate(time.Second))
}
