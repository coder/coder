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

	// serve runs the middleware with the given token and returns the status
	// code. handler is only reached when the token is accepted.
	serve := func(t *testing.T, token string, handler http.HandlerFunc) int {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		if token != "" {
			r.Header.Set(codersdk.ExitNodeTokenHeader, token)
		}
		httpmw.ExtractExitNode(db)(handler).ServeHTTP(rw, r)
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
	newNode := func(t *testing.T) (database.ExitNode, string) {
		t.Helper()
		return dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: org.ID})
	}

	for _, tc := range []struct {
		name  string
		token func(t *testing.T) string
	}{
		{"NoHeader", func(*testing.T) string { return "" }},
		{"InvalidFormat", func(*testing.T) string { return "not-a-token" }},
		{"InvalidID", func(t *testing.T) string { return "test:" + randomSecret(t) }},
		{"InvalidSecretLength", func(*testing.T) string { return uuid.NewString() + ":wow" }},
		{"NotFound", func(t *testing.T) string { return uuid.NewString() + ":" + randomSecret(t) }},
		{"InvalidSecret", func(t *testing.T) string {
			node, _ := newNode(t)
			// Use a different secret so they don't match.
			return fmt.Sprintf("%s:%s", node.ID, randomSecret(t))
		}},
		{"Deleted", func(t *testing.T) string {
			node, secret := newNode(t)
			require.NoError(t, db.DeleteExitNodeByID(context.Background(), node.ID))
			return fmt.Sprintf("%s:%s", node.ID, secret)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status := serve(t, tc.token(t), func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler must not run for a rejected token")
			})
			require.Equal(t, http.StatusUnauthorized, status)
		})
	}

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		node, secret := newNode(t)
		status := serve(t, fmt.Sprintf("%s:%s", node.ID, secret), func(rw http.ResponseWriter, r *http.Request) {
			require.Equal(t, node.ID, httpmw.ExitNode(r).ID)
			fromCtx, ok := httpmw.ExitNodeFromContext(r.Context())
			require.True(t, ok)
			require.Equal(t, node.ID, fromCtx.ID)
			httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{Message: "It worked!"})
		})
		require.Equal(t, http.StatusOK, status)
	})
}
