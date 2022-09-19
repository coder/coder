package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func licenses() http.Handler {
	r := chi.NewRouter()
	r.NotFound(unsupported)
	return r
}

func unsupported(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
		Message:     "Unsupported",
		Detail:      "These endpoints are not supported in AGPL-licensed Coder",
		Validations: nil,
	})
}
