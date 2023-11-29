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
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

type debugHealthMetrics struct {
	accessURLSeverityGauge       *prometheus.GaugeVec
	accessURLReachableGauge      *prometheus.GaugeVec
	accessURLStatusCodeGauge     *prometheus.GaugeVec
	accessURLResponseLengthGauge *prometheus.GaugeVec
	databaseSeverityGauge        *prometheus.GaugeVec
	databaseReachableGauge       *prometheus.GaugeVec
	databaseLatencyGauge         *prometheus.GaugeVec
	databaseThresholdGauge       *prometheus.GaugeVec
	derpSeverityGauge            *prometheus.GaugeVec
	derpNodeSeverityGauge        *prometheus.GaugeVec
	derpNodeRoundTripPingGauge   *prometheus.GaugeVec
	derpNodeUsesWebsocketGauge   *prometheus.GaugeVec
	derpNodeStunEnabledGauge     *prometheus.GaugeVec
	websocketSeverityGauge       *prometheus.GaugeVec
	websocketResponseLengthGauge *prometheus.GaugeVec
	websocketStatusCodeGauge     *prometheus.GaugeVec
}

func (api *API) DebugHealthcheckLoop(ctx context.Context, refresh time.Duration) {
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	// Run the first healthcheck on startup.
	r := api.HealthcheckFunc(ctx)
	api.healthCheckCache.Store(r)
	api.reportHealthcheckPrometheusMetrics(r)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r := api.HealthcheckFunc(ctx)
			api.healthCheckCache.Store(r)
			api.reportHealthcheckPrometheusMetrics(r)
		}
	}
}

func (api *API) reportHealthcheckPrometheusMetrics(report *healthcheck.Report) {
	// Access URL
	api.debugHealthMetrics.accessURLSeverityGauge.WithLabelValues().Set(float64(report.AccessURL.Severity.Value()))
	api.debugHealthMetrics.accessURLReachableGauge.WithLabelValues().Set(boolToFloat64(report.AccessURL.Reachable))
	api.debugHealthMetrics.accessURLStatusCodeGauge.WithLabelValues().Set(float64(report.AccessURL.StatusCode))
	api.debugHealthMetrics.accessURLResponseLengthGauge.WithLabelValues().Set(float64(len(report.AccessURL.HealthzResponse)))
	// Database
	api.debugHealthMetrics.databaseSeverityGauge.WithLabelValues().Set(float64(report.Database.Severity.Value()))
	api.debugHealthMetrics.databaseReachableGauge.WithLabelValues().Set(boolToFloat64(report.Database.Reachable))
	api.debugHealthMetrics.databaseLatencyGauge.WithLabelValues().Set(float64(report.Database.LatencyMS))
	api.debugHealthMetrics.databaseThresholdGauge.WithLabelValues().Set(float64(report.Database.ThresholdMS))
	// DERP
	for regionID, regionReport := range report.DERP.Regions {
		for _, nodeReport := range regionReport.NodeReports {
			api.debugHealthMetrics.derpNodeSeverityGauge.WithLabelValues(fmt.Sprintf("%d", regionID), nodeReport.Node.Name).Set(float64(nodeReport.Severity.Value()))
			api.debugHealthMetrics.derpNodeRoundTripPingGauge.WithLabelValues(fmt.Sprintf("%d", regionID), nodeReport.Node.Name).Set(float64(nodeReport.RoundTripPingMs))
			api.debugHealthMetrics.derpNodeUsesWebsocketGauge.WithLabelValues(fmt.Sprintf("%d", regionID), nodeReport.Node.Name).Set(boolToFloat64(nodeReport.UsesWebsocket))
			api.debugHealthMetrics.derpNodeStunEnabledGauge.WithLabelValues(fmt.Sprintf("%d", regionID), nodeReport.Node.Name).Set(boolToFloat64(nodeReport.STUN.Enabled))
		}
	}
	// Websocket
	api.debugHealthMetrics.websocketSeverityGauge.WithLabelValues().Set(float64(report.Websocket.Severity.Value()))
	api.debugHealthMetrics.websocketResponseLengthGauge.WithLabelValues().Set(float64(len(report.Websocket.Body)))
	api.debugHealthMetrics.websocketStatusCodeGauge.WithLabelValues().Set(float64(report.Websocket.Code))
}

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
// @Success 200 {object} healthcheck.Report
// @Router /debug/health [get]
// @Param force query boolean false "Force a healthcheck to run"
func (api *API) debugDeploymentHealth(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), api.Options.HealthcheckTimeout)
	defer cancel()

	// Check if the forced query parameter is set.
	forced := r.URL.Query().Get("force") == "true"

	// Get cached report if it exists and the requester did not force a refresh.
	if !forced {
		if report := api.healthCheckCache.Load(); report != nil {
			if time.Since(report.Time) < api.Options.HealthcheckRefresh {
				formatHealthcheck(ctx, rw, r, report)
				return
			}
		}
	}

	resChan := api.healthCheckGroup.DoChan("", func() (*healthcheck.Report, error) {
		// Create a new context not tied to the request.
		ctx, cancel := context.WithTimeout(context.Background(), api.Options.HealthcheckTimeout)
		defer cancel()

		report := api.HealthcheckFunc(ctx)
		api.healthCheckCache.Store(report)
		api.reportHealthcheckPrometheusMetrics(report)

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

// @Summary Get health settings
// @ID get-health-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Debug
// @Success 200 {object} codersdk.HealthSettings
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

	var settings codersdk.HealthSettings
	err = json.Unmarshal([]byte(settingsJSON), &settings)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unmarshal health settings.",
			Detail:  err.Error(),
		})
		return
	}

	if len(settings.DismissedHealthchecks) == 0 {
		settings.DismissedHealthchecks = []string{}
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, settings)
}

// @Summary Update health settings
// @ID update-health-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Debug
// @Param request body codersdk.UpdateHealthSettings true "Update health settings"
// @Success 200 {object} codersdk.UpdateHealthSettings
// @Router /debug/health/settings [put]
func (api *API) putDeploymentHealthSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceDeploymentValues) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update health settings.",
		})
		return
	}

	var settings codersdk.HealthSettings
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
		httpapi.Write(r.Context(), rw, http.StatusNotModified, nil)
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

func validateHealthSettings(settings codersdk.HealthSettings) error {
	for _, dismissed := range settings.DismissedHealthchecks {
		ok := slices.Contains(healthcheck.Sections, dismissed)
		if !ok {
			return xerrors.Errorf("unknown healthcheck section: %s", dismissed)
		}
	}
	return nil
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

func loadDismissedHealthchecks(ctx context.Context, db database.Store, logger slog.Logger) []string {
	dismissedHealthchecks := []string{}
	settingsJSON, err := db.GetHealthSettings(ctx)
	if err == nil {
		var settings codersdk.HealthSettings
		err = json.Unmarshal([]byte(settingsJSON), &settings)
		if len(settings.DismissedHealthchecks) > 0 {
			dismissedHealthchecks = settings.DismissedHealthchecks
		}
	}
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		logger.Error(ctx, "unable to fetch health settings: %w", err)
	}
	return dismissedHealthchecks
}

// newDebugHealthMetrics registers debug health metrics with prometheus.
func newDebugHealthMetrics(prometheusRegisterer prometheus.Registerer) (*debugHealthMetrics, error) {
	dh := &debugHealthMetrics{}
	// Access URL
	dh.accessURLSeverityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "access_url_severity",
		Help:      "Access URL Severity",
	}, []string{})
	err := prometheusRegisterer.Register(dh.accessURLSeverityGauge)
	if err != nil {
		return nil, err
	}
	dh.accessURLReachableGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "access_url_reachable",
		Help:      "Access URL Reachable",
	}, []string{})
	err = prometheusRegisterer.Register(dh.accessURLReachableGauge)
	if err != nil {
		return nil, err
	}
	dh.accessURLStatusCodeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "access_url_status_code",
		Help:      "Access URL Status Code",
	}, []string{})
	err = prometheusRegisterer.Register(dh.accessURLStatusCodeGauge)
	if err != nil {
		return nil, err
	}
	dh.accessURLResponseLengthGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "access_url_response_len",
		Help:      "Access URL Response Length",
	}, []string{})
	err = prometheusRegisterer.Register(dh.accessURLResponseLengthGauge)
	if err != nil {
		return nil, err
	}

	// Database
	dh.databaseSeverityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "database_severity",
		Help:      "Database Severity",
	}, []string{})
	err = prometheusRegisterer.Register(dh.databaseSeverityGauge)
	if err != nil {
		return nil, err
	}
	dh.databaseReachableGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "database_reachable",
		Help:      "Database Reachable",
	}, []string{})
	err = prometheusRegisterer.Register(dh.databaseReachableGauge)
	if err != nil {
		return nil, err
	}
	dh.databaseLatencyGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "database_latency_ms",
		Help:      "Database Latency",
	}, []string{})
	err = prometheusRegisterer.Register(dh.databaseLatencyGauge)
	if err != nil {
		return nil, err
	}
	dh.databaseThresholdGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "database_threshold_ms",
		Help:      "Database Threshold",
	}, []string{})
	err = prometheusRegisterer.Register(dh.databaseThresholdGauge)
	if err != nil {
		return nil, err
	}

	// DERP
	dh.derpSeverityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "derp_severity",
		Help:      "Derp Severity",
	}, []string{})
	err = prometheusRegisterer.Register(dh.derpSeverityGauge)
	if err != nil {
		return nil, err
	}
	dh.derpNodeSeverityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "derp_node_severity",
		Help:      "Derp Node Severity",
	}, []string{"region", "node"})
	err = prometheusRegisterer.Register(dh.derpNodeSeverityGauge)
	if err != nil {
		return nil, err
	}
	dh.derpNodeRoundTripPingGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "derp_node_round_trip_ping_ms",
		Help:      "Derp Node Round Trip Ping",
	}, []string{"region", "node"})
	err = prometheusRegisterer.Register(dh.derpNodeRoundTripPingGauge)
	if err != nil {
		return nil, err
	}
	dh.derpNodeUsesWebsocketGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "derp_node_uses_websocket",
		Help:      "Derp Node Uses Websocket",
	}, []string{"region", "node"})
	err = prometheusRegisterer.Register(dh.derpNodeUsesWebsocketGauge)
	if err != nil {
		return nil, err
	}
	dh.derpNodeStunEnabledGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "derp_node_stun_enabled",
		Help:      "Derp Node STUN Enabled",
	}, []string{"region", "node"})
	err = prometheusRegisterer.Register(dh.derpNodeStunEnabledGauge)
	if err != nil {
		return nil, err
	}

	// Websocket
	dh.websocketSeverityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "websocket_severity",
		Help:      "Websocket Severity",
	}, []string{})
	err = prometheusRegisterer.Register(dh.websocketSeverityGauge)
	if err != nil {
		return nil, err
	}
	dh.websocketResponseLengthGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "websocket_response_len",
		Help:      "Websocket Response Length",
	}, []string{})
	err = prometheusRegisterer.Register(dh.websocketResponseLengthGauge)
	if err != nil {
		return nil, err
	}
	dh.websocketStatusCodeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "health",
		Name:      "websocket_status_code",
		Help:      "Websocket Status Code",
	}, []string{})
	err = prometheusRegisterer.Register(dh.websocketStatusCodeGauge)
	if err != nil {
		return nil, err
	}

	return dh, nil
}

//nolint:revive
func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}

	return 0
}
