package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
)

// @Summary SSH information for clients
// @ID cli-ssh-config
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.BuildInfoResponse
// @Router /config-ssh [get]
func (a *API) cliSSHConfig(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, a.ConfigSSH)
}
