package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

const (
	maxListInterceptionsLimit     = 1000
	defaultListInterceptionsLimit = 100
)

func (api *API) aiBridgeListInterceptions(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceAibridgeInterception) {
		httpapi.Forbidden(rw)
		return
	}

	ctx := r.Context()
	var req codersdk.AIBridgeListInterceptionsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if !req.PeriodStart.IsZero() && !req.PeriodEnd.IsZero() && req.PeriodEnd.Before(req.PeriodStart) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid time frame.",
			Detail:  "End of the search period must be before start.",
		})
		return
	}

	if req.Limit == 0 {
		req.Limit = defaultListInterceptionsLimit
	}

	if req.Limit > maxListInterceptionsLimit || req.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid limit value.",
			Detail:  "Limit value must be in range <1, 1000>",
		})
		return
	}

	// Database returns one row for each tuple (interception, tool, prompt).
	// Right now there is a single promp per interception although model allows multiple so this could change in the future.
	// There can be multiple tools used in single interception.
	// Results are ordered by Interception.StartedAt, Interception.ID, Tool.CreatedAt
	rows, err := api.Database.ListAIBridgeInterceptions(ctx, database.ListAIBridgeInterceptionsParams{
		PeriodStart: req.PeriodStart,
		PeriodEnd:   req.PeriodEnd,
		CursorTime:  req.Cursor.Time,
		CursorID:    req.Cursor.ID,
		InitiatorID: req.InitiatorID,
		LimitOpt:    req.Limit,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AIBridge interceptions.",
			Detail:  err.Error(),
		})
		return
	}

	resp := prepareResponse(rows)
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func prepareResponse(rows []database.ListAIBridgeInterceptionsRow) codersdk.AIBridgeListInterceptionsResponse {
	resp := codersdk.AIBridgeListInterceptionsResponse{
		Results: []codersdk.AIBridgeListInterceptionsResult{},
	}

	if len(rows) > 0 {
		resp.Cursor.ID = rows[len(rows)-1].ID
		resp.Cursor.Time = rows[len(rows)-1].StartedAt.UTC()
	}

	for i := 0; i < len(rows); {
		row := rows[i]
		row.StartedAt = row.StartedAt.UTC()

		result := codersdk.AIBridgeListInterceptionsResult{
			InterceptionID: row.ID,
			UserID:         row.InitiatorID,
			Provider:       row.Provider,
			Model:          row.Model,
			Prompt:         row.Prompt.String,
			StartedAt:      row.StartedAt,
			Tokens: codersdk.AIBridgeListInterceptionsTokens{
				Input:  row.InputTokens,
				Output: row.OutputTokens,
			},
			Tools: []codersdk.AIBridgeListInterceptionsTool{},
		}

		interceptionID := row.ID
		for ; i < len(rows) && interceptionID == rows[i].ID; i++ {
			if rows[i].ServerUrl.Valid || rows[i].Tool.Valid || rows[i].Input.Valid {
				result.Tools = append(result.Tools, codersdk.AIBridgeListInterceptionsTool{
					Server: rows[i].ServerUrl.String,
					Tool:   rows[i].Tool.String,
					Input:  rows[i].Input.String,
				})
			}
		}

		resp.Results = append(resp.Results, result)
	}
	return resp
}
