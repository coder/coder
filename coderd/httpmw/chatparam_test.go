package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestChatParam(t *testing.T) {
	t.Parallel()

	setupAuthentication := func(db database.Store) (*http.Request, database.User) {
		user := dbgen.User(t, db, database.User{})
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: user.ID,
		})

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		ctx := chi.NewRouteContext()
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r, user
	}

	insertChat := func(t *testing.T, db database.Store, ownerID, organizationID uuid.UUID) database.Chat {
		t.Helper()

		_ = dbgen.ChatProvider(t, db, database.ChatProvider{
			APIKey:    "test-api-key",
			BaseUrl:   "https://api.openai.com/v1",
			CreatedBy: uuid.NullUUID{UUID: ownerID, Valid: true},
		})

		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			IsDefault: true,
		})

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    organizationID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfig.ID,
			Title:             "Test chat",
		})

		return chat
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractChatParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractChatParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("chat", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("BadUUID", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractChatParam(db))
		rtr.Get("/", nil)

		r, _ := setupAuthentication(db)
		chi.RouteContext(r.Context()).URLParams.Add("chat", "not-a-uuid")
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractChatParam(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.ChatParam(r)
			rw.WriteHeader(http.StatusOK)
		})

		r, user := setupAuthentication(db)
		org := dbgen.Organization(t, db, database.Organization{})
		chat := insertChat(t, db, user.ID, org.ID)

		chi.RouteContext(r.Context()).URLParams.Add("chat", chat.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
