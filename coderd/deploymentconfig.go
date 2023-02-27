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
// @Success 200 {object} codersdk.DeploymentConfigAndOptions
// @Router /config/deployment [get]
func (api *API) deploymentConfig(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	scrubbedConfig, err := api.DeploymentConfig.Scrub()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.DeploymentConfigAndOptions{
			Config:  scrubbedConfig,
			Options: scrubbedConfig.ConfigOptions(),
		},
	)
}
