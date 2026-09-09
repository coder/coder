package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func TestWorkspaceAppCors(t *testing.T) {
	t.Parallel()

	regex, err := appurl.CompileHostnamePattern("*--apps.dev.coder.com")
	require.NoError(t, err)

	methods := []string{
		http.MethodOptions,
		http.MethodHead,
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	tests := []struct {
		name    string
		origin  string
		app     appurl.ApplicationURL
		allowed bool
	}{
		{
			name:   "Self",
			origin: "https://3000--agent--ws--user--apps.dev.coder.com",
			app: appurl.ApplicationURL{
				AppSlugOrPort: "3000",
				AgentName:     "agent",
				WorkspaceName: "ws",
				Username:      "user",
			},
			allowed: true,
		},
		{
			name:   "SameWorkspace",
			origin: "https://8000--agent--ws--user--apps.dev.coder.com",
			app: appurl.ApplicationURL{
				AppSlugOrPort: "3000",
				AgentName:     "agent",
				WorkspaceName: "ws",
				Username:      "user",
			},
			allowed: true,
		},
		{
			name:   "SameUser",
			origin: "https://8000--agent2--ws2--user--apps.dev.coder.com",
			app: appurl.ApplicationURL{
				AppSlugOrPort: "3000",
				AgentName:     "agent",
				WorkspaceName: "ws",
				Username:      "user",
			},
			allowed: true,
		},
		{
			name:   "DifferentOriginOwner",
			origin: "https://3000--agent--ws--user2--apps.dev.coder.com",
			app: appurl.ApplicationURL{
				AppSlugOrPort: "3000",
				AgentName:     "agent",
				WorkspaceName: "ws",
				Username:      "user",
			},
			allowed: false,
		},
		{
			name:   "DifferentHostOwner",
			origin: "https://3000--agent--ws--user--apps.dev.coder.com",
			app: appurl.ApplicationURL{
				AppSlugOrPort: "3000",
				AgentName:     "agent",
				WorkspaceName: "ws",
				Username:      "user2",
			},
			allowed: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, method := range methods {
				r := httptest.NewRequest(method, "http://localhost", nil)
				r.Header.Set("Origin", test.origin)
				rw := httptest.NewRecorder()

				// Preflight requests need to know what method will be requested.
				if method == http.MethodOptions {
					r.Header.Set("Access-Control-Request-Method", method)
				}

				handler := httpmw.WorkspaceAppCors(regex, test.app)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusNoContent)
				}))

				handler.ServeHTTP(rw, r)

				if test.allowed {
					require.Equal(t, test.origin, rw.Header().Get("Access-Control-Allow-Origin"))
				} else {
					require.Equal(t, "", rw.Header().Get("Access-Control-Allow-Origin"))
				}

				// For options we should never get to our handler as the middleware
				// short-circuits with a 200.
				if method == http.MethodOptions {
					require.Equal(t, http.StatusOK, rw.Code)
				} else {
					require.Equal(t, http.StatusNoContent, rw.Code)
				}
			}
		})
	}
}

func TestCors(t *testing.T) {
	t.Parallel()

	const origin = "https://app.example.com"

	handler := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	})

	// send returns the recorded response for a cross-origin request. An
	// OPTIONS request is sent as a preflight for a POST.
	send := func(t *testing.T, allowAll bool, method, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Origin", origin)
		if method == http.MethodOptions {
			r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		}
		rw := httptest.NewRecorder()
		httpmw.Cors(allowAll)(handler).ServeHTTP(rw, r)
		return rw
	}

	tests := []struct {
		name       string
		path       string
		permissive bool
	}{
		{name: "Authorize", path: "/oauth2/authorize", permissive: false},
		{name: "AuthorizeTrailingSlash", path: "/oauth2/authorize/", permissive: false},
		{name: "AuthorizeSubpath", path: "/oauth2/authorize/extra", permissive: false},
		{name: "Tokens", path: "/oauth2/tokens", permissive: true},
		{name: "Revoke", path: "/oauth2/revoke", permissive: true},
		{name: "Register", path: "/oauth2/register", permissive: true},
		{name: "Clients", path: "/oauth2/clients/abc", permissive: true},
		{name: "AuthorizationServerMetadata", path: "/.well-known/oauth-authorization-server", permissive: true},
		{name: "ProtectedResourceMetadata", path: "/.well-known/oauth-protected-resource", permissive: true},
		{name: "MCP", path: "/api/v2/mcp/http", permissive: true},
		{name: "API", path: "/api/v2/users/me", permissive: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
				rw := send(t, false, method, test.path)

				if test.permissive {
					require.Equal(t, "*", rw.Header().Get("Access-Control-Allow-Origin"), method)
					if method == http.MethodOptions {
						require.Equal(t, "86400", rw.Header().Get("Access-Control-Max-Age"))
					}
				} else {
					for name := range rw.Header() {
						require.False(t, strings.HasPrefix(name, "Access-Control-"), "%s %s sent %s", method, test.path, name)
					}
				}

				// The middleware answers preflights itself with a 200.
				if method == http.MethodOptions {
					require.Equal(t, http.StatusOK, rw.Code)
				} else {
					require.Equal(t, http.StatusNoContent, rw.Code)
				}
			}
		})
	}

	// With every origin allowed, the authorization endpoint behaves like the
	// rest of the site.
	t.Run("AllowAll", func(t *testing.T) {
		t.Parallel()

		rw := send(t, true, http.MethodGet, "/oauth2/authorize")
		require.Equal(t, "*", rw.Header().Get("Access-Control-Allow-Origin"))
	})
}
