package scim

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"
)

// New constructs the SCIM HTTP handler. The returned handler should be
// mounted under /scim/v2; it owns routing for /Users, /Schemas,
// /ResourceTypes, and /ServiceProviderConfig within that prefix.
//
// The handler performs its own bearer-token authentication using
// opts.APIKey. Callers should still gate the mount with the SCIM
// feature flag via RequireFeatureMW.
func New(opts Options) (http.Handler, error) {
	if opts.Database == nil {
		return nil, xerrors.New("scim: Database is required")
	}
	if opts.Auditor == nil {
		return nil, xerrors.New("scim: Auditor is required")
	}
	if opts.IDPSync == nil {
		return nil, xerrors.New("scim: IDPSync is required")
	}
	if opts.CreateUser == nil {
		return nil, xerrors.New("scim: CreateUser is required")
	}

	uh := &userHandler{opts: opts}
	cfg := serviceProviderConfig()
	srv, err := scim.NewServer(&scim.ServerArgs{
		ServiceProviderConfig: &cfg,
		ResourceTypes:         []scim.ResourceType{userResourceType(uh)},
	})
	if err != nil {
		return nil, xerrors.Errorf("scim: build server: %w", err)
	}

	return authMiddleware(opts.APIKey)(chiPathRewriter(srv)), nil
}

// authMiddleware verifies the SCIM bearer token on every request
// except GET /ServiceProviderConfig, which the spec allows to be
// unauthenticated for capability discovery.
func authMiddleware(apiKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isPublicEndpoint(r) && !verifyAuthHeader(r, apiKey) {
				writeSCIMError(w, &scimerrors.ScimError{
					ScimType: "invalidAuthorization",
					Detail:   "invalid authorization",
					Status:   http.StatusUnauthorized,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isPublicEndpoint reports whether the request targets a SCIM endpoint
// that does not require authentication. Today only
// /ServiceProviderConfig is public, matching pre-refactor behavior.
func isPublicEndpoint(r *http.Request) bool {
	path := routePath(r)
	return path == "/ServiceProviderConfig"
}

// verifyAuthHeader compares the request's Authorization header against
// the configured shared key in constant time. The check is case
// insensitive on the "Bearer" prefix.
func verifyAuthHeader(r *http.Request, apiKey []byte) bool {
	if len(apiKey) == 0 {
		return false
	}
	bearer := []byte("bearer ")
	hdr := []byte(r.Header.Get("Authorization"))
	if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(bytes.ToLower(hdr[:len(bearer)]), bearer) == 1 {
		hdr = hdr[len(bearer):]
	}
	return subtle.ConstantTimeCompare(hdr, apiKey) == 1
}

// chiPathRewriter rewrites the request URL to the chi-stripped
// RoutePath before delegating to the elimity Server. chi does not
// modify r.URL.Path when matching nested routes; it stashes the
// remaining path in chi.RouteContext.RoutePath instead. The elimity
// Server reads URL.Path directly, so without this shim every request
// would fall through to the framework's "not found" branch.
func chiPathRewriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
			r2 := *r
			url2 := *r.URL
			url2.Path = rctx.RoutePath
			r2.URL = &url2
			next.ServeHTTP(w, &r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// routePath returns the request path as seen by the SCIM server: the
// chi-stripped RoutePath when present, otherwise URL.Path.
func routePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
		return rctx.RoutePath
	}
	return r.URL.Path
}

// writeSCIMError writes a SCIM-shaped error response. The elimity
// framework owns this format internally but exposes no exported helper;
// we duplicate the minimal envelope for our auth middleware.
func writeSCIMError(w http.ResponseWriter, e *scimerrors.ScimError) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(e.Status)
	body := map[string]interface{}{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"status":  http.StatusText(e.Status),
		"detail":  e.Detail,
	}
	if e.ScimType != "" {
		body["scimType"] = string(e.ScimType)
	}
	_ = json.NewEncoder(w).Encode(body)
}
