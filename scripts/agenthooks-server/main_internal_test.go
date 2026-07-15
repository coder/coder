package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForwardedHTTPSHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		forwardedProto string
		wantScheme     string
	}{
		{name: "direct HTTP", wantScheme: "http"},
		{name: "TLS terminator", forwardedProto: "https", wantScheme: "https"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var scheme string
			handler := forwardedHTTPSHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				scheme = r.URL.Scheme
				rw.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodPost, "http://hooks.example.test", nil)
			request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			require.Equal(t, http.StatusNoContent, response.Code)
			require.Equal(t, test.wantScheme, scheme)
		})
	}
}
