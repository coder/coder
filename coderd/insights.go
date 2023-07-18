package coderd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"

	"github.com/coder/coder/coderd/database"
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

	// TODO(mafredri): Client or deployment timezone?
	// Example:
	// - I want data from Monday - Friday
	// - I'm UTC+3 and the deployment is UTC+0
	// - Do we select Monday - Friday in UTC+0 or UTC+3?
	// - Considering users can be in different timezones, perhaps this should be per-user (but we don't keep track of user timezones).
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

	// Should we verify all template IDs exist, or just return no rows?
	// _, err := api.Database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
	// 	IDs: templateIDs,
	// })

	rows, err := api.Database.GetTemplateUserLatencyStats(ctx, database.GetTemplateUserLatencyStatsParams{
		StartTime:   startTime,
		EndTime:     endTime,
		TemplateIDs: templateIDs,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Fetch all users so that we can still include users that have no
	// latency data.
	users, err := api.Database.GetUsers(ctx, database.GetUsersParams{})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	templateIDSet := make(map[uuid.UUID]struct{})
	usersWithLatencyByID := make(map[uuid.UUID]codersdk.UserLatency)
	for _, row := range rows {
		for _, templateID := range row.TemplateIDs {
			templateIDSet[templateID] = struct{}{}
		}
		usersWithLatencyByID[row.UserID] = codersdk.UserLatency{
			TemplateIDs: row.TemplateIDs,
			UserID:      row.UserID,
			Username:    row.Username,
			LatencyMS: &codersdk.ConnectionLatency{
				P50: row.WorkspaceConnectionLatency50,
				P95: row.WorkspaceConnectionLatency95,
			},
		}
	}
	userLatencies := []codersdk.UserLatency{}
	for _, user := range users {
		userLatency, ok := usersWithLatencyByID[user.ID]
		if !ok {
			// TODO(mafredri): Other cases?
			// We only include deleted/inactive users if they were
			// active as part of the requested timeframe.
			if user.Deleted || user.Status != database.UserStatusActive {
				continue
			}

			userLatency = codersdk.UserLatency{
				TemplateIDs: []uuid.UUID{},
				UserID:      user.ID,
				Username:    user.Username,
			}
		}
		userLatencies = append(userLatencies, userLatency)
	}

	// TemplateIDs that contributed to the data.
	seenTemplateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		seenTemplateIDs = append(seenTemplateIDs, templateID)
	}
	slices.SortFunc(seenTemplateIDs, func(a, b uuid.UUID) bool {
		return a.String() < b.String()
	})

	resp := codersdk.UserLatencyInsightsResponse{
		Report: codersdk.UserLatencyInsightsReport{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: seenTemplateIDs,
			Users:       userLatencies,
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
