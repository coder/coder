package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
)

// @Summary Removed: Get parameters by template version
// @ID removed-get-parameters-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /templateversions/{templateversion}/parameters [get]
func templateVersionParametersDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}

// @Summary Removed: Get schema by template version
// @ID removed-get-schema-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /templateversions/{templateversion}/schema [get]
func templateVersionSchemaDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}
