package coderd

import (
	"context"
	"net/http"

	"cdr.dev/slog"
	"github.com/coder/coder/database"
	"github.com/coder/coder/site"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
)

// Options are requires parameters for Coder to start.
type Options struct {
	Logger   slog.Logger
	Database database.Store
}

const (
	provisionerTerraform = "provisioner:terraform"
	provisionerBasic     = "provisioner:basic"
)

// New constructs the Coder API into an HTTP handler.
func New(options *Options) http.Handler {
	api := NewAPI(context.Background())
	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			render.JSON(w, r, struct {
				Message string `json:"message"`
			}{
				Message: "👋",
			})
		})

		// Projects endpoint
		r.Route("/projects", func(r chi.Router) {
			r.Route("/{organization}", func(r chi.Router) {
				// TODO: Authentication
				// TODO: User extraction
				// TODO: Extract organization and add to context
				r.Get("/", api.projectService.getProjects)
				r.Post("/", api.projectService.createProject)
			})
		})

	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}
