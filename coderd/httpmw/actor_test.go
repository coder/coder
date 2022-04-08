package httpmw_test

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

	"github.com/coder/coder/coderd/access/session"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
)

func TestRequireAuthentication(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(rw, http.StatusOK, httpapi.Response{
			Message: "success",
		})
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		var (
			db               = databasefake.New()
			u, apiKey, token = setupUserAndAPIKey(t, db)
			r                = httptest.NewRequest("GET", "/", nil)
			rw               = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  session.AuthCookie,
			Value: token,
		})

		// Run ExtractAPIKey, then RequireAuthentication, then our success
		// handler.
		h := httpmw.RequireAuthentication()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check the actor.
			act := httpmw.RequestActor(r)
			require.Equal(t, session.ActorTypeUser, act.Type())
			userActor, ok := act.(session.UserActor)
			require.True(t, ok)
			require.Equal(t, u, *userActor.User())
			require.Equal(t, apiKey, *userActor.APIKey())

			httpapi.Write(rw, http.StatusOK, httpapi.Response{
				Message: "success",
			})
		}))
		httpmw.ExtractActor(db)(h).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		// Run ExtractAPIKey, then RequireAuthentication, then our success
		// handler (which should not be hit).
		h := httpmw.RequireAuthentication()(successHandler)
		httpmw.ExtractActor(db)(h).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

// TODO: Dean - write a test for an incorrect actor type once we have more actor
// types.
func TestRequireActor(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(rw, http.StatusOK, httpapi.Response{
			Message: "success",
		})
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		var (
			db               = databasefake.New()
			u, apiKey, token = setupUserAndAPIKey(t, db)
			r                = httptest.NewRequest("GET", "/", nil)
			rw               = httptest.NewRecorder()
		)
		r.AddCookie(&http.Cookie{
			Name:  session.AuthCookie,
			Value: token,
		})

		// Run ExtractAPIKey, then RequireAuthentication, then our success
		// handler.
		h := httpmw.RequireActor(session.ActorTypeUser)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check the actor.
			act := httpmw.RequestActor(r)
			require.Equal(t, session.ActorTypeUser, act.Type())
			userActor, ok := act.(session.UserActor)
			require.True(t, ok)
			require.Equal(t, u, *userActor.User())
			require.Equal(t, apiKey, *userActor.APIKey())

			httpapi.Write(rw, http.StatusOK, httpapi.Response{
				Message: "success",
			})
		}))
		httpmw.ExtractActor(db)(h).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		// Run ExtractAPIKey, then RequireAuthentication, then our success
		// handler (which should not be hit).
		h := httpmw.RequireActor(session.ActorTypeUser)(successHandler)
		httpmw.ExtractActor(db)(h).ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func setupUserAndAPIKey(t *testing.T, db database.Store) (database.User, database.APIKey, string) {
	t.Helper()

	var (
		keyID, keySecret = randomAPIKeyParts()
		hashed           = sha256.Sum256([]byte(keySecret))
		now              = database.Now()
	)

	id, err := uuid.NewRandom()
	require.NoError(t, err)

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

	apiKey, err := db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
		ID:           keyID,
		HashedSecret: hashed[:],
		UserID:       user.ID,
		Application:  false,
		Name:         "test-key-" + keyID,
		LastUsed:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
		CreatedAt:    now,
		UpdatedAt:    now,
		LoginType:    database.LoginTypeBuiltIn,
	})
	require.NoError(t, err, "insert API key")

	return user, apiKey, fmt.Sprintf("%v-%v", keyID, keySecret)
}
