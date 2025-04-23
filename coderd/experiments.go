package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get enabled experiments
// @ID get-enabled-experiments
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {array} codersdk.Experiment
// @Router /experiments [get]
func (api *API) handleExperimentsGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	httpapi.Write(ctx, rw, http.StatusOK, api.Experiments)
}

// @Summary Get safe experiments
// @ID get-safe-experiments
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {array} codersdk.Experiment
// @Router /experiments/available [get]
func handleExperimentsSafe(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AvailableExperiments{
		Safe: codersdk.ExperimentsSafe,
	})
}
