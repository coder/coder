package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get experiments
// @ID get-experiments
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Param include_all query bool false "All available experiments"
// @Success 200 {array} codersdk.Experiment
// @Router /experiments [get]
func (api *API) handleExperimentsGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all := r.URL.Query().Has("include_all")

	if !all {
		httpapi.Write(ctx, rw, http.StatusOK, api.Experiments)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ExperimentsAll)
}
