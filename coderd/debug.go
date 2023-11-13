package coderd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
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
	ctx, cancel := context.WithTimeout(r.Context(), api.Options.HealthcheckTimeout)
	defer cancel()

	// Get cached report if it exists.
	if report := api.healthCheckCache.Load(); report != nil {
		if time.Since(report.Time) < api.Options.HealthcheckRefresh {
			formatHealthcheck(ctx, rw, r, report)
			return
		}
	}

	resChan := api.healthCheckGroup.DoChan("", func() (*healthcheck.Report, error) {
		// Create a new context not tied to the request.
		ctx, cancel := context.WithTimeout(context.Background(), api.Options.HealthcheckTimeout)
		defer cancel()

		report := api.HealthcheckFunc(ctx, apiKey)
		api.healthCheckCache.Store(report)
		return report, nil
	})

	select {
	case <-ctx.Done():
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Healthcheck is in progress and did not complete in time. Try again in a few seconds.",
		})
		return
	case res := <-resChan:
		formatHealthcheck(ctx, rw, r, res.Val)
		return
	}
}

func formatHealthcheck(ctx context.Context, rw http.ResponseWriter, r *http.Request, hc *healthcheck.Report) {
	format := r.URL.Query().Get("format")
	switch format {
	case "text":
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.WriteHeader(http.StatusOK)

		_, _ = fmt.Fprintln(rw, "time:", hc.Time.Format(time.RFC3339))
		_, _ = fmt.Fprintln(rw, "healthy:", hc.Healthy)
		_, _ = fmt.Fprintln(rw, "derp:", hc.DERP.Healthy)
		_, _ = fmt.Fprintln(rw, "access_url:", hc.AccessURL.Healthy)
		_, _ = fmt.Fprintln(rw, "websocket:", hc.Websocket.Healthy)
		_, _ = fmt.Fprintln(rw, "database:", hc.Database.Healthy)

	case "", "json":
		httpapi.WriteIndent(ctx, rw, http.StatusOK, hc)

	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid format option %q.", format),
			Detail:  "Allowed values are: \"json\", \"simple\".",
		})
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
