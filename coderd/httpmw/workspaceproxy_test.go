package httpmw_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

func TestExtractWorkspaceProxy(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("NoHeader", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, "test:wow-hello")

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidID", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, "test:wow")

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", uuid.NewString(), "wow"))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		secret, err := cryptorand.HexString(64)
		require.NoError(t, err)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", uuid.NewString(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)

		// Use a different secret so they don't match!
		secret, err := cryptorand.HexString(64)
		require.NoError(t, err)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID.String(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, secret = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID.String(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.WorkspaceProxy(r)
			successHandler.ServeHTTP(rw, r)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, secret = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)
		err := db.UpdateWorkspaceProxyDeleted(context.Background(), database.UpdateWorkspaceProxyDeletedParams{
			ID:      proxy.ID,
			Deleted: true,
		})
		require.NoError(t, err, "failed to delete workspace proxy")

		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID.String(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func TestExtractWorkspaceProxyParam(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("OKName", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("workspaceproxy", proxy.Name)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))

		httpmw.ExtractWorkspaceProxyParam(db, uuid.NewString(), nil)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.WorkspaceProxyParam(request)
			successHandler.ServeHTTP(writer, request)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("OKID", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("workspaceproxy", proxy.ID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))

		httpmw.ExtractWorkspaceProxyParam(db, uuid.NewString(), nil)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.WorkspaceProxyParam(request)
			successHandler.ServeHTTP(writer, request)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbmem.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("workspaceproxy", uuid.NewString())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))

		httpmw.ExtractWorkspaceProxyParam(db, uuid.NewString(), nil)(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("FetchPrimary", func(t *testing.T) {
		t.Parallel()
		var (
			db           = dbmem.New()
			r            = httptest.NewRequest("GET", "/", nil)
			rw           = httptest.NewRecorder()
			deploymentID = uuid.New()
			primaryProxy = database.WorkspaceProxy{
				ID:               deploymentID,
				Name:             "primary",
				DisplayName:      "Default",
				Icon:             "Icon",
				Url:              "Url",
				WildcardHostname: "Wildcard",
			}
			fetchPrimary = func(ctx context.Context) (database.WorkspaceProxy, error) {
				return primaryProxy, nil
			}
		)

		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("workspaceproxy", deploymentID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))

		httpmw.ExtractWorkspaceProxyParam(db, deploymentID.String(), fetchPrimary)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Checks that it exists on the context!
			found := httpmw.WorkspaceProxyParam(request)
			require.Equal(t, primaryProxy, found)
			successHandler.ServeHTTP(writer, request)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
