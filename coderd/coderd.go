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
}

// New constructs the Coder API into an HTTP handler.
func New(options *Options) http.Handler {
	projects := &projects{
		Database: options.Database,
	}
	users := &users{
		Database: options.Database,
	}
	workspaces := &workspaces{
		Database: options.Database,
	}

	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
		r.Post("/login", users.loginWithPassword)
		r.Post("/logout", users.logout)
		// Used for setup.
		r.Post("/user", users.createInitialUser)
		r.Route("/users", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Post("/", users.createUser)
			r.Group(func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/{user}", users.user)
				r.Get("/{user}/organizations", users.userOrganizations)
			})
		})
		r.Route("/projects", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Get("/", projects.allProjects)
			r.Route("/{organization}", func(r chi.Router) {
				r.Use(httpmw.ExtractOrganizationParam(options.Database))
				r.Get("/", projects.allProjectsForOrganization)
				r.Post("/", projects.createProject)
				r.Route("/{project}", func(r chi.Router) {
					r.Use(httpmw.ExtractProjectParam(options.Database))
					r.Get("/", projects.project)
					r.Route("/history", func(r chi.Router) {
						r.Get("/", projects.allProjectHistory)
						r.Post("/", projects.createProjectHistory)
					})
					r.Get("/workspaces", workspaces.allWorkspacesForProject)
				})
			})
		})

		// Listing operations specific to resources should go under
		// their respective routes. eg. /orgs/<name>/workspaces
		r.Route("/workspaces", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Get("/", workspaces.listAllWorkspaces)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/", workspaces.listAllWorkspaces)
				r.Post("/", workspaces.createWorkspaceForUser)
				r.Route("/{workspace}", func(r chi.Router) {
					r.Use(httpmw.ExtractWorkspaceParam(options.Database))
					r.Get("/", workspaces.singleWorkspace)
					r.Route("/history", func(r chi.Router) {
						r.Post("/", workspaces.createWorkspaceHistory)
						r.Get("/", workspaces.listAllWorkspaceHistory)
						r.Get("/latest", workspaces.latestWorkspaceHistory)
					})
				})
			})
		})
	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}
