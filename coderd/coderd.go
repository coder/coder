package coderd

import (
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

// New constructs the Coder API into an HTTP handler.
func New(options *Options) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			render.JSON(w, r, struct {
				Message string `json:"message"`
			}{
				Message: "👋",
			})
		})
	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}
