package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/legacyscim"
	"github.com/coder/coder/v2/enterprise/coderd/scim"
)

func (api *API) mountScimRoute(opt *Options, r chi.Router) error {
	if len(opt.SCIMAPIKey) == 0 {
		// Show a helpful 404 error. Because this is not under the /api/v2 routes,
		// the frontend is the fallback. A html page is not a helpful error for
		// a SCIM provider. This JSON has a call to action that __may__ resolve
		// the issue.
		//
		// Using mount to cover all subroute possibilities
		r.Mount("/", http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusNotFound, codersdk.Response{
				Message: "SCIM is disabled, please contact your administrator if you believe this is an error",
				Detail:  "SCIM endpoints are disabled if no SCIM is configured. Configure 'CODER_SCIM_AUTH_HEADER' to enable.",
			})
		})))
		return nil
	}

	if opt.UseLegacySCIM {
		// Legacy SCIM handler (imulab/go-scim based). Opt-in for
		// backward compatibility during the transition period.
		legacySrv := &legacyscim.LegacyServer{
			Logger:     opt.Logger,
			Database:   opt.Database,
			IDPSync:    opt.IDPSync,
			AGPL:       api.AGPL,
			AccessURL:  api.AccessURL,
			SCIMAPIKey: opt.SCIMAPIKey,
			Auditor:    &api.AGPL.Auditor,
		}
		r.Mount("/v2", chi.Chain(
			api.RequireFeatureMW(codersdk.FeatureSCIM),
			legacySrv.AuthMiddleware,
		).Handler(legacySrv.Handler()))
		return nil
	}

	// SCIM 2.0 handler (elimity-com/scim based).
	scimSrv, err := scim.New(&scim.Options{
		DB:         opt.Database,
		Auditor:    &api.AGPL.Auditor,
		IDPSync:    opt.IDPSync,
		Logger:     opt.Logger,
		AGPL:       api.AGPL,
		SCIMAPIKey: opt.SCIMAPIKey,
	})
	if err != nil {
		return xerrors.Errorf("create scim server: %w", err)
	}

	// The elimity-com/scim library reads r.URL.Path and strips "/v2"
	// internally. Chi's Route/Mount modifies its own routing context
	// but not r.URL.Path, so we use http.StripPrefix to ensure the
	// library sees paths like "/v2/Users" instead of "/scim/v2/Users".
	r.Mount("/", chi.Chain(
		api.RequireFeatureMW(codersdk.FeatureSCIM),
		middleware.StripPrefix("/scim"),
	).Handler(scimSrv.Handler()))
	return nil
}
