package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestRequireOAuth2Provider(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		enabled    bool
		wantStatus int
		wantNext   bool
	}{
		{name: "Disabled", enabled: false, wantStatus: http.StatusNotFound, wantNext: false},
		{name: "Enabled", enabled: true, wantStatus: http.StatusOK, wantNext: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nextCalled := false
			handler := httpmw.RequireOAuth2Provider(tc.enabled)(
				http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
					nextCalled = true
					rw.WriteHeader(http.StatusOK)
				}),
			)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth2/authorize", nil))

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantNext, nextCalled)
			if !tc.enabled {
				require.JSONEq(t, `{"message":"Route not found."}`, rec.Body.String())
			}
		})
	}
}
