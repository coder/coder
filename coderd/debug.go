package coderd

import "net/http"

// @Summary Wireguard Coordinator Debug Info
// @ID debuginfo-coordinator
// @Security CoderSessionToken
// @Produce html
// @Tags Debug
// @Success 200
// @Router /debug/coordinator [get]
func (api *API) debugCoordinator(rw http.ResponseWriter, r *http.Request) {
	(*api.TailnetCoordinator.Load()).ServeHTTPDebug(rw, r)
}
