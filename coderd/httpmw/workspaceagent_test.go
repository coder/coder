package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	setup := func(db database.Store) (*http.Request, uuid.UUID) {
		token := uuid.New()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, token.String())
		return r, token
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceAgent(db),
		)
		rtr.Get("/", nil)
		r, _ := setup(db)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractWorkspaceAgent(db),
		)
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			_ = httpmw.WorkspaceAgent(r)
			rw.WriteHeader(http.StatusOK)
		})
		r, token := setup(db)
		_, err := db.InsertWorkspaceAgent(context.Background(), database.InsertWorkspaceAgentParams{
			ID:        uuid.New(),
			AuthToken: token,
		})
		require.NoError(t, err)
		require.NoError(t, err)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
