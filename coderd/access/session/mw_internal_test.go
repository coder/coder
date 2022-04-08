package session

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/httpapi"
)

func TestMiddleware(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(rw, http.StatusOK, httpapi.Response{
			Message: "it worked!",
		})
	})

	t.Run("UserActor", func(t *testing.T) {
		t.Parallel()

		t.Run("Error", func(t *testing.T) {
			t.Parallel()
			var (
				db = databasefake.New()
				r  = httptest.NewRequest("GET", "/", nil)
				rw = httptest.NewRecorder()
			)
			r.AddCookie(&http.Cookie{
				Name:  AuthCookie,
				Value: "invalid-api-key",
			})

			ExtractActor(db)(successHandler).ServeHTTP(rw, r)
			res := rw.Result()
			defer res.Body.Close()
			require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			var (
				db       = databasefake.New()
				u        = newUser(t, db)
				_, token = newAPIKey(t, db, u, time.Time{}, time.Time{})
				r        = httptest.NewRequest("GET", "/", nil)
				rw       = httptest.NewRecorder()
			)
			r.AddCookie(&http.Cookie{
				Name:  AuthCookie,
				Value: token,
			})

			var (
				called  int64
				handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					atomic.AddInt64(&called, 1)

					// Double check the UserActor.
					act := RequestActor(r)
					require.NotNil(t, act)
					require.Equal(t, ActorTypeUser, act.Type())
					require.Equal(t, u.ID.String(), act.ID())
					require.Equal(t, u.Username, act.Name())

					userActor, ok := act.(UserActor)
					require.True(t, ok)
					require.Equal(t, u, *userActor.User())

					httpapi.Write(rw, http.StatusOK, httpapi.Response{
						Message: "success",
					})
				})
			)

			ExtractActor(db)(handler).ServeHTTP(rw, r)
			res := rw.Result()
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			require.EqualValues(t, 1, called)
		})
	})

	t.Run("Fallthrough", func(t *testing.T) {
		t.Parallel()

		var (
			db = databasefake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		var (
			called  int64
			handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&called, 1)

				// Actor should be nil.
				act := RequestActor(r)
				require.Nil(t, act)

				httpapi.Write(rw, http.StatusOK, httpapi.Response{
					Message: "success",
				})
			})
		)

		// No auth provided.
		ExtractActor(db)(handler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		require.EqualValues(t, 1, called)
	})
}
