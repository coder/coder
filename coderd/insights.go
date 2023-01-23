package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Get deployment DAUs
// @ID get-deployment-daus
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Success 200 {object} codersdk.DeploymentDAUsResponse
// @Router /insights/daus [get]
func (api *API) deploymentDAUs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	resp, _ := api.metricsCache.DeploymentDAUs()
	if resp == nil || resp.Entries == nil {
		httpapi.Write(ctx, rw, http.StatusOK, &codersdk.DeploymentDAUsResponse{
			Entries: []codersdk.DAUEntry{},
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
