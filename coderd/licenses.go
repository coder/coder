package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func Licenses() http.Handler {
	r := chi.NewRouter()
	r.NotFound(licensesUnsupported)
	return r
}

func licensesUnsupported(rw http.ResponseWriter, _ *http.Request) {
	httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
		Message:     "Unsupported",
		Detail:      "These endpoints are not supported in AGPL-licensed Coder",
		Validations: nil,
	})
}
