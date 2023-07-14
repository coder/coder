package coderd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Get deployment DAUs
// @ID get-deployment-daus
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Success 200 {object} codersdk.DAUsResponse
// @Router /insights/daus [get]
func (api *API) deploymentDAUs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentValues) {
		httpapi.Forbidden(rw)
		return
	}

	vals := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	tzOffset := p.Int(vals, 0, "tz_offset")
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	_, resp, _ := api.metricsCache.DeploymentDAUs(tzOffset)
	if resp == nil || resp.Entries == nil {
		httpapi.Write(ctx, rw, http.StatusOK, &codersdk.DAUsResponse{
			Entries: []codersdk.DAUEntry{},
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get insights about user latency
// @ID get-insights-about-user-latency
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Success 200 {object} codersdk.UserLatencyInsightsResponse
// @Router /insights/user-latency [get]
func (api *API) insightsUserLatency(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentValues) {
		httpapi.Forbidden(rw)
		return
	}

	p := httpapi.NewQueryParamParser().
		Required("start_time").
		Required("end_time")
	vals := r.URL.Query()
	var (
		startTime   = p.Time3339Nano(vals, time.Time{}, "start_time")
		endTime     = p.Time3339Nano(vals, time.Time{}, "end_time")
		templateIDs = p.UUIDs(vals, []uuid.UUID{}, "template_ids")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	// TODO(mafredri) Verify template IDs.
	_ = templateIDs

	resp := codersdk.UserLatencyInsightsResponse{
		Report: codersdk.UserLatencyInsightsReport{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: []uuid.UUID{},
			Users: []codersdk.UserLatency{
				{
					UserID: uuid.New(),
					Name:   "Some User",
					LatencyMS: codersdk.ConnectionLatency{
						P50: 14.45,
						P95: 32.16,
					},
				},
			},
		},
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get insights about templates
// @ID get-insights-about-templates
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Success 200 {object} codersdk.TemplateInsightsResponse
// @Router /insights/templates [get]
func (api *API) insightsTemplates(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentValues) {
		httpapi.Forbidden(rw)
		return
	}

	p := httpapi.NewQueryParamParser().
		Required("start_time").
		Required("end_time")
	vals := r.URL.Query()
	var (
		startTime      = p.Time3339Nano(vals, time.Time{}, "start_time")
		endTime        = p.Time3339Nano(vals, time.Time{}, "end_time")
		intervalString = p.String(vals, "day", "interval")
		templateIDs    = p.UUIDs(vals, []uuid.UUID{}, "template_ids")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	// TODO(mafredri) Verify template IDs.
	_ = templateIDs

	var interval codersdk.InsightsReportInterval
	switch v := codersdk.InsightsReportInterval(intervalString); v {
	case codersdk.InsightsReportIntervalDay:
		interval = v
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameter has invalid value.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "interval",
					Detail: fmt.Sprintf("must be %q", codersdk.InsightsReportIntervalDay),
				},
			},
		})
		return
	}

	intervalReports := []codersdk.TemplateInsightsIntervalReport{}
	if interval != "" {
		intervalStart := startTime
		intervalEnd := startTime.Add(time.Hour * 24)
		for !intervalEnd.After(endTime) {
			intervalReports = append(intervalReports, codersdk.TemplateInsightsIntervalReport{
				StartTime:   intervalStart,
				EndTime:     intervalEnd,
				Interval:    interval,
				TemplateIDs: []uuid.UUID{},
				ActiveUsers: 10,
			})
			intervalStart = intervalEnd
			intervalEnd = intervalEnd.Add(time.Hour * 24)
		}
	}

	resp := codersdk.TemplateInsightsResponse{
		Report: codersdk.TemplateInsightsReport{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: []uuid.UUID{},
			ActiveUsers: 10,
			AppsUsage: []codersdk.TemplateAppUsage{
				{
					TemplateIDs: []uuid.UUID{},
					Type:        codersdk.TemplateAppsTypeBuiltin,
					DisplayName: "Visual Studio Code",
					Slug:        "vscode",
					Icon:        "/icons/vscode.svg",
					Seconds:     80500,
				},
			},
		},
		IntervalReports: intervalReports,
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
