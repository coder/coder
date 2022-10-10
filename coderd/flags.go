package coderd

import (
	"net/http"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

func (api *API) deploymentFlags(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentFlags) {
		httpapi.Forbidden(rw)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, deployment.RemoveSensitiveValues(*api.DeploymentFlags))
}
