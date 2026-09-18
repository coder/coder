package httpmw_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

func TestExtractExitNode(t *testing.T) {
	t.Parallel()

	// All subtests only insert uniquely named rows, so one database is
	// shared across them.
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the token passes through the middleware.
		_ = httpmw.ExitNode(r)
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	serve := func(t *testing.T, token string) int {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		if token != "" {
			r.Header.Set(codersdk.ExitNodeTokenHeader, token)
		}
		httpmw.ExtractExitNode(db)(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		return res.StatusCode
	}

	randomSecret := func(t *testing.T) string {
		t.Helper()
		secret, err := cryptorand.HexString(64)
		require.NoError(t, err)
		return secret
	}

	t.Run("NoHeader", func(t *testing.T) {
		t.Parallel()
		status := serve(t, "")
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		status := serve(t, "not-a-token")
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("InvalidID", func(t *testing.T) {
		t.Parallel()
		status := serve(t, "test:"+randomSecret(t))
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		status := serve(t, fmt.Sprintf("%s:%s", uuid.NewString(), "wow"))
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		status := serve(t, fmt.Sprintf("%s:%s", uuid.NewString(), randomSecret(t)))
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		node, _ := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: org.ID})
		// Use a different secret so they don't match.
		status := serve(t, fmt.Sprintf("%s:%s", node.ID, randomSecret(t)))
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		node, secret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: org.ID})
		err := db.DeleteExitNodeByID(context.Background(), node.ID)
		require.NoError(t, err)
		status := serve(t, fmt.Sprintf("%s:%s", node.ID, secret))
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		node, secret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: org.ID})

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		r.Header.Set(codersdk.ExitNodeTokenHeader, fmt.Sprintf("%s:%s", node.ID, secret))
		httpmw.ExtractExitNode(db)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			got := httpmw.ExitNode(r)
			require.Equal(t, node.ID, got.ID)
			fromCtx, ok := httpmw.ExitNodeFromContext(r.Context())
			require.True(t, ok)
			require.Equal(t, node.ID, fromCtx.ID)
			successHandler.ServeHTTP(rw, r)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
