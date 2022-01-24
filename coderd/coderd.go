package coderd

import (
	"net/http"

	"github.com/go-chi/chi"

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

	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
		r.Post("/login", users.loginWithPassword)
		r.Route("/users", func(r chi.Router) {
			r.Post("/", users.createInitialUser)

			r.Group(func(r chi.Router) {
				r.Use(
					httpmw.ExtractAPIKey(options.Database, nil),
					httpmw.ExtractUserParam(options.Database),
				)
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
					r.Use(httpmw.ExtractProjectParameter(options.Database))
					r.Get("/", projects.project)
					r.Route("/versions", func(r chi.Router) {
						r.Get("/", projects.projectVersions)
						r.Post("/", projects.createProjectVersion)
					})
				})
			})
		})
	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}
