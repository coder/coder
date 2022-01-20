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
		r.Post("/user", users.createInitialUser)
		r.Post("/login", users.loginWithPassword)
		// Require an API key and authenticated user for this group.
		r.Group(func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractUser(options.Database),
			)
			r.Get("/user", users.getAuthenticatedUser)
		})
	})
	r.NotFound(site.Handler().ServeHTTP)
	return r
}
