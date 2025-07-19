package httpmw

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/cors"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

const (
	// Server headers.
	AccessControlAllowOriginHeader      = "Access-Control-Allow-Origin"
	AccessControlAllowCredentialsHeader = "Access-Control-Allow-Credentials"
	AccessControlAllowMethodsHeader     = "Access-Control-Allow-Methods"
	AccessControlAllowHeadersHeader     = "Access-Control-Allow-Headers"
	VaryHeader                          = "Vary"

	// Client headers.
	OriginHeader                      = "Origin"
	AccessControlRequestMethodsHeader = "Access-Control-Request-Methods"
	AccessControlRequestHeadersHeader = "Access-Control-Request-Headers"
)

//nolint:revive
func Cors(allowAll bool, origins ...string) func(next http.Handler) http.Handler {
	if len(origins) == 0 {
		// The default behavior is '*', so putting the empty string defaults to
		// the secure behavior of blocking CORS requests.
		origins = []string{""}
	}
	if allowAll {
		origins = []string{"*"}
	}

	// Standard CORS for most endpoints
	standardCors := cors.Handler(cors.Options{
		AllowedOrigins: origins,
		// We only need GET for latency requests
		AllowedMethods: []string{http.MethodOptions, http.MethodGet},
		AllowedHeaders: []string{"Accept", "Content-Type", "X-LATENCY-CHECK", "X-CSRF-TOKEN"},
		// Do not send any cookies
		AllowCredentials: false,
	})

	// Permissive CORS for OAuth2 and MCP endpoints
	permissiveCors := cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Content-Type",
			"Accept",
			"Authorization",
			"x-api-key",
			"Mcp-Session-Id",
			"MCP-Protocol-Version",
			"Last-Event-ID",
		},
		ExposedHeaders: []string{
			"Content-Type",
			"Authorization",
			"x-api-key",
			"Mcp-Session-Id",
			"MCP-Protocol-Version",
		},
		MaxAge:           86400, // 24 hours in seconds
		AllowCredentials: false,
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use permissive CORS for OAuth2, MCP, and well-known endpoints
			if strings.HasPrefix(r.URL.Path, "/oauth2/") ||
				strings.HasPrefix(r.URL.Path, "/api/experimental/mcp/") ||
				strings.HasPrefix(r.URL.Path, "/.well-known/oauth-") {
				permissiveCors(next).ServeHTTP(w, r)
				return
			}

			// Use standard CORS for all other endpoints
			standardCors(next).ServeHTTP(w, r)
		})
	}
}

func WorkspaceAppCors(regex *regexp.Regexp, app appurl.ApplicationURL) func(next http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowOriginFunc: func(_ *http.Request, rawOrigin string) bool {
			origin, err := url.Parse(rawOrigin)
			if rawOrigin == "" || origin.Host == "" || err != nil {
				return false
			}
			subdomain, ok := appurl.ExecuteHostnamePattern(regex, origin.Host)
			if !ok {
				return false
			}
			originApp, err := appurl.ParseSubdomainAppURL(subdomain)
			if err != nil {
				return false
			}
			return ok && originApp.Username == app.Username
		},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
}
