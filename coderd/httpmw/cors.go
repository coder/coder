package httpmw

import (
	"net/http"
	"net/url"
	"regexp"

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
		// the secure behavior of blocking CORs requests.
		origins = []string{""}
	}
	if allowAll {
		origins = []string{"*"}
	}
	return cors.Handler(cors.Options{
		AllowedOrigins: origins,
		// We only need GET for latency requests
		AllowedMethods: []string{http.MethodOptions, http.MethodGet},
		AllowedHeaders: []string{"Accept", "Content-Type", "X-LATENCY-CHECK", "X-CSRF-TOKEN"},
		// Do not send any cookies
		AllowCredentials: false,
	})
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
