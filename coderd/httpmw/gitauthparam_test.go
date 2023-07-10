package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpmw"
)

//nolint:bodyclose
func TestGitAuthParam(t *testing.T) {
	t.Parallel()
	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("gitauth", "my-id")
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
		res := httptest.NewRecorder()

		httpmw.ExtractGitAuthParam([]*gitauth.Config{{
			ID: "my-id",
		}})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "my-id", httpmw.GitAuthParam(r).ID)
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, r)

		require.Equal(t, http.StatusOK, res.Result().StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("gitauth", "my-id")
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
		res := httptest.NewRecorder()

		httpmw.ExtractGitAuthParam([]*gitauth.Config{})(nil).ServeHTTP(res, r)

		require.Equal(t, http.StatusNotFound, res.Result().StatusCode)
	})
}
