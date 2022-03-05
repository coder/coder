package coderd

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/go-chi/chi/v5"
	"google.golang.org/api/idtoken"

	"cdr.dev/slog"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	AccessURL *url.URL
	Logger    slog.Logger
	Database  database.Store
	Pubsub    database.Pubsub

	GoogleTokenValidator *idtoken.Validator
}

// New constructs the Coder API into an HTTP handler.
//
// A wait function is returned to handle awaiting closure
// of hijacked HTTP requests.
func New(options *Options) (http.Handler, func()) {
	api := &api{
		Options: options,
	}

	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
		r.Post("/login", api.postLogin)
		r.Post("/logout", api.postLogout)

		r.Route("/user", func(r chi.Router) {
			r.Get("/first", api.user)
			r.Post("/first", api.user)
			r.Group(func(r chi.Router) {
				r.Use(httpmw.ExtractAPIKey(options.Database, nil))
				r.Post("/", nil)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/", api.userByName)
					r.Get("/organizations", api.organizationsByUser)
					r.Post("/keys", api.postKeyForUser)
				})
			})
		})

		r.Route("/organization/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Post("/users", nil)
			r.Get("/provisionerdaemons", nil)
			r.Route("/projects", func(r chi.Router) {
				r.Post("/", api.postProjectsByOrganization)
				r.Get("/", api.projectsByOrganization)
				r.Get("/{projectname}", api.projectByOrganizationAndName)
			})
		})
		r.Route("/project/{project}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractProjectParam(options.Database),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Get("/", api.projectByOrganization)
			r.Get("/workspaces", api.workspacesByProject)
			r.Get("/parameters", api.parametersByProject)
			r.Post("/parameters", api.postParametersByProject)
			r.Get("/versions", api.projectVersionsByOrganization)
			r.Post("/versions", api.postProjectVersionByOrganization)
		})
		r.Route("/projectversion/{projectversion}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractProjectVersionParam(options.Database),
				httpmw.ExtractOrganizationParam(options.Database),
			)

			r.Get("/", nil)
			r.Get("/schema", nil)
			r.Get("/parameters", nil)
			r.Get("/logs", nil)
			r.Get("/resources", nil)
		})
		r.Route("/workspace/{workspace}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractWorkspaceParam(options.Database),
				httpmw.ExtractUserParam(options.Database),
			)
			r.Get("/", nil)
			r.Get("/builds", nil)
			r.Post("/builds", nil)
		})
		r.Route("/workspacebuild/{workspacebuild}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/logs", nil)
			r.Get("/resources", nil)
			r.Route("/resources/{workspaceresource}", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceResourceParam(options.Database))
				r.Get("/", nil)
				r.Get("/dial", nil)
			})
		})
		r.Route("/workspaceagent", func(r chi.Router) {
			r.Route("/auth", func(r chi.Router) {
				r.Post("/google-instance-identity", api.postAuthenticateWorkspaceAgentUsingGoogleInstanceIdentity)
			})
			r.Route("/me", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/listen", nil)
			})
		})
		r.Route("/provisionerdaemon", func(r chi.Router) {
			r.Route("/me", func(r chi.Router) {
				r.Get("/listen", api.provisionerDaemonsServe)
			})
		})
		r.Route("/upload", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Post("/", api.postUpload)
		})
	})
	r.NotFound(site.Handler(options.Logger).ServeHTTP)
	return r, api.websocketWaitGroup.Wait
}

// API contains all route handlers. Only HTTP handlers should
// be added to this struct for code clarity.
type api struct {
	*Options

	websocketWaitGroup sync.WaitGroup
}
