package codersdk_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestLivenessCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		accessURLFn func(t *testing.T) *url.URL
		expectedErr string
	}{
		{
			name: "OK",
			accessURLFn: func(t *testing.T) *url.URL {
				mux := http.NewServeMux()
				mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("OK"))
					require.NoError(t, err)
				})

				srv := httptest.NewServer(mux)
				t.Cleanup(srv.Close)

				u, err := url.Parse(srv.URL)
				require.NoError(t, err)
				return u
			},
		},
		{
			name: "server not running",
			accessURLFn: func(t *testing.T) *url.URL {
				// See coderd/workspaceapps/apptest/setup.go.
				u, err := url.Parse("http://127.1.0.1:396")
				require.NoError(t, err)
				return u
			},
			expectedErr: "liveness check failed",
		},
		{
			name: "server is not yet live",
			accessURLFn: func(t *testing.T) *url.URL {
				mux := http.NewServeMux()
				mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("NOK"))
					require.NoError(t, err)
				})

				srv := httptest.NewServer(mux)
				t.Cleanup(srv.Close)

				u, err := url.Parse(srv.URL)
				require.NoError(t, err)
				return u
			},
			expectedErr: "liveness check returned non-OK body",
		},
		{
			name: "liveness check route not found",
			accessURLFn: func(t *testing.T) *url.URL {
				srv := httptest.NewServer(http.NewServeMux())
				t.Cleanup(srv.Close)

				u, err := url.Parse(srv.URL)
				require.NoError(t, err)
				return u
			},
			expectedErr: "liveness check returned non-200 response: HTTP 404",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			u := tc.accessURLFn(t)
			client := codersdk.New(u)
			err := client.CheckLiveness(ctx)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.expectedErr)
			}
		})
	}
}
