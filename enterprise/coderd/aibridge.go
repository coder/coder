package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

const (
	maxListInterceptionsLimit     = 1000
	maxListSessionsLimit          = 1000
	maxListModelsLimit            = 1000
	defaultListInterceptionsLimit = 100
	defaultListSessionsLimit      = 100
	defaultListModelsLimit        = 100
	// aiBridgeRateLimitWindow is the fixed duration for rate limiting AI Bridge
	// requests. This is hardcoded to keep configuration simple.
	aiBridgeRateLimitWindow = time.Second
)

// aibridgeHandler handles all aibridged-related endpoints.
func aibridgeHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	// Build the overload protection middleware chain for the aibridged handler.
	// These limits are applied per-replica.
	bridgeCfg := api.DeploymentValues.AI.BridgeConfig
	concurrencyLimiter := httpmw.ConcurrencyLimit(bridgeCfg.MaxConcurrency.Value(), "AI Bridge")
	rateLimiter := httpmw.RateLimitByAuthToken(int(bridgeCfg.RateLimit.Value()), aiBridgeRateLimitWindow)

	return func(r chi.Router) {
		r.Use(api.RequireFeatureMW(codersdk.FeatureAIBridge))
		r.Group(func(r chi.Router) {
			r.Use(middlewares...)
			r.Get("/interceptions", api.aiBridgeListInterceptions)
			r.Get("/sessions", api.aiBridgeListSessions)
			r.Get("/models", api.aiBridgeListModels)
		})

		// Apply overload protection middleware to the aibridged handler.
		// Concurrency limit is checked first for faster rejection under load.
		r.Group(func(r chi.Router) {
			r.Use(concurrencyLimiter, rateLimiter)
			// This is a bit funky but since aibridge only exposes a HTTP
			// handler, this is how it has to be.
			r.HandleFunc("/*", func(rw http.ResponseWriter, r *http.Request) {
				if api.aibridgedHandler == nil {
					httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
						Message: "aibridged handler not mounted",
					})
					return
				}

				http.StripPrefix("/api/v2/aibridge", api.aibridgedHandler).ServeHTTP(rw, r)
			})
		})
	}
}

// aiBridgeListInterceptions returns all AI Bridge interceptions a user can read.
// Optional filters with query params
//
// @Summary List AI Bridge interceptions
// @ID list-ai-bridge-interceptions
// @Security CoderSessionToken
// @Produce json
// @Tags AI Bridge
// @Param q query string false "Search query in the format `key:value`. Available keys are: initiator, provider, model, started_after, started_before."
// @Param limit query int false "Page limit"
// @Param after_id query string false "Cursor pagination after ID (cannot be used with offset)"
// @Param offset query int false "Offset pagination (cannot be used with after_id)"
// @Success 200 {object} codersdk.AIBridgeListInterceptionsResponse
// @Router /aibridge/interceptions [get]
func (api *API) aiBridgeListInterceptions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := coderd.ParsePagination(rw, r)
	if !ok {
		return
	}
	if page.AfterID != uuid.Nil && page.Offset != 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameters have invalid values.",
			Detail:  "Cannot use both after_id and offset pagination in the same request.",
		})
		return
	}
	if page.Limit == 0 {
		page.Limit = defaultListInterceptionsLimit
	}
	if page.Limit > maxListInterceptionsLimit || page.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid pagination limit value.",
			Detail:  fmt.Sprintf("Pagination limit must be in range (0, %d]", maxListInterceptionsLimit),
		})
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AIBridgeInterceptions(ctx, api.Database, queryStr, page, apiKey.UserID)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid workspace search query.",
			Validations: errs,
		})
		return
	}

	var (
		count int64
		rows  []database.ListAIBridgeInterceptionsRow
	)
	err := api.Database.InTx(func(db database.Store) error {
		// Ensure the after_id interception exists and is visible to the user.
		if page.AfterID != uuid.Nil {
			_, err := db.GetAIBridgeInterceptionByID(ctx, page.AfterID)
			if err != nil {
				return xerrors.Errorf("get aibridge interception by id %s for cursor pagination: %w", page.AfterID, err)
			}
		}

		var err error
		// Get the full count of authorized interceptions matching the filter
		// for pagination purposes.
		count, err = db.CountAIBridgeInterceptions(ctx, database.CountAIBridgeInterceptionsParams{
			StartedAfter:  filter.StartedAfter,
			StartedBefore: filter.StartedBefore,
			InitiatorID:   filter.InitiatorID,
			Provider:      filter.Provider,
			Model:         filter.Model,
			Client:        filter.Client,
		})
		if err != nil {
			return xerrors.Errorf("count authorized aibridge interceptions: %w", err)
		}

		// This only returns authorized interceptions (when using dbauthz).
		rows, err = db.ListAIBridgeInterceptions(ctx, filter)
		if err != nil {
			return xerrors.Errorf("list aibridge interceptions: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Bridge interceptions.",
			Detail:  err.Error(),
		})
		return
	}

	// This fetches the other rows associated with the interceptions.
	items, err := populatedAndConvertAIBridgeInterceptions(ctx, api.Database, rows)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting database rows to API response.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIBridgeListInterceptionsResponse{
		Count:   count,
		Results: items,
	})
}

// aiBridgeListSessions returns AI Bridge sessions (aggregated interceptions).
//
// @Summary List AI Bridge sessions
// @ID list-ai-bridge-sessions
// @Security CoderSessionToken
// @Produce json
// @Tags AI Bridge
// @Param q query string false "Search query in the format `key:value`. Available keys are: initiator, provider, model, client, session_id, started_after, started_before."
// @Param limit query int false "Page limit"
// @Param after_session_id query string false "Cursor pagination after session ID (cannot be used with offset)"
// @Param offset query int false "Offset pagination (cannot be used with after_session_id)"
// @Success 200 {object} codersdk.AIBridgeListSessionsResponse
// @Router /aibridge/sessions [get]
func (api *API) aiBridgeListSessions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := coderd.ParsePagination(rw, r)
	if !ok {
		return
	}

	afterSessionID := r.URL.Query().Get("after_session_id")
	if afterSessionID != "" && page.Offset != 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query parameters have invalid values.",
			Detail:  "Cannot use both after_session_id and offset pagination in the same request.",
		})
		return
	}
	if page.Limit == 0 {
		page.Limit = defaultListSessionsLimit
	}
	if page.Limit > maxListSessionsLimit || page.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid pagination limit value.",
			Detail:  fmt.Sprintf("Pagination limit must be in range (0, %d]", maxListSessionsLimit),
		})
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AIBridgeSessions(ctx, api.Database, queryStr, page, apiKey.UserID, afterSessionID)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid session search query.",
			Validations: errs,
		})
		return
	}

	// Validate the cursor session exists before running the main query.
	if afterSessionID != "" {
		//nolint:exhaustruct // Only need session_id filter and limit.
		cursor, err := api.Database.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
			SessionID: afterSessionID,
			Limit:     1,
		})
		if err != nil {
			api.Logger.Error(ctx, "error validating after_session_id cursor", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error validating after_session_id cursor.",
				Detail:  "", // Don't leak database issue to client.
			})
			return
		}
		if len(cursor) == 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query parameter has invalid value.",
				Detail:  fmt.Sprintf("after_session_id: session %q not found", afterSessionID),
			})
			return
		}
	}

	var (
		count int64
		rows  []database.ListAIBridgeSessionsRow
	)
	err := api.Database.InTx(func(db database.Store) error {
		var err error
		count, err = db.CountAIBridgeSessions(ctx, database.CountAIBridgeSessionsParams{
			StartedAfter:  filter.StartedAfter,
			StartedBefore: filter.StartedBefore,
			InitiatorID:   filter.InitiatorID,
			Provider:      filter.Provider,
			Model:         filter.Model,
			Client:        filter.Client,
			SessionID:     filter.SessionID,
		})
		if err != nil {
			return xerrors.Errorf("count authorized aibridge sessions: %w", err)
		}

		rows, err = db.ListAIBridgeSessions(ctx, filter)
		if err != nil {
			return xerrors.Errorf("list aibridge sessions: %w", err)
		}

		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // Consistency across queries tables while writes may be occurring.
		ReadOnly:     true,
		TxIdentifier: "aibridge_list_sessions",
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Bridge sessions.",
			Detail:  err.Error(),
		})
		return
	}

	sessions := make([]codersdk.AIBridgeSession, len(rows))
	for i, row := range rows {
		sessions[i] = db2sdk.AIBridgeSession(row)
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIBridgeListSessionsResponse{
		Count:    count,
		Sessions: sessions,
	})
}

// aiBridgeListModels returns all AI Bridge models a user can see.
//
// @Summary List AI Bridge models
// @ID list-ai-bridge-models
// @Security CoderSessionToken
// @Produce json
// @Tags AI Bridge
// @Success 200 {array} string
// @Router /aibridge/models [get]
func (api *API) aiBridgeListModels(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	page, ok := coderd.ParsePagination(rw, r)
	if !ok {
		return
	}

	if page.Limit == 0 {
		page.Limit = defaultListModelsLimit
	}

	if page.Limit > maxListModelsLimit || page.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid pagination limit value.",
			Detail:  fmt.Sprintf("Pagination limit must be in range (0, %d]", maxListModelsLimit),
		})
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AIBridgeModels(queryStr, page)

	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI Bridge models search query.",
			Validations: errs,
		})
		return
	}

	models, err := api.Database.ListAIBridgeModels(ctx, filter)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Bridge models.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, models)
}

func populatedAndConvertAIBridgeInterceptions(ctx context.Context, db database.Store, dbInterceptions []database.ListAIBridgeInterceptionsRow) ([]codersdk.AIBridgeInterception, error) {
	if len(dbInterceptions) == 0 {
		return []codersdk.AIBridgeInterception{}, nil
	}

	ids := make([]uuid.UUID, len(dbInterceptions))
	for i, row := range dbInterceptions {
		ids[i] = row.AIBridgeInterception.ID
	}

	tokenUsagesRows, err := db.ListAIBridgeTokenUsagesByInterceptionIDs(ctx, ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge token usages from database: %w", err)
	}
	tokenUsagesMap := make(map[uuid.UUID][]database.AIBridgeTokenUsage, len(dbInterceptions))
	for _, row := range tokenUsagesRows {
		tokenUsagesMap[row.InterceptionID] = append(tokenUsagesMap[row.InterceptionID], row)
	}

	userPromptRows, err := db.ListAIBridgeUserPromptsByInterceptionIDs(ctx, ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge user prompts from database: %w", err)
	}
	userPromptsMap := make(map[uuid.UUID][]database.AIBridgeUserPrompt, len(dbInterceptions))
	for _, row := range userPromptRows {
		userPromptsMap[row.InterceptionID] = append(userPromptsMap[row.InterceptionID], row)
	}

	toolUsagesRows, err := db.ListAIBridgeToolUsagesByInterceptionIDs(ctx, ids)
	if err != nil {
		return nil, xerrors.Errorf("get linked aibridge tool usages from database: %w", err)
	}
	toolUsagesMap := make(map[uuid.UUID][]database.AIBridgeToolUsage, len(dbInterceptions))
	for _, row := range toolUsagesRows {
		toolUsagesMap[row.InterceptionID] = append(toolUsagesMap[row.InterceptionID], row)
	}

	items := make([]codersdk.AIBridgeInterception, len(dbInterceptions))
	for i, row := range dbInterceptions {
		items[i] = db2sdk.AIBridgeInterception(
			row.AIBridgeInterception,
			row.VisibleUser,
			tokenUsagesMap[row.AIBridgeInterception.ID],
			userPromptsMap[row.AIBridgeInterception.ID],
			toolUsagesMap[row.AIBridgeInterception.ID],
		)
	}

	return items, nil
}
