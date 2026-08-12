package scim

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/elimity-com/scim"
	scimErrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/idpsync"
)

// Handler wraps the elimity-com/scim library's Server to implement
// SCIM 2.0 endpoints. The library auto-serves /Schemas, /ResourceTypes,
// and /ServiceProviderConfig from schema definitions.
type Handler struct {
	opts *Options
	srv  *scim.Server
}

// Options holds all the dependencies needed by SCIM resource handlers.
type Options struct {
	DB      database.Store
	Auditor *atomic.Pointer[audit.Auditor]
	IDPSync idpsync.IDPSync
	Logger  slog.Logger

	// AGPL is needed for CreateUser.
	AGPL *agpl.API

	// SCIMAPIKey is the bearer token used to authenticate SCIM requests.
	SCIMAPIKey []byte
}

func New(opts *Options) (*Handler, error) {
	userHandler := &ResourceUser{
		store: opts.DB,
		opts:  opts,
	}

	args := &scim.ServerArgs{
		ServiceProviderConfig: &scim.ServiceProviderConfig{
			DocumentationURI: optional.NewString("https://coder.com/docs/admin/users/oidc-auth#scim"),
			AuthenticationSchemes: []scim.AuthenticationScheme{
				{
					Type:        scim.AuthenticationTypeOauthBearerToken,
					Name:        "HTTP Header Authentication",
					Description: "Authentication scheme using the Authorization header with the shared token",
					// TODO: Add documentation links for these specific docs once they exist.
					SpecURI:          optional.String{},
					DocumentationURI: optional.String{},
					Primary:          true,
				},
			},
			MaxResults: 0,
			// SupportFiltering is set to false, as all filtering operations are not
			// supported. A minimal filtering syntax is supported because Okta seems to
			// ignore this field and attempt to filter anyway.
			SupportFiltering: false,
			SupportPatch:     true,
		},
		ResourceTypes: []scim.ResourceType{
			{
				ID:               optional.NewString("User"),
				Name:             "User",
				Description:      optional.NewString("User Account"),
				Endpoint:         "/Users",
				Schema:           schema.CoreUserSchema(),
				Handler:          userHandler,
				SchemaExtensions: nil,
			},
		},
	}

	srv, err := scim.NewServer(args)
	if err != nil {
		return nil, err
	}

	return &Handler{
		opts: opts,
		srv:  &srv,
	}, nil
}

func (s *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !s.verifyAuthHeader(r) {
			scimUnauthorized(rw)
			return
		}

		// All authenticated requests are treated as coming from the SCIM provisioner
		//nolint:gocritic // auth header authenticates as this identity
		ctx := dbauthz.AsSCIMProvisioner(r.Context())
		r = r.WithContext(ctx)

		next.ServeHTTP(rw, r)
	})
}

// Handler returns the SCIM 2.0 handler. Middleware passed as inner runs after
// the SCIM API key check and before the SCIM server, so work that a caller can
// trigger without authenticating stays out of reach of an unauthenticated
// caller. The size bound on the request body is mounted this way: buffering a
// body for a caller who is about to be rejected at the API key check is work an
// unauthenticated caller should not be able to cause.
func (s *Handler) Handler(inner ...func(http.Handler) http.Handler) http.Handler {
	return s.authMiddleware(chi.Chain(inner...).Handler(s.srv))
}

func (s *Handler) verifyAuthHeader(r *http.Request) bool {
	bearer := []byte("bearer ")
	hdr := []byte(r.Header.Get("Authorization"))

	// Case-insensitive comparison of the "Bearer " prefix.
	if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(bytes.ToLower(hdr[:len(bearer)]), bearer) == 1 {
		hdr = hdr[len(bearer):]
	}

	return len(s.opts.SCIMAPIKey) != 0 && subtle.ConstantTimeCompare(hdr, s.opts.SCIMAPIKey) == 1
}

// WriteError writes an RFC 7644 section 3.12 error response, per
// https://datatracker.ietf.org/doc/html/rfc7644#section-3.12. SCIM providers
// parse that shape, so a codersdk.Response here would be a protocol violation
// even with the right status.
//
// RFC 7644 expresses status as a JSON string, which is what ScimError marshals.
// The legacy implementation's library writes it as a number, so the two paths
// differ on that field; this is the compliant form and is not the one to change.
func WriteError(rw http.ResponseWriter, status int, detail string) {
	rw.Header().Set("Content-Type", "application/scim+json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(scimErrors.ScimError{
		// RFC 7644 defines scimType keywords only for 400 responses.
		ScimType: "",
		Detail:   detail,
		Status:   status,
	})
}

func scimUnauthorized(rw http.ResponseWriter) {
	WriteError(rw, http.StatusUnauthorized, "invalid authorization")
}
