package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	Logger   slog.Logger
	Database database.Store
	Pubsub   database.Pubsub
}

// New constructs the Coder API into an HTTP handler.
func New(options *Options) http.Handler {
	api := &api{
		Database: options.Database,
		Logger:   options.Logger,
		Pubsub:   options.Pubsub,
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
		r.Post("/user", api.postUser)
		r.Route("/users", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Post("/", api.postUsers)
			r.Group(func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/{user}", api.userByName)
				r.Get("/{user}/organizations", api.organizationsByUser)
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
					r.Route("/history", func(r chi.Router) {
						r.Get("/", api.projectHistoryByOrganization)
						r.Post("/", api.postProjectHistoryByOrganization)
						r.Route("/{projecthistory}", func(r chi.Router) {
							r.Use(httpmw.ExtractProjectHistoryParam(api.Database))
							r.Get("/", api.projectHistoryByOrganizationAndName)
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
				r.Get("/", api.workspaces)
				r.Post("/", api.postWorkspaceByUser)
				r.Route("/{workspace}", func(r chi.Router) {
					r.Use(httpmw.ExtractWorkspaceParam(options.Database))
					r.Get("/", api.workspaceByUser)
					r.Route("/history", func(r chi.Router) {
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

		r.Route("/provisioners/daemons", func(r chi.Router) {
			r.Get("/", api.provisionerDaemons)
			r.Get("/serve", api.provisionerDaemonsServe)
		})
	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}

// API contains all route handlers. Only HTTP handlers should
// be added to this struct for code clarity.
type api struct {
	Database database.Store
	Logger   slog.Logger
	Pubsub   database.Pubsub
}
