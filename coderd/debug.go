package coderd

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// @Summary Debug Info Wireguard Coordinator
// @ID debug-info-wireguard-coordinator
// @Security CoderSessionToken
// @Produce text/html
// @Tags Debug
// @Success 200
// @Router /debug/coordinator [get]
func (api *API) debugCoordinator(rw http.ResponseWriter, r *http.Request) {
	(*api.TailnetCoordinator.Load()).ServeHTTPDebug(rw, r)
}

// @Summary Debug Info Deployment Health
// @ID debug-info-deployment-health
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 200 {object} healthcheck.Report
// @Router /debug/health [get]
func (api *API) debugDeploymentHealth(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), api.HealthcheckTimeout)
	defer cancel()

	resChan := api.healthCheckGroup.DoChan("", func() (*healthcheck.Report, error) {
		return api.HealthcheckFunc(ctx)
	})

	select {
	case <-ctx.Done():
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Healthcheck is in progress and did not complete in time. Try again in a few seconds.",
		})
		return
	case res := <-resChan:
		if time.Since(res.Val.Time) > api.HealthcheckRefresh {
			api.healthCheckGroup.Forget("")
			api.debugDeploymentHealth(rw, r)
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, res.Val)
		return
	}
}
