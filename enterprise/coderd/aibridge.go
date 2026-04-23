package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
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
	maxListClientsLimit           = 1000
	defaultListInterceptionsLimit = 100
	defaultListSessionsLimit      = 100
	defaultListModelsLimit        = 100
	defaultListClientsLimit       = 100
	// aiBridgeRateLimitWindow is the fixed duration for rate limiting AI Bridge
	// requests. This is hardcoded to keep configuration simple.
	aiBridgeRateLimitWindow = time.Second
)

// errInvalidCursor is returned when a pagination cursor does not
// reference a valid resource in the expected scope.
var errInvalidCursor = xerrors.New("invalid pagination cursor")

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
			r.Get("/sessions/{session_id}", api.aiBridgeGetSessionThreads)
			r.Get("/models", api.aiBridgeListModels)
			r.Get("/clients", api.aiBridgeListClients)
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

				// Reject BYOK requests when the deployment has not
				// enabled bring-your-own-key mode.
				if agplaibridge.IsBYOK(r.Header) && !bridgeCfg.AllowBYOK.Value() {
					httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
						Message: "Bring Your Own Key (BYOK) mode is not enabled.",
						Detail:  "Contact your administrator to enable it with --aibridge-allow-byok.",
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
		// Validate the cursor interception exists and is visible.
		if err := validateInterceptionCursor(ctx, db, page.AfterID, "after_id", ""); err != nil {
			return err
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
		if errors.Is(err, errInvalidCursor) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid pagination cursor.",
				Detail:  err.Error(),
			})
			return
		}
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

// aiBridgeGetSessionThreads returns a single session with fully expanded
// threads including agentic actions and thinking blocks.
//
// @Summary Get AI Bridge session threads
// @ID get-ai-bridge-session-threads
// @Security CoderSessionToken
// @Produce json
// @Tags AI Bridge
// @Param session_id path string true "Session ID (client_session_id or interception UUID)"
// @Param after_id query string false "Thread pagination cursor (forward/older)"
// @Param before_id query string false "Thread pagination cursor (backward/newer)"
// @Param limit query int false "Number of threads per page (default 50)"
// @Success 200 {object} codersdk.AIBridgeSessionThreadsResponse
// @Router /aibridge/sessions/{session_id} [get]
func (api *API) aiBridgeGetSessionThreads(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessionIDParam := chi.URLParam(r, "session_id")
	if sessionIDParam == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing session_id path parameter.",
		})
		return
	}

	// Parse optional pagination cursors.
	var afterID, beforeID uuid.UUID
	if v := r.URL.Query().Get("after_id"); v != "" {
		var err error
		afterID, err = uuid.Parse(v)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid after_id query parameter.",
				Detail:  err.Error(),
			})
			return
		}
	}
	if v := r.URL.Query().Get("before_id"); v != "" {
		var err error
		beforeID, err = uuid.Parse(v)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid before_id query parameter.",
				Detail:  err.Error(),
			})
			return
		}
	}
	if afterID != uuid.Nil && beforeID != uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot use both after_id and before_id in the same request.",
		})
		return
	}

	var limit int32 = 50
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 32)
		if err != nil || parsed < 1 || parsed > 200 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid limit query parameter.",
				Detail:  "Limit must be between 1 and 200.",
			})
			return
		}
		limit = int32(parsed)
	}

	// Fetch session metadata by reusing the sessions list query
	// with a session_id filter.
	//nolint:exhaustruct // Let's keep things concise.
	sessions, err := api.Database.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
		Limit:     1,
		SessionID: sessionIDParam,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching session.",
			Detail:  err.Error(),
		})
		return
	}
	if len(sessions) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Session not found.",
		})
		return
	}
	session := sessions[0]

	// Fetch paginated session threads and their sub-resources inside
	// a repeatable-read transaction so the data is consistent.
	var (
		allRows       []database.ListAIBridgeSessionThreadsRow
		threadRows    []database.ListAIBridgeSessionThreadsRow
		tokenUsages   []database.AIBridgeTokenUsage
		toolUsages    []database.AIBridgeToolUsage
		userPrompts   []database.AIBridgeUserPrompt
		modelThoughts []database.AIBridgeModelThought
	)
	err = api.Database.InTx(func(db database.Store) error {
		// Validate cursor IDs before querying threads. The SQL
		// subquery returns NULL for unknown cursors, which silently
		// filters out all rows instead of surfacing an error.
		if err := validateInterceptionCursor(ctx, db, afterID, "after_id", sessionIDParam); err != nil {
			return err
		}
		if err := validateInterceptionCursor(ctx, db, beforeID, "before_id", sessionIDParam); err != nil {
			return err
		}

		var err error

		// Fetch all interceptions (unpaginated) so we can aggregate
		// session-level token metadata across every thread.
		//nolint:exhaustruct // Let's be concise.
		allRows, err = db.ListAIBridgeSessionThreads(ctx, database.ListAIBridgeSessionThreadsParams{
			SessionID: sessionIDParam,
		})
		if err != nil {
			return xerrors.Errorf("list all session threads: %w", err)
		}

		threadRows, err = db.ListAIBridgeSessionThreads(ctx, database.ListAIBridgeSessionThreadsParams{
			SessionID: sessionIDParam,
			AfterID:   afterID,
			BeforeID:  beforeID,
			Limit:     limit,
		})
		if err != nil {
			return xerrors.Errorf("list session threads: %w", err)
		}

		// Use all interception IDs for token usage (session-level
		// metadata aggregation needs every thread). Use only the
		// page's IDs for other sub-resources.
		allIDs := make([]uuid.UUID, len(allRows))
		for i, row := range allRows {
			allIDs[i] = row.AIBridgeInterception.ID
		}
		ids := make([]uuid.UUID, len(threadRows))
		for i, row := range threadRows {
			ids[i] = row.AIBridgeInterception.ID
		}

		tokenUsages, err = db.ListAIBridgeTokenUsagesByInterceptionIDs(ctx, allIDs)
		if err != nil {
			return xerrors.Errorf("list token usages: %w", err)
		}

		toolUsages, err = db.ListAIBridgeToolUsagesByInterceptionIDs(ctx, ids)
		if err != nil {
			return xerrors.Errorf("list tool usages: %w", err)
		}

		userPrompts, err = db.ListAIBridgeUserPromptsByInterceptionIDs(ctx, ids)
		if err != nil {
			return xerrors.Errorf("list user prompts: %w", err)
		}

		modelThoughts, err = db.ListAIBridgeModelThoughtsByInterceptionIDs(ctx, ids)
		if err != nil {
			return xerrors.Errorf("list model thoughts: %w", err)
		}

		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead,
		ReadOnly:     true,
		TxIdentifier: "aibridge_get_session_threads",
	})
	if err != nil {
		if errors.Is(err, errInvalidCursor) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid pagination cursor.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching session threads.",
			Detail:  err.Error(),
		})
		return
	}

	resp := db2sdk.AIBridgeSessionThreads(session, threadRows, tokenUsages, toolUsages, userPrompts, modelThoughts)

	httpapi.Write(ctx, rw, http.StatusOK, resp)
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

// aiBridgeListClients returns all AI Bridge clients a user can see.
//
// @Summary List AI Bridge clients
// @ID list-ai-bridge-clients
// @Security CoderSessionToken
// @Produce json
// @Tags AI Bridge
// @Success 200 {array} string
// @Router /aibridge/clients [get]
func (api *API) aiBridgeListClients(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	page, ok := coderd.ParsePagination(rw, r)
	if !ok {
		return
	}

	if page.Limit == 0 {
		page.Limit = defaultListClientsLimit
	}

	if page.Limit > maxListClientsLimit || page.Limit < 1 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid pagination limit value.",
			Detail:  fmt.Sprintf("Pagination limit must be in range (0, %d]", maxListClientsLimit),
		})
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.AIBridgeClients(queryStr, page)

	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI Bridge clients search query.",
			Validations: errs,
		})
		return
	}

	clients, err := api.Database.ListAIBridgeClients(ctx, filter)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Bridge clients.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, clients)
}

// validateInterceptionCursor checks that a pagination cursor refers to an
// existing interception. When sessionID is non-empty the interception must
// also belong to that session. Returns errInvalidCursor on failure so
// callers can distinguish bad cursors from internal errors.
func validateInterceptionCursor(ctx context.Context, db database.Store, cursorID uuid.UUID, cursorName, sessionID string) error {
	if cursorID == uuid.Nil {
		return nil
	}
	interception, err := db.GetAIBridgeInterceptionByID(ctx, cursorID)
	if err != nil {
		return xerrors.Errorf("%s: interception %s not found: %w", cursorName, cursorID, errInvalidCursor)
	}
	if sessionID != "" && interception.SessionID != sessionID {
		return xerrors.Errorf("%s: interception %s does not belong to session %s: %w", cursorName, cursorID, sessionID, errInvalidCursor)
	}
	return nil
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
