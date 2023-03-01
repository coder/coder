package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Get deployment config
// @ID get-deployment-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.DeploymentConfig
// @Router /config/deployment [get]
func (api *API) deploymentValues(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentValues) {
		httpapi.Forbidden(rw)
		return
	}

	values, err := api.DeploymentValues.Scrub()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.DeploymentConfig{
			Values:  values,
			Options: values.Options(),
		},
	)
}
