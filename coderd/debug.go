package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
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

// @Summary Debug Info Tailnet
// @ID debug-info-tailnet
// @Security CoderSessionToken
// @Produce text/html
// @Tags Debug
// @Success 200
// @Router /debug/tailnet [get]
func (api *API) debugTailnet(rw http.ResponseWriter, r *http.Request) {
	api.agentProvider.ServeHTTPDebug(rw, r)
}

// @Summary Debug Info Deployment Health
// @ID debug-info-deployment-health
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 200 {object} healthsdk.HealthcheckReport
// @Router /debug/health [get]
// @Param force query boolean false "Force a healthcheck to run"
func (api *API) debugDeploymentHealth(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APITokenFromRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), api.Options.HealthcheckTimeout)
	defer cancel()

	// Load sections previously marked as dismissed.
	// We hydrate this here as we cache the healthcheck and hydrating in the
	// healthcheck function itself can lead to stale results.
	dismissed := loadDismissedHealthchecks(ctx, api.Database, api.Logger)

	// Check if the forced query parameter is set.
	forced := r.URL.Query().Get("force") == "true"

	// Get cached report if it exists and the requester did not force a refresh.
	if !forced {
		if report := api.healthCheckCache.Load(); report != nil {
			if time.Since(report.Time) < api.Options.HealthcheckRefresh {
				formatHealthcheck(ctx, rw, r, *report, dismissed...)
				return
			}
		}
	}

	resChan := api.healthCheckGroup.DoChan("", func() (*healthsdk.HealthcheckReport, error) {
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
		report := res.Val
		if report == nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "There was an unknown error completing the healthcheck.",
				Detail:  "nil report from healthcheck result channel",
			})
			return
		}
		formatHealthcheck(ctx, rw, r, *report, dismissed...)
		return
	}
}

func formatHealthcheck(ctx context.Context, rw http.ResponseWriter, r *http.Request, hc healthsdk.HealthcheckReport, dismissed ...healthsdk.HealthSection) {
	// Mark any sections previously marked as dismissed.
	for _, d := range dismissed {
		switch d {
		case healthsdk.HealthSectionAccessURL:
			hc.AccessURL.Dismissed = true
		case healthsdk.HealthSectionDERP:
			hc.DERP.Dismissed = true
		case healthsdk.HealthSectionDatabase:
			hc.Database.Dismissed = true
		case healthsdk.HealthSectionWebsocket:
			hc.Websocket.Dismissed = true
		case healthsdk.HealthSectionWorkspaceProxy:
			hc.WorkspaceProxy.Dismissed = true
		}
	}

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

// @Summary Get health settings
// @ID get-health-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 200 {object} healthsdk.HealthSettings
// @Router /debug/health/settings [get]
func (api *API) deploymentHealthSettings(rw http.ResponseWriter, r *http.Request) {
	settingsJSON, err := api.Database.GetHealthSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch health settings.",
			Detail:  err.Error(),
		})
		return
	}

	var settings healthsdk.HealthSettings
	err = json.Unmarshal([]byte(settingsJSON), &settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unmarshal health settings.",
			Detail:  err.Error(),
		})
		return
	}

	if len(settings.DismissedHealthchecks) == 0 {
		settings.DismissedHealthchecks = []healthsdk.HealthSection{}
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Update health settings
// @ID update-health-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Debug
// @Param request body healthsdk.UpdateHealthSettings true "Update health settings"
// @Success 200 {object} healthsdk.UpdateHealthSettings
// @Router /debug/health/settings [put]
func (api *API) putDeploymentHealthSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceDeploymentValues) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update health settings.",
		})
		return
	}

	var settings healthsdk.HealthSettings
	if !httpapi.Read(ctx, rw, r, &settings) {
		return
	}

	err := validateHealthSettings(settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to validate health settings.",
			Detail:  err.Error(),
		})
		return
	}

	settingsJSON, err := json.Marshal(&settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal health settings.",
			Detail:  err.Error(),
		})
		return
	}

	currentSettingsJSON, err := api.Database.GetHealthSettings(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch current health settings.",
			Detail:  err.Error(),
		})
		return
	}

	if bytes.Equal(settingsJSON, []byte(currentSettingsJSON)) {
		// See: https://www.rfc-editor.org/rfc/rfc7231#section-6.3.5
		httpapi.Write(r.Context(), rw, http.StatusNoContent, nil)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.HealthSettings](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	aReq.New = database.HealthSettings{
		ID:                    uuid.New(),
		DismissedHealthchecks: settings.DismissedHealthchecks,
	}

	err = api.Database.UpsertHealthSettings(ctx, string(settingsJSON))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update health settings.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

func validateHealthSettings(settings healthsdk.HealthSettings) error {
	for _, dismissed := range settings.DismissedHealthchecks {
		ok := slices.Contains(healthsdk.HealthSections, dismissed)
		if !ok {
			return xerrors.Errorf("unknown healthcheck section: %s", dismissed)
		}
	}
	return nil
}

// For some reason the swagger docs need to be attached to a function.

// @Summary Debug Info Websocket Test
// @ID debug-info-websocket-test
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 201 {object} codersdk.Response
// @Router /debug/ws [get]
// @x-apidocgen {"skip": true}
func _debugws(http.ResponseWriter, *http.Request) {} //nolint:unused

// @Summary Debug DERP traffic
// @ID debug-derp-traffic
// @Security CoderSessionToken
// @Produce json
// @Success 200 {array} derp.BytesSentRecv
// @Tags Debug
// @Router /debug/derp/traffic [get]
// @x-apidocgen {"skip": true}
func _debugDERPTraffic(http.ResponseWriter, *http.Request) {} //nolint:unused

// @Summary Debug expvar
// @ID debug-expvar
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 200 {object} map[string]any
// @Router /debug/expvar [get]
// @x-apidocgen {"skip": true}
func _debugExpVar(http.ResponseWriter, *http.Request) {} //nolint:unused

func loadDismissedHealthchecks(ctx context.Context, db database.Store, logger slog.Logger) []healthsdk.HealthSection {
	dismissedHealthchecks := []healthsdk.HealthSection{}
	settingsJSON, err := db.GetHealthSettings(ctx)
	if err == nil {
		var settings healthsdk.HealthSettings
		err = json.Unmarshal([]byte(settingsJSON), &settings)
		if len(settings.DismissedHealthchecks) > 0 {
			dismissedHealthchecks = settings.DismissedHealthchecks
		}
	}
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		logger.Error(ctx, "unable to fetch health settings", slog.Error(err))
	}
	return dismissedHealthchecks
}
