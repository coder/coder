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

// PermissiveCors creates a very permissive CORS middleware that allows all origins,
// methods, and headers. This bypasses go-chi's CORS library for maximum compatibility.
func PermissiveCors() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD, PATCH, SNARF") // TODO: remove SNARF.
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ConditionalCors applies permissive CORS for requests with the specified prefix,
// and regular CORS for all other requests.
func ConditionalCors(prefix string, regularCors, permissiveCors func(next http.Handler) http.Handler) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, prefix) {
				permissiveCors(next).ServeHTTP(w, r)
			} else {
				regularCors(next).ServeHTTP(w, r)
			}
		})
	}
}
