package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestNoStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		Handler http.HandlerFunc

		expectStatus       int
		expectCacheControl string
		expectPragma       string
		// assert runs extra checks against the recorded response.
		assert func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			// The POST /oauth2/tokens shape.
			Name: "OK",
			Handler: func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusOK)
				_, _ = rw.Write([]byte(`{"access_token":"secret"}`))
			},
			expectStatus:       http.StatusOK,
			expectCacheControl: "no-store",
			expectPragma:       "no-cache",
		},
		{
			// The DELETE /oauth2/clients/{client_id} shape: a response with
			// no body still carries headers.
			Name: "NoContent",
			Handler: func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusNoContent)
			},
			expectStatus:       http.StatusNoContent,
			expectCacheControl: "no-store",
			expectPragma:       "no-cache",
		},
		{
			// The POST /oauth2/authorize shape: http.Redirect adds Location
			// and calls WriteHeader without clearing the header map.
			Name: "Redirect",
			Handler: func(rw http.ResponseWriter, r *http.Request) {
				http.Redirect(rw, r, "https://example.com/callback?code=abc", http.StatusFound)
			},
			expectStatus:       http.StatusFound,
			expectCacheControl: "no-store",
			expectPragma:       "no-cache",
			assert: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, "https://example.com/callback?code=abc", res.Header().Get("Location"))
			},
		},
		{
			// The revoke.go and registration.go shape: a bare WriteHeader
			// that never reaches httpapi.Write.
			Name: "BareWriteHeaderError",
			Handler: func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusBadRequest)
			},
			expectStatus:       http.StatusBadRequest,
			expectCacheControl: "no-store",
			expectPragma:       "no-cache",
		},
		{
			// The headers are advisory: a handler that writes its own
			// Cache-Control wins. No handler under /oauth2 does, which the
			// integration tests pin, but the contract belongs in a test
			// rather than only in prose.
			Name: "HandlerOverwrites",
			Handler: func(rw http.ResponseWriter, _ *http.Request) {
				rw.Header().Set("Cache-Control", "private")
				rw.WriteHeader(http.StatusOK)
			},
			expectStatus:       http.StatusOK,
			expectCacheControl: "private",
			expectPragma:       "no-cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			res := httptest.NewRecorder()
			httpmw.NoStore(tt.Handler).ServeHTTP(res, req)

			require.Equal(t, tt.expectStatus, res.Code)
			require.Equal(t, tt.expectCacheControl, res.Header().Get("Cache-Control"))
			require.Equal(t, tt.expectPragma, res.Header().Get("Pragma"))
			if tt.assert != nil {
				tt.assert(t, res)
			}
		})
	}
}

// TestNoStoreAfterExperimentGate pins the ordering consequence of mounting
// NoStore after the experiment gate on the /oauth2 tree: a request the gate
// rejects never reaches NoStore, so its response carries neither header. That
// is correct, since the rejection contains no credential. The gate that runs
// in production, RequireExperimentWithDevBypass, cannot be exercised from a
// test binary because buildinfo.IsDev() is true there and bypasses the check,
// so this asserts against the RequireExperiment it delegates to.
func TestNoStoreAfterExperimentGate(t *testing.T) {
	t.Parallel()

	handler := httpmw.RequireExperiment(codersdk.Experiments{}, codersdk.ExperimentOAuth2)(
		httpmw.NoStore(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/oauth2/tokens", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code)
	require.Empty(t, res.Header().Get("Cache-Control"))
	require.Empty(t, res.Header().Get("Pragma"))
}
