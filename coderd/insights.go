package coderd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// Duplicated in codersdk.
const insightsTimeLayout = time.RFC3339

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

// @Summary Get insights about user activity
// @ID get-insights-about-user-activity
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Success 200 {object} codersdk.UserActivityInsightsResponse
// @Router /insights/user-activity [get]
func (api *API) insightsUserActivity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	rows, err := api.Database.GetUserActivityInsights(ctx, database.GetUserActivityInsightsParams{
		StartTime:   startTime,
		EndTime:     endTime,
		TemplateIDs: templateIDs,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user activity.",
			Detail:  err.Error(),
		})
		return
	}

	templateIDSet := make(map[uuid.UUID]struct{})
	userActivities := make([]codersdk.UserActivity, 0, len(rows))
	for _, row := range rows {
		for _, templateID := range row.TemplateIDs {
			templateIDSet[templateID] = struct{}{}
		}
		userActivities = append(userActivities, codersdk.UserActivity{
			TemplateIDs: row.TemplateIDs,
			UserID:      row.UserID,
			Username:    row.Username,
			AvatarURL:   row.AvatarURL.String,
			Seconds:     row.UsageSeconds,
		})
	}

	// TemplateIDs that contributed to the data.
	seenTemplateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		seenTemplateIDs = append(seenTemplateIDs, templateID)
	}
	slices.SortFunc(seenTemplateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})

	resp := codersdk.UserActivityInsightsResponse{
		Report: codersdk.UserActivityInsightsReport{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: seenTemplateIDs,
			Users:       userActivities,
		},
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
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user latency.",
			Detail:  err.Error(),
		})
		return
	}

	templateIDSet := make(map[uuid.UUID]struct{})
	userLatencies := make([]codersdk.UserLatency, 0, len(rows))
	for _, row := range rows {
		for _, templateID := range row.TemplateIDs {
			templateIDSet[templateID] = struct{}{}
		}
		userLatencies = append(userLatencies, codersdk.UserLatency{
			TemplateIDs: row.TemplateIDs,
			UserID:      row.UserID,
			Username:    row.Username,
			AvatarURL:   row.AvatarURL.String,
			LatencyMS: codersdk.ConnectionLatency{
				P50: row.WorkspaceConnectionLatency50,
				P95: row.WorkspaceConnectionLatency95,
			},
		})
	}

	// TemplateIDs that contributed to the data.
	seenTemplateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		seenTemplateIDs = append(seenTemplateIDs, templateID)
	}
	slices.SortFunc(seenTemplateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
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

	p := httpapi.NewQueryParamParser().
		Required("start_time").
		Required("end_time")
	vals := r.URL.Query()
	var (
		// The QueryParamParser does not preserve timezone, so we need
		// to parse the time ourselves.
		startTimeString = p.String(vals, "", "start_time")
		endTimeString   = p.String(vals, "", "end_time")
		intervalString  = p.String(vals, "", "interval")
		templateIDs     = p.UUIDs(vals, []uuid.UUID{}, "template_ids")
		sectionStrings  = p.Strings(vals, templateInsightsSectionAsStrings(codersdk.TemplateInsightsSectionIntervalReports, codersdk.TemplateInsightsSectionReport), "sections")
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
	interval, ok := parseInsightsInterval(ctx, rw, intervalString, startTime, endTime)
	if !ok {
		return
	}
	sections, ok := parseTemplateInsightsSections(ctx, rw, sectionStrings)
	if !ok {
		return
	}

	var usage database.GetTemplateInsightsRow
	var appUsage []database.GetTemplateAppInsightsRow
	var dailyUsage []database.GetTemplateInsightsByIntervalRow
	var parameterRows []database.GetTemplateParameterInsightsRow

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(4)

	// The following insights data queries have a theoretical chance to be
	// inconsistent between each other when looking at "today", however, the
	// overhead from a transaction is not worth it.
	eg.Go(func() error {
		var err error
		if interval != "" && slices.Contains(sections, codersdk.TemplateInsightsSectionIntervalReports) {
			dailyUsage, err = api.Database.GetTemplateInsightsByInterval(egCtx, database.GetTemplateInsightsByIntervalParams{
				StartTime:    startTime,
				EndTime:      endTime,
				TemplateIDs:  templateIDs,
				IntervalDays: interval.Days(),
			})
			if err != nil {
				return xerrors.Errorf("get template daily insights: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if !slices.Contains(sections, codersdk.TemplateInsightsSectionReport) {
			return nil
		}

		var err error
		usage, err = api.Database.GetTemplateInsights(egCtx, database.GetTemplateInsightsParams{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: templateIDs,
		})
		if err != nil {
			return xerrors.Errorf("get template insights: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if !slices.Contains(sections, codersdk.TemplateInsightsSectionReport) {
			return nil
		}

		var err error
		appUsage, err = api.Database.GetTemplateAppInsights(egCtx, database.GetTemplateAppInsightsParams{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: templateIDs,
		})
		if err != nil {
			return xerrors.Errorf("get template app insights: %w", err)
		}
		return nil
	})

	// Template parameter insights have no risk of inconsistency with the other
	// insights.
	eg.Go(func() error {
		if !slices.Contains(sections, codersdk.TemplateInsightsSectionReport) {
			return nil
		}

		var err error
		parameterRows, err = api.Database.GetTemplateParameterInsights(ctx, database.GetTemplateParameterInsightsParams{
			StartTime:   startTime,
			EndTime:     endTime,
			TemplateIDs: templateIDs,
		})
		if err != nil {
			return xerrors.Errorf("get template parameter insights: %w", err)
		}
		return nil
	})

	err := eg.Wait()
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template insights.",
			Detail:  err.Error(),
		})
		return
	}

	parametersUsage, err := db2sdk.TemplateInsightsParameters(parameterRows)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting template parameter insights.",
			Detail:  err.Error(),
		})
		return
	}

	resp := codersdk.TemplateInsightsResponse{
		IntervalReports: []codersdk.TemplateInsightsIntervalReport{},
	}

	if slices.Contains(sections, codersdk.TemplateInsightsSectionReport) {
		resp.Report = &codersdk.TemplateInsightsReport{
			StartTime:       startTime,
			EndTime:         endTime,
			TemplateIDs:     convertTemplateInsightsTemplateIDs(usage, appUsage),
			ActiveUsers:     convertTemplateInsightsActiveUsers(usage, appUsage),
			AppsUsage:       convertTemplateInsightsApps(usage, appUsage),
			ParametersUsage: parametersUsage,
		}
	}

	for _, row := range dailyUsage {
		resp.IntervalReports = append(resp.IntervalReports, codersdk.TemplateInsightsIntervalReport{
			// NOTE(mafredri): This might not be accurate over DST since the
			// parsed location only contains the offset.
			StartTime:   row.StartTime.In(startTime.Location()),
			EndTime:     row.EndTime.In(startTime.Location()),
			Interval:    interval,
			TemplateIDs: row.TemplateIDs,
			ActiveUsers: row.ActiveUsers,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func convertTemplateInsightsTemplateIDs(usage database.GetTemplateInsightsRow, appUsage []database.GetTemplateAppInsightsRow) []uuid.UUID {
	templateIDSet := make(map[uuid.UUID]struct{})
	for _, id := range usage.TemplateIDs {
		templateIDSet[id] = struct{}{}
	}
	for _, app := range appUsage {
		for _, id := range app.TemplateIDs {
			templateIDSet[id] = struct{}{}
		}
	}
	templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for id := range templateIDSet {
		templateIDs = append(templateIDs, id)
	}
	slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})
	return templateIDs
}

func convertTemplateInsightsActiveUsers(usage database.GetTemplateInsightsRow, appUsage []database.GetTemplateAppInsightsRow) int64 {
	activeUserIDSet := make(map[uuid.UUID]struct{})
	for _, id := range usage.ActiveUserIDs {
		activeUserIDSet[id] = struct{}{}
	}
	for _, app := range appUsage {
		for _, id := range app.ActiveUserIDs {
			activeUserIDSet[id] = struct{}{}
		}
	}
	return int64(len(activeUserIDSet))
}

// convertTemplateInsightsApps builds the list of builtin apps and template apps
// from the provided database rows, builtin apps are implicitly a part of all
// templates.
func convertTemplateInsightsApps(usage database.GetTemplateInsightsRow, appUsage []database.GetTemplateAppInsightsRow) []codersdk.TemplateAppUsage {
	// Builtin apps.
	apps := []codersdk.TemplateAppUsage{
		{
			TemplateIDs: usage.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: "Visual Studio Code",
			Slug:        "vscode",
			Icon:        "/icon/code.svg",
			Seconds:     usage.UsageVscodeSeconds,
		},
		{
			TemplateIDs: usage.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: "JetBrains",
			Slug:        "jetbrains",
			Icon:        "/icon/intellij.svg",
			Seconds:     usage.UsageJetbrainsSeconds,
		},
		// TODO(mafredri): We could take Web Terminal usage from appUsage since
		// that should be more accurate. The difference is that this reflects
		// the rpty session as seen by the agent (can live past the connection),
		// whereas appUsage reflects the lifetime of the client connection. The
		// condition finding the corresponding app entry in appUsage is:
		// !app.IsApp && app.AccessMethod == "terminal" && app.SlugOrPort == ""
		{
			TemplateIDs: usage.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: "Web Terminal",
			Slug:        "reconnecting-pty",
			Icon:        "/icon/terminal.svg",
			Seconds:     usage.UsageReconnectingPtySeconds,
		},
		{
			TemplateIDs: usage.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: "SSH",
			Slug:        "ssh",
			Icon:        "/icon/terminal.svg",
			Seconds:     usage.UsageSshSeconds,
		},
	}

	// Use a stable sort, similarly to how we would sort in the query, note that
	// we don't sort in the query because order varies depending on the table
	// collation.
	//
	// ORDER BY access_method, slug_or_port, display_name, icon, is_app
	slices.SortFunc(appUsage, func(a, b database.GetTemplateAppInsightsRow) int {
		if a.AccessMethod != b.AccessMethod {
			return strings.Compare(a.AccessMethod, b.AccessMethod)
		}
		if a.SlugOrPort != b.SlugOrPort {
			return strings.Compare(a.SlugOrPort, b.SlugOrPort)
		}
		if a.DisplayName.String != b.DisplayName.String {
			return strings.Compare(a.DisplayName.String, b.DisplayName.String)
		}
		if a.Icon.String != b.Icon.String {
			return strings.Compare(a.Icon.String, b.Icon.String)
		}
		if !a.IsApp && b.IsApp {
			return -1
		} else if a.IsApp && !b.IsApp {
			return 1
		}
		return 0
	})

	// Template apps.
	for _, app := range appUsage {
		if !app.IsApp {
			continue
		}
		apps = append(apps, codersdk.TemplateAppUsage{
			TemplateIDs: app.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeApp,
			DisplayName: app.DisplayName.String,
			Slug:        app.SlugOrPort,
			Icon:        app.Icon.String,
			Seconds:     app.UsageSeconds,
		})
	}

	return apps
}

// parseInsightsStartAndEndTime parses the start and end time query parameters
// and returns the parsed values. The client provided timezone must be preserved
// when parsing the time. Verification is performed so that the start and end
// time are not zero and that the end time is not before the start time. The
// clock must be set to 00:00:00, except for "today", where end time is allowed
// to provide the hour of the day (e.g. 14:00:00).
func parseInsightsStartAndEndTime(ctx context.Context, rw http.ResponseWriter, startTimeString, endTimeString string) (startTime, endTime time.Time, ok bool) {
	now := time.Now()

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

		// Round upwards one hour to ensure we can fetch the latest data.
		if t.After(now.Truncate(time.Hour).Add(time.Hour)) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  qp.name,
						Detail: fmt.Sprintf("Query param %q must not be in the future", qp.name),
					},
				},
			})
			return time.Time{}, time.Time{}, false
		}

		ensureZeroHour := true
		if qp.name == "end_time" {
			ey, em, ed := t.Date()
			ty, tm, td := now.Date()

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
					Detail: fmt.Sprintf("Query param %q must be after than %q", "end_time", "start_time"),
				},
			},
		})
		return time.Time{}, time.Time{}, false
	}

	return startTime, endTime, true
}

func parseInsightsInterval(ctx context.Context, rw http.ResponseWriter, intervalString string, startTime, endTime time.Time) (codersdk.InsightsReportInterval, bool) {
	switch v := codersdk.InsightsReportInterval(intervalString); v {
	case codersdk.InsightsReportIntervalDay, "":
		return v, true
	case codersdk.InsightsReportIntervalWeek:
		if !lastReportIntervalHasAtLeastSixDays(startTime, endTime) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Detail:  "Last report interval should have at least 6 days.",
			})
			return "", false
		}
		return v, true
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameter has invalid value.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "interval",
					Detail: fmt.Sprintf("must be one of %v", []codersdk.InsightsReportInterval{codersdk.InsightsReportIntervalDay, codersdk.InsightsReportIntervalWeek}),
				},
			},
		})
		return "", false
	}
}

func lastReportIntervalHasAtLeastSixDays(startTime, endTime time.Time) bool {
	lastReportIntervalDays := endTime.Sub(startTime) % (7 * 24 * time.Hour)
	if lastReportIntervalDays == 0 {
		return true // this is a perfectly full week!
	}
	// Ensure that the last interval has at least 6 days, or check the special case, forward DST change,
	// when the duration can be shorter than 6 days: 5 days 23 hours.
	return lastReportIntervalDays >= 6*24*time.Hour || startTime.AddDate(0, 0, 6).Equal(endTime)
}

func templateInsightsSectionAsStrings(sections ...codersdk.TemplateInsightsSection) []string {
	t := make([]string, len(sections))
	for i, s := range sections {
		t[i] = string(s)
	}
	return t
}

func parseTemplateInsightsSections(ctx context.Context, rw http.ResponseWriter, sections []string) ([]codersdk.TemplateInsightsSection, bool) {
	t := make([]codersdk.TemplateInsightsSection, len(sections))
	for i, s := range sections {
		switch v := codersdk.TemplateInsightsSection(s); v {
		case codersdk.TemplateInsightsSectionIntervalReports, codersdk.TemplateInsightsSectionReport:
			t[i] = v
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Validations: []codersdk.ValidationError{
					{
						Field:  "sections",
						Detail: fmt.Sprintf("must be one of %v", []codersdk.TemplateInsightsSection{codersdk.TemplateInsightsSectionIntervalReports, codersdk.TemplateInsightsSectionReport}),
					},
				},
			})
			return nil, false
		}
	}
	return t, true
}
