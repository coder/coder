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

		// Used for setup.
		r.Get("/user", api.user)
		r.Post("/user", api.postUser)
		r.Route("/users", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Post("/", api.postUsers)

			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/", api.userByName)
				r.Get("/organizations", api.organizationsByUser)
				r.Post("/keys", api.postKeyForUser)
			})
		})
		r.Route("/projects", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Get("/", api.projects)
			r.Route("/{organization}", func(r chi.Router) {
				r.Use(httpmw.ExtractOrganizationParam(options.Database))
				r.Get("/", api.projectsByOrganization)
				r.Post("/", api.postProjectsByOrganization)
				r.Route("/{project}", func(r chi.Router) {
					r.Use(httpmw.ExtractProjectParam(options.Database))
					r.Get("/", api.projectByOrganization)
					r.Get("/workspaces", api.workspacesByProject)
					r.Route("/parameters", func(r chi.Router) {
						r.Get("/", api.parametersByProject)
						r.Post("/", api.postParametersByProject)
					})
					r.Route("/versions", func(r chi.Router) {
						r.Get("/", api.projectVersionsByOrganization)
						r.Post("/", api.postProjectVersionByOrganization)
						r.Route("/{projectversion}", func(r chi.Router) {
							r.Use(httpmw.ExtractProjectVersionParam(api.Database))
							r.Get("/", api.projectVersionByOrganizationAndName)
						})
					})
				})
			})
		})

		// Listing operations specific to resources should go under
		// their respective routes. eg. /orgs/<name>/workspaces
		r.Route("/workspaces", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Get("/", api.workspaces)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Post("/", api.postWorkspaceByUser)
				r.Route("/{workspace}", func(r chi.Router) {
					r.Use(httpmw.ExtractWorkspaceParam(options.Database))
					r.Get("/", api.workspaceByUser)
					r.Route("/version", func(r chi.Router) {
						r.Post("/", api.postWorkspaceHistoryByUser)
						r.Get("/", api.workspaceHistoryByUser)
						r.Route("/{workspacehistory}", func(r chi.Router) {
							r.Use(httpmw.ExtractWorkspaceHistoryParam(options.Database))
							r.Get("/", api.workspaceHistoryByName)
						})
					})
				})
			})
		})

		r.Route("/workspaceagent", func(r chi.Router) {
			r.Route("/authenticate", func(r chi.Router) {
				r.Post("/google-instance-identity", api.postAuthenticateWorkspaceAgentUsingGoogleInstanceIdentity)
			})
		})

		r.Route("/upload", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Post("/", api.postUpload)
		})

		r.Route("/projectimport/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Post("/", api.postProjectImportByOrganization)
			r.Route("/{provisionerjob}", func(r chi.Router) {
				r.Use(httpmw.ExtractProvisionerJobParam(options.Database))
				r.Get("/", api.provisionerJobByID)
				r.Get("/schemas", api.projectImportJobSchemasByID)
				r.Get("/parameters", api.projectImportJobParametersByID)
				r.Get("/resources", api.projectImportJobResourcesByID)
				r.Get("/logs", api.provisionerJobLogsByID)
			})
		})

		r.Route("/workspaceprovision/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Route("/{provisionerjob}", func(r chi.Router) {
				r.Use(httpmw.ExtractProvisionerJobParam(options.Database))
				r.Get("/", api.provisionerJobByID)
				r.Get("/logs", api.provisionerJobLogsByID)
			})
		})

		r.Route("/provisioners/daemons", func(r chi.Router) {
			r.Get("/", api.provisionerDaemons)
			r.Get("/serve", api.provisionerDaemonsServe)
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
