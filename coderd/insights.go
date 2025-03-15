package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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
// @Param tz_offset query int true "Time-zone offset (e.g. -2)"
// @Success 200 {object} codersdk.DAUsResponse
// @Router /insights/daus [get]
func (api *API) deploymentDAUs(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	api.returnDAUsInternal(rw, r, nil)
}

func (api *API) returnDAUsInternal(rw http.ResponseWriter, r *http.Request, templateIDs []uuid.UUID) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()
	tzOffset := p.Int(vals, 0, "tz_offset")
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	loc := time.FixedZone("", tzOffset*3600)
	// If the time is 14:01 or 14:31, we still want to include all the
	// data between 14:00 and 15:00. Our rollups buckets are 30 minutes
	// so this works nicely. It works just as well for 23:59 as well.
	nextHourInLoc := time.Now().In(loc).Truncate(time.Hour).Add(time.Hour)
	// Always return 60 days of data (2 months).
	sixtyDaysAgo := nextHourInLoc.In(loc).Truncate(24*time.Hour).AddDate(0, 0, -60)

	rows, err := api.Database.GetTemplateInsightsByInterval(ctx, database.GetTemplateInsightsByIntervalParams{
		StartTime:    sixtyDaysAgo,
		EndTime:      nextHourInLoc,
		IntervalDays: 1,
		TemplateIDs:  templateIDs,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching DAUs.",
			Detail:  err.Error(),
		})
	}

	resp := codersdk.DAUsResponse{
		TZHourOffset: tzOffset,
		Entries:      make([]codersdk.DAUEntry, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Entries = append(resp.Entries, codersdk.DAUEntry{
			Date:   row.StartTime.In(loc).Format(time.DateOnly),
			Amount: int(row.ActiveUsers),
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get insights about user activity
// @ID get-insights-about-user-activity
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Param start_time query string true "Start time" format(date-time)
// @Param end_time query string true "End time" format(date-time)
// @Param template_ids query []string false "Template IDs" collectionFormat(csv)
// @Success 200 {object} codersdk.UserActivityInsightsResponse
// @Router /insights/user-activity [get]
func (api *API) insightsUserActivity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser().
		RequiredNotEmpty("start_time").
		RequiredNotEmpty("end_time")
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

	startTime, endTime, ok := parseInsightsStartAndEndTime(ctx, rw, time.Now(), startTimeString, endTimeString)
	if !ok {
		return
	}

	rows, err := api.Database.GetUserActivityInsights(ctx, database.GetUserActivityInsightsParams{
		StartTime:   startTime,
		EndTime:     endTime,
		TemplateIDs: templateIDs,
	})
	if err != nil {
		// No data is not an error.
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserActivityInsightsResponse{
				Report: codersdk.UserActivityInsightsReport{
					StartTime:   startTime,
					EndTime:     endTime,
					TemplateIDs: []uuid.UUID{},
					Users:       []codersdk.UserActivity{},
				},
			})
			return
		}
		// Check authorization.
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
			AvatarURL:   row.AvatarURL,
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
// @Param start_time query string true "Start time" format(date-time)
// @Param end_time query string true "End time" format(date-time)
// @Param template_ids query []string false "Template IDs" collectionFormat(csv)
// @Success 200 {object} codersdk.UserLatencyInsightsResponse
// @Router /insights/user-latency [get]
func (api *API) insightsUserLatency(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser().
		RequiredNotEmpty("start_time").
		RequiredNotEmpty("end_time")
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

	startTime, endTime, ok := parseInsightsStartAndEndTime(ctx, rw, time.Now(), startTimeString, endTimeString)
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
			AvatarURL:   row.AvatarURL,
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

// @Summary Get insights about user status counts
// @ID get-insights-about-user-status-counts
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Param tz_offset query int true "Time-zone offset (e.g. -2)"
// @Success 200 {object} codersdk.GetUserStatusCountsResponse
// @Router /insights/user-status-counts [get]
func (api *API) insightsUserStatusCounts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()
	tzOffset := p.Int(vals, 0, "tz_offset")
	interval := p.Int(vals, int((24 * time.Hour).Seconds()), "interval")
	p.ErrorExcessParams(vals)

	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	loc := time.FixedZone("", tzOffset*3600)
	nextHourInLoc := dbtime.Now().Truncate(time.Hour).Add(time.Hour).In(loc)
	sixtyDaysAgo := dbtime.StartOfDay(nextHourInLoc).AddDate(0, 0, -60)

	rows, err := api.Database.GetUserStatusCounts(ctx, database.GetUserStatusCountsParams{
		StartTime: sixtyDaysAgo,
		EndTime:   nextHourInLoc,
		Interval:  int32(interval),
	})
	if err != nil {
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user status counts over time.",
			Detail:  err.Error(),
		})
		return
	}

	resp := codersdk.GetUserStatusCountsResponse{
		StatusCounts: make(map[codersdk.UserStatus][]codersdk.UserStatusChangeCount),
	}

	for _, row := range rows {
		status := codersdk.UserStatus(row.Status)
		resp.StatusCounts[status] = append(resp.StatusCounts[status], codersdk.UserStatusChangeCount{
			Date:  row.Date,
			Count: row.Count,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get insights about templates
// @ID get-insights-about-templates
// @Security CoderSessionToken
// @Produce json
// @Tags Insights
// @Param start_time query string true "Start time" format(date-time)
// @Param end_time query string true "End time" format(date-time)
// @Param interval query string true "Interval" enums(week,day)
// @Param template_ids query []string false "Template IDs" collectionFormat(csv)
// @Success 200 {object} codersdk.TemplateInsightsResponse
// @Router /insights/templates [get]
func (api *API) insightsTemplates(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser().
		RequiredNotEmpty("start_time").
		RequiredNotEmpty("end_time")
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

	startTime, endTime, ok := parseInsightsStartAndEndTime(ctx, rw, time.Now(), startTimeString, endTimeString)
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
			TemplateIDs:     usage.TemplateIDs,
			ActiveUsers:     usage.ActiveUsers,
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

// convertTemplateInsightsApps builds the list of builtin apps and template apps
// from the provided database rows, builtin apps are implicitly a part of all
// templates.
func convertTemplateInsightsApps(usage database.GetTemplateInsightsRow, appUsage []database.GetTemplateAppInsightsRow) []codersdk.TemplateAppUsage {
	// Builtin apps.
	apps := []codersdk.TemplateAppUsage{
		{
			TemplateIDs: usage.VscodeTemplateIds,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameVSCode,
			Slug:        "vscode",
			Icon:        "/icon/code.svg",
			Seconds:     usage.UsageVscodeSeconds,
		},
		{
			TemplateIDs: usage.JetbrainsTemplateIds,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameJetBrains,
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
			TemplateIDs: usage.ReconnectingPtyTemplateIds,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameWebTerminal,
			Slug:        "reconnecting-pty",
			Icon:        "/icon/terminal.svg",
			Seconds:     usage.UsageReconnectingPtySeconds,
		},
		{
			TemplateIDs: usage.SshTemplateIds,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameSSH,
			Slug:        "ssh",
			Icon:        "/icon/terminal.svg",
			Seconds:     usage.UsageSshSeconds,
		},
		{
			TemplateIDs: usage.SftpTemplateIds,
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameSFTP,
			Slug:        "sftp",
			Icon:        "/icon/terminal.svg",
			Seconds:     usage.UsageSftpSeconds,
		},
	}

	// Use a stable sort, similarly to how we would sort in the query, note that
	// we don't sort in the query because order varies depending on the table
	// collation.
	//
	// ORDER BY slug, display_name, icon
	slices.SortFunc(appUsage, func(a, b database.GetTemplateAppInsightsRow) int {
		if a.Slug != b.Slug {
			return strings.Compare(a.Slug, b.Slug)
		}
		if a.DisplayName != b.DisplayName {
			return strings.Compare(a.DisplayName, b.DisplayName)
		}
		return strings.Compare(a.Icon, b.Icon)
	})

	// Template apps.
	for _, app := range appUsage {
		apps = append(apps, codersdk.TemplateAppUsage{
			TemplateIDs: app.TemplateIDs,
			Type:        codersdk.TemplateAppsTypeApp,
			DisplayName: app.DisplayName,
			Slug:        app.Slug,
			Icon:        app.Icon,
			Seconds:     app.UsageSeconds,
			TimesUsed:   app.TimesUsed,
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
func parseInsightsStartAndEndTime(ctx context.Context, rw http.ResponseWriter, now time.Time, startTimeString, endTimeString string) (startTime, endTime time.Time, ok bool) {
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

		// Change now to the same timezone as the parsed time.
		now := now.In(t.Location())

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
						Detail: fmt.Sprintf("Query param %q must have the clock set to 00:00:00, got %s", qp.name, qp.value),
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
						Detail: fmt.Sprintf("Query param %q must have the clock set to %02d:00:00, got %s", qp.name, h, qp.value),
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
