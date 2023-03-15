package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
)

// @Summary CLI SSH Config
// @ID cli-ssh-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.SSHConfigResponse
// @Router /deployment/ssh [get]
func (a *API) cliSSHConfig(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, a.SSHConfig)
}
