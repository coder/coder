package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// @Summary Get deployment config
// @ID get-deployment-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.DeploymentConfig
// @Router /config/deployment [get]
func (api *API) deploymentConfig(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, api.DeploymentConfig)
}
