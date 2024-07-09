package httpmw

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// ParseUUIDParam consumes a url parameter and parses it as a UUID.
func ParseUUIDParam(rw http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	rawID := r.PathValue(param)
	if rawID == "" {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing UUID in URL.",
			// Url params mean nothing to a user
			Detail: fmt.Sprintf("%q URL param missing", param),
		})
		return uuid.UUID{}, false
	}

	parsed, err := uuid.Parse(rawID)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid UUID %q.", rawID),
			Detail:  err.Error(),
		})
		return uuid.UUID{}, false
	}

	return parsed, true
}
