package coderd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

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

	p := httpapi.NewQueryParamParser().
		Required("start_time").
		Required("end_time")
	vals := r.URL.Query()
	var (
		// The QueryParamParser does not preserve timezone, so we need
		// to parse the time ourselves.
		startTimeString = p.String(vals, "", "start_time")
		endTimeString   = p.String(vals, "", "end_time")
		templateIDs     = p.UUIDs(vals, []uuid.UUID{}, "template_ids")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	startTime, endTime, ok := parseInsightsStartAndEndTime(ctx, rw, startTimeString, endTimeString)
	if !ok {
		return
	}

	rows, err := api.Database.GetUserLatencyInsights(ctx, database.GetUserLatencyInsightsParams{
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
		// The QueryParamParser does not preserve timezone, so we need
		// to parse the time ourselves.
		startTimeString = p.String(vals, "", "start_time")
		endTimeString   = p.String(vals, "", "end_time")
		intervalString  = p.String(vals, string(codersdk.InsightsReportIntervalNone), "interval")
		templateIDs     = p.UUIDs(vals, []uuid.UUID{}, "template_ids")
	)
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	startTime, endTime, ok := parseInsightsStartAndEndTime(ctx, rw, startTimeString, endTimeString)
	if !ok {
		return
	}
	interval, ok := verifyInsightsInterval(ctx, rw, intervalString)
	if !ok {
		return
	}

	var usage database.GetTemplateInsightsRow
	var dailyUsage []database.GetTemplateDailyInsightsRow
	// Use a transaction to ensure that we get consistent data between
	// the full and interval report.
	err := api.Database.InTx(func(db database.Store) error {
		var err error

		if interval != codersdk.InsightsReportIntervalNone {
			dailyUsage, err = db.GetTemplateDailyInsights(ctx, database.GetTemplateDailyInsightsParams{
				StartTime:   startTime,
				EndTime:     endTime,
				TemplateIDs: templateIDs,
			})
			if err != nil {
				return xerrors.Errorf("get template daily insights: %w", err)
			}
		}

		usage, err = db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: templateIDs,
		})
		if err != nil {
			return xerrors.Errorf("get template insights: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	intervalReports := []codersdk.TemplateInsightsIntervalReport{}
	for _, row := range dailyUsage {
		intervalReports = append(intervalReports, codersdk.TemplateInsightsIntervalReport{
			StartTime:   row.StartTime,
			EndTime:     row.EndTime,
			Interval:    interval,
			TemplateIDs: row.TemplateIDs,
			ActiveUsers: row.ActiveUsers,
		})
	}

	resp := codersdk.TemplateInsightsResponse{
		Report: codersdk.TemplateInsightsReport{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: usage.TemplateIDs,
			ActiveUsers: usage.ActiveUsers,
			AppsUsage: []codersdk.TemplateAppUsage{
				{
					TemplateIDs: usage.TemplateIDs, // TODO(mafredri): Update query to return template IDs/app?
					Type:        codersdk.TemplateAppsTypeBuiltin,
					DisplayName: "Visual Studio Code",
					Slug:        "vscode",
					Icon:        "/icons/code.svg",
					Seconds:     usage.UsageVscodeSeconds,
				},
				{
					TemplateIDs: usage.TemplateIDs, // TODO(mafredri): Update query to return template IDs/app?
					Type:        codersdk.TemplateAppsTypeBuiltin,
					DisplayName: "JetBrains",
					Slug:        "jetbrains",
					Icon:        "/icons/intellij.svg",
					Seconds:     usage.UsageJetbrainsSeconds,
				},
				{
					TemplateIDs: usage.TemplateIDs, // TODO(mafredri): Update query to return template IDs/app?
					Type:        codersdk.TemplateAppsTypeBuiltin,
					DisplayName: "Web Terminal",
					Slug:        "reconnecting-pty",
					Icon:        "/icons/terminal.svg",
					Seconds:     usage.UsageReconnectingPtySeconds,
				},
				{
					TemplateIDs: usage.TemplateIDs, // TODO(mafredri): Update query to return template IDs/app?
					Type:        codersdk.TemplateAppsTypeBuiltin,
					DisplayName: "SSH",
					Slug:        "ssh",
					Icon:        "/icons/terminal.svg",
					Seconds:     usage.UsageSshSeconds,
				},
			},
		},
		IntervalReports: intervalReports,
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// parseInsightsStartAndEndTime parses the start and end time query parameters
// and returns the parsed values. The client provided timezone must be preserved
// when parsing the time. Verification is performed so that the start and end
// time are not zero and that the end time is not before the start time. The
// clock must be set to 00:00:00, except for "today", where end time is allowed
// to provide the hour of the day (e.g. 14:00:00).
func parseInsightsStartAndEndTime(ctx context.Context, rw http.ResponseWriter, startTimeString, endTimeString string) (startTime, endTime time.Time, ok bool) {
	const insightsTimeLayout = time.RFC3339Nano

	for _, qp := range []struct {
		name, value string
		dest        *time.Time
	}{
		{"start_time", startTimeString, &startTime},
		{"end_time", endTimeString, &endTime},
	} {
		t, err := time.Parse(insightsTimeLayout, qp.value)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  qp.name,
						Detail: fmt.Sprintf("Query param %q must be a valid date format (%s): %s", qp.name, insightsTimeLayout, err.Error()),
					},
				},
			})
			return time.Time{}, time.Time{}, false
		}
		if t.IsZero() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  qp.name,
						Detail: fmt.Sprintf("Query param %q must not be zero", qp.name),
					},
				},
			})
			return time.Time{}, time.Time{}, false
		}
		ensureZeroHour := true
		if qp.name == "end_time" {
			ey, em, ed := t.Date()
			ty, tm, td := time.Now().Date()

			ensureZeroHour = ey != ty || em != tm || ed != td
		}
		h, m, s := t.Clock()
		if ensureZeroHour && (h != 0 || m != 0 || s != 0) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  qp.name,
						Detail: fmt.Sprintf("Query param %q must have the clock set to 00:00:00", qp.name),
					},
				},
			})
			return time.Time{}, time.Time{}, false
		} else if m != 0 || s != 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  qp.name,
						Detail: fmt.Sprintf("Query param %q must have the clock set to %02d:00:00", qp.name, h),
					},
				},
			})
			return time.Time{}, time.Time{}, false
		}
		*qp.dest = t
	}
	if endTime.Before(startTime) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameter has invalid value.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "end_time",
					Detail: fmt.Sprintf("Query param %q must be greater than %q", "end_time", "start_time"),
				},
			},
		})
		return time.Time{}, time.Time{}, false
	}

	return startTime, endTime, true
}

func verifyInsightsInterval(ctx context.Context, rw http.ResponseWriter, intervalString string) (codersdk.InsightsReportInterval, bool) {
	switch v := codersdk.InsightsReportInterval(intervalString); v {
	case codersdk.InsightsReportIntervalDay, codersdk.InsightsReportIntervalNone:
		return v, true
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameter has invalid value.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "interval",
					Detail: fmt.Sprintf("must be one of %v", []codersdk.InsightsReportInterval{codersdk.InsightsReportIntervalNone, codersdk.InsightsReportIntervalDay}),
				},
			},
		})
		return "", false
	}
}
