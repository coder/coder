package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/testutil"
)

func TestHTTPRoute(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		reqFn          func() *http.Request
		registerRoutes map[string]string
		mws            []func(http.Handler) http.Handler
		expectedRoute  string
		expectedMethod string
		expectNotFound bool
	}{
		{
			name: "without middleware",
			reqFn: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			registerRoutes: map[string]string{http.MethodGet: "/"},
			mws:            []func(http.Handler) http.Handler{},
			expectedRoute:  "",
			expectedMethod: "",
		},
		{
			name: "root",
			reqFn: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			registerRoutes: map[string]string{http.MethodGet: "/"},
			mws:            []func(http.Handler) http.Handler{httpmw.HTTPRoute},
			expectedRoute:  "/",
			expectedMethod: http.MethodGet,
		},
		{
			name: "parameterized route",
			reqFn: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/users/123", nil)
			},
			registerRoutes: map[string]string{http.MethodPut: "/users/{id}"},
			mws:            []func(http.Handler) http.Handler{httpmw.HTTPRoute},
			expectedRoute:  "/users/{id}",
			expectedMethod: http.MethodPut,
		},
		{
			name: "unknown",
			reqFn: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/a", nil)
			},
			registerRoutes: map[string]string{http.MethodGet: "/api/b"},
			mws:            []func(http.Handler) http.Handler{httpmw.HTTPRoute},
			expectedRoute:  "UNKNOWN",
			expectedMethod: http.MethodGet,
		},
		{
			name: "static",
			reqFn: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/some/static/file.png", nil)
			},
			registerRoutes: map[string]string{http.MethodGet: "/"},
			mws:            []func(http.Handler) http.Handler{httpmw.HTTPRoute},
			expectedRoute:  "STATIC",
			expectedMethod: http.MethodGet,
			expectNotFound: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			r := chi.NewRouter()
			done := make(chan string)
			for _, mw := range tc.mws {
				r.Use(mw)
			}
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer close(done)
					method := httpmw.ExtractHTTPMethod(r.Context())
					route := httpmw.ExtractHTTPRoute(r.Context())
					assert.Equal(t, tc.expectedMethod, method, "expected method mismatch")
					assert.Equal(t, tc.expectedRoute, route, "expected route mismatch")
					next.ServeHTTP(w, r)
				})
			})
			for method, route := range tc.registerRoutes {
				r.MethodFunc(method, route, func(w http.ResponseWriter, r *http.Request) {})
			}
			req := tc.reqFn()
			r.ServeHTTP(httptest.NewRecorder(), req)
			_ = testutil.TryReceive(ctx, t, done)
		})
	}
}
