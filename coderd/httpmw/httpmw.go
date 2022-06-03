package httpmw

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
)

// parseUUID consumes a url parameter and parses it as a UUID.
func parseUUID(rw http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	rawID := chi.URLParam(r, param)
	if rawID == "" {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Missing uuid in url",
			// Url params mean nothing to a user
			Internal: fmt.Sprintf("%q url param missing", param),
		})
		return uuid.UUID{}, false
	}

	parsed, err := uuid.Parse(rawID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message:  fmt.Sprintf("Invalid uuid %q", param),
			Internal: err.Error(),
		})
		return uuid.UUID{}, false
	}

	return parsed, true
}
