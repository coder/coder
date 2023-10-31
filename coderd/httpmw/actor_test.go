package httpmw_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestRequireAPIKeyOrWorkspaceProxyAuth(t *testing.T) {
	t.Parallel()

	t.Run("None", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()

		httpmw.RequireAPIKeyOrWorkspaceProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not have been called")
		})).ServeHTTP(rw, r)

		require.Equal(t, http.StatusUnauthorized, rw.Code)
	})

	t.Run("APIKey", func(t *testing.T) {
		t.Parallel()

		var (
			db       = dbmem.New()
			user     = dbgen.User(t, db, database.User{})
			_, token = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, token)

		var called int64
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(
			httpmw.RequireAPIKeyOrWorkspaceProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&called, 1)
				rw.WriteHeader(http.StatusOK)
			}))).
			ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		dump, err := httputil.DumpResponse(res, true)
		require.NoError(t, err)
		t.Log(string(dump))

		require.Equal(t, http.StatusOK, rw.Code)
		require.Equal(t, int64(1), atomic.LoadInt64(&called))
	})

	t.Run("WorkspaceProxy", func(t *testing.T) {
		t.Parallel()

		var (
			db           = dbmem.New()
			user         = dbgen.User(t, db, database.User{})
			_, userToken = dbgen.APIKey(t, db, database.APIKey{
				UserID:    user.ID,
				ExpiresAt: dbtime.Now().AddDate(0, 0, 1),
			})
			proxy, proxyToken = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(codersdk.SessionTokenHeader, userToken)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID, proxyToken))

		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
			DB:              db,
			RedirectToLogin: false,
		})(
			httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
				DB: db,
			})(
				httpmw.RequireAPIKeyOrWorkspaceProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusOK)
				})))).
			ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		dump, err := httputil.DumpResponse(res, true)
		require.NoError(t, err)
		t.Log(string(dump))

		require.Equal(t, http.StatusBadRequest, rw.Code)
	})

	t.Run("Both", func(t *testing.T) {
		t.Parallel()

		var (
			db           = dbmem.New()
			proxy, token = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID, token))

		var called int64
		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(
			httpmw.RequireAPIKeyOrWorkspaceProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&called, 1)
				rw.WriteHeader(http.StatusOK)
			}))).
			ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		dump, err := httputil.DumpResponse(res, true)
		require.NoError(t, err)
		t.Log(string(dump))

		require.Equal(t, http.StatusOK, rw.Code)
		require.Equal(t, int64(1), atomic.LoadInt64(&called))
	})
}
