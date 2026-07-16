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
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/codersdk"
)

const (
	maxListSessionsLimit     = 1000
	maxListModelsLimit       = 1000
	maxListClientsLimit      = 1000
	defaultListSessionsLimit = 100
	defaultListModelsLimit   = 100
	defaultListClientsLimit  = 100
	// aiBridgeRateLimitWindow is the fixed duration for rate limiting AI Bridge
	// requests. This is hardcoded to keep configuration simple.
	aiBridgeRateLimitWindow              = time.Second
	maxOrganizationGroupsAISpendGroupIDs = 100
	maxGroupMembersAISpendUserIDs        = 100
)

// errInvalidCursor is returned when a pagination cursor does not
// reference a valid resource in the expected scope.
var errInvalidCursor = xerrors.New("invalid pagination cursor")

// This name is raised by a trigger function with USING CONSTRAINT.
// It is not a table CHECK constraint, so dbgen does not emit it in
// check_constraint.go.
const userAIBudgetOverridesMustBeGroupMemberConstraint database.CheckConstraint = "user_ai_budget_overrides_must_be_group_member"

// aibridgeHTTPHandler returns the legacy /api/v2/aibridge route tree.
// Kept for backward compatibility only.
//
// NOTE: new endpoints must be registered on the enterprise API
// handler under /api/v2/ai-gateway, not in this shared route builder.
func aibridgeHTTPHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return aiBridgeRoutes(api, agplaibridge.AIBridgeRootPath, middlewares...)
}

// aiGatewayHTTPHandler returns the /api/v2/ai-gateway route tree.
// This shares the same route builder as /aibridge for endpoints that
// existed before the rename.
//
// NOTE: new endpoints must be registered on the enterprise API
// handler under /api/v2/ai-gateway, not in this shared route builder.
func aiGatewayHTTPHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return aiBridgeRoutes(api, agplaibridge.AIGatewayRootPath, middlewares...)
}

// aiBridgeRoutes builds the shared route tree for the legacy /aibridge
// and /ai-gateway prefixes. It contains the upstream AI provider
// catch-all handler and the management endpoints that were released
// under /aibridge. The stripPrefix parameter selects which URL prefix
// to strip before forwarding to the in-memory aibridged handler.
func aiBridgeRoutes(api *API, stripPrefix string, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return func(r chi.Router) {
		r.Use(api.RequireFeatureMW(codersdk.FeatureAIBridge))
		r.Group(func(r chi.Router) {
			r.Use(middlewares...)
			r.Get("/sessions", api.aiBridgeListSessions)
			r.Get("/sessions/{session_id}", api.aiBridgeGetSessionThreads)
			r.Get("/models", api.aiBridgeListModels)
			r.Get("/clients", api.aiBridgeListClients)
		})

		// Apply the shared per-request data-plane middleware (per-replica
		// overload protection plus BYOK gating) to the aibridged handler.
		r.Group(func(r chi.Router) {
			r.Use(AIGatewayDataPlaneMiddleware(api.DeploymentValues.AI.BridgeConfig))
			// This is a bit funky but since aibridge only exposes a HTTP
			// handler, this is how it has to be.
			r.HandleFunc("/*", func(rw http.ResponseWriter, r *http.Request) {
				handler := api.AGPL.AIGatewayHandler()
				if handler == nil {
					httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
						Message: "aibridged handler not mounted",
					})
					return
				}

				// Strip the prefix and relay to the aibridged handler.
				http.StripPrefix(stripPrefix, handler).ServeHTTP(rw, r)
			})
		})
	}
}

// AIGatewayDataPlaneMiddleware returns the per-request middleware chain that
// guards the AI Gateway data-plane handler. It is the single source of truth
// shared by the embedded route and the standalone gateway.
func AIGatewayDataPlaneMiddleware(cfg codersdk.AIBridgeConfig) func(http.Handler) http.Handler {
	concurrencyLimiter := httpmw.ConcurrencyLimit(cfg.MaxConcurrency.Value(), "AI Gateway")
	rateLimiter := httpmw.RateLimitByAuthToken(int(cfg.RateLimit.Value()), aiBridgeRateLimitWindow)
	byokGuard := aiGatewayBYOKGuard(cfg)
	return func(next http.Handler) http.Handler {
		return concurrencyLimiter(rateLimiter(byokGuard(next)))
	}
}

func aiGatewayBYOKGuard(cfg codersdk.AIBridgeConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if agplaibridge.IsBYOK(r.Header) && !cfg.AllowBYOK.Value() {
				httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
					Message: "Bring Your Own Key (BYOK) mode is not enabled.",
					Detail:  "Contact your administrator to enable it with --ai-gateway-allow-byok.",
				})
				return
			}
			next.ServeHTTP(rw, r)
		})
	}
}

// aiBridgeListSessions returns AI Bridge sessions (aggregated interceptions).
//
// @Summary List AI Gateway sessions
// @Description Alias: also available at /api/v2/aibridge/sessions for backward compatibility.
// @ID list-ai-gateway-sessions
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param q query string false "Search query in the format `key:value`. Available keys are: initiator, provider, provider_name, model, client, session_id, started_after, started_before."
// @Param limit query int false "Page limit"
// @Param after_session_id query string false "Cursor pagination after session ID (cannot be used with offset)"
// @Param offset query int false "Offset pagination (cannot be used with after_session_id)"
// @Success 200 {object} codersdk.AIBridgeListSessionsResponse
// @Router /api/v2/ai-gateway/sessions [get]
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
			ProviderName:  filter.ProviderName,
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
			Message: "Internal error getting AI Gateway sessions.",
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
// @Summary Get AI Gateway session threads
// @Description Alias: also available at /api/v2/aibridge/sessions/{session_id} for backward compatibility.
// @ID get-ai-gateway-session-threads
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param session_id path string true "Session ID (client_session_id or interception UUID)"
// @Param after_id query string false "Thread pagination cursor (forward/older)"
// @Param before_id query string false "Thread pagination cursor (backward/newer)"
// @Param limit query int false "Number of threads per page (default 50)"
// @Success 200 {object} codersdk.AIBridgeSessionThreadsResponse
// @Router /api/v2/ai-gateway/sessions/{session_id} [get]
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
// @Summary List AI Gateway models
// @Description Alias: also available at /api/v2/aibridge/models for backward compatibility.
// @ID list-ai-gateway-models
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Success 200 {array} string
// @Router /api/v2/ai-gateway/models [get]
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
			Message:     "Invalid AI Gateway models search query.",
			Validations: errs,
		})
		return
	}

	models, err := api.Database.ListAIBridgeModels(ctx, filter)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Gateway models.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, models)
}

// aiBridgeListClients returns all AI Bridge clients a user can see.
//
// @Summary List AI Gateway clients
// @Description Alias: also available at /api/v2/aibridge/clients for backward compatibility.
// @ID list-ai-gateway-clients
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Success 200 {array} string
// @Router /api/v2/ai-gateway/clients [get]
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
			Message:     "Invalid AI Gateway clients search query.",
			Validations: errs,
		})
		return
	}

	clients, err := api.Database.ListAIBridgeClients(ctx, filter)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting AI Gateway clients.",
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

// @Summary Get group AI budget
// @ID get-group-ai-budget
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Success 200 {object} codersdk.GroupAIBudget
// @Router /api/v2/groups/{group}/ai/budget [get]
func (api *API) groupAIBudget(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)

	groupBudget, err := api.Database.GetGroupAIBudget(ctx, group.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "get group AI budget", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.GroupAIBudget(groupBudget))
}

// @Summary Upsert group AI budget
// @ID upsert-group-ai-budget
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Param request body codersdk.UpsertGroupAIBudgetRequest true "Upsert group AI budget request"
// @Success 200 {object} codersdk.GroupAIBudget
// @Router /api/v2/groups/{group}/ai/budget [put]
func (api *API) upsertGroupAIBudget(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableGroupAIBudget](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: group.OrganizationID,
		})
	)
	defer commitAudit()

	var req codersdk.UpsertGroupAIBudgetRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Capture the existing budget (if any) so the audit log records the
	// before-state. An absent row leaves aReq.Old as the zero value.
	oldBudget, err := api.Database.GetGroupAIBudget(ctx, group.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		api.Logger.Error(ctx, "fetch existing group AI budget for audit", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = oldBudget.Auditable(group.Name)

	newBudget, err := api.Database.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
		GroupID:          group.ID,
		SpendLimitMicros: req.SpendLimitMicros,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "upsert group AI budget", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = newBudget.Auditable(group.Name)

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.GroupAIBudget(newBudget))
}

// @Summary Delete group AI budget
// @ID delete-group-ai-budget
// @Security CoderSessionToken
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Success 204
// @Router /api/v2/groups/{group}/ai/budget [delete]
func (api *API) deleteGroupAIBudget(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableGroupAIBudget](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: group.OrganizationID,
		})
	)
	defer commitAudit()

	deleted, err := api.Database.DeleteGroupAIBudget(ctx, group.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "delete group AI budget", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = deleted.Auditable(group.Name)

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get user AI budget override
// @ID get-user-ai-budget-override
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, username, or me"
// @Success 200 {object} codersdk.UserAIBudgetOverride
// @Router /api/v2/users/{user}/ai/budget [get]
func (api *API) userAIBudgetOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	override, err := api.Database.GetUserAIBudgetOverride(ctx, user.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "get user AI budget override", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserAIBudgetOverride(override))
}

// @Summary Upsert user AI budget override
// @ID upsert-user-ai-budget-override
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.UpsertUserAIBudgetOverrideRequest true "Upsert user AI budget override request"
// @Success 200 {object} codersdk.UserAIBudgetOverride
// @Router /api/v2/users/{user}/ai/budget [put]
func (api *API) upsertUserAIBudgetOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	var req codersdk.UpsertUserAIBudgetOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Look up the new group first so a missing or forbidden group_id
	// returns 404. We also need the group for the audit log.
	newGroup, err := api.Database.GetGroupByID(ctx, req.GroupID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		api.Logger.Error(ctx, "get group for user AI budget override", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	auditor := api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.AuditableUserAIBudgetOverride](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: newGroup.OrganizationID,
	})
	defer commitAudit()

	// Capture the existing override (if any) so the audit log records the
	// before-state. An absent row leaves aReq.Old as the zero value.
	oldOverride, overrideErr := api.Database.GetUserAIBudgetOverride(ctx, user.ID)
	if overrideErr != nil && !errors.Is(overrideErr, sql.ErrNoRows) {
		api.Logger.Error(ctx, "fetch existing user AI budget override for audit", slog.Error(overrideErr))
		httpapi.InternalServerError(rw, overrideErr)
		return
	}
	var oldGroupName string
	if overrideErr == nil {
		// This lookup exists only to record the old group's name in the audit
		// diff. Use a system context so it does not add a read requirement on
		// the old group that the upsert itself does not impose.
		oldGroup, groupErr := api.Database.GetGroupByID(dbauthz.AsSystemRestricted(ctx), oldOverride.GroupID) //nolint:gocritic // see above
		if groupErr != nil {
			api.Logger.Error(ctx, "fetch old group for user AI budget override audit", slog.Error(groupErr))
			httpapi.InternalServerError(rw, groupErr)
			return
		}
		oldGroupName = oldGroup.Name
	}
	aReq.Old = oldOverride.Auditable(user.Username, oldGroupName)

	override, err := api.Database.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
		UserID:           user.ID,
		GroupID:          req.GroupID,
		SpendLimitMicros: req.SpendLimitMicros,
	})
	// A trigger enforces that the user must be a member of the attributed
	// group; it raises check_violation with this constraint name. Map
	// the violation to a structured 400.
	if database.IsCheckViolation(err, userAIBudgetOverridesMustBeGroupMemberConstraint) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "User is not a member of the referenced group.",
			Validations: []codersdk.ValidationError{{
				Field:  "group_id",
				Detail: "user must be a member of this group",
			}},
		})
		return
	}
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "upsert user AI budget override", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = override.Auditable(user.Username, newGroup.Name)

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserAIBudgetOverride(override))
}

// @Summary Delete user AI budget override
// @ID delete-user-ai-budget-override
// @Security CoderSessionToken
// @Tags Enterprise
// @Param user path string true "User ID, username, or me"
// @Success 204
// @Router /api/v2/users/{user}/ai/budget [delete]
func (api *API) deleteUserAIBudgetOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	// Fetch the existing override first for audit purposes.
	userOverride, err := api.Database.GetUserAIBudgetOverride(ctx, user.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "fetch user AI budget override for delete", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	group, err := api.Database.GetGroupByID(ctx, userOverride.GroupID)
	if err != nil {
		api.Logger.Error(ctx, "get group for user AI budget override delete audit", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	auditor := api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.AuditableUserAIBudgetOverride](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: group.OrganizationID,
	})
	defer commitAudit()

	_, err = api.Database.DeleteUserAIBudgetOverride(ctx, user.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "delete user AI budget override", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	// Populate the audit snapshot only after delete succeeds. Setting
	// it earlier would record a phantom entry if delete races a
	// concurrent delete and returns 404.
	aReq.Old = userOverride.Auditable(user.Username, group.Name)

	rw.WriteHeader(http.StatusNoContent)
}

// currentAIBudgetWindow returns the current AI budget period window based on
// the configured budget period.
func (api *API) currentAIBudgetWindow() (budget.PeriodWindow, error) {
	period := codersdk.NewAIBudgetPeriodFromString(api.DeploymentValues.AI.BridgeConfig.BudgetPeriod)
	return budget.CurrentPeriod(api.Clock.Now(), period)
}

// @Summary Get user AI spend
// @ID get-user-ai-spend
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, username, or me"
// @Success 200 {object} codersdk.UserAISpendStatus
// @Router /api/v2/users/{user}/ai/spend [get]
func (api *API) userAISpendStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	logger := api.Logger.With(slog.F("user_id", user.ID))

	periodWindow, err := api.currentAIBudgetWindow()
	if err != nil {
		logger.Error(ctx, "failed to compute AI budget period", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	logger = logger.With(
		slog.F("period_start", periodWindow.Start),
		slog.F("period_end", periodWindow.End),
	)

	policy := codersdk.NewAIBudgetPolicyFromString(api.DeploymentValues.AI.BridgeConfig.BudgetPolicy)
	effectiveBudget, ok, err := budget.ResolveUserAIBudget(ctx, api.Database, user.ID, policy)
	if err != nil {
		logger.Error(ctx, "failed to resolve user AI budget", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.UserAISpendStatus{
		UserAIBudgetSummary: codersdk.UserAIBudgetSummary{
			UserID: user.ID,
		},
		AISpendPeriodWindow: codersdk.AISpendPeriodWindow{
			PeriodStart: periodWindow.Start,
			PeriodEnd:   periodWindow.End,
		},
	}

	if ok {
		resp.EffectiveGroupID = &effectiveBudget.GroupID
		resp.SpendLimitMicros = &effectiveBudget.SpendLimitMicros
		resp.LimitSource = &effectiveBudget.Source
		logger = logger.With(slog.F("effective_group_id", effectiveBudget.GroupID))

		spend, err := api.Database.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
			UserID:           user.ID,
			EffectiveGroupID: effectiveBudget.GroupID,
			PeriodStart:      periodWindow.Start,
		})
		if err != nil {
			logger.Error(ctx, "failed to get user AI spend", slog.Error(err))
			httpapi.InternalServerError(rw, err)
			return
		}
		resp.CurrentSpendMicros = spend.SpendMicros
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get organization groups AI spend
// @Description Returns AI spend limits and aggregate spend for the requested groups.
// @Description A maximum of 100 group IDs may be requested per call, and requests with more are rejected, so callers are expected to batch across multiple requests.
// @Description Unknown or unreadable group IDs are silently omitted.
// @ID get-organization-groups-ai-spend
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param group_ids query string true "Comma-separated list of group IDs (maximum 100)"
// @Success 200 {object} codersdk.OrganizationGroupsAISpend
// @Router /api/v2/organizations/{organization}/groups/ai/spend [get]
func (api *API) organizationGroupsAISpend(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	logger := api.Logger.With(slog.F("organization_id", org.ID))

	parser := httpapi.NewQueryParamParser()
	parser.RequiredNotEmpty("group_ids")
	groupIDs := parser.UUIDs(r.URL.Query(), nil, "group_ids")
	parser.ErrorExcessParams(r.URL.Query())
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if len(groupIDs) > maxOrganizationGroupsAISpendGroupIDs {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(
				"group_ids has %d entries, maximum is %d.",
				len(groupIDs), maxOrganizationGroupsAISpendGroupIDs,
			),
		})
		return
	}

	periodWindow, err := api.currentAIBudgetWindow()
	if err != nil {
		logger.Error(ctx, "failed to compute AI budget period", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	logger = logger.With(
		slog.F("period_start", periodWindow.Start),
		slog.F("period_end", periodWindow.End),
	)

	rows, err := api.Database.GetOrganizationGroupsAISpend(ctx, database.GetOrganizationGroupsAISpendParams{
		OrganizationID: org.ID,
		GroupIds:       groupIDs,
		PeriodStart:    periodWindow.Start,
	})
	if err != nil {
		logger.Error(ctx, "failed to get organization groups AI spend", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.OrganizationGroupsAISpend{
		AISpendPeriodWindow: codersdk.AISpendPeriodWindow{
			PeriodStart: periodWindow.Start,
			PeriodEnd:   periodWindow.End,
		},
		Groups: make([]codersdk.OrganizationGroupAISpend, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Groups = append(resp.Groups, db2sdk.OrganizationGroupAISpend(row))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get group members AI spend by organization
// @Description Returns aggregate AI spend attributed to the group per requested user.
// @Description A maximum of 100 user IDs may be requested per call, and requests with more are rejected, so callers are expected to batch across multiple requests.
// @Description User IDs that are not members of the group, or that the caller has no read access to, are silently omitted.
// @ID get-group-members-ai-spend-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param groupName path string true "Group name"
// @Param user_ids query string true "Comma-separated list of user IDs (maximum 100)"
// @Success 200 {object} codersdk.GroupMembersAISpend
// @Router /api/v2/organizations/{organization}/groups/{groupName}/members/ai/spend [get]
func (api *API) groupMembersAISpendByOrganization(rw http.ResponseWriter, r *http.Request) {
	api.groupMembersAISpend(rw, r)
}

// @Summary Get group members AI spend
// @Description Returns aggregate AI spend attributed to the group per requested user.
// @Description A maximum of 100 user IDs may be requested per call, and requests with more are rejected, so callers are expected to batch across multiple requests.
// @Description User IDs that are not members of the group, or that the caller has no read access to, are silently omitted.
// @ID get-group-members-ai-spend
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Param user_ids query string true "Comma-separated list of user IDs (maximum 100)"
// @Success 200 {object} codersdk.GroupMembersAISpend
// @Router /api/v2/groups/{group}/members/ai/spend [get]
func (api *API) groupMembersAISpend(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)
	logger := api.Logger.With(slog.F("group_id", group.ID))

	parser := httpapi.NewQueryParamParser()
	parser.RequiredNotEmpty("user_ids")
	userIDs := parser.UUIDs(r.URL.Query(), nil, "user_ids")
	parser.ErrorExcessParams(r.URL.Query())
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if len(userIDs) > maxGroupMembersAISpendUserIDs {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(
				"user_ids has %d entries, maximum is %d.",
				len(userIDs), maxGroupMembersAISpendUserIDs,
			),
		})
		return
	}

	periodWindow, err := api.currentAIBudgetWindow()
	if err != nil {
		logger.Error(ctx, "failed to compute AI budget period", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	logger = logger.With(
		slog.F("period_start", periodWindow.Start),
		slog.F("period_end", periodWindow.End),
	)

	rows, err := api.Database.GetGroupMembersAISpend(ctx, database.GetGroupMembersAISpendParams{
		GroupID:     group.ID,
		UserIds:     userIDs,
		PeriodStart: periodWindow.Start,
	})
	if err != nil {
		logger.Error(ctx, "failed to get group members AI spend", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.GroupMembersAISpend{
		AISpendPeriodWindow: codersdk.AISpendPeriodWindow{
			PeriodStart: periodWindow.Start,
			PeriodEnd:   periodWindow.End,
		},
		Members: make([]codersdk.GroupMemberAISpend, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Members = append(resp.Members, db2sdk.GroupMemberAISpend(row))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
