package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractChat(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.User) {
		r := httptest.NewRequest("GET", "/", nil)

		user := dbgen.User(t, db, database.User{
			ID: uuid.New(),
		})
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: user.ID,
		})
		r.Header.Set(codersdk.SessionTokenHeader, token)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		return r, user
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractChatParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("chat", "not-a-uuid")
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractChatParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode) // Changed from NotFound in org test to BadRequest as per chat.go
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db   = dbmem.New()
			rw   = httptest.NewRecorder()
			r, _ = setupAuthentication(db)
			rtr  = chi.NewRouter()
		)
		chi.RouteContext(r.Context()).URLParams.Add("chat", uuid.NewString())
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractChatParam(db),
		)
		rtr.Get("/", nil)
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		var (
			db      = dbmem.New()
			rw      = httptest.NewRecorder()
			r, user = setupAuthentication(db)
			rtr     = chi.NewRouter()
		)

		// Create a test chat
		testChat := dbgen.Chat(t, db, database.Chat{
			ID:        uuid.New(),
			OwnerID:   user.ID,
			CreatedAt: dbtime.Now(),
			UpdatedAt: dbtime.Now(),
			Title:     "Test Chat",
		})

		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractChatParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			chat := httpmw.ChatParam(r)
			require.NotZero(t, chat)
			assert.Equal(t, testChat.ID, chat.ID)
			assert.WithinDuration(t, testChat.CreatedAt, chat.CreatedAt, time.Second)
			assert.WithinDuration(t, testChat.UpdatedAt, chat.UpdatedAt, time.Second)
			assert.Equal(t, testChat.Title, chat.Title)
			rw.WriteHeader(http.StatusOK)
		})

		// Try by ID
		chi.RouteContext(r.Context()).URLParams.Add("chat", testChat.ID.String())
		rtr.ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "by id")
	})
}
