package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary API root handler
// @Description Returns a short greeting from the API root, which you can use to check that the API is reachable.
// @Description Doesn't require a session token.
// @ID api-root-handler
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.Response
// @Router /api/v2/ [get]
func apiRoot(w http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Response{
		//nolint:gocritic
		Message: "👋",
	})
}
