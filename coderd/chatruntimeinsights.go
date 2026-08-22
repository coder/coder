package coderd

import (
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// writeAgentRuntimeInsightsError maps authz denials to 403 and everything
// else to 500, matching the convention used across coderd handlers.
func writeAgentRuntimeInsightsError(rw http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal error fetching agent runtime insights.",
		Detail:  err.Error(),
	})
}

// @Summary Get insights about Coder Agents runtime
// @ID get-insights-about-coder-agents-runtime
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param start_time query string true "Start time" format(date-time)
// @Param end_time query string true "End time" format(date-time)
// @Success 200 {object} codersdk.AgentRuntimeInsightsResponse
// @Router /experimental/chats/agent-runtime-insights [get]
func (api *API) agentRuntimeInsights(rw http.ResponseWriter, r *http.Request) {
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

	totalMs, err := api.Database.GetAgentRuntimeInsightsTotal(ctx, database.GetAgentRuntimeInsightsTotalParams{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		writeAgentRuntimeInsightsError(rw, r, err)
		return
	}

	dayRows, err := api.Database.GetAgentRuntimeInsightsByDay(ctx, database.GetAgentRuntimeInsightsByDayParams{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		writeAgentRuntimeInsightsError(rw, r, err)
		return
	}

	byDay := make([]codersdk.AgentRuntimeInsightsDay, 0, len(dayRows))
	for _, row := range dayRows {
		byDay = append(byDay, codersdk.AgentRuntimeInsightsDay{
			Day:     row.Day,
			TotalMs: row.TotalRuntimeMs,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentRuntimeInsightsResponse{
		StartTime: startTime,
		EndTime:   endTime,
		TotalMs:   totalMs,
		ByDay:     byDay,
	})
}

// @Summary Get insights about Coder Agents runtime by user
// @ID get-insights-about-coder-agents-runtime-by-user
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param start_time query string true "Start time" format(date-time)
// @Param end_time query string true "End time" format(date-time)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.AgentRuntimeInsightsByUserResponse
// @Router /experimental/chats/agent-runtime-insights/users [get]
func (api *API) agentRuntimeInsightsByUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	p := httpapi.NewQueryParamParser().
		RequiredNotEmpty("start_time").
		RequiredNotEmpty("end_time")
	vals := r.URL.Query()
	var (
		startTimeString = p.String(vals, "", "start_time")
		endTimeString   = p.String(vals, "", "end_time")
		limit           = p.PositiveInt32(vals, 25, "limit")
		offset          = p.PositiveInt32(vals, 0, "offset")
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
	if limit <= 0 {
		limit = 25
	}

	count, err := api.Database.GetAgentRuntimeInsightsByUserCount(ctx, database.GetAgentRuntimeInsightsByUserCountParams{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		writeAgentRuntimeInsightsError(rw, r, err)
		return
	}

	rows, err := api.Database.GetAgentRuntimeInsightsByUser(ctx, database.GetAgentRuntimeInsightsByUserParams{
		StartTime: startTime,
		EndTime:   endTime,
		LimitOpt:  limit,
		OffsetOpt: offset,
	})
	if err != nil {
		writeAgentRuntimeInsightsError(rw, r, err)
		return
	}

	users := make([]codersdk.AgentRuntimeInsightsUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, codersdk.AgentRuntimeInsightsUser{
			UserID:       row.UserID,
			Username:     row.Username,
			AvatarURL:    row.AvatarURL,
			TotalMs:      row.TotalRuntimeMs,
			MessageCount: row.MessageCount,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentRuntimeInsightsByUserResponse{
		Users: users,
		Count: count,
	})
}
