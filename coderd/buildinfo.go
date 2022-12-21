package coderd

import (
	"net/http"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// @Summary Build info
// @ID build-info
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.BuildInfoResponse
// @Router /buildinfo [get]
func buildInfo(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.BuildInfoResponse{
		ExternalURL: buildinfo.ExternalURL(),
		Version:     buildinfo.Version(),
	})
}
