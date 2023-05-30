package coderd

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
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
	apiKey := httpmw.APITokenFromRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), api.HealthcheckTimeout)
	defer cancel()

	// Get cached report if it exists.
	if report := api.healthCheckCache.Load(); report != nil {
		if time.Since(report.Time) < api.HealthcheckRefresh {
			httpapi.WriteIndent(ctx, rw, http.StatusOK, report)
			return
		}
	}

	resChan := api.healthCheckGroup.DoChan("", func() (*healthcheck.Report, error) {
		// Create a new context not tied to the request.
		ctx, cancel := context.WithTimeout(context.Background(), api.HealthcheckTimeout)
		defer cancel()

		report, err := api.HealthcheckFunc(ctx, apiKey)
		if err == nil {
			api.healthCheckCache.Store(report)
		}
		return report, err
	})

	select {
	case <-ctx.Done():
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Healthcheck is in progress and did not complete in time. Try again in a few seconds.",
		})
		return
	case res := <-resChan:
		httpapi.WriteIndent(ctx, rw, http.StatusOK, res.Val)
		return
	}
}

// For some reason the swagger docs need to be attached to a function.
//
// @Summary Debug Info Websocket Test
// @ID debug-info-websocket-test
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 201 {object} codersdk.Response
// @Router /debug/ws [get]
// @x-apidocgen {"skip": true}
func _debugws(http.ResponseWriter, *http.Request) {} //nolint:unused
