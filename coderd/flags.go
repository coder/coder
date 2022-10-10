package coderd

import (
	"net/http"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd/httpapi"
)

func (api *API) deploymentFlags(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, deployment.RemoveSensitiveValues(*api.DeploymentFlags))
}
