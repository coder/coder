package coderd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/template"
)

func (api *api) listTemplates(rw http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, template.List())
}

func (api *api) templateArchive(rw http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	archive, exists := template.Archive(id)
	if !exists {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("template does not exists with id %q", id),
		})
		return
	}

	rw.Header().Set("Content-Type", "application/x-tar")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(archive)
}
