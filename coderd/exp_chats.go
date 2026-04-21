package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/chatfiles"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/xjson"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/gitsync"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const (
	chatStreamBatchSize = 256

	chatContextLimitModelConfigKey                = "context_limit"
	chatContextCompressionThresholdModelConfigKey = "context_compression_threshold"
	defaultChatContextCompressionThreshold        = int32(70)
	minChatContextCompressionThreshold            = int32(0)
	maxChatContextCompressionThreshold            = int32(100)
	maxSystemPromptLenBytes                       = 131072 // 128 KiB
)

// chatGitRef holds the branch, remote origin, and optional chat
// ID reported by the workspace agent during a git operation.
type chatGitRef struct {
	Branch       string
	RemoteOrigin string
	ChatID       uuid.UUID
}

type chatRepositoryRef struct {
	Provider     string
	RemoteOrigin string
	Branch       string
	Owner        string
	Repo         string
}

type chatDiffReference struct {
	PullRequestURL string
	RepositoryRef  *chatRepositoryRef
}

func writeChatUsageLimitExceeded(
	ctx context.Context,
	rw http.ResponseWriter,
	limitErr *chatd.UsageLimitExceededError,
) {
	httpapi.Write(ctx, rw, http.StatusConflict, codersdk.ChatUsageLimitExceededResponse{
		Response: codersdk.Response{
			Message: "Chat usage limit exceeded.",
		},
		SpentMicros: limitErr.ConsumedMicros,
		LimitMicros: limitErr.LimitMicros,
		ResetsAt:    limitErr.PeriodEnd,
	})
}

func maybeWriteLimitErr(ctx context.Context, rw http.ResponseWriter, err error) bool {
	var limitErr *chatd.UsageLimitExceededError
	if errors.As(err, &limitErr) {
		writeChatUsageLimitExceeded(ctx, rw, limitErr)
		return true
	}
	return false
}

func publishChatTitleChange(logger slog.Logger, ps dbpubsub.Pubsub, chat database.Chat) {
	if ps == nil {
		return
	}
	event := codersdk.ChatWatchEvent{
		Kind: codersdk.ChatWatchEventKindTitleChange,
		Chat: db2sdk.Chat(chat, nil, nil),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		logger.Error(context.Background(), "failed to marshal chat title change event",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if err := ps.Publish(pubsub.ChatWatchEventChannel(chat.OwnerID), payload); err != nil {
		logger.Error(context.Background(), "failed to publish chat title change event",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
	}
}

func publishChatConfigEvent(logger slog.Logger, ps dbpubsub.Pubsub, kind pubsub.ChatConfigEventKind, entityID uuid.UUID) {
	payload, err := json.Marshal(pubsub.ChatConfigEvent{
		Kind:     kind,
		EntityID: entityID,
	})
	if err != nil {
		logger.Error(context.Background(), "failed to marshal chat config event",
			slog.F("kind", kind),
			slog.F("entity_id", entityID),
			slog.Error(err),
		)
		return
	}
	if err := ps.Publish(pubsub.ChatConfigEventChannel, payload); err != nil {
		logger.Error(context.Background(), "failed to publish chat config event",
			slog.F("kind", kind),
			slog.F("entity_id", entityID),
			slog.Error(err),
		)
	}
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) watchChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	logger := api.Logger.Named("chat_watcher")

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat watch stream.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	_ = conn.CloseRead(context.Background())

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close()

	go httpapi.HeartbeatClose(ctx, logger, cancel, conn)

	// The encoder is only written from the SubscribeWithErr callback,
	// which delivers serially per subscription. Do not add a second
	// write path without introducing synchronization.
	encoder := json.NewEncoder(wsNetConn)

	cancelSubscribe, err := api.Pubsub.SubscribeWithErr(pubsub.ChatWatchEventChannel(apiKey.UserID),
		pubsub.HandleChatWatchEvent(
			func(ctx context.Context, payload codersdk.ChatWatchEvent, err error) {
				if err != nil {
					logger.Error(ctx, "chat watch event subscription error", slog.Error(err))
					return
				}
				if err := encoder.Encode(payload); err != nil {
					logger.Debug(ctx, "failed to send chat watch event", slog.Error(err))
					cancel()
					return
				}
			},
		))
	if err != nil {
		logger.Error(ctx, "failed to subscribe to chat watch events", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, "Failed to subscribe to chat events.")
		return
	}
	defer cancelSubscribe()

	<-ctx.Done()
}

// EXPERIMENTAL: chatsByWorkspace returns a mapping of workspace ID to
// the latest non-archived chat ID for each requested workspace.
// The query returns all matching chats and RBAC post-filters them;
// the handler then picks the latest per workspace in Go. This avoids
// the DISTINCT ON + post-filter bug where the sole candidate is
// silently dropped when the caller can't read it.
//
// TODO:
//  1. move aggregation to a SQL view with proper in-query authz so we
//     can return a single row per workspace without this two-pass approach.
//  2. Restore the below router annotation and un-skip docs gen
//     <at>Router /experimental/chats/by-workspace [post]
//
// @Summary Get latest chats by workspace IDs
// @ID get-latest-chats-by-workspace-ids
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Success 200
// @x-apidocgen {"skip": true}
func (api *API) chatsByWorkspace(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idsParam := r.URL.Query().Get("workspace_ids")
	if idsParam == "" {
		httpapi.Write(ctx, rw, http.StatusOK, map[uuid.UUID]uuid.UUID{})
		return
	}

	raw := strings.Split(idsParam, ",")

	// maxWorkspaceIDs is coupled to DEFAULT_RECORDS_PER_PAGE (25) in
	// site/src/components/PaginationWidget/utils.ts.
	// If the page size changes, this limit should too.
	const maxWorkspaceIDs = 25
	if len(raw) > maxWorkspaceIDs {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Too many workspace IDs, maximum is %d.", maxWorkspaceIDs),
		})
		return
	}

	workspaceIDs := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid workspace ID %q: %s", s, err),
			})
			return
		}
		workspaceIDs = append(workspaceIDs, id)
	}

	chats, err := api.Database.GetChatsByWorkspaceIDs(ctx, workspaceIDs)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chats by workspace.",
			Detail:  err.Error(),
		})
		return
	}

	// The SQL orders by (workspace_id, updated_at DESC), so the first
	// chat seen per workspace after RBAC filtering is the latest
	// readable one.
	result := make(map[uuid.UUID]uuid.UUID, len(chats))
	for _, chat := range chats {
		if chat.WorkspaceID.Valid {
			if _, exists := result[chat.WorkspaceID.UUID]; !exists {
				result[chat.WorkspaceID.UUID] = chat.ID
			}
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) listChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	paginationParams, ok := ParsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	searchParams, errs := searchquery.Chats(queryStr)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid chat search query.",
			Validations: errs,
		})
		return
	}

	var labelFilter pqtype.NullRawMessage
	if labelParams := r.URL.Query()["label"]; len(labelParams) > 0 {
		labelMap := make(map[string]string, len(labelParams))
		for _, lp := range labelParams {
			key, value, ok := strings.Cut(lp, ":")
			if !ok || key == "" || value == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Invalid label filter: %q (expected format key:value, both must be non-empty)", lp),
				})
				return
			}
			labelMap[key] = value
		}
		labelsJSON, err := json.Marshal(labelMap)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to marshal label filter.",
				Detail:  err.Error(),
			})
			return
		}
		labelFilter = pqtype.NullRawMessage{
			RawMessage: labelsJSON,
			Valid:      true,
		}
	}

	params := database.GetChatsParams{
		OwnerID:     apiKey.UserID,
		Archived:    searchParams.Archived,
		AfterID:     paginationParams.AfterID,
		LabelFilter: labelFilter,
		// #nosec G115 - Pagination offsets are small and fit in int32
		OffsetOpt: int32(paginationParams.Offset),
		// #nosec G115 - Pagination limits are small and fit in int32
		LimitOpt: int32(paginationParams.Limit),
	}

	chatRows, err := api.Database.GetChats(ctx, params)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chats.",
			Detail:  err.Error(),
		})
		return
	}

	// Collect root chat IDs so we can fetch their children.
	rootIDs := make([]uuid.UUID, len(chatRows))
	for i, row := range chatRows {
		rootIDs[i] = row.Chat.ID
	}

	// Embed children matching the caller's archive filter so
	// sidebar views don't surface state-mismatched rows.
	var childRows []database.GetChildChatsByParentIDsRow
	if len(rootIDs) > 0 {
		childRows, err = api.Database.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
			ParentIds: rootIDs,
			Archived:  searchParams.Archived,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to list child chats.",
				Detail:  err.Error(),
			})
			return
		}
	}

	// Collect all chat objects (root + child) for diff status lookup.
	allChats := make([]database.Chat, 0, len(chatRows)+len(childRows))
	for _, row := range chatRows {
		allChats = append(allChats, row.Chat)
	}
	for _, row := range childRows {
		allChats = append(allChats, row.Chat)
	}

	diffStatusesByChatID, err := api.getChatDiffStatusesByChatID(ctx, allChats)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chats.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatRowsWithChildren(chatRows, childRows, diffStatusesByChatID))
}

func (api *API) getChatDiffStatusesByChatID(
	ctx context.Context,
	chats []database.Chat,
) (map[uuid.UUID]database.ChatDiffStatus, error) {
	if len(chats) == 0 {
		return map[uuid.UUID]database.ChatDiffStatus{}, nil
	}

	chatIDs := make([]uuid.UUID, 0, len(chats))
	for _, chat := range chats {
		chatIDs = append(chatIDs, chat.ID)
	}

	statuses, err := api.Database.GetChatDiffStatusesByChatIDs(ctx, chatIDs)
	if err != nil {
		return nil, xerrors.Errorf("get chat diff statuses: %w", err)
	}

	statusesByChatID := make(map[uuid.UUID]database.ChatDiffStatus, len(statuses))
	for _, status := range statuses {
		statusesByChatID[status.ChatID] = status
	}
	return statusesByChatID, nil
}

func planModeToNullChatPlanMode(mode codersdk.ChatPlanMode) database.NullChatPlanMode {
	if mode == "" {
		return database.NullChatPlanMode{}
	}
	return database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanMode(mode),
		Valid:        true,
	}
}

func validateChatPlanMode(mode codersdk.ChatPlanMode) bool {
	switch mode {
	case "", codersdk.ChatPlanModePlan:
		return true
	default:
		return false
	}
}

func parseChatExploreModelOverride(raw string) (*uuid.UUID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		//nolint:nilnil // Empty site-config value means the override is unset.
		return nil, nil
	}
	modelConfigID, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, xerrors.Errorf("parse explore model override: %w", err)
	}
	return &modelConfigID, nil
}

func formatChatExploreModelOverride(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func validateChatExploreModelOverrideID(
	ctx context.Context,
	db database.Store,
	id *uuid.UUID,
) (int, *codersdk.Response) {
	if id == nil {
		return 0, nil
	}
	if *id == uuid.Nil {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id.",
		}
	}
	//nolint:gocritic // Validation lookup uses system context to check model
	// availability independently of the caller's read permissions.
	_, err := db.GetEnabledChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), *id)
	if err == nil {
		return 0, nil
	}
	if xerrors.Is(err, sql.ErrNoRows) {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id.",
		}
	}
	return http.StatusInternalServerError, &codersdk.Response{
		Message: "Internal error validating model config override.",
		Detail:  err.Error(),
	}
}

func (api *API) getChatExploreModelOverrideConfig(
	ctx context.Context,
) (*uuid.UUID, bool, error) {
	raw, err := api.Database.GetChatExploreModelOverride(ctx)
	if err != nil {
		return nil, false, xerrors.Errorf("get explore model override: %w", err)
	}
	id, err := parseChatExploreModelOverride(raw)
	if err != nil {
		// Degrade malformed values to unset so the admin settings page
		// remains accessible and the bad value can be cleared.
		api.Logger.Warn(ctx, "malformed explore model override in site config, treating as unset",
			slog.F("raw_value", raw),
			slog.Error(err),
		)
		return nil, true, nil
	}
	return id, false, nil
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) postChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use
	// from large dynamic tool schemas.
	r.Body = http.MaxBytesReader(rw, r.Body, int64(2*maxSystemPromptLenBytes))

	var req codersdk.CreateChatRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: req.OrganizationID,
	})
	defer commitAudit()

	// Validate organization membership.
	if req.OrganizationID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "organization_id is required.",
		})
		return
	}
	isMember, err := httpmw.UserAuthorization(ctx).HasOrganizationMembership(req.OrganizationID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to validate organization membership.",
			Detail:  xerrors.Errorf("check organization membership: %w", err).Error(),
		})
		return
	}
	if !isMember {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You are not a member of the specified organization.",
		})
		return
	}

	// Validate per-chat system prompt length.
	const maxSystemPromptLen = 10000
	if len(req.SystemPrompt) > maxSystemPromptLen {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "System prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("System prompt must be at most %d characters, got %d.", maxSystemPromptLen, len(req.SystemPrompt)),
		})
		return
	}
	contentBlocks, titleSource, fileIDs, inputError := createChatInputFromRequest(ctx, api.Database, req)
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *inputError)
		return
	}

	workspaceSelection, validationStatus, validationError := api.validateCreateChatWorkspaceSelection(ctx, r, req)
	if validationError != nil {
		httpapi.Write(ctx, rw, validationStatus, *validationError)
		return
	}

	title := chatTitleFromMessage(titleSource)

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	modelConfigID, modelConfigStatus, modelConfigError := api.resolveCreateChatModelConfigID(ctx, req)
	if modelConfigError != nil {
		httpapi.Write(ctx, rw, modelConfigStatus, *modelConfigError)
		return
	}

	if !validateChatPlanMode(req.PlanMode) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid plan_mode value.",
		})
		return
	}

	// Validate MCP server IDs exist.
	if len(req.MCPServerIDs) > 0 {
		//nolint:gocritic // Need to validate MCP server IDs exist.
		existingConfigs, err := api.Database.GetMCPServerConfigsByIDs(dbauthz.AsSystemRestricted(ctx), req.MCPServerIDs)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to validate MCP server IDs.",
				Detail:  err.Error(),
			})
			return
		}
		if len(existingConfigs) != len(req.MCPServerIDs) {
			found := make(map[uuid.UUID]struct{}, len(existingConfigs))
			for _, c := range existingConfigs {
				found[c.ID] = struct{}{}
			}
			var missing []string
			for _, id := range req.MCPServerIDs {
				if _, ok := found[id]; !ok {
					missing = append(missing, id.String())
				}
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "One or more MCP server IDs are invalid.",
				Detail:  fmt.Sprintf("Invalid IDs: %s", strings.Join(missing, ", ")),
			})
			return
		}
	}

	mcpServerIDs := req.MCPServerIDs
	if mcpServerIDs == nil {
		mcpServerIDs = []uuid.UUID{}
	}

	labels := req.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	if errs := httpapi.ValidateChatLabels(labels); len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid labels.",
			Validations: errs,
		})
		return
	}

	if len(req.UnsafeDynamicTools) > 250 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Too many dynamic tools.",
			Detail:  "Maximum 250 dynamic tools per chat.",
		})
		return
	}

	// Validate that dynamic tool names are non-empty and unique
	// within the list. Name collision with built-in tools is
	// checked at chatloop time when the full tool set is known.
	if len(req.UnsafeDynamicTools) > 0 {
		seenNames := make(map[string]struct{}, len(req.UnsafeDynamicTools))
		for _, dt := range req.UnsafeDynamicTools {
			if dt.Name == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Dynamic tool name must not be empty.",
				})
				return
			}
			if _, exists := seenNames[dt.Name]; exists {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Duplicate dynamic tool name.",
					Detail:  fmt.Sprintf("Tool %q appears more than once.", dt.Name),
				})
				return
			}
			seenNames[dt.Name] = struct{}{}
		}
	}

	var dynamicToolsJSON json.RawMessage
	if len(req.UnsafeDynamicTools) > 0 {
		var err error
		dynamicToolsJSON, err = json.Marshal(req.UnsafeDynamicTools)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to marshal dynamic tools.",
				Detail:  err.Error(),
			})
			return
		}
	}

	clientType := database.ChatClientTypeApi
	if req.ClientType != "" {
		clientType = database.ChatClientType(req.ClientType)
		if !clientType.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid client_type.",
				Detail:  fmt.Sprintf("got %q, want one of %v", req.ClientType, database.AllChatClientTypeValues()),
			})
			return
		}
	}

	chat, err := api.chatDaemon.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     req.OrganizationID,
		OwnerID:            apiKey.UserID,
		WorkspaceID:        workspaceSelection.WorkspaceID,
		Title:              title,
		ModelConfigID:      modelConfigID,
		PlanMode:           planModeToNullChatPlanMode(req.PlanMode),
		ClientType:         clientType,
		SystemPrompt:       req.SystemPrompt,
		InitialUserContent: contentBlocks,
		MCPServerIDs:       mcpServerIDs,
		Labels:             labels,
		DynamicTools:       dynamicToolsJSON,
		// IMPORTANT: users can only create root chats at the time of writing.
		ParentChatID: uuid.NullUUID{},
	})
	if err != nil {
		if maybeWriteLimitErr(ctx, rw, err) {
			return
		}
		if database.IsForeignKeyViolation(
			err,
			database.ForeignKeyChatsLastModelConfigID,
			database.ForeignKeyChatMessagesModelConfigID,
		) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config ID.",
				Detail:  err.Error(),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = chat

	if chat.ParentChatID.Valid {
		// Should not be possible. If we get here, something is very wrong. Bail.
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Developer error: ParentChatID got set somehow in api.postChats. This should never happen.",
		})
		return
	}

	// Link any user-uploaded files referenced in the initial
	// message to this newly created chat (best-effort; cap
	// enforced in SQL).
	unlinked, capExceeded := api.linkFilesToChat(ctx, chat.ID, fileIDs)

	// Re-read the chat so the response reflects the authoritative
	// database state (file links are deduped in the join table).
	chat, err = api.Database.GetChatByID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to read back chat after creation.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)
	response := db2sdk.Chat(chat, nil, chatFiles)
	if len(unlinked) > 0 {
		if capExceeded {
			response.Warnings = append(response.Warnings, fileLinkCapWarning(len(unlinked)))
		} else {
			response.Warnings = append(response.Warnings, fileLinkErrorWarning(len(unlinked)))
		}
	}
	httpapi.Write(ctx, rw, http.StatusCreated, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) listChatModels(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	//nolint:gocritic // System context required to read enabled chat models.
	systemCtx := dbauthz.AsSystemRestricted(ctx)

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	enabledProviders, err := api.Database.GetEnabledChatProviders(
		systemCtx,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load chat model configuration.",
			Detail:  err.Error(),
		})
		return
	}
	enabledModels, err := api.Database.GetEnabledChatModelConfigs(
		systemCtx,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load chat model configuration.",
			Detail:  err.Error(),
		})
		return
	}

	configuredProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(enabledProviders),
	)
	enabledProviderNames := make(map[string]struct{}, len(enabledProviders))
	for _, provider := range enabledProviders {
		configuredProviders = append(
			configuredProviders, chatprovider.ConfiguredProvider{
				ProviderID:                 provider.ID,
				Provider:                   provider.Provider,
				APIKey:                     provider.APIKey,
				BaseURL:                    provider.BaseUrl,
				CentralAPIKeyEnabled:       provider.CentralApiKeyEnabled,
				AllowUserAPIKey:            provider.AllowUserApiKey,
				AllowCentralAPIKeyFallback: provider.AllowCentralApiKeyFallback,
			},
		)
		normalizedProvider := chatprovider.NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		enabledProviderNames[normalizedProvider] = struct{}{}
	}
	configuredModels := make(
		[]chatprovider.ConfiguredModel, 0, len(enabledModels),
	)
	for _, model := range enabledModels {
		configuredModels = append(configuredModels, chatprovider.ConfiguredModel{
			Provider:    model.Provider,
			Model:       model.Model,
			DisplayName: model.DisplayName,
		})
	}

	userKeyRows, err := api.Database.GetUserChatProviderKeys(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load user chat provider keys.",
			Detail:  err.Error(),
		})
		return
	}
	userKeys := make([]chatprovider.UserProviderKey, 0, len(userKeyRows))
	for _, userKey := range userKeyRows {
		userKeys = append(userKeys, chatprovider.UserProviderKey{
			ChatProviderID: userKey.ChatProviderID,
			APIKey:         userKey.APIKey,
		})
	}

	_, providerAvailability := chatprovider.ResolveUserProviderKeys(
		ChatProviderAPIKeysFromDeploymentValues(api.DeploymentValues),
		configuredProviders,
		userKeys,
	)
	catalog := chatprovider.NewModelCatalog()
	var response codersdk.ChatModelsResponse
	if configured, ok := catalog.ListConfiguredModels(
		configuredProviders, configuredModels, providerAvailability, enabledProviderNames,
	); ok {
		response = configured
	} else {
		response = catalog.ListConfiguredProviderAvailability(
			providerAvailability,
			enabledProviderNames,
		)
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func (api *API) chatCostSummary(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Default date range: last 30 days.
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -30)

	qp := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	startDate := p.Time(qp, defaultStart, "start_date", time.RFC3339)
	endDate := p.Time(qp, now, "end_date", time.RFC3339)
	p.ErrorExcessParams(qp)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}

	targetUser := httpmw.UserParam(r)
	if targetUser.ID != apiKey.UserID && !api.Authorize(r, policy.ActionRead, rbac.ResourceChat.WithOwner(targetUser.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	summary, err := api.Database.GetChatCostSummary(ctx, database.GetChatCostSummaryParams{
		OwnerID:   targetUser.ID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	byModel, err := api.Database.GetChatCostPerModel(ctx, database.GetChatCostPerModelParams{
		OwnerID:   targetUser.ID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	byChat, err := api.Database.GetChatCostPerChat(ctx, database.GetChatCostPerChatParams{
		OwnerID:   targetUser.ID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	modelBreakdowns := make([]codersdk.ChatCostModelBreakdown, 0, len(byModel))
	for _, model := range byModel {
		modelBreakdowns = append(modelBreakdowns, convertChatCostModelBreakdown(model))
	}

	chatBreakdowns := make([]codersdk.ChatCostChatBreakdown, 0, len(byChat))
	for _, chat := range byChat {
		chatBreakdowns = append(chatBreakdowns, convertChatCostChatBreakdown(chat))
	}

	// TODO(CODAGT-161): pass real organization ID
	// when the HTTP endpoint supports org-scoped queries.
	usageStatus, err := chatd.ResolveUsageLimitStatus(ctx, api.Database, targetUser.ID, uuid.NullUUID{}, time.Now())
	if err != nil {
		api.Logger.Warn(ctx, "failed to resolve usage limit status", slog.Error(err))
	}

	response := codersdk.ChatCostSummary{
		StartDate:                startDate,
		EndDate:                  endDate,
		TotalCostMicros:          summary.TotalCostMicros,
		PricedMessageCount:       summary.PricedMessageCount,
		UnpricedMessageCount:     summary.UnpricedMessageCount,
		TotalInputTokens:         summary.TotalInputTokens,
		TotalOutputTokens:        summary.TotalOutputTokens,
		TotalCacheReadTokens:     summary.TotalCacheReadTokens,
		TotalCacheCreationTokens: summary.TotalCacheCreationTokens,
		TotalRuntimeMs:           summary.TotalRuntimeMs,
		ByModel:                  modelBreakdowns,
		ByChat:                   chatBreakdowns,
	}
	if usageStatus != nil {
		response.UsageLimit = usageStatus
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func (api *API) chatCostUsers(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceChat) {
		httpapi.Forbidden(rw)
		return
	}

	now := time.Now()
	defaultStart := now.AddDate(0, 0, -30)

	qp := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	startDate := p.Time(qp, defaultStart, "start_date", time.RFC3339)
	endDate := p.Time(qp, now, "end_date", time.RFC3339)
	username := strings.TrimSpace(p.String(qp, "", "username"))
	limit := p.Int(qp, 10, "limit")
	offset := p.Int(qp, 0, "offset")
	p.ErrorExcessParams(qp)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 || offset > math.MaxInt32 || limit > math.MaxInt32 {
		validations := make([]codersdk.ValidationError, 0, 2)
		if offset < 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  "offset",
				Detail: "Must be greater than or equal to 0.",
			})
		}
		if offset > math.MaxInt32 {
			validations = append(validations, codersdk.ValidationError{
				Field:  "offset",
				Detail: fmt.Sprintf("Must be less than or equal to %d.", math.MaxInt32),
			})
		}
		if limit > math.MaxInt32 {
			validations = append(validations, codersdk.ValidationError{
				Field:  "limit",
				Detail: fmt.Sprintf("Must be less than or equal to %d.", math.MaxInt32),
			})
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: validations,
		})
		return
	}

	users, err := api.Database.GetChatCostPerUser(ctx, database.GetChatCostPerUserParams{
		StartDate: startDate,
		EndDate:   endDate,
		Username:  username,
		// #nosec G115 - Pagination limits are validated to fit in int32 above.
		PageLimit: int32(limit),
		// #nosec G115 - Pagination offsets are validated to fit in int32 above.
		PageOffset: int32(offset),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rollups := make([]codersdk.ChatCostUserRollup, 0, len(users))
	count := int64(0)
	for _, user := range users {
		count = user.TotalCount
		rollups = append(rollups, convertChatCostUserRollup(user))
	}

	if len(users) == 0 && offset > 0 {
		countUsers, countErr := api.Database.GetChatCostPerUser(ctx, database.GetChatCostPerUserParams{
			StartDate:  startDate,
			EndDate:    endDate,
			Username:   username,
			PageLimit:  1,
			PageOffset: 0,
		})
		if countErr != nil {
			httpapi.InternalServerError(rw, countErr)
			return
		}
		if len(countUsers) > 0 {
			count = countUsers[0].TotalCount
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatCostUsersResponse{
		StartDate: startDate,
		EndDate:   endDate,
		Count:     count,
		Users:     rollups,
	})
}

// @Summary Get chat usage limit config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatUsageLimitConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	config, configErr := api.Database.GetChatUsageLimitConfig(ctx)
	if configErr != nil && !errors.Is(configErr, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat usage limit config.",
			Detail:  configErr.Error(),
		})
		return
	}

	overrideRows, err := api.Database.ListChatUsageLimitOverrides(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat usage limit overrides.",
			Detail:  err.Error(),
		})
		return
	}

	groupOverrides, err := api.Database.ListChatUsageLimitGroupOverrides(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list group usage limit overrides.",
			Detail:  err.Error(),
		})
		return
	}

	unpricedModelCount, err := api.Database.CountEnabledModelsWithoutPricing(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to count unpriced chat models.",
			Detail:  err.Error(),
		})
		return
	}

	response := codersdk.ChatUsageLimitConfigResponse{
		ChatUsageLimitConfig: codersdk.ChatUsageLimitConfig{},
		UnpricedModelCount:   unpricedModelCount,
		Overrides:            make([]codersdk.ChatUsageLimitOverride, 0, len(overrideRows)),
		GroupOverrides:       make([]codersdk.ChatUsageLimitGroupOverride, 0, len(groupOverrides)),
	}
	if configErr == nil {
		response.Period = codersdk.ChatUsageLimitPeriod(config.Period)
		response.UpdatedAt = config.UpdatedAt
		if config.Enabled {
			response.SpendLimitMicros = ptr.Ref(config.DefaultLimitMicros)
		}
	}

	for _, row := range overrideRows {
		response.Overrides = append(response.Overrides, codersdk.ChatUsageLimitOverride{
			UserID:           row.UserID,
			Username:         row.Username,
			Name:             row.Name,
			AvatarURL:        row.AvatarURL,
			SpendLimitMicros: nullInt64Ptr(row.SpendLimitMicros),
		})
	}

	for _, glo := range groupOverrides {
		response.GroupOverrides = append(response.GroupOverrides, codersdk.ChatUsageLimitGroupOverride{
			GroupID:          glo.GroupID,
			GroupName:        glo.GroupName,
			GroupDisplayName: glo.GroupDisplayName,
			GroupAvatarURL:   glo.GroupAvatarUrl,
			MemberCount:      glo.MemberCount,
			SpendLimitMicros: nullInt64Ptr(glo.SpendLimitMicros),
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Update chat usage limit config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) updateChatUsageLimitConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.ChatUsageLimitConfig
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	params := database.UpsertChatUsageLimitConfigParams{
		Enabled:            false,
		DefaultLimitMicros: 0,
		Period:             "",
	}
	if req.SpendLimitMicros == nil {
		if req.Period != "" && !req.Period.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid chat usage limit period.",
				Detail:  "Period must be one of: day, week, month.",
			})
			return
		}

		params.Enabled = false
		params.DefaultLimitMicros = 0
		params.Period = string(req.Period)
		if params.Period == "" {
			params.Period = string(codersdk.ChatUsageLimitPeriodMonth)
		}
	} else {
		if *req.SpendLimitMicros <= 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid chat usage limit spend limit.",
				Detail:  "Spend limit must be greater than 0.",
			})
			return
		}
		if !req.Period.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid chat usage limit period.",
				Detail:  "Period must be one of: day, week, month.",
			})
			return
		}

		params.Enabled = true
		params.DefaultLimitMicros = *req.SpendLimitMicros
		params.Period = string(req.Period)
	}

	config, err := api.Database.UpsertChatUsageLimitConfig(ctx, params)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat usage limit config.",
			Detail:  err.Error(),
		})
		return
	}

	response := codersdk.ChatUsageLimitConfig{
		Period:    codersdk.ChatUsageLimitPeriod(config.Period),
		UpdatedAt: config.UpdatedAt,
	}
	if config.Enabled {
		response.SpendLimitMicros = ptr.Ref(config.DefaultLimitMicros)
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Get my chat usage limit status
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// getMyChatUsageLimitStatus returns the current usage-limit status for the
// authenticated user. No additional RBAC check is required because the
// endpoint always operates on the requesting user's own data via
// httpmw.APIKey(r).UserID.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getMyChatUsageLimitStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// TODO(CODAGT-161): pass real organization ID
	// when the HTTP endpoint supports org-scoped queries.
	status, err := chatd.ResolveUsageLimitStatus(ctx, api.Database, httpmw.APIKey(r).UserID, uuid.NullUUID{}, time.Now())
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat usage limit status.",
			Detail:  err.Error(),
		})
		return
	}
	if status == nil {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatUsageLimitStatus{IsLimited: false})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, status)
}

// @Summary Upsert chat usage limit override
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) upsertChatUsageLimitOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	userID, ok := parseChatUsageLimitUserID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpsertChatUsageLimitOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.SpendLimitMicros <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat usage limit override.",
			Detail:  "Spend limit must be greater than 0.",
		})
		return
	}

	user, err := api.Database.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "User not found.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up chat usage limit user.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.UpsertChatUsageLimitUserOverride(ctx, database.UpsertChatUsageLimitUserOverrideParams{
		UserID:           userID,
		SpendLimitMicros: req.SpendLimitMicros,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upsert chat usage limit override.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatUsageLimitOverride{
		UserID:           user.ID,
		Username:         user.Username,
		Name:             user.Name,
		AvatarURL:        user.AvatarURL,
		SpendLimitMicros: nullInt64Ptr(sql.NullInt64{Int64: req.SpendLimitMicros, Valid: true}),
	})
}

// @Summary Delete chat usage limit override
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteChatUsageLimitOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	userID, ok := parseChatUsageLimitUserID(rw, r)
	if !ok {
		return
	}

	if _, err := api.Database.GetUserByID(ctx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeChatUsageLimitUserNotFound(ctx, rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up chat usage limit user.",
			Detail:  err.Error(),
		})
		return
	}
	if _, err := api.Database.GetChatUsageLimitUserOverride(ctx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeChatUsageLimitOverrideNotFound(ctx, rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up chat usage limit override.",
			Detail:  err.Error(),
		})
		return
	}
	if err := api.Database.DeleteChatUsageLimitUserOverride(ctx, userID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat usage limit override.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Upsert chat usage limit group override
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) upsertChatUsageLimitGroupOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	groupIDStr := chi.URLParam(r, "group")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid group ID.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.UpdateChatUsageLimitGroupOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.SpendLimitMicros <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat usage limit group override.",
			Detail:  "Spend limit (in microdollars) must be greater than 0.",
		})
		return
	}

	group, err := api.Database.GetGroupByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Group not found.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up group details.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.UpsertChatUsageLimitGroupOverride(ctx, database.UpsertChatUsageLimitGroupOverrideParams{
		GroupID:          groupID,
		SpendLimitMicros: req.SpendLimitMicros,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upsert group usage limit override.",
			Detail:  err.Error(),
		})
		return
	}

	memberCount, err := api.Database.GetGroupMembersCountByGroupID(ctx, database.GetGroupMembersCountByGroupIDParams{
		GroupID:       groupID,
		IncludeSystem: false,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeChatUsageLimitGroupNotFound(ctx, rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch group member count.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatUsageLimitGroupOverride{
		GroupID:          group.ID,
		GroupName:        group.Name,
		GroupDisplayName: group.DisplayName,
		GroupAvatarURL:   group.AvatarURL,
		MemberCount:      memberCount,
		SpendLimitMicros: nullInt64Ptr(sql.NullInt64{Int64: req.SpendLimitMicros, Valid: true}),
	})
}

// @Summary Delete chat usage limit group override
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteChatUsageLimitGroupOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	groupIDStr := chi.URLParam(r, "group")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid group ID.",
			Detail:  err.Error(),
		})
		return
	}

	if _, err := api.Database.GetGroupByID(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeChatUsageLimitGroupNotFound(ctx, rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up group details.",
			Detail:  err.Error(),
		})
		return
	}
	if _, err := api.Database.GetChatUsageLimitGroupOverride(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeChatUsageLimitGroupOverrideNotFound(ctx, rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up group usage limit override.",
			Detail:  err.Error(),
		})
		return
	}
	if err := api.Database.DeleteChatUsageLimitGroupOverride(ctx, groupID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete group usage limit override.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	// Use the cached diff status from the database rather than
	// resolving it inline. Inline resolution calls out to the
	// git provider API (e.g. GitHub) on every request which
	// blocks the response for 200-800ms. The background gitsync
	// worker keeps the cached status fresh.
	var diffStatus *database.ChatDiffStatus
	status, err := api.Database.GetChatDiffStatusByChatID(ctx, chat.ID)
	switch {
	case err == nil:
		diffStatus = &status
	case !xerrors.Is(err, sql.ErrNoRows):
		api.Logger.Error(ctx, "failed to get cached chat diff status",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
	}

	// Hydrate file metadata for all files linked to this chat.
	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)

	sdkChat := db2sdk.Chat(chat, diffStatus, chatFiles)

	// For root chats, embed children so callers get a complete
	// tree in a single response.
	if !chat.ParentChatID.Valid {
		// Embed children matching the parent's archive state.
		childRows, err := api.Database.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
			ParentIds: []uuid.UUID{chat.ID},
			Archived:  sql.NullBool{Bool: chat.Archived, Valid: true},
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to fetch child chats.",
				Detail:  err.Error(),
			})
			return
		}
		// Look up diff statuses for children.
		childChats := make([]database.Chat, len(childRows))
		for i, row := range childRows {
			childChats[i] = row.Chat
		}
		childDiffStatuses, err := api.getChatDiffStatusesByChatID(ctx, childChats)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to fetch child chat diff statuses.",
				Detail:  err.Error(),
			})
			return
		}

		sdkChat.Children = db2sdk.ChildChatRows(childRows, childDiffStatuses)
	}

	httpapi.Write(ctx, rw, http.StatusOK, sdkChat)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	// Parse optional cursor-based pagination parameters.
	queryParams := r.URL.Query()
	parser := httpapi.NewQueryParamParser()
	beforeID := parser.PositiveInt64(queryParams, 0, "before_id")
	limit := parser.PositiveInt32(queryParams, 50, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if limit < 1 || limit > 200 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid limit parameter (1-200).",
		})
		return
	}
	// Fetch limit+1 rows to detect whether more pages exist.
	messages, err := api.Database.GetChatMessagesByChatIDDescPaginated(ctx, database.GetChatMessagesByChatIDDescPaginatedParams{
		ChatID:   chatID,
		BeforeID: beforeID,
		LimitVal: limit + 1,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages.",
			Detail:  err.Error(),
		})
		return
	}

	hasMore := len(messages) > int(limit)
	if hasMore {
		messages = messages[:limit]
	}

	// Only fetch queued messages on the first page (no cursor).
	var queuedMessages []database.ChatQueuedMessage
	if beforeID == 0 {
		queuedMessages, err = api.Database.GetChatQueuedMessages(ctx, chatID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get queued messages.",
				Detail:  err.Error(),
			})
			return
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatMessagesResponse{
		Messages:       convertChatMessages(messages),
		QueuedMessages: convertChatQueuedMessages(queuedMessages),
		HasMore:        hasMore,
	})
}

// authorizeChatWorkspaceExec enforces the workspace-level permissions
// shared by the chat stream endpoints that proxy a live websocket into
// the workspace agent (currently /stream/git and /stream/desktop).
//
// The chat row only authorizes the chat owner, so callers also need
// exec-level access (ApplicationConnect or SSH) to the bound workspace.
// The chat owner's workspace permissions may have been revoked after
// the chat was bound; skipping this check enabled CODAGT-184.
//
// On any failure the response is written and ok=false is returned.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) authorizeChatWorkspaceExec(
	rw http.ResponseWriter,
	r *http.Request,
	chat database.Chat,
	noWorkspaceMessage string,
) (database.Workspace, bool) {
	ctx := r.Context()

	if !chat.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: noWorkspaceMessage,
		})
		return database.Workspace{}, false
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat workspace not found.",
		})
		return database.Workspace{}, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat workspace.",
			Detail:  err.Error(),
		})
		return database.Workspace{}, false
	}

	if !api.Authorize(r, policy.ActionApplicationConnect, workspace) &&
		!api.Authorize(r, policy.ActionSSH, workspace) {
		httpapi.Forbidden(rw)
		return database.Workspace{}, false
	}

	return workspace, true
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) watchChatGit(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		chat   = httpmw.ChatParam(r)
		logger = api.Logger.Named("chat_git_watcher").With(slog.F("chat_id", chat.ID))
	)

	if _, ok := api.authorizeChatWorkspaceExec(rw, r, chat, "Chat has no workspace to watch."); !ok {
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agents.",
			Detail:  err.Error(),
		})
		return
	}
	if len(agents) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat workspace has no agents.",
		})
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		agents[0],
		nil,
		nil,
		nil,
		api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()

	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, agents[0].ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error dialing workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	agentStream, err := agentConn.WatchGit(ctx, logger, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error watching agent's git state.",
			Detail:  err.Error(),
		})
		return
	}
	defer agentStream.Close(websocket.StatusGoingAway)

	clientConn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	if err != nil {
		logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}

	clientStream := wsjson.NewStream[
		codersdk.WorkspaceAgentGitClientMessage,
		codersdk.WorkspaceAgentGitServerMessage,
	](clientConn, websocket.MessageText, websocket.MessageText, logger)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go httpapi.HeartbeatClose(ctx, logger, cancel, clientConn)

	// Proxy agent → client.
	agentCh := agentStream.Chan()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-api.ctx.Done():
				return
			case <-ctx.Done():
				return
			case msg, ok := <-agentCh:
				if !ok {
					cancel()
					return
				}
				if err := clientStream.Send(msg); err != nil {
					logger.Debug(ctx, "failed to forward agent message to client", slog.Error(err))
					cancel()
					return
				}
			}
		}
	}()

	// Proxy client → agent.
	clientCh := clientStream.Chan()
proxyLoop:
	for {
		select {
		case <-api.ctx.Done():
			break proxyLoop
		case <-ctx.Done():
			break proxyLoop
		case msg, ok := <-clientCh:
			if !ok {
				break proxyLoop
			}
			if err := agentStream.Send(msg); err != nil {
				logger.Debug(ctx, "failed to forward client message to agent", slog.Error(err))
				break proxyLoop
			}
		}
	}

	cancel()
	wg.Wait()
	_ = clientStream.Close(websocket.StatusGoingAway)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) watchChatDesktop(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		chat   = httpmw.ChatParam(r)
		logger = api.Logger.Named("chat_desktop").With(slog.F("chat_id", chat.ID))
	)

	if _, ok := api.authorizeChatWorkspaceExec(rw, r, chat, "Chat has no workspace."); !ok {
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agents.",
			Detail:  err.Error(),
		})
		return
	}
	if len(agents) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat workspace has no agents.",
		})
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		agents[0],
		nil,
		nil,
		nil,
		api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, must be connected.", apiAgent.Status),
		})
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()

	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, agents[0].ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to dial workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	desktopConn, err := agentConn.ConnectDesktopVNC(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to connect to agent desktop.",
			Detail:  err.Error(),
		})
		return
	}
	defer desktopConn.Close()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}

	// No read limit — RFB framebuffer updates can be large.
	conn.SetReadLimit(-1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx, wsNetConn := workspaceapps.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	go httpapi.HeartbeatClose(ctx, logger, cancel, conn)

	agentssh.Bicopy(ctx, wsNetConn, desktopConn)
	logger.Debug(ctx, "desktop Bicopy finished")
}

func (api *API) applyChatTitleUpdate(
	ctx context.Context,
	rw http.ResponseWriter,
	chat database.Chat,
	rawTitle string,
) (database.Chat, bool) {
	trimmedTitle := strings.TrimSpace(rawTitle)
	if trimmedTitle == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Title cannot be empty.",
		})
		return chat, true
	}
	const maxChatTitleRunes = 200
	if utf8.RuneCountInString(trimmedTitle) > maxChatTitleRunes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Title must be at most %d characters.", maxChatTitleRunes),
		})
		return chat, true
	}
	if trimmedTitle == chat.Title {
		return chat, false
	}

	var (
		updatedChat database.Chat
		wrote       bool
		err         error
	)
	if api.chatDaemon != nil {
		updatedChat, wrote, err = api.chatDaemon.RenameChatTitle(ctx, chat, trimmedTitle)
	} else {
		err = api.Database.InTx(func(tx database.Store) error {
			currentChat, txErr := tx.GetChatByID(ctx, chat.ID)
			if txErr != nil {
				return txErr
			}
			if trimmedTitle == currentChat.Title {
				updatedChat = currentChat
				wrote = false
				return nil
			}
			updatedChat, txErr = tx.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
				ID:    chat.ID,
				Title: trimmedTitle,
			})
			if txErr != nil {
				return txErr
			}
			wrote = true
			return nil
		}, nil)
	}
	if err != nil {
		if errors.Is(err, chatd.ErrManualTitleRegenerationInProgress) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Title regeneration already in progress for this chat.",
			})
			return chat, true
		}
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return chat, true
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat title.",
			Detail:  err.Error(),
		})
		return chat, true
	}
	if wrote {
		if api.chatDaemon != nil {
			api.chatDaemon.PublishTitleChange(updatedChat)
		} else {
			publishChatTitleChange(api.Logger, api.Pubsub, updatedChat)
		}
	}
	return updatedChat, false
}

// patchChat updates a chat resource. Supports updating labels,
// workspace binding, archiving, pinning, and pinned-chat ordering.
func (api *API) patchChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()
	aReq.Old = chat
	aReq.UpdateOrganizationID(chat.OrganizationID)

	var req codersdk.UpdateChatRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var planModeUpdate *database.NullChatPlanMode
	if req.PlanMode != nil {
		if !validateChatPlanMode(*req.PlanMode) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid plan_mode value.",
			})
			return
		}
		resolvedPlanMode := planModeToNullChatPlanMode(*req.PlanMode)
		planModeUpdate = &resolvedPlanMode
	}

	if req.Title != nil {
		updatedChat, handled := api.applyChatTitleUpdate(ctx, rw, chat, *req.Title)
		if handled {
			return
		}
		chat = updatedChat
	}
	if req.Labels != nil {
		if errs := httpapi.ValidateChatLabels(*req.Labels); len(errs) > 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid labels.",
				Validations: errs,
			})
			return
		}
		labelsJSON, err := json.Marshal(*req.Labels)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to marshal labels.",
				Detail:  err.Error(),
			})
			return
		}
		updatedChat, err := api.Database.UpdateChatLabelsByID(ctx, database.UpdateChatLabelsByIDParams{
			ID:     chat.ID,
			Labels: labelsJSON,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat labels.",
				Detail:  err.Error(),
			})
			return
		}
		chat = updatedChat
	}

	if req.Archived != nil {
		archived := *req.Archived
		if archived == chat.Archived {
			state := "archived"
			if !archived {
				state = "not archived"
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Chat is already %s.", state),
			})
			return
		}

		// Archive invariant is one-way: parent archived implies
		// child archived. Parent archive/unarchive cascade via
		// root_chat_id; individual child archive is permitted;
		// child unarchive while the parent is archived is rejected
		// (enforced atomically in chatd.Server.UnarchiveChat).
		if chat.ParentChatID.Valid && !archived {
			if done := api.writeChildUnarchiveGuard(ctx, rw, chat); done {
				return
			}
		}
		var err error
		// Use chatDaemon when available so it can interrupt active
		// processing before broadcasting archive state. Fall back to
		// direct DB when no daemon is running.
		if archived {
			if api.chatDaemon != nil {
				err = api.chatDaemon.ArchiveChat(ctx, chat)
			} else {
				_, err = api.Database.ArchiveChatByID(ctx, chat.ID)
			}
		} else {
			if api.chatDaemon != nil {
				err = api.chatDaemon.UnarchiveChat(ctx, chat)
			} else {
				_, err = api.Database.UnarchiveChatByID(ctx, chat.ID)
			}
		}
		if err != nil {
			if errors.Is(err, chatd.ErrChildUnarchiveParentArchived) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Cannot unarchive a child chat while its parent is archived. Unarchive the parent chat to cascade.",
				})
				return
			}
			action := "archive"
			if !archived {
				action = "unarchive"
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Failed to %s chat.", action),
				Detail:  err.Error(),
			})
			return
		}
	}

	if req.PinOrder != nil {
		pinOrder := *req.PinOrder
		if pinOrder < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Pin order must be non-negative.",
			})
			return
		}

		if pinOrder > 0 && chat.Archived {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot pin an archived chat.",
			})
			return
		}

		// The behavior depends on current pin state:
		// - pinOrder == 0: unpin.
		// - pinOrder > 0 && already pinned: reorder (shift
		//   neighbors, clamp to [1, count]).
		// - pinOrder > 0 && not pinned: append to end. The
		//   requested value is intentionally ignored; the
		//   SQL ORDER BY sorts pinned chats first so they
		//   appear on page 1 of the paginated sidebar.
		var err error
		errMsg := "Failed to pin chat."
		switch {
		case pinOrder == 0:
			errMsg = "Failed to unpin chat."
			err = api.Database.UnpinChatByID(ctx, chat.ID)
		case chat.PinOrder > 0:
			errMsg = "Failed to reorder pinned chat."
			err = api.Database.UpdateChatPinOrder(ctx, database.UpdateChatPinOrderParams{
				ID:       chat.ID,
				PinOrder: pinOrder,
			})
		default:
			err = api.Database.PinChatByID(ctx, chat.ID)
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: errMsg,
				Detail:  err.Error(),
			})
			return
		}
	}

	if req.WorkspaceID != nil {
		workspaceID := uuid.NullUUID{}
		workspace := database.Workspace{}
		if *req.WorkspaceID != uuid.Nil {
			var status int
			var resp *codersdk.Response
			workspaceID, workspace, status, resp = api.validateChatWorkspaceSelection(ctx, r, req.WorkspaceID)
			if resp != nil {
				httpapi.Write(ctx, rw, status, *resp)
				return
			}
			if workspace.OrganizationID != chat.OrganizationID {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Workspace does not belong to this chat's organization.",
				})
				return
			}
		}

		updatedChat, err := api.Database.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
			ID:          chat.ID,
			WorkspaceID: workspaceID,
			BuildID:     uuid.NullUUID{},
			AgentID:     uuid.NullUUID{},
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat workspace binding.",
				Detail:  err.Error(),
			})
			return
		}
		chat = updatedChat
	}

	if planModeUpdate != nil {
		updatedChat, err := api.Database.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{
			PlanMode: *planModeUpdate,
			ID:       chat.ID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat plan mode.",
				Detail:  err.Error(),
			})
			return
		}
		chat = updatedChat
	}

	if refreshed, err := api.Database.GetChatByID(ctx, chat.ID); err == nil {
		aReq.New = refreshed
	} else {
		aReq.New = chat // fallback
		api.Logger.Error(ctx, "failed to refresh chat for audit", slog.F("chat_id", chat.ID), slog.Error(err))
	}

	rw.WriteHeader(http.StatusNoContent)
}

// writeChildUnarchiveGuard returns a 400 early when a child unarchive
// request obviously races an archived parent. The durable invariant
// is enforced atomically in chatd.Server.UnarchiveChat; this guard
// just surfaces the error before we take any locks.
//
// Returns true when a response has been written.
func (api *API) writeChildUnarchiveGuard(
	ctx context.Context,
	rw http.ResponseWriter,
	chat database.Chat,
) bool {
	parent, err := api.Database.GetChatByID(ctx, chat.ParentChatID.UUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return true
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load parent chat.",
			Detail:  err.Error(),
		})
		return true
	}
	if parent.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot unarchive a child chat while its parent is archived. Unarchive the parent chat to cascade.",
		})
		return true
	}
	return false
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) postChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	// Gate message sending behind the same agents-access check
	// used by postChats. Sending a message triggers AI/LLM
	// inference, so it should require the same authorization as
	// chat creation. This is a handler-level band-aid; the
	// structural fix is to make agents-access org-aware so
	// dbauthz enforces this at the RBAC layer.
	// See: https://github.com/coder/coder/issues/24250
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	var req codersdk.CreateChatMessageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	contentBlocks, _, fileIDs, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
		})
		return
	}

	// Validate MCP server IDs exist.
	if req.MCPServerIDs != nil && len(*req.MCPServerIDs) > 0 {
		//nolint:gocritic // Need to validate MCP server IDs exist.
		existingConfigs, err := api.Database.GetMCPServerConfigsByIDs(dbauthz.AsSystemRestricted(ctx), *req.MCPServerIDs)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to validate MCP server IDs.",
				Detail:  err.Error(),
			})
			return
		}
		if len(existingConfigs) != len(*req.MCPServerIDs) {
			found := make(map[uuid.UUID]struct{}, len(existingConfigs))
			for _, c := range existingConfigs {
				found[c.ID] = struct{}{}
			}
			var missing []string
			for _, id := range *req.MCPServerIDs {
				if _, ok := found[id]; !ok {
					missing = append(missing, id.String())
				}
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "One or more MCP server IDs are invalid.",
				Detail:  fmt.Sprintf("Invalid IDs: %s", strings.Join(missing, ", ")),
			})
			return
		}
	}

	if req.PlanMode != nil {
		if !validateChatPlanMode(*req.PlanMode) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid plan_mode value.",
			})
			return
		}
	}

	var sendPlanMode *database.NullChatPlanMode
	if req.PlanMode != nil {
		resolvedPlanMode := planModeToNullChatPlanMode(*req.PlanMode)
		sendPlanMode = &resolvedPlanMode
	}

	busyBehavior := chatd.SendMessageBusyBehaviorQueue
	switch req.BusyBehavior {
	case codersdk.ChatBusyBehaviorInterrupt:
		busyBehavior = chatd.SendMessageBusyBehaviorInterrupt
	case codersdk.ChatBusyBehaviorQueue, "":
		// Default to queue.
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid busy_behavior value.",
			Detail:  `Must be "queue" or "interrupt".`,
		})
		return
	}

	sendResult, sendErr := api.chatDaemon.SendMessage(
		ctx,
		chatd.SendMessageOptions{
			ChatID:        chatID,
			CreatedBy:     apiKey.UserID,
			Content:       contentBlocks,
			ModelConfigID: req.ModelConfigID,
			BusyBehavior:  busyBehavior,
			PlanMode:      sendPlanMode,
			MCPServerIDs:  req.MCPServerIDs,
		},
	)
	if sendErr != nil {
		if maybeWriteLimitErr(ctx, rw, sendErr) {
			return
		}
		if xerrors.Is(sendErr, chatd.ErrMessageQueueFull) {
			httpapi.Write(ctx, rw, http.StatusTooManyRequests, codersdk.Response{
				Message: "Message queue is full.",
				Detail:  fmt.Sprintf("Maximum %d messages can be queued.", chatd.MaxQueueSize),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat message.",
			Detail:  sendErr.Error(),
		})
		return
	}

	// Link any user-uploaded files referenced in this message
	// to the chat (best-effort; cap enforced in SQL).
	unlinked, capExceeded := api.linkFilesToChat(ctx, chatID, fileIDs)
	response := codersdk.CreateChatMessageResponse{Queued: sendResult.Queued}
	if sendResult.Queued {
		if sendResult.QueuedMessage != nil {
			response.QueuedMessage = convertChatQueuedMessagePtr(*sendResult.QueuedMessage)
		}
	} else {
		message := convertChatMessage(sendResult.Message)
		response.Message = &message
	}
	if len(unlinked) > 0 {
		if capExceeded {
			response.Warnings = append(response.Warnings, fileLinkCapWarning(len(unlinked)))
		} else {
			response.Warnings = append(response.Warnings, fileLinkErrorWarning(len(unlinked)))
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) patchChatMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	messageIDStr := chi.URLParam(r, "message")
	messageID, err := strconv.ParseInt(messageIDStr, 10, 64)
	if err != nil || messageID <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat message ID.",
			Detail:  "Message ID must be a positive integer.",
		})
		return
	}

	var req codersdk.EditChatMessageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	contentBlocks, _, fileIDs, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
		})
		return
	}

	editResult, editErr := api.chatDaemon.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       apiKey.UserID,
		EditedMessageID: messageID,
		Content:         contentBlocks,
	})
	if editErr != nil {
		if maybeWriteLimitErr(ctx, rw, editErr) {
			return
		}

		switch {
		case xerrors.Is(editErr, chatd.ErrEditedMessageNotFound):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Chat message not found.",
				Detail:  "Message does not belong to this chat.",
			})
		case xerrors.Is(editErr, chatd.ErrEditedMessageNotUser):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Only user messages can be edited.",
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to edit chat message.",
				Detail:  editErr.Error(),
			})
		}
		return
	}

	// Link any user-uploaded files referenced in the edited
	// message to the chat (best-effort; cap enforced in SQL).
	unlinked, capExceeded := api.linkFilesToChat(ctx, chat.ID, fileIDs)
	response := codersdk.EditChatMessageResponse{
		Message: convertChatMessage(editResult.Message),
	}
	if len(unlinked) > 0 {
		if capExceeded {
			response.Warnings = append(response.Warnings, fileLinkCapWarning(len(unlinked)))
		} else {
			response.Warnings = append(response.Warnings, fileLinkErrorWarning(len(unlinked)))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteChatQueuedMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	queuedMessageIDStr := chi.URLParam(r, "queuedMessage")
	queuedMessageID, err := strconv.ParseInt(queuedMessageIDStr, 10, 64)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid queued message ID.",
			Detail:  err.Error(),
		})
		return
	}

	if api.chatDaemon != nil {
		err = api.chatDaemon.DeleteQueued(ctx, chatID, queuedMessageID)
	} else {
		err = api.Database.DeleteChatQueuedMessage(ctx, database.DeleteChatQueuedMessageParams{
			ID:     queuedMessageID,
			ChatID: chatID,
		})
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete queued message.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) promoteChatQueuedMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	// Gate queued-message promotion behind agents-access.
	// Promoting a queued message triggers AI/LLM inference,
	// same as sending a new message.
	// See: https://github.com/coder/coder/issues/24250
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	queuedMessageIDStr := chi.URLParam(r, "queuedMessage")
	queuedMessageID, err := strconv.ParseInt(queuedMessageIDStr, 10, 64)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid queued message ID.",
			Detail:  err.Error(),
		})
		return
	}

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	promoteResult, txErr := api.chatDaemon.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chatID,
		CreatedBy:       apiKey.UserID,
		QueuedMessageID: queuedMessageID,
	})

	if txErr != nil {
		if maybeWriteLimitErr(ctx, rw, txErr) {
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to promote queued message.",
			Detail:  txErr.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChatMessage(promoteResult.PromotedMessage))
}

// markChatAsRead updates the last read message ID for a chat to the
// latest message, so subsequent unread checks treat all current
// messages as seen. This is called on stream connect and disconnect
// to avoid per-message API calls during active streaming.
func (api *API) markChatAsRead(ctx context.Context, chatID uuid.UUID) {
	lastMsg, err := api.Database.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// No assistant messages yet, nothing to mark as read.
		return
	}
	if err != nil {
		api.Logger.Warn(ctx, "failed to get last assistant message for read marker",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return
	}

	err = api.Database.UpdateChatLastReadMessageID(ctx, database.UpdateChatLastReadMessageIDParams{
		ID:                chatID,
		LastReadMessageID: lastMsg.ID,
	})
	if err != nil {
		api.Logger.Warn(ctx, "failed to update chat last read message ID",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) streamChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_streamer").With(slog.F("chat_id", chatID))

	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat streaming is not available.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	var afterMessageID int64
	if v := r.URL.Query().Get("after_id"); v != "" {
		var err error
		afterMessageID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid after_id parameter.",
				Detail:  err.Error(),
			})
			return
		}
	}

	// Subscribe before accepting the WebSocket so that failures
	// can still be reported as normal HTTP errors.
	snapshot, events, cancelSub, ok := api.chatDaemon.Subscribe(ctx, chatID, r.Header, afterMessageID)
	// Subscribe only fails today when the receiver is nil, which
	// the chatDaemon == nil guard above already catches. This is
	// defensive against future Subscribe failure modes.
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat streaming is not available.",
			Detail:  "Chat stream state is not configured.",
		})
		return
	}
	defer cancelSub()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat stream.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	_ = conn.CloseRead(context.Background())

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close()

	go httpapi.HeartbeatClose(ctx, logger, cancel, conn)

	// Mark the chat as read when the stream connects and again
	// when it disconnects so we avoid per-message API calls while
	// messages are actively streaming.
	api.markChatAsRead(ctx, chatID)
	defer api.markChatAsRead(context.WithoutCancel(ctx), chatID)

	encoder := json.NewEncoder(wsNetConn)

	sendChatStreamBatch := func(batch []codersdk.ChatStreamEvent) error {
		if len(batch) == 0 {
			return nil
		}
		return encoder.Encode(batch)
	}

	drainChatStreamBatch := func(
		first codersdk.ChatStreamEvent,
		maxBatchSize int,
	) ([]codersdk.ChatStreamEvent, bool) {
		batch := []codersdk.ChatStreamEvent{first}
		if maxBatchSize <= 1 {
			return batch, false
		}

		for len(batch) < maxBatchSize {
			select {
			case event, ok := <-events:
				if !ok {
					return batch, true
				}
				batch = append(batch, event)
			default:
				return batch, false
			}
		}

		return batch, false
	}

	for start := 0; start < len(snapshot); start += chatStreamBatchSize {
		end := start + chatStreamBatchSize
		if end > len(snapshot) {
			end = len(snapshot)
		}
		if err := sendChatStreamBatch(snapshot[start:end]); err != nil {
			logger.Debug(ctx, "failed to send chat stream snapshot", slog.Error(err))
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case firstEvent, ok := <-events:
			if !ok {
				return
			}
			batch, streamClosed := drainChatStreamBatch(
				firstEvent,
				chatStreamBatchSize,
			)
			if err := sendChatStreamBatch(batch); err != nil {
				logger.Debug(ctx, "failed to send chat stream event", slog.Error(err))
				return
			}
			if streamClosed {
				return
			}
		}
	}
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) interruptChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_interrupt").With(slog.F("chat_id", chatID))

	if api.chatDaemon != nil {
		chat = api.chatDaemon.InterruptChat(ctx, chat)
	} else {
		updatedChat, updateErr := api.Database.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		if updateErr != nil {
			logger.Error(ctx, "failed to mark chat as waiting", slog.Error(updateErr))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to interrupt chat.",
				Detail:  updateErr.Error(),
			})
			return
		}
		chat = updatedChat
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(chat, nil, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) regenerateChatTitle(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	updatedChat, err := api.chatDaemon.RegenerateChatTitle(ctx, chat)
	if err != nil {
		if errors.Is(err, chatd.ErrManualTitleRegenerationInProgress) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Title regeneration already in progress for this chat.",
			})
			return
		}
		if maybeWriteLimitErr(ctx, rw, err) {
			return
		}
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to regenerate chat title.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(updatedChat, nil, nil))
}

//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) proposeChatTitle(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	title, err := api.chatDaemon.ProposeChatTitle(ctx, chat)
	if err != nil {
		if errors.Is(err, chatd.ErrManualTitleRegenerationInProgress) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Title regeneration already in progress for this chat.",
			})
			return
		}
		if maybeWriteLimitErr(ctx, rw, err) {
			return
		}
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate chat title.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ProposeChatTitleResponse{Title: title})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatDiffContents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	diff, err := api.resolveChatDiffContents(ctx, chat)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat diff.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, diff)
}

// chatCreateWorkspace provides workspace creation for the chat
// processor. RBAC authorization uses context-based checks via
// dbauthz.As rather than fake *http.Request objects.
func (api *API) chatCreateWorkspace(
	ctx context.Context,
	ownerID uuid.UUID,
	req codersdk.CreateWorkspaceRequest,
) (codersdk.Workspace, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, ownerID, rbac.ScopeAll)
	if err != nil {
		return codersdk.Workspace{}, xerrors.Errorf("load user authorization: %w", err)
	}
	ctx = dbauthz.As(ctx, actor)

	ownerUser, err := api.Database.GetUserByID(ctx, ownerID)
	if err != nil {
		return codersdk.Workspace{}, xerrors.Errorf("get workspace owner: %w", err)
	}
	owner := workspaceOwner{
		ID:        ownerUser.ID,
		Username:  ownerUser.Username,
		AvatarURL: ownerUser.AvatarURL,
	}

	auditor := api.Auditor.Load()
	if auditor == nil {
		return codersdk.Workspace{}, xerrors.New("auditor is not configured")
	}

	// The audit system requires a ResponseWriter to capture the
	// HTTP status code. Since this is a programmatic call, we use
	// a recorder. The audit entry still captures the owner, action,
	// and resource correctly.
	rw := httptest.NewRecorder()
	sw := &tracing.StatusWriter{ResponseWriter: rw}

	// Build a minimal synthetic request so the audit commit
	// closure can extract a request ID and user agent. The RBAC
	// subject is already on the context via dbauthz.As above.
	auditReq, err := http.NewRequestWithContext(
		httpmw.WithRequestID(ctx, uuid.New()),
		http.MethodPost,
		"http://localhost/internal/chat/workspace",
		nil,
	)
	if err != nil {
		return codersdk.Workspace{}, xerrors.Errorf("create audit request: %w", err)
	}

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](sw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: auditReq,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})
	aReq.UserID = ownerID
	defer commitAudit()

	workspace, err := createWorkspace(ctx, aReq, ownerID, api, owner, req, nil)
	if err != nil {
		sw.WriteHeader(chatWorkspaceAuditStatus(err))
		return codersdk.Workspace{}, err
	}

	sw.WriteHeader(http.StatusCreated)
	return workspace, nil
}

// chatStartWorkspace starts a stopped workspace by creating a new
// build with the "start" transition. It mirrors chatCreateWorkspace
// but for the start path.
func (api *API) chatStartWorkspace(
	ctx context.Context,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
	req codersdk.CreateWorkspaceBuildRequest,
) (codersdk.WorkspaceBuild, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, ownerID, rbac.ScopeAll)
	if err != nil {
		return codersdk.WorkspaceBuild{}, xerrors.Errorf("load user authorization: %w", err)
	}
	ctx = dbauthz.As(ctx, actor)

	workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return codersdk.WorkspaceBuild{}, xerrors.Errorf("get workspace: %w", err)
	}

	updatedToActiveVersion := false
	if req.Transition == codersdk.WorkspaceTransitionStart {
		template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			return codersdk.WorkspaceBuild{}, xerrors.Errorf("get template: %w", err)
		}

		templateAccessControl := (*(api.AccessControlStore.Load())).GetTemplateAccessControl(template)
		if templateAccessControl.RequireActiveVersion {
			latestBuild, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				return codersdk.WorkspaceBuild{}, xerrors.Errorf("get latest workspace build: %w", err)
			}

			updatedToActiveVersion = latestBuild.TemplateVersionID != template.ActiveVersionID
			req.TemplateVersionID = template.ActiveVersionID
		}
	}

	// Build a synthetic API key so postWorkspaceBuildsInternal can
	// record the correct initiator.
	syntheticKey := database.APIKey{
		UserID: ownerID,
	}

	apiBuild, err := api.postWorkspaceBuildsInternal(
		ctx,
		syntheticKey,
		workspace,
		req,
		func(action policy.Action, object rbac.Objecter) bool {
			// Authorization is handled by dbauthz on the context.
			authErr := api.HTTPAuth.Authorizer.Authorize(ctx, actor, action, object.RBACObject())
			return authErr == nil
		},
		audit.WorkspaceBuildBaggage{},
	)
	if err != nil {
		if updatedToActiveVersion && isChatStartWorkspaceManualUpdateRequiredError(err) {
			const retryInstructions = "The workspace needs the template's active version before it can start. Use read_template with this workspace's template_id to inspect the active version's required parameters, then retry start_workspace with a parameters object that supplies any missing or changed values. If the correct value for a parameter is not obvious from its description or defaults, ask the user rather than guessing."
			if responder, ok := httperror.IsResponder(err); ok {
				status, resp := responder.Response()
				resp = rewriteChatStartWorkspaceManualUpdateResponse(resp, err.Error(), retryInstructions)
				return codersdk.WorkspaceBuild{}, httperror.NewResponseError(status, resp)
			}
			return codersdk.WorkspaceBuild{}, httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
				Message: retryInstructions,
				Detail:  err.Error(),
			})
		}
		return codersdk.WorkspaceBuild{}, xerrors.Errorf("create workspace build: %w", err)
	}

	return apiBuild, nil
}

func rewriteChatStartWorkspaceManualUpdateResponse(resp codersdk.Response, fallbackDetail string, retryInstructions string) codersdk.Response {
	originalMessage := resp.Message
	resp.Message = retryInstructions
	if len(resp.Validations) == 0 && originalMessage != "" {
		if resp.Detail == "" {
			resp.Detail = originalMessage
		} else {
			resp.Detail = originalMessage + ": " + resp.Detail
		}
	} else if resp.Detail == "" {
		resp.Detail = fallbackDetail
	}
	return resp
}

func isChatStartWorkspaceManualUpdateRequiredError(err error) bool {
	var diagnosticErr *dynamicparameters.DiagnosticError
	if errors.As(err, &diagnosticErr) {
		return true
	}

	return errors.Is(err, wsbuilder.ErrParameterValidation)
}

func chatWorkspaceAuditStatus(err error) int {
	if responder, ok := httperror.IsResponder(err); ok {
		status, _ := responder.Response()
		return status
	}
	return http.StatusInternalServerError
}

func (api *API) resolveChatDiffContents(
	ctx context.Context,
	chat database.Chat,
) (codersdk.ChatDiffContents, error) {
	result := codersdk.ChatDiffContents{ChatID: chat.ID}

	status, found, err := api.getCachedChatDiffStatus(ctx, chat.ID)
	if err != nil {
		return result, err
	}

	reference, err := api.resolveChatDiffReference(ctx, chat, found, status)
	if err != nil {
		return result, err
	}

	if reference.RepositoryRef != nil {
		provider := strings.TrimSpace(reference.RepositoryRef.Provider)
		if provider != "" {
			result.Provider = &provider
		}

		origin := strings.TrimSpace(reference.RepositoryRef.RemoteOrigin)
		if origin != "" {
			result.RemoteOrigin = &origin
		}

		branch := strings.TrimSpace(reference.RepositoryRef.Branch)
		if branch != "" {
			result.Branch = &branch
		}
	}

	if reference.PullRequestURL != "" {
		pullRequestURL := strings.TrimSpace(reference.PullRequestURL)
		result.PullRequestURL = &pullRequestURL
		if !found || !strings.EqualFold(strings.TrimSpace(status.Url.String), pullRequestURL) {
			_, err := api.upsertChatDiffStatusReference(ctx, chat.ID, pullRequestURL, time.Now().UTC().Add(-time.Second))
			if err != nil {
				return result, err
			}
		}
	}

	if reference.RepositoryRef == nil {
		return result, nil
	}

	gp := api.resolveGitProvider(reference.RepositoryRef.RemoteOrigin)
	if gp == nil {
		return result, nil
	}

	token, err := api.resolveChatGitAccessToken(ctx, chat.OwnerID, reference.RepositoryRef.RemoteOrigin)
	if err != nil {
		return result, xerrors.Errorf("resolve git access token: %w", err)
	} else if token == nil {
		return result, xerrors.New("nil git access token")
	}

	if reference.PullRequestURL != "" {
		ref, ok := gp.ParsePullRequestURL(reference.PullRequestURL)
		if !ok {
			return result, xerrors.Errorf("invalid pull request URL %q", reference.PullRequestURL)
		}
		diff, err := gp.FetchPullRequestDiff(ctx, *token, ref)
		if err != nil {
			return result, err
		}
		result.Diff = diff
		return result, nil
	}
	diff, err := gp.FetchBranchDiff(ctx, *token, gitprovider.BranchRef{
		Owner:  reference.RepositoryRef.Owner,
		Repo:   reference.RepositoryRef.Repo,
		Branch: reference.RepositoryRef.Branch,
	})
	if err != nil {
		return result, err
	}
	result.Diff = diff
	return result, nil
}

// resolveChatDiffReference builds the diff reference from the cached
// status stored in the database. The git branch and remote origin are
// populated by the workspace agent during git operations (via the
// gitaskpass flow), so no SSH into the workspace is needed here.
//
//nolint:revive // Boolean indicates whether diff status was found.
func (api *API) resolveChatDiffReference(
	ctx context.Context,
	chat database.Chat,
	found bool,
	status database.ChatDiffStatus,
) (chatDiffReference, error) {
	reference := chatDiffReference{}
	if !found {
		return reference, nil
	}

	reference.PullRequestURL = strings.TrimSpace(status.Url.String)

	// Build the repository ref from the stored git branch/origin
	// that the agent reported.
	reference.RepositoryRef = api.buildChatRepositoryRefFromStatus(status)

	// If we have a repo ref with a branch, try to resolve the
	// current open PR. This picks up new PRs after the previous
	// one was closed.
	if reference.RepositoryRef != nil && reference.RepositoryRef.Owner != "" {
		gp := api.resolveGitProvider(reference.RepositoryRef.RemoteOrigin)
		if gp != nil {
			token, err := api.resolveChatGitAccessToken(ctx, chat.OwnerID, reference.RepositoryRef.RemoteOrigin)
			if token == nil || errors.Is(err, gitsync.ErrNoTokenAvailable) {
				// No token available yet.
				return reference, nil
			} else if err != nil {
				return chatDiffReference{}, xerrors.Errorf("resolve git access token: %w", err)
			}
			prRef, lookupErr := gp.ResolveBranchPullRequest(ctx, *token, gitprovider.BranchRef{
				Owner:  reference.RepositoryRef.Owner,
				Repo:   reference.RepositoryRef.Repo,
				Branch: reference.RepositoryRef.Branch,
			})
			if lookupErr != nil {
				api.Logger.Debug(ctx, "failed to resolve pull request from repository reference",
					slog.F("chat_id", chat.ID),
					slog.F("provider", reference.RepositoryRef.Provider),
					slog.F("remote_origin", reference.RepositoryRef.RemoteOrigin),
					slog.F("branch", reference.RepositoryRef.Branch),
					slog.Error(lookupErr),
				)
			} else if prRef != nil {
				reference.PullRequestURL = gp.BuildPullRequestURL(*prRef)
			}
			reference.PullRequestURL = gp.NormalizePullRequestURL(reference.PullRequestURL)
		}
	}

	// If we have a PR URL but no repo ref (e.g. the agent hasn't
	// reported branch/origin yet), derive a partial ref from the
	// PR URL so the caller can still show provider/owner/repo.
	if reference.RepositoryRef == nil && reference.PullRequestURL != "" {
		for _, extAuth := range api.ExternalAuthConfigs {
			gp := extAuth.Git(api.HTTPClient)
			if gp == nil {
				continue
			}
			if parsed, ok := gp.ParsePullRequestURL(reference.PullRequestURL); ok {
				reference.RepositoryRef = &chatRepositoryRef{
					Provider:     strings.ToLower(extAuth.Type),
					Owner:        parsed.Owner,
					Repo:         parsed.Repo,
					RemoteOrigin: gp.BuildRepositoryURL(parsed.Owner, parsed.Repo),
				}
				break
			}
		}
	}

	return reference, nil
}

// buildChatRepositoryRefFromStatus constructs a chatRepositoryRef
// from the git branch and remote origin stored in the cached status.
// Returns nil if no ref data is available.
func (api *API) buildChatRepositoryRefFromStatus(status database.ChatDiffStatus) *chatRepositoryRef {
	branch := strings.TrimSpace(status.GitBranch)
	origin := strings.TrimSpace(status.GitRemoteOrigin)
	if branch == "" || origin == "" {
		return nil
	}

	providerType, gp := api.resolveExternalAuth(origin)
	repoRef := &chatRepositoryRef{
		Provider:     providerType,
		RemoteOrigin: origin,
		Branch:       branch,
	}
	if gp != nil {
		if owner, repo, normalizedOrigin, ok := gp.ParseRepositoryOrigin(repoRef.RemoteOrigin); ok {
			repoRef.RemoteOrigin = normalizedOrigin
			repoRef.Owner = owner
			repoRef.Repo = repo
		}
	}

	if repoRef.Provider == "" {
		return nil
	}

	return repoRef
}

func (api *API) upsertChatDiffStatusReference(
	ctx context.Context,
	chatID uuid.UUID,
	pullRequestURL string,
	staleAt time.Time,
) (database.ChatDiffStatus, error) {
	status, err := api.Database.UpsertChatDiffStatusReference(
		ctx,
		database.UpsertChatDiffStatusReferenceParams{
			ChatID: chatID,
			Url: sql.NullString{
				String: pullRequestURL,
				Valid:  strings.TrimSpace(pullRequestURL) != "",
			},
			// Empty strings preserve existing values via the
			// CASE expression in the SQL query.
			GitBranch:       "",
			GitRemoteOrigin: "",
			StaleAt:         staleAt,
		},
	)
	if err != nil {
		return database.ChatDiffStatus{}, xerrors.Errorf("upsert chat diff status reference: %w", err)
	}
	return status, nil
}

func (api *API) getCachedChatDiffStatus(
	ctx context.Context,
	chatID uuid.UUID,
) (database.ChatDiffStatus, bool, error) {
	status, err := api.Database.GetChatDiffStatusByChatID(ctx, chatID)
	if err == nil {
		return status, true, nil
	}
	if xerrors.Is(err, sql.ErrNoRows) {
		return database.ChatDiffStatus{}, false, nil
	}
	return database.ChatDiffStatus{}, false, xerrors.Errorf(
		"get chat diff status: %w",
		err,
	)
}

// resolveExternalAuth finds the external auth config matching the
// given remote origin URL and returns both the provider type string
// (e.g. "github") and the gitprovider.Provider. Returns ("", nil)
// if no matching config is found.
func (api *API) resolveExternalAuth(origin string) (providerType string, gp gitprovider.Provider) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", nil
	}
	for _, extAuth := range api.ExternalAuthConfigs {
		if extAuth.Regex == nil || !extAuth.Regex.MatchString(origin) {
			continue
		}
		return strings.ToLower(strings.TrimSpace(extAuth.Type)),
			extAuth.Git(api.HTTPClient)
	}
	return "", nil
}

// resolveGitProvider finds the external auth config matching the
// given remote origin URL and returns its git provider. Returns
// nil if no matching git provider is configured.
func (api *API) resolveGitProvider(origin string) gitprovider.Provider {
	_, gp := api.resolveExternalAuth(origin)
	return gp
}

func (api *API) resolveChatGitAccessToken(
	ctx context.Context,
	userID uuid.UUID,
	origin string,
) (*string, error) {
	origin = strings.TrimSpace(origin)

	// If we have an origin, find the specific matching config first.
	// This ensures multi-provider setups (github.com + GHE) get the
	// correct token.
	if origin != "" {
		for _, config := range api.ExternalAuthConfigs {
			if config.Regex == nil || !config.Regex.MatchString(origin) {
				continue
			}
			//nolint:gocritic // System access needed to read external auth
			// links when called from the gitsync worker (chatd context).
			link, err := api.Database.GetExternalAuthLink(dbauthz.AsSystemRestricted(ctx),
				database.GetExternalAuthLinkParams{
					ProviderID: config.ID,
					UserID:     userID,
				},
			)
			if err != nil {
				continue
			}
			//nolint:gocritic // System context carried through for token refresh.
			refreshed, refreshErr := config.RefreshToken(dbauthz.AsSystemRestricted(ctx), api.Database, link)
			if refreshErr == nil {
				link = refreshed
			}
			token := strings.TrimSpace(link.OAuthAccessToken)
			if token != "" {
				return ptr.Ref(token), nil
			}
		}
	}

	// Fallback: iterate all external auth configs.
	// Used when origin is empty (inline refresh from HTTP handler)
	// or when the origin-specific lookup above failed.
	configs := make(map[string]*externalauth.Config)
	providerIDs := []string{}
	for _, config := range api.ExternalAuthConfigs {
		providerIDs = append(providerIDs, config.ID)
		configs[config.ID] = config
	}

	seen := map[string]struct{}{}
	for _, providerID := range providerIDs {
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}

		//nolint:gocritic // System access needed to read external auth
		// links when called from the gitsync worker (chatd context).
		link, err := api.Database.GetExternalAuthLink(
			dbauthz.AsSystemRestricted(ctx),
			database.GetExternalAuthLinkParams{
				ProviderID: providerID,
				UserID:     userID,
			},
		)
		if err != nil {
			continue
		}

		// Refresh the token if there is a matching config, mirroring
		// the same code path used by provisionerdserver when handing
		// tokens to provisioners.
		if cfg, ok := configs[providerID]; ok {
			//nolint:gocritic // System context carried through for token refresh.
			refreshed, refreshErr := cfg.RefreshToken(dbauthz.AsSystemRestricted(ctx), api.Database, link)
			if refreshErr != nil {
				api.Logger.Debug(ctx, "failed to refresh external auth token for chat diff",
					slog.F("provider_id", providerID),
					slog.F("user_id", userID),
					slog.Error(refreshErr),
				)
				// Fall through — the existing token may still work
				// (e.g. GitHub tokens with no expiry).
			} else {
				link = refreshed
			}
		}

		token := strings.TrimSpace(link.OAuthAccessToken)
		if token != "" {
			return ptr.Ref(token), nil
		}
	}

	return nil, gitsync.ErrNoTokenAvailable
}

type createChatWorkspaceSelection struct {
	WorkspaceID uuid.NullUUID
}

func (api *API) validateChatWorkspaceSelection(
	ctx context.Context,
	r *http.Request,
	workspaceID *uuid.UUID,
) (
	uuid.NullUUID,
	database.Workspace,
	int,
	*codersdk.Response,
) {
	if workspaceID == nil {
		return uuid.NullUUID{}, database.Workspace{}, 0, nil
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, *workspaceID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return uuid.NullUUID{}, database.Workspace{}, http.StatusBadRequest, &codersdk.Response{
				Message: "Workspace not found or you do not have access to this resource",
			}
		}
		return uuid.NullUUID{}, database.Workspace{}, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		}
	}

	selection := uuid.NullUUID{
		UUID:  workspace.ID,
		Valid: true,
	}
	if !api.Authorize(r, policy.ActionSSH, workspace) {
		return uuid.NullUUID{}, database.Workspace{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Workspace not found or you do not have access to this resource",
		}
	}

	return selection, workspace, 0, nil
}

func (api *API) validateCreateChatWorkspaceSelection(
	ctx context.Context,
	r *http.Request,
	req codersdk.CreateChatRequest,
) (
	createChatWorkspaceSelection,
	int,
	*codersdk.Response,
) {
	selection := createChatWorkspaceSelection{}
	workspaceID, workspace, status, resp := api.validateChatWorkspaceSelection(ctx, r, req.WorkspaceID)
	if resp != nil {
		return selection, status, resp
	}
	selection.WorkspaceID = workspaceID
	if !workspaceID.Valid {
		return selection, 0, nil
	}
	if workspace.OrganizationID != req.OrganizationID {
		return selection, http.StatusBadRequest, &codersdk.Response{
			Message: "Workspace does not belong to the specified organization.",
		}
	}

	return selection, 0, nil
}

func (api *API) resolveCreateChatModelConfigID(
	ctx context.Context,
	req codersdk.CreateChatRequest,
) (uuid.UUID, int, *codersdk.Response) {
	if req.ModelConfigID != nil {
		if *req.ModelConfigID == uuid.Nil {
			return uuid.Nil, http.StatusBadRequest, &codersdk.Response{
				Message: "Invalid model config ID.",
			}
		}
		return *req.ModelConfigID, 0, nil
	}

	defaultModelConfig, err := api.Database.GetDefaultChatModelConfig(ctx)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, http.StatusBadRequest, &codersdk.Response{
				Message: "No default chat model config is configured.",
			}
		}
		return uuid.Nil, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat model config.",
			Detail:  err.Error(),
		}
	}

	return defaultModelConfig.ID, 0, nil
}

func normalizeChatCompressionThreshold(
	requested *int32,
	fallback int32,
) (int32, error) {
	threshold := fallback
	if requested != nil {
		threshold = *requested
	}

	if threshold < minChatContextCompressionThreshold ||
		threshold > maxChatContextCompressionThreshold {
		return 0, xerrors.Errorf(
			"context_compression_threshold must be between %d and %d",
			minChatContextCompressionThreshold,
			maxChatContextCompressionThreshold,
		)
	}

	return threshold, nil
}

func parseCompactionThresholdKey(key string) (uuid.UUID, error) {
	if !strings.HasPrefix(key, codersdk.ChatCompactionThresholdKeyPrefix) {
		return uuid.Nil, xerrors.Errorf("invalid compaction threshold key: %q", key)
	}
	id, err := uuid.Parse(key[len(codersdk.ChatCompactionThresholdKeyPrefix):])
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid model config ID in key %q: %w", key, err)
	}
	return id, nil
}

const (
	// maxChatFileSize is the maximum size of a chat file upload (10 MB).
	maxChatFileSize = 10 << 20
)

//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatSystemPrompt(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}
	config, err := api.Database.GetChatSystemPromptConfig(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat system prompt configuration.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatSystemPromptResponse{
		SystemPrompt:               config.ChatSystemPrompt,
		IncludeDefaultSystemPrompt: config.IncludeDefaultSystemPrompt,
		DefaultSystemPrompt:        chatd.DefaultSystemPrompt,
	})
}

func (api *API) putChatSystemPrompt(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	r.Body = http.MaxBytesReader(rw, r.Body, int64(2*maxSystemPromptLenBytes))
	var req codersdk.UpdateChatSystemPromptRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	sanitizedPrompt := chatd.SanitizePromptText(req.SystemPrompt)
	// 128 KiB is generous for a system prompt while still
	// preventing abuse or accidental pastes of large content.
	if len(sanitizedPrompt) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "System prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(sanitizedPrompt)),
		})
		return
	}
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.UpsertChatSystemPrompt(ctx, sanitizedPrompt); err != nil {
			return err
		}
		// Only update the include-default flag when the caller explicitly
		// provides it. Omitting the field preserves whatever is currently
		// stored (or the schema-level default for new deployments),
		// avoiding a backward-compatibility regression for older clients
		// that only send system_prompt.
		if req.IncludeDefaultSystemPrompt != nil {
			return tx.UpsertChatIncludeDefaultSystemPrompt(ctx, *req.IncludeDefaultSystemPrompt)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat system prompt configuration.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatPlanModeInstructions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}

	instructions, err := api.Database.GetChatPlanModeInstructions(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching plan mode instructions.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatPlanModeInstructionsResponse{
		PlanModeInstructions: instructions,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatPlanModeInstructions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	r.Body = http.MaxBytesReader(rw, r.Body, int64(2*maxSystemPromptLenBytes))

	var req codersdk.UpdateChatPlanModeInstructionsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	sanitizedInstructions := chatd.SanitizePromptText(req.PlanModeInstructions)
	if len(sanitizedInstructions) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Plan mode instructions exceed maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(sanitizedInstructions)),
		})
		return
	}

	if err := api.Database.UpsertChatPlanModeInstructions(ctx, sanitizedInstructions); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating plan mode instructions.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatExploreModelOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}

	modelConfigID, hasMalformedOverride, err := api.getChatExploreModelOverrideConfig(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching Explore model override.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatExploreModelOverrideResponse{
		ModelConfigID:        modelConfigID,
		HasMalformedOverride: hasMalformedOverride,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatExploreModelOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatExploreModelOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	status, resp := validateChatExploreModelOverrideID(ctx, api.Database, req.ModelConfigID)
	if resp != nil {
		httpapi.Write(ctx, rw, status, *resp)
		return
	}

	if err := api.Database.UpsertChatExploreModelOverride(ctx, formatChatExploreModelOverride(req.ModelConfigID)); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating Explore model override.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatDesktopEnabled(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabled, err := api.Database.GetChatDesktopEnabled(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching desktop setting.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatDesktopEnabledResponse{
		EnableDesktop: enabled,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatDesktopEnabled(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatDesktopEnabledRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if err := api.Database.UpsertChatDesktopEnabled(ctx, req.EnableDesktop); httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating desktop setting.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) deploymentChatDebugLoggingEnabled() bool {
	return api.DeploymentValues != nil && api.DeploymentValues.AI.Chat.DebugLoggingEnabled.Value()
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatDebugLogging(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}

	allowUsers, err := api.Database.GetChatDebugLoggingAllowUsers(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat debug logging setting.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatDebugLoggingAdminSettings{
		AllowUsers:         err == nil && allowUsers,
		ForcedByDeployment: api.deploymentChatDebugLoggingEnabled(),
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatDebugLogging(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatDebugLoggingAllowUsersRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if err := api.Database.UpsertChatDebugLoggingAllowUsers(ctx, req.AllowUsers); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat debug logging setting.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getUserChatDebugLogging(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	forcedByDeployment := api.deploymentChatDebugLoggingEnabled()
	allowUsers := false
	if !forcedByDeployment {
		enabled, err := api.Database.GetChatDebugLoggingAllowUsers(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching chat debug logging setting.",
				Detail:  err.Error(),
			})
			return
		}
		allowUsers = err == nil && enabled
	}

	debugEnabled := forcedByDeployment
	if allowUsers {
		enabled, err := api.Database.GetUserChatDebugLoggingEnabled(ctx, apiKey.UserID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user chat debug logging setting.",
				Detail:  err.Error(),
			})
			return
		}
		debugEnabled = err == nil && enabled
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatDebugLoggingSettings{
		DebugLoggingEnabled: debugEnabled,
		UserToggleAllowed:   !forcedByDeployment && allowUsers,
		ForcedByDeployment:  forcedByDeployment,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putUserChatDebugLogging(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if api.deploymentChatDebugLoggingEnabled() {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Chat debug logging is already forced on by deployment configuration.",
		})
		return
	}

	allowUsers, err := api.Database.GetChatDebugLoggingAllowUsers(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat debug logging setting.",
			Detail:  err.Error(),
		})
		return
	}
	if err != nil || !allowUsers {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "An administrator has not enabled user-controlled chat debug logging.",
		})
		return
	}

	var req codersdk.UpdateUserChatDebugLoggingRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if err := api.Database.UpsertUserChatDebugLoggingEnabled(ctx, database.UpsertUserChatDebugLoggingEnabledParams{
		UserID:              apiKey.UserID,
		DebugLoggingEnabled: req.DebugLoggingEnabled,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user chat debug logging setting.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatWorkspaceTTL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw, err := api.Database.GetChatWorkspaceTTL(ctx)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace TTL setting.",
			Detail:  err.Error(),
		})
		return
	}
	// Validate/default the stored value so callers always receive a
	// well-formed duration string.
	d, err := codersdk.ParseChatWorkspaceTTL(raw)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Stored workspace TTL is invalid.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatWorkspaceTTLResponse{
		WorkspaceTTLMillis: d.Milliseconds(),
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatWorkspaceTTL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatWorkspaceTTLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate before converting to avoid int64 overflow in the
	// multiplication by time.Millisecond.
	if req.WorkspaceTTLMillis < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Workspace TTL must be non-negative.",
		})
		return
	}

	// Convert milliseconds to duration.
	d := time.Duration(req.WorkspaceTTLMillis) * time.Millisecond

	// Technically a duplication of validWorkspaceTTL but this is not scoped to templates.
	if d > 0 && d < ttlMinimum {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Workspace TTL must not be less than 1 minute.",
		})
		return
	}
	if d > ttlMaximum {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Workspace TTL must not exceed 30 days.",
		})
		return
	}

	// Store the canonicalized duration string.
	if err := api.Database.UpsertChatWorkspaceTTL(ctx, d.String()); httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace TTL setting.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get chat retention days
// @ID get-chat-retention-days
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {object} codersdk.ChatRetentionDaysResponse
// @Router /experimental/chats/config/retention-days [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatRetentionDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	retentionDays, err := api.Database.GetChatRetentionDays(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat retention days.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatRetentionDaysResponse{
		RetentionDays: retentionDays,
	})
}

// Keep in sync with retentionDaysMaximum in
// site/src/pages/AgentsPage/AgentSettingsBehaviorPageView.tsx.
const retentionDaysMaximum = 3650 // ~10 years

// @Summary Update chat retention days
// @ID update-chat-retention-days
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param request body codersdk.UpdateChatRetentionDaysRequest true "Request body"
// @Success 204
// @Router /experimental/chats/config/retention-days [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatRetentionDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	var req codersdk.UpdateChatRetentionDaysRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.RetentionDays < 0 || req.RetentionDays > retentionDaysMaximum {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Retention days must be between 0 and %d.", retentionDaysMaximum),
		})
		return
	}
	if err := api.Database.UpsertChatRetentionDays(ctx, req.RetentionDays); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat retention days.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatTemplateAllowlist(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}
	raw, err := api.Database.GetChatTemplateAllowlist(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat template allowlist.",
			Detail:  err.Error(),
		})
		return
	}
	parsed, parseErr := xjson.ParseUUIDList(raw)
	if parseErr != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Stored template allowlist is corrupt.",
			Detail:  parseErr.Error(),
		})
		return
	}
	ids := make([]string, len(parsed))
	for i, id := range parsed {
		ids[i] = id.String()
	}
	resp := codersdk.ChatTemplateAllowlist{
		TemplateIDs: ids,
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatTemplateAllowlist(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.ChatTemplateAllowlist
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate all entries are valid UUIDs and deduplicate.
	seen := make(map[string]struct{}, len(req.TemplateIDs))
	deduped := make([]string, 0, len(req.TemplateIDs))
	for _, id := range req.TemplateIDs {
		parsed, err := uuid.Parse(id)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid template ID in allowlist.",
				Detail:  fmt.Sprintf("%q is not a valid UUID.", id),
			})
			return
		}
		// Canonicalize to lowercase so deduplication is
		// case-insensitive and stored values are consistent.
		canonical := parsed.String()
		if _, ok := seen[canonical]; !ok {
			seen[canonical] = struct{}{}
			deduped = append(deduped, canonical)
		}
	}

	// Convert to UUIDs for the database query.
	parsedUUIDs := make([]uuid.UUID, len(deduped))
	for i, s := range deduped {
		// Already validated above, safe to ignore error.
		parsedUUIDs[i], _ = uuid.Parse(s)
	}

	raw, err := json.Marshal(deduped)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error encoding template allowlist.",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		// Verify all IDs refer to existing, non-deprecated templates
		// in a single query.
		if len(parsedUUIDs) > 0 {
			found, err := tx.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
				IDs: parsedUUIDs,
				Deprecated: sql.NullBool{
					Bool:  false,
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("fetch templates: %w", err)
			}
			if len(found) != len(parsedUUIDs) {
				foundSet := make(map[uuid.UUID]struct{}, len(found))
				for _, t := range found {
					foundSet[t.ID] = struct{}{}
				}
				var missing []string
				for _, id := range parsedUUIDs {
					if _, ok := foundSet[id]; !ok {
						missing = append(missing, id.String())
					}
				}
				return xerrors.Errorf("templates not found or deprecated: %s", strings.Join(missing, ", "))
			}
		}
		return tx.UpsertChatTemplateAllowlist(ctx, string(raw))
	}, nil)
	if err != nil {
		// If the error mentions "not found or deprecated", it's a
		// validation failure, not an internal error.
		if strings.Contains(err.Error(), "not found or deprecated") {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "One or more templates not found or deprecated.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat template allowlist.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getUserChatCustomPrompt(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	customPrompt, err := api.Database.GetUserChatCustomPrompt(ctx, apiKey.UserID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Error reading user chat custom prompt.",
				Detail:  err.Error(),
			})
			return
		}

		customPrompt = ""
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatCustomPrompt{
		CustomPrompt: customPrompt,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putUserChatCustomPrompt(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)
	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	r.Body = http.MaxBytesReader(rw, r.Body, int64(2*maxSystemPromptLenBytes))

	var params codersdk.UserChatCustomPrompt
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	sanitizedPrompt := chatd.SanitizePromptText(params.CustomPrompt)
	// Apply the same 128 KiB limit as the deployment system prompt.
	if len(sanitizedPrompt) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Custom prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(sanitizedPrompt)),
		})
		return
	}

	updatedConfig, err := api.Database.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           apiKey.UserID,
		ChatCustomPrompt: sanitizedPrompt,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error updating user chat custom prompt.",
			Detail:  err.Error(),
		})
		return
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventUserPrompt, apiKey.UserID)

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatCustomPrompt{
		CustomPrompt: updatedConfig.Value,
	})
}

// @Summary Get user chat compaction thresholds
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getUserChatCompactionThresholds(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	rows, err := api.Database.ListUserChatCompactionThresholds(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error listing user chat compaction thresholds.",
			Detail:  err.Error(),
		})
		return
	}

	resp := codersdk.UserChatCompactionThresholds{
		Thresholds: make([]codersdk.UserChatCompactionThreshold, 0, len(rows)),
	}
	for _, row := range rows {
		modelConfigID, err := parseCompactionThresholdKey(row.Key)
		if err != nil {
			api.Logger.Warn(ctx, "skipping malformed user chat compaction threshold key",
				slog.F("key", row.Key),
				slog.F("value", row.Value),
				slog.Error(err),
			)
			continue
		}

		thresholdPercent, err := strconv.ParseInt(row.Value, 10, 32)
		if err != nil {
			api.Logger.Warn(ctx, "skipping malformed user chat compaction threshold value",
				slog.F("key", row.Key),
				slog.F("value", row.Value),
				slog.Error(err),
			)
			continue
		}
		if thresholdPercent < int64(minChatContextCompressionThreshold) ||
			thresholdPercent > int64(maxChatContextCompressionThreshold) {
			api.Logger.Warn(ctx, "skipping out-of-range user chat compaction threshold",
				slog.F("key", row.Key),
				slog.F("value", row.Value),
			)
			continue
		}

		resp.Thresholds = append(resp.Thresholds, codersdk.UserChatCompactionThreshold{
			ModelConfigID:    modelConfigID,
			ThresholdPercent: int32(thresholdPercent),
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Set user chat compaction threshold for a model config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putUserChatCompactionThreshold(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	modelConfigID, ok := parseChatModelConfigID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateUserChatCompactionThresholdRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.ThresholdPercent < minChatContextCompressionThreshold ||
		req.ThresholdPercent > maxChatContextCompressionThreshold {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "threshold_percent is out of range.",
			Detail: fmt.Sprintf(
				"threshold_percent must be between %d and %d, got %d.",
				minChatContextCompressionThreshold,
				maxChatContextCompressionThreshold,
				req.ThresholdPercent,
			),
		})
		return
	}

	// Use system context because GetChatModelConfigByID requires
	// deployment-config read access, which non-admin users lack.
	// The user is only checking if the model exists and is enabled
	// before writing their own personal preference.
	//nolint:gocritic // Non-admin users need this lookup to save their own setting.
	modelConfig, err := api.Database.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), modelConfigID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat model config.",
			Detail:  err.Error(),
		})
		return
	}
	if !modelConfig.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Model config is disabled.",
		})
		return
	}

	_, err = api.Database.UpdateUserChatCompactionThreshold(ctx, database.UpdateUserChatCompactionThresholdParams{
		UserID:           apiKey.UserID,
		Key:              codersdk.CompactionThresholdKey(modelConfigID),
		ThresholdPercent: req.ThresholdPercent,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error updating user chat compaction threshold.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatCompactionThreshold{
		ModelConfigID:    modelConfigID,
		ThresholdPercent: req.ThresholdPercent,
	})
}

// @Summary Delete user chat compaction threshold for a model config
// @x-apidocgen {"skip": true}
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteUserChatCompactionThreshold(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	modelConfigID, ok := parseChatModelConfigID(rw, r)
	if !ok {
		return
	}

	if err := api.Database.DeleteUserChatCompactionThreshold(ctx, database.DeleteUserChatCompactionThresholdParams{
		UserID: apiKey.UserID,
		Key:    codersdk.CompactionThresholdKey(modelConfigID),
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error deleting user chat compaction threshold.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) postChatFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	orgIDStr := r.URL.Query().Get("organization")
	if orgIDStr == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing organization query parameter.",
		})
		return
	}
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid organization ID.",
		})
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Strip parameters (e.g. "image/png; charset=utf-8" → "image/png")
	// so the allowlist check matches the base media type.
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = mediaType
	}
	if !chatfiles.IsAllowedStoredMediaType(contentType) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unsupported file type.",
			Detail:  fmt.Sprintf("Allowed types: %s.", chatfiles.AllowedStoredMediaTypesString()),
		})
		return
	}

	// Extract filename from Content-Disposition header if provided.
	var filename string
	if cd := r.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			filename = params["filename"]
		}
	}

	r.Body = http.MaxBytesReader(rw, r.Body, maxChatFileSize)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "File too large.",
				Detail:  fmt.Sprintf("Maximum file size is %d bytes.", maxChatFileSize),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read file from request.",
			Detail:  err.Error(),
		})
		return
	}

	// Verify the actual content matches an allowed file type so that
	// a client cannot spoof Content-Type to serve active content.
	filename, detected, err := chatfiles.PrepareStoredFile(filename, filename, data)
	if err != nil {
		switch {
		case errors.Is(err, chatfiles.ErrStoredFileNameRequired):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Filename is required.",
				Detail:  "Provide a filename in the Content-Disposition header.",
			})
		case errors.Is(err, chatfiles.ErrUnsupportedStoredFileType):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unsupported file type.",
				Detail:  fmt.Sprintf("Allowed types: %s.", chatfiles.AllowedStoredMediaTypesString()),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid file.",
				Detail:  err.Error(),
			})
		}
		return
	}
	// The compatibility check below is security-critical: it keeps exact
	// media-type matching by default while allowing safe text/plain
	// refinements such as JSON, CSV, and Markdown now that upload
	// classification can return richer stored media types. Combined with
	// the X-Content-Type-Options: nosniff header applied globally, this
	// still prevents clients from smuggling binary or active content under
	// a safer declared Content-Type.
	if !chatfiles.IsCompatibleUploadMediaType(contentType, detected) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "File content type does not match Content-Type header.",
			Detail:  fmt.Sprintf("Header declared %q but file content was detected as %q.", contentType, detected),
		})
		return
	}
	chatFile, err := api.Database.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        apiKey.UserID,
		OrganizationID: orgID,
		Name:           filename,
		Mimetype:       detected,
		Data:           data,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to save chat file.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.UploadChatFileResponse{
		ID: chatFile.ID,
	})
}

func (api *API) chatFileByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fileIDStr := chi.URLParam(r, "file")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid file ID.",
		})
		return
	}

	chatFile, err := api.Database.GetChatFileByID(ctx, fileID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat file.",
			Detail:  err.Error(),
		})
		return
	}

	rw.Header().Set("Content-Type", chatFile.Mimetype)
	disposition := "attachment"
	if chatfiles.IsInlineRenderableStoredMediaType(chatFile.Mimetype) {
		disposition = "inline"
	}
	if chatFile.Name != "" {
		rw.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": chatFile.Name}))
	} else {
		rw.Header().Set("Content-Disposition", disposition)
	}
	rw.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	rw.Header().Set("Content-Length", strconv.Itoa(len(chatFile.Data)))
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(chatFile.Data); err != nil {
		api.Logger.Debug(ctx, "failed to write chat file response", slog.Error(err))
	}
}

func createChatInputFromRequest(ctx context.Context, db database.Store, req codersdk.CreateChatRequest) (
	[]codersdk.ChatMessagePart,
	string,
	[]uuid.UUID,
	*codersdk.Response,
) {
	return createChatInputFromParts(ctx, db, req.Content, "content")
}

func createChatInputFromParts(
	ctx context.Context,
	db database.Store,
	parts []codersdk.ChatInputPart,
	fieldName string,
) ([]codersdk.ChatMessagePart, string, []uuid.UUID, *codersdk.Response) {
	if len(parts) == 0 {
		return nil, "", nil, &codersdk.Response{
			Message: "Content is required.",
			Detail:  "Content cannot be empty.",
		}
	}

	var fileIDs []uuid.UUID
	content := make([]codersdk.ChatMessagePart, 0, len(parts))
	textParts := make([]string, 0, len(parts))
	for i, part := range parts {
		switch strings.ToLower(strings.TrimSpace(string(part.Type))) {
		case string(codersdk.ChatInputPartTypeText):
			text := strings.TrimSpace(part.Text)
			if text == "" {
				return nil, "", nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].text cannot be empty.", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageText(text))
			textParts = append(textParts, text)
		case string(codersdk.ChatInputPartTypeFile):
			if part.FileID == uuid.Nil {
				return nil, "", nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].file_id is required for file parts.", fieldName, i),
				}
			}
			// Validate that the file exists and get its media type.
			// File data is not loaded here; it's resolved at LLM
			// dispatch time via chatFileResolver.
			chatFile, err := db.GetChatFileByID(ctx, part.FileID)
			if err != nil {
				if httpapi.Is404Error(err) {
					return nil, "", nil, &codersdk.Response{
						Message: "Invalid input part.",
						Detail:  fmt.Sprintf("%s[%d].file_id references a file that does not exist.", fieldName, i),
					}
				}
				return nil, "", nil, &codersdk.Response{
					Message: "Internal error.",
					Detail:  fmt.Sprintf("Failed to retrieve file for %s[%d].", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageFile(part.FileID, chatFile.Mimetype, chatFile.Name))
			fileIDs = append(fileIDs, part.FileID)
		// file-reference parts carry inline code snippets, not uploaded
		// files. They have no FileID and are excluded from file tracking.
		case string(codersdk.ChatInputPartTypeFileReference):
			if part.FileName == "" {
				return nil, "", nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].file_name cannot be empty for file-reference.", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageFileReference(part.FileName, part.StartLine, part.EndLine, part.Content))
			// Build text representation for title generation.
			lineRange := fmt.Sprintf("%d", part.StartLine)
			if part.StartLine != part.EndLine {
				lineRange = fmt.Sprintf("%d-%d", part.StartLine, part.EndLine)
			}
			var sb strings.Builder
			_, _ = fmt.Fprintf(&sb, "[file-reference] %s:%s", part.FileName, lineRange)
			if strings.TrimSpace(part.Content) != "" {
				_, _ = fmt.Fprintf(&sb, "\n```%s\n%s\n```", part.FileName, strings.TrimSpace(part.Content))
			}
			textParts = append(textParts, sb.String())
		default:
			return nil, "", nil, &codersdk.Response{
				Message: "Invalid input part.",
				Detail: fmt.Sprintf(
					"%s[%d].type %q is not supported.",
					fieldName,
					i,
					part.Type,
				),
			}
		}
	}

	// Allow file-only messages. The titleSource may be empty
	// when only file parts are provided, callers handle this.
	if len(content) == 0 {
		return nil, "", nil, &codersdk.Response{
			Message: "Content is required.",
			Detail:  fmt.Sprintf("%s must include at least one text or file part.", fieldName),
		}
	}
	titleSource := strings.TrimSpace(strings.Join(textParts, " "))
	return content, titleSource, fileIDs, nil
}

func chatTitleFromMessage(message string) string {
	const maxWords = 6
	const maxRunes = 80
	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}
	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}
	title := strings.Join(words, " ")
	if truncated {
		title += "…"
	}
	return truncateRunes(title, maxRunes)
}

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}

	return string(runes[:maxLen])
}

// linkFilesToChat inserts file-link rows into the chat_file_links
// join table. Cap enforcement and dedup are handled atomically in
// SQL. On success returns (nil, false). On failure returns the full
// input fileIDs slice — linking is all-or-nothing because the
// SQL operates on the batch atomically. capExceeded indicates
// whether the failure was due to the cap being exceeded (true)
// or a database error (false).
// Failures are logged but never block the caller.
func (api *API) linkFilesToChat(ctx context.Context, chatID uuid.UUID, fileIDs []uuid.UUID) (unlinked []uuid.UUID, capExceeded bool) {
	if len(fileIDs) == 0 {
		return nil, false
	}
	rejected, err := api.Database.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      fileIDs,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to link files to chat",
			slog.F("chat_id", chatID),
			slog.F("file_ids", fileIDs),
			slog.Error(err),
		)
		return fileIDs, false
	}
	if rejected > 0 {
		api.Logger.Warn(ctx, "file cap reached, files not linked",
			slog.F("chat_id", chatID),
			slog.F("file_ids", fileIDs),
			slog.F("max_file_links", codersdk.MaxChatFileIDs),
		)
		return fileIDs, true
	}
	return nil, false
}

// fileLinkCapWarning builds a user-facing warning when a batch
// of file IDs was atomically rejected because the resulting
// array would exceed the per-chat file cap.
func fileLinkCapWarning(count int) string {
	return fmt.Sprintf("file linking skipped: batch of %d file(s) would exceed limit of %d", count, codersdk.MaxChatFileIDs)
}

// fileLinkErrorWarning builds a user-facing warning when a
// database error prevented linking files to a chat.
func fileLinkErrorWarning(count int) string {
	return fmt.Sprintf("%d file(s) could not be linked due to a server error", count)
}

// fetchChatFileMetadata returns metadata for all files linked to
// the given chat. Errors are logged and result in a nil return
// (callers treat file metadata as best-effort).
func (api *API) fetchChatFileMetadata(ctx context.Context, chatID uuid.UUID) []database.GetChatFileMetadataByChatIDRow {
	rows, err := api.Database.GetChatFileMetadataByChatID(ctx, chatID)
	if err != nil {
		api.Logger.Error(ctx, "failed to fetch chat file metadata",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return nil
	}
	return rows
}

func convertChatCostModelBreakdown(model database.GetChatCostPerModelRow) codersdk.ChatCostModelBreakdown {
	displayName := strings.TrimSpace(model.DisplayName)
	if displayName == "" {
		displayName = model.Model
	}
	return codersdk.ChatCostModelBreakdown{
		ModelConfigID:            model.ModelConfigID,
		DisplayName:              displayName,
		Provider:                 model.Provider,
		Model:                    model.Model,
		TotalCostMicros:          model.TotalCostMicros,
		MessageCount:             model.MessageCount,
		TotalInputTokens:         model.TotalInputTokens,
		TotalOutputTokens:        model.TotalOutputTokens,
		TotalCacheReadTokens:     model.TotalCacheReadTokens,
		TotalCacheCreationTokens: model.TotalCacheCreationTokens,
		TotalRuntimeMs:           model.TotalRuntimeMs,
	}
}

func convertChatCostChatBreakdown(chat database.GetChatCostPerChatRow) codersdk.ChatCostChatBreakdown {
	return codersdk.ChatCostChatBreakdown{
		RootChatID:               chat.RootChatID,
		ChatTitle:                chat.ChatTitle,
		TotalCostMicros:          chat.TotalCostMicros,
		MessageCount:             chat.MessageCount,
		TotalInputTokens:         chat.TotalInputTokens,
		TotalOutputTokens:        chat.TotalOutputTokens,
		TotalCacheReadTokens:     chat.TotalCacheReadTokens,
		TotalCacheCreationTokens: chat.TotalCacheCreationTokens,
		TotalRuntimeMs:           chat.TotalRuntimeMs,
	}
}

func convertChatCostUserRollup(user database.GetChatCostPerUserRow) codersdk.ChatCostUserRollup {
	return codersdk.ChatCostUserRollup{
		UserID:                   user.UserID,
		Username:                 user.Username,
		Name:                     user.Name,
		AvatarURL:                user.AvatarURL,
		TotalCostMicros:          user.TotalCostMicros,
		MessageCount:             user.MessageCount,
		ChatCount:                user.ChatCount,
		TotalInputTokens:         user.TotalInputTokens,
		TotalOutputTokens:        user.TotalOutputTokens,
		TotalCacheReadTokens:     user.TotalCacheReadTokens,
		TotalCacheCreationTokens: user.TotalCacheCreationTokens,
		TotalRuntimeMs:           user.TotalRuntimeMs,
	}
}

func convertChatQueuedMessage(m database.ChatQueuedMessage) codersdk.ChatQueuedMessage {
	return db2sdk.ChatQueuedMessage(m)
}

func convertChatQueuedMessagePtr(m database.ChatQueuedMessage) *codersdk.ChatQueuedMessage {
	qm := convertChatQueuedMessage(m)
	return &qm
}

func convertChatQueuedMessages(msgs []database.ChatQueuedMessage) []codersdk.ChatQueuedMessage {
	result := make([]codersdk.ChatQueuedMessage, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, convertChatQueuedMessage(m))
	}
	return result
}

func convertChatMessage(m database.ChatMessage) codersdk.ChatMessage {
	return db2sdk.ChatMessage(m)
}

func convertChatMessages(messages []database.ChatMessage) []codersdk.ChatMessage {
	result := make([]codersdk.ChatMessage, 0, len(messages))
	for _, m := range messages {
		result = append(result, convertChatMessage(m))
	}
	return result
}

func (api *API) listChatProviders(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	//nolint:gocritic // System context required to read enabled chat providers.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	providers, err := api.Database.GetChatProviders(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat providers.",
			Detail:  err.Error(),
		})
		return
	}

	providersByName := make(map[string]database.ChatProvider, len(providers))
	configuredProviders := make([]chatprovider.ConfiguredProvider, 0, len(providers))
	for _, provider := range providers {
		normalizedProvider := normalizeChatProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		provider.Provider = normalizedProvider
		providersByName[normalizedProvider] = provider
		configuredProviders = append(configuredProviders, chatprovider.ConfiguredProvider{
			Provider: normalizedProvider,
			APIKey:   provider.APIKey,
			BaseURL:  provider.BaseUrl,
		})
	}
	if api.chatDaemon == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat processor is unavailable.",
			Detail:  "Chat processor is not configured.",
		})
		return
	}

	enabledProviders, err := api.Database.GetEnabledChatProviders(
		systemCtx,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to resolve provider API keys.",
			Detail:  err.Error(),
		})
		return
	}

	enabledConfiguredProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(enabledProviders),
	)
	for _, provider := range enabledProviders {
		normalizedProvider := normalizeChatProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		enabledConfiguredProviders = append(
			enabledConfiguredProviders, chatprovider.ConfiguredProvider{
				Provider: normalizedProvider,
				APIKey:   provider.APIKey,
				BaseURL:  provider.BaseUrl,
			},
		)
	}

	effectiveKeys := chatprovider.MergeProviderAPIKeys(
		ChatProviderAPIKeysFromDeploymentValues(api.DeploymentValues),
		enabledConfiguredProviders,
	)
	effectiveKeys = chatprovider.MergeProviderAPIKeys(
		effectiveKeys, configuredProviders,
	)

	supportedProviders := chatprovider.SupportedProviders()
	resp := make([]codersdk.ChatProviderConfig, 0, len(supportedProviders))
	for _, provider := range supportedProviders {
		configured, ok := providersByName[provider]
		if ok {
			resp = append(
				resp,
				convertChatProviderConfig(
					configured,
					api.hasEffectiveProviderAPIKey(ctx, configured),
					codersdk.ChatProviderConfigSourceDatabase,
				),
			)
			continue
		}

		source := codersdk.ChatProviderConfigSourceSupported
		hasAPIKey := effectiveKeys.APIKey(provider) != ""
		enabled := false
		if chatprovider.IsEnvPresetProvider(provider) && hasAPIKey {
			source = codersdk.ChatProviderConfigSourceEnvPreset
			enabled = true
		}

		resp = append(resp, codersdk.ChatProviderConfig{
			ID:                         uuid.Nil,
			Provider:                   provider,
			DisplayName:                chatprovider.ProviderDisplayName(provider),
			Enabled:                    enabled,
			HasAPIKey:                  hasAPIKey,
			CentralAPIKeyEnabled:       true,
			AllowUserAPIKey:            false,
			AllowCentralAPIKeyFallback: false,
			BaseURL:                    effectiveKeys.BaseURL(provider),
			Source:                     source,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) createChatProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	var inserted database.ChatProvider
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateChatProviderConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	provider := normalizeChatProvider(req.Provider)
	if provider == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid provider.",
			Detail:  chatProviderValidationDetail(),
		})
		return
	}

	if err := validateChatProviderAPIKeySize(strings.TrimSpace(req.APIKey)); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "API key too large.",
			Detail:  err.Error(),
		})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	baseURL, err := normalizeChatProviderBaseURL(req.BaseURL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid provider base URL.",
			Detail:  err.Error(),
		})
		return
	}

	centralAPIKeyEnabled := true
	if req.CentralAPIKeyEnabled != nil {
		centralAPIKeyEnabled = *req.CentralAPIKeyEnabled
	}
	allowUserAPIKey := false
	if req.AllowUserAPIKey != nil {
		allowUserAPIKey = *req.AllowUserAPIKey
	}
	allowCentralAPIKeyFallback := false
	if req.AllowCentralAPIKeyFallback != nil {
		allowCentralAPIKeyFallback = *req.AllowCentralAPIKeyFallback
	}

	if err := validateChatProviderCredentialPolicy(
		centralAPIKeyEnabled,
		allowUserAPIKey,
		allowCentralAPIKeyFallback,
	); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid credential policy.",
			Detail:  err.Error(),
		})
		return
	}

	if err := validateChatProviderCentralAPIKey(
		centralAPIKeyEnabled,
		api.hasEffectiveCentralProviderAPIKey(ctx, database.ChatProvider{
			Provider:             provider,
			APIKey:               strings.TrimSpace(req.APIKey),
			BaseUrl:              baseURL,
			CentralApiKeyEnabled: centralAPIKeyEnabled,
		}, uuid.Nil),
	); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	inserted, err = api.Database.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:                   provider,
		DisplayName:                strings.TrimSpace(req.DisplayName),
		APIKey:                     strings.TrimSpace(req.APIKey),
		BaseUrl:                    baseURL,
		ApiKeyKeyID:                sql.NullString{},
		CreatedBy:                  uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		Enabled:                    enabled,
		CentralApiKeyEnabled:       centralAPIKeyEnabled,
		AllowUserApiKey:            allowUserAPIKey,
		AllowCentralApiKeyFallback: allowCentralAPIKeyFallback,
	})
	if err != nil {
		switch {
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat provider already exists.",
				Detail:  err.Error(),
			})
			return
		case database.IsCheckViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid provider.",
				Detail:  err.Error(),
			})
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat provider.",
				Detail:  err.Error(),
			})
			return
		}
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventProviders, uuid.Nil)

	httpapi.Write(
		ctx,
		rw,
		http.StatusCreated,
		convertChatProviderConfig(
			inserted,
			api.hasEffectiveProviderAPIKey(ctx, inserted),
			codersdk.ChatProviderConfigSourceDatabase,
		),
	)
}

func (api *API) updateChatProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		existing database.ChatProvider
		updated  database.ChatProvider
	)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	providerID, ok := parseChatProviderID(rw, r)
	if !ok {
		return
	}

	existing, err := api.Database.GetChatProviderByID(ctx, providerID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat provider.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.UpdateChatProviderConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	displayName := existing.DisplayName
	if trimmed := strings.TrimSpace(req.DisplayName); trimmed != "" {
		displayName = trimmed
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	apiKey := existing.APIKey
	apiKeyKeyID := existing.ApiKeyKeyID
	if req.APIKey != nil {
		trimmedAPIKey := strings.TrimSpace(*req.APIKey)
		if trimmedAPIKey != "" {
			if err := validateChatProviderAPIKeySize(trimmedAPIKey); err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "API key too large.",
					Detail:  err.Error(),
				})
				return
			}
		}
		apiKey = trimmedAPIKey
		apiKeyKeyID = sql.NullString{}
	}
	baseURL := existing.BaseUrl
	if req.BaseURL != nil {
		baseURL, err = normalizeChatProviderBaseURL(*req.BaseURL)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid provider base URL.",
				Detail:  err.Error(),
			})
			return
		}
	}

	centralAPIKeyEnabled := existing.CentralApiKeyEnabled
	if req.CentralAPIKeyEnabled != nil {
		centralAPIKeyEnabled = *req.CentralAPIKeyEnabled
	}
	allowUserAPIKey := existing.AllowUserApiKey
	if req.AllowUserAPIKey != nil {
		allowUserAPIKey = *req.AllowUserAPIKey
	}
	allowCentralAPIKeyFallback := existing.AllowCentralApiKeyFallback
	if req.AllowCentralAPIKeyFallback != nil {
		allowCentralAPIKeyFallback = *req.AllowCentralAPIKeyFallback
	}

	if err := validateChatProviderCredentialPolicy(
		centralAPIKeyEnabled,
		allowUserAPIKey,
		allowCentralAPIKeyFallback,
	); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid credential policy.",
			Detail:  err.Error(),
		})
		return
	}

	if err := validateChatProviderCentralAPIKey(
		centralAPIKeyEnabled,
		api.hasEffectiveCentralProviderAPIKey(ctx, database.ChatProvider{
			ID:                   existing.ID,
			Provider:             existing.Provider,
			APIKey:               apiKey,
			BaseUrl:              baseURL,
			CentralApiKeyEnabled: centralAPIKeyEnabled,
		}, existing.ID),
	); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	updated, err = api.Database.UpdateChatProvider(ctx, database.UpdateChatProviderParams{
		DisplayName:                displayName,
		APIKey:                     apiKey,
		BaseUrl:                    baseURL,
		ApiKeyKeyID:                apiKeyKeyID,
		Enabled:                    enabled,
		CentralApiKeyEnabled:       centralAPIKeyEnabled,
		AllowUserApiKey:            allowUserAPIKey,
		AllowCentralApiKeyFallback: allowCentralAPIKeyFallback,
		ID:                         existing.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat provider.",
			Detail:  err.Error(),
		})
		return
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventProviders, uuid.Nil)

	httpapi.Write(
		ctx,
		rw,
		http.StatusOK,
		convertChatProviderConfig(
			updated,
			api.hasEffectiveProviderAPIKey(ctx, updated),
			codersdk.ChatProviderConfigSourceDatabase,
		),
	)
}

func (api *API) deleteChatProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	providerID, ok := parseChatProviderID(rw, r)
	if !ok {
		return
	}

	if _, err := api.Database.GetChatProviderByID(ctx, providerID); err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat provider.",
			Detail:  err.Error(),
		})
		return
	}

	if err := api.Database.DeleteChatProviderByID(ctx, providerID); err != nil {
		if database.IsForeignKeyViolation(err,
			database.ForeignKeyChatMessagesModelConfigID,
			database.ForeignKeyChatsLastModelConfigID,
		) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Provider models are still referenced by existing chats.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat provider.",
			Detail:  err.Error(),
		})
		return
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventProviders, uuid.Nil)

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) listUserChatProviderConfigs(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	//nolint:gocritic // Non-admin users need to read provider configs to manage their own chat credentials.
	chatdCtx := dbauthz.AsChatd(ctx)
	providers, err := api.Database.GetChatProviders(chatdCtx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat providers.",
			Detail:  err.Error(),
		})
		return
	}

	userKeys, err := api.Database.GetUserChatProviderKeys(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list user chat provider keys.",
			Detail:  err.Error(),
		})
		return
	}

	hasUserAPIKeyByProviderID := make(map[uuid.UUID]bool, len(userKeys))
	for _, userKey := range userKeys {
		hasUserAPIKeyByProviderID[userKey.ChatProviderID] = true
	}

	resp := make([]codersdk.UserChatProviderConfig, 0, len(providers))
	for _, provider := range providers {
		if !provider.Enabled || !provider.AllowUserApiKey {
			continue
		}
		hasUserAPIKey := hasUserAPIKeyByProviderID[provider.ID]
		hasCentralAPIKeyFallback := provider.Enabled &&
			provider.AllowCentralApiKeyFallback &&
			api.hasEffectiveCentralProviderAPIKey(ctx, provider, uuid.Nil)
		resp = append(
			resp,
			convertUserChatProviderConfig(
				provider,
				hasUserAPIKey,
				hasCentralAPIKeyFallback,
			),
		)
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) upsertUserChatProviderKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	providerID, ok := parseChatProviderID(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // Non-admin users need to validate provider availability before storing their own key.
	provider, err := api.Database.GetChatProviderByID(dbauthz.AsChatd(ctx), providerID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat provider.",
			Detail:  err.Error(),
		})
		return
	}
	if !provider.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Provider is disabled.",
		})
		return
	}
	if !provider.AllowUserApiKey {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Provider does not allow user API keys.",
		})
		return
	}

	var req codersdk.CreateUserChatProviderKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	trimmedAPIKey := strings.TrimSpace(req.APIKey)
	if err := validateChatProviderAPIKeySize(trimmedAPIKey); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "API key too large.",
			Detail:  err.Error(),
		})
		return
	}
	if trimmedAPIKey == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "API key is required.",
		})
		return
	}

	if _, err := api.Database.UpsertUserChatProviderKey(ctx, database.UpsertUserChatProviderKeyParams{
		UserID:         apiKey.UserID,
		ChatProviderID: providerID,
		APIKey:         trimmedAPIKey,
		ApiKeyKeyID:    sql.NullString{},
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to save user chat provider key.",
			Detail:  err.Error(),
		})
		return
	}

	hasCentralAPIKeyFallback := provider.Enabled &&
		provider.AllowCentralApiKeyFallback &&
		api.hasEffectiveCentralProviderAPIKey(ctx, provider, uuid.Nil)
	httpapi.Write(
		ctx,
		rw,
		http.StatusOK,
		convertUserChatProviderConfig(
			provider,
			true,
			hasCentralAPIKeyFallback,
		),
	)
}

func (api *API) deleteUserChatProviderKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	providerID, ok := parseChatProviderID(rw, r)
	if !ok {
		return
	}

	if err := api.Database.DeleteUserChatProviderKey(ctx, database.DeleteUserChatProviderKeyParams{
		UserID:         apiKey.UserID,
		ChatProviderID: providerID,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete user chat provider key.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) listChatModelConfigs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Admin users can see all model configs (including disabled ones)
	// for management purposes. Non-admin users see only enabled
	// configs, which is sufficient for using the chat feature.
	isAdmin := api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig)

	var configs []database.ChatModelConfig
	var err error
	if isAdmin {
		configs, err = api.Database.GetChatModelConfigs(ctx)
	} else {
		//nolint:gocritic // All authenticated users need to read enabled model configs to use the chat feature.
		configs, err = api.Database.GetEnabledChatModelConfigs(dbauthz.AsSystemRestricted(ctx))
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat model configs.",
			Detail:  err.Error(),
		})
		return
	}

	resp := make([]codersdk.ChatModelConfig, 0, len(configs))
	for _, config := range configs {
		resp = append(resp, convertChatModelConfig(config))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) createChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateChatModelConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	provider := normalizeChatProvider(req.Provider)
	if provider == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid provider.",
			Detail:  chatProviderValidationDetail(),
		})
		return
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Model is required.",
		})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	if req.ContextLimit == nil || *req.ContextLimit <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Context limit is required.",
			Detail:  "context_limit must be greater than zero.",
		})
		return
	}
	contextLimit := *req.ContextLimit

	compressionThreshold, thresholdErr := normalizeChatCompressionThreshold(
		req.CompressionThreshold,
		defaultChatContextCompressionThreshold,
	)
	if thresholdErr != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid compression threshold.",
			Detail:  thresholdErr.Error(),
		})
		return
	}

	modelConfigRaw, modelConfigErr := marshalChatModelCallConfig(req.ModelConfig)
	if modelConfigErr != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid model config.",
			Detail:  modelConfigErr.Error(),
		})
		return
	}

	insertParams := database.InsertChatModelConfigParams{
		Provider:             provider,
		Model:                model,
		DisplayName:          strings.TrimSpace(req.DisplayName),
		Enabled:              enabled,
		IsDefault:            isDefault,
		ContextLimit:         contextLimit,
		CompressionThreshold: compressionThreshold,
		Options:              modelConfigRaw,
		CreatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		UpdatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
	}

	var inserted database.ChatModelConfig
	err := api.Database.InTx(func(tx database.Store) error {
		insertAsDefault := isDefault
		if !insertAsDefault {
			_, err := tx.GetDefaultChatModelConfig(ctx)
			switch {
			case err == nil:
				// A default already exists.
			case xerrors.Is(err, sql.ErrNoRows):
				insertAsDefault = true
			default:
				return xerrors.Errorf("get default model config: %w", err)
			}
		}

		if insertAsDefault {
			if err := tx.UnsetDefaultChatModelConfigs(ctx); err != nil {
				return xerrors.Errorf("unset default model configs: %w", err)
			}
		}
		insertParams.IsDefault = insertAsDefault

		config, err := tx.InsertChatModelConfig(ctx, insertParams)
		if err != nil {
			return err
		}
		inserted = config

		if err := ensureDefaultChatModelConfig(ctx, tx); err != nil {
			return err
		}

		refreshedConfig, err := tx.GetChatModelConfigByID(ctx, inserted.ID)
		if err != nil {
			return xerrors.Errorf("refresh inserted chat model config: %w", err)
		}
		inserted = refreshedConfig
		return nil
	}, nil)
	if err != nil {
		switch {
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat model config already exists.",
				Detail:  err.Error(),
			})
			return
		case database.IsForeignKeyViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Chat provider is not configured.",
				Detail:  err.Error(),
			})
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat model config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, inserted.ID)

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatModelConfig(inserted))
}

func (api *API) updateChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	modelConfigID, ok := parseChatModelConfigID(rw, r)
	if !ok {
		return
	}

	existing, err := api.Database.GetChatModelConfigByID(ctx, modelConfigID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat model config.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.UpdateChatModelConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	provider := existing.Provider
	if strings.TrimSpace(req.Provider) != "" {
		provider = normalizeChatProvider(req.Provider)
		if provider == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid provider.",
				Detail:  chatProviderValidationDetail(),
			})
			return
		}
	}

	model := existing.Model
	if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
		model = trimmed
	}

	displayName := existing.DisplayName
	if trimmed := strings.TrimSpace(req.DisplayName); trimmed != "" {
		displayName = trimmed
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	isDefault := existing.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	contextLimit := existing.ContextLimit
	if req.ContextLimit != nil {
		if *req.ContextLimit <= 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Context limit must be greater than zero.",
			})
			return
		}
		contextLimit = *req.ContextLimit
	}

	compressionThreshold, thresholdErr := normalizeChatCompressionThreshold(
		req.CompressionThreshold,
		existing.CompressionThreshold,
	)
	if thresholdErr != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid compression threshold.",
			Detail:  thresholdErr.Error(),
		})
		return
	}

	modelConfigRaw := existing.Options
	if req.ModelConfig != nil {
		encodedModelConfig, modelConfigErr := marshalChatModelCallConfig(req.ModelConfig)
		if modelConfigErr != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config.",
				Detail:  modelConfigErr.Error(),
			})
			return
		}
		modelConfigRaw = encodedModelConfig
	}

	updateParams := database.UpdateChatModelConfigParams{
		Provider:             provider,
		Model:                model,
		DisplayName:          displayName,
		Enabled:              enabled,
		IsDefault:            isDefault,
		ContextLimit:         contextLimit,
		CompressionThreshold: compressionThreshold,
		Options:              modelConfigRaw,
		UpdatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		ID:                   existing.ID,
	}

	var updated database.ChatModelConfig
	err = api.Database.InTx(func(tx database.Store) error {
		setAsDefault := updateParams.IsDefault && !existing.IsDefault
		if setAsDefault {
			if err := tx.UnsetDefaultChatModelConfigs(ctx); err != nil {
				return xerrors.Errorf("unset default model configs: %w", err)
			}
		}

		_, err := tx.UpdateChatModelConfig(ctx, updateParams)
		if err != nil {
			return err
		}

		excludeConfigID := uuid.Nil
		if existing.IsDefault && req.IsDefault != nil && !*req.IsDefault {
			excludeConfigID = existing.ID
		}

		if err := ensureDefaultChatModelConfig(
			ctx,
			tx,
			excludeConfigID,
		); err != nil {
			return err
		}

		refreshedConfig, err := tx.GetChatModelConfigByID(ctx, existing.ID)
		if err != nil {
			return xerrors.Errorf("refresh updated chat model config: %w", err)
		}
		updated = refreshedConfig
		return nil
	}, nil)
	if err != nil {
		switch {
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat model config already exists.",
				Detail:  err.Error(),
			})
			return
		case database.IsForeignKeyViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Chat provider is not configured.",
				Detail:  err.Error(),
			})
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat model config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, updated.ID)

	httpapi.Write(ctx, rw, http.StatusOK, convertChatModelConfig(updated))
}

func (api *API) deleteChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	modelConfigID, ok := parseChatModelConfigID(rw, r)
	if !ok {
		return
	}

	if _, err := api.Database.GetChatModelConfigByID(ctx, modelConfigID); err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat model config.",
			Detail:  err.Error(),
		})
		return
	}

	if err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.DeleteChatModelConfigByID(ctx, modelConfigID); err != nil {
			return err
		}
		return ensureDefaultChatModelConfig(ctx, tx)
	}, nil); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat model config.",
			Detail:  err.Error(),
		})
		return
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, modelConfigID)

	rw.WriteHeader(http.StatusNoContent)
}

func ensureDefaultChatModelConfig(
	ctx context.Context,
	tx database.Store,
	excludedConfigIDs ...uuid.UUID,
) error {
	_, err := tx.GetDefaultChatModelConfig(ctx)
	switch {
	case err == nil:
		return nil
	case !xerrors.Is(err, sql.ErrNoRows):
		return xerrors.Errorf("get default model config: %w", err)
	}

	modelConfigs, err := tx.GetChatModelConfigs(ctx)
	if err != nil {
		return xerrors.Errorf("list chat model configs: %w", err)
	}
	if len(modelConfigs) == 0 {
		return nil
	}

	candidateConfig := modelConfigs[0]
	excluded := make(map[uuid.UUID]struct{}, len(excludedConfigIDs))
	for _, configID := range excludedConfigIDs {
		if configID == uuid.Nil {
			continue
		}
		excluded[configID] = struct{}{}
	}
	for _, config := range modelConfigs {
		if _, skip := excluded[config.ID]; skip {
			continue
		}
		candidateConfig = config
		break
	}

	if err := tx.UnsetDefaultChatModelConfigs(ctx); err != nil {
		return xerrors.Errorf("unset default model configs: %w", err)
	}

	params := chatModelConfigToUpdateParams(candidateConfig)
	params.IsDefault = true
	if _, err := tx.UpdateChatModelConfig(ctx, params); err != nil {
		return xerrors.Errorf("set default model config: %w", err)
	}
	return nil
}

func chatModelConfigToUpdateParams(
	config database.ChatModelConfig,
) database.UpdateChatModelConfigParams {
	return database.UpdateChatModelConfigParams{
		Provider:             config.Provider,
		Model:                config.Model,
		DisplayName:          config.DisplayName,
		Enabled:              config.Enabled,
		IsDefault:            config.IsDefault,
		ContextLimit:         config.ContextLimit,
		CompressionThreshold: config.CompressionThreshold,
		Options:              config.Options,
		UpdatedBy:            uuid.NullUUID{},
		ID:                   config.ID,
	}
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

func writeChatUsageLimitUserNotFound(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "User not found.",
	})
}

func writeChatUsageLimitOverrideNotFound(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Chat usage limit override not found.",
	})
}

func writeChatUsageLimitGroupOverrideNotFound(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Chat usage limit group override not found.",
	})
}

func writeChatUsageLimitGroupNotFound(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Group not found.",
	})
}

func parseChatUsageLimitUserID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(chi.URLParam(r, "user"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat usage limit user ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return userID, true
}

func parseChatProviderID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	providerID, err := uuid.Parse(chi.URLParam(r, "providerConfig"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat provider ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return providerID, true
}

func parseChatModelConfigID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	modelConfigID, err := uuid.Parse(chi.URLParam(r, "modelConfig"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat model config ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return modelConfigID, true
}

func convertChatProviderConfig(
	provider database.ChatProvider,
	hasAPIKey bool,
	source codersdk.ChatProviderConfigSource,
) codersdk.ChatProviderConfig {
	displayName := strings.TrimSpace(provider.DisplayName)
	if displayName == "" {
		displayName = chatprovider.ProviderDisplayName(provider.Provider)
	}

	return codersdk.ChatProviderConfig{
		ID:                         provider.ID,
		Provider:                   provider.Provider,
		DisplayName:                displayName,
		Enabled:                    provider.Enabled,
		HasAPIKey:                  hasAPIKey,
		CentralAPIKeyEnabled:       provider.CentralApiKeyEnabled,
		AllowUserAPIKey:            provider.AllowUserApiKey,
		AllowCentralAPIKeyFallback: provider.AllowCentralApiKeyFallback,
		BaseURL:                    strings.TrimSpace(provider.BaseUrl),
		Source:                     source,
		CreatedAt:                  provider.CreatedAt,
		UpdatedAt:                  provider.UpdatedAt,
	}
}

func convertUserChatProviderConfig(
	provider database.ChatProvider,
	hasUserAPIKey bool,
	hasCentralAPIKeyFallback bool,
) codersdk.UserChatProviderConfig {
	displayName := strings.TrimSpace(provider.DisplayName)
	if displayName == "" {
		displayName = chatprovider.ProviderDisplayName(provider.Provider)
	}

	return codersdk.UserChatProviderConfig{
		ProviderID:               provider.ID,
		Provider:                 provider.Provider,
		DisplayName:              displayName,
		HasUserAPIKey:            hasUserAPIKey,
		HasCentralAPIKeyFallback: hasCentralAPIKeyFallback,
	}
}

func convertChatModelConfig(config database.ChatModelConfig) codersdk.ChatModelConfig {
	return codersdk.ChatModelConfig{
		ID:                   config.ID,
		Provider:             config.Provider,
		Model:                config.Model,
		DisplayName:          config.DisplayName,
		Enabled:              config.Enabled,
		IsDefault:            config.IsDefault,
		ContextLimit:         config.ContextLimit,
		CompressionThreshold: config.CompressionThreshold,
		ModelConfig:          unmarshalChatModelCallConfig(config.Options),
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}
}

func marshalChatModelCallConfig(
	modelConfig *codersdk.ChatModelCallConfig,
) (json.RawMessage, error) {
	if modelConfig == nil {
		return json.RawMessage("{}"), nil
	}

	if err := validateChatModelCallConfig(modelConfig); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(modelConfig)
	if err != nil {
		return nil, xerrors.Errorf("encode model config: %w", err)
	}
	return encoded, nil
}

func validateChatModelCallConfig(modelConfig *codersdk.ChatModelCallConfig) error {
	if modelConfig == nil {
		return nil
	}

	costConfig := codersdk.ModelCostConfig{}
	if modelConfig.Cost != nil {
		costConfig = *modelConfig.Cost
	}

	pricingFields := []struct {
		name  string
		value *decimal.Decimal
	}{
		{name: "cost.input_price_per_million_tokens", value: costConfig.InputPricePerMillionTokens},
		{name: "cost.output_price_per_million_tokens", value: costConfig.OutputPricePerMillionTokens},
		{name: "cost.cache_read_price_per_million_tokens", value: costConfig.CacheReadPricePerMillionTokens},
		{name: "cost.cache_write_price_per_million_tokens", value: costConfig.CacheWritePricePerMillionTokens},
	}
	for _, field := range pricingFields {
		if err := validateNonNegativeDecimalField(field.name, field.value); err != nil {
			return err
		}
	}

	return nil
}

func validateNonNegativeDecimalField(name string, value *decimal.Decimal) error {
	if value == nil {
		return nil
	}
	if value.IsNegative() {
		return xerrors.Errorf("%s must be greater than or equal to zero", name)
	}
	return nil
}

func unmarshalChatModelCallConfig(
	raw json.RawMessage,
) *codersdk.ChatModelCallConfig {
	if len(raw) == 0 {
		return nil
	}

	decoded := &codersdk.ChatModelCallConfig{}
	if err := json.Unmarshal(raw, decoded); err != nil {
		return nil
	}
	if isZeroChatModelCallConfig(decoded) {
		return nil
	}
	return decoded
}

func isZeroChatModelCallConfig(config *codersdk.ChatModelCallConfig) bool {
	if config == nil {
		return true
	}

	return config.MaxOutputTokens == nil &&
		config.Temperature == nil &&
		config.TopP == nil &&
		config.TopK == nil &&
		config.PresencePenalty == nil &&
		config.FrequencyPenalty == nil &&
		isZeroModelCostConfig(config.Cost) &&
		isZeroChatModelProviderOptions(config.ProviderOptions)
}

func isZeroModelCostConfig(cost *codersdk.ModelCostConfig) bool {
	if cost == nil {
		return true
	}

	return cost.InputPricePerMillionTokens == nil &&
		cost.OutputPricePerMillionTokens == nil &&
		cost.CacheReadPricePerMillionTokens == nil &&
		cost.CacheWritePricePerMillionTokens == nil
}

func isZeroChatModelProviderOptions(options *codersdk.ChatModelProviderOptions) bool {
	if options == nil {
		return true
	}

	return options.OpenAI == nil &&
		options.Anthropic == nil &&
		options.Google == nil &&
		options.OpenAICompat == nil &&
		options.OpenRouter == nil &&
		options.Vercel == nil
}

func normalizeChatProvider(provider string) string {
	return chatprovider.NormalizeProvider(provider)
}

func normalizeChatProviderBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", xerrors.New("Base URL must be an absolute URL with scheme and host.")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", xerrors.New("Base URL scheme must be http or https.")
	}
	return parsed.String(), nil
}

func chatProviderValidationDetail() string {
	return "Provider must be one of: " + strings.Join(chatprovider.SupportedProviders(), ", ") + "."
}

const maxChatProviderAPIKeySize = 10240 // 10 KB

func validateChatProviderAPIKeySize(apiKey string) error {
	if len(apiKey) > maxChatProviderAPIKeySize {
		return xerrors.Errorf("API key exceeds maximum size of %d bytes", maxChatProviderAPIKeySize)
	}
	return nil
}

//nolint:revive // This helper validates the explicit credential policy tuple.
func validateChatProviderCredentialPolicy(
	centralEnabled, allowUserKey, allowFallback bool,
) error {
	if !centralEnabled && !allowUserKey {
		return xerrors.New(
			"At least one credential source must be enabled: central API key or user API key.",
		)
	}
	if allowFallback && !centralEnabled {
		return xerrors.New(
			"Central API key fallback requires central API key to be enabled.",
		)
	}
	if allowFallback && !allowUserKey {
		return xerrors.New(
			"Central API key fallback requires user API key to be enabled.",
		)
	}
	return nil
}

//nolint:revive // This helper validates central-key requirements.
func validateChatProviderCentralAPIKey(
	centralEnabled bool,
	hasCentralAPIKey bool,
) error {
	if centralEnabled && !hasCentralAPIKey {
		return xerrors.New(
			"API key is required when central API key is enabled.",
		)
	}
	return nil
}

// ChatProviderAPIKeysFromDeploymentValues returns deployment-backed chat
// provider API keys.
func ChatProviderAPIKeysFromDeploymentValues(
	_ *codersdk.DeploymentValues,
) chatprovider.ProviderAPIKeys {
	// AI bridge deployment config is intentionally not reused for chat
	// provider credentials. Bridge keys serve the AI task subsystem and
	// should not silently broaden into chat execution paths.
	return chatprovider.ProviderAPIKeys{}
}

func (api *API) hasEffectiveProviderAPIKey(ctx context.Context, provider database.ChatProvider) bool {
	return api.hasEffectiveCentralProviderAPIKey(ctx, provider, uuid.Nil)
}

func (api *API) hasEffectiveCentralProviderAPIKey(
	ctx context.Context,
	provider database.ChatProvider,
	excludeProviderID uuid.UUID,
) bool {
	if !provider.CentralApiKeyEnabled {
		return false
	}
	if strings.TrimSpace(provider.APIKey) != "" {
		return true
	}
	deploymentKeys := ChatProviderAPIKeysFromDeploymentValues(api.DeploymentValues)
	if deploymentKeys.APIKey(provider.Provider) != "" {
		return true
	}
	if api.chatDaemon == nil {
		return false
	}
	//nolint:gocritic // System context required to read enabled chat providers.
	systemCtx := dbauthz.AsSystemRestricted(ctx)

	enabledProviders, err := api.Database.GetEnabledChatProviders(
		systemCtx,
	)
	if err != nil {
		api.Logger.Warn(ctx, "failed to resolve provider API keys",
			slog.F("provider", provider.Provider),
			slog.Error(err),
		)
		return false
	}

	enabledConfiguredProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(enabledProviders),
	)
	for _, configured := range enabledProviders {
		if excludeProviderID != uuid.Nil && configured.ID == excludeProviderID {
			continue
		}
		enabledConfiguredProviders = append(
			enabledConfiguredProviders, chatprovider.ConfiguredProvider{
				Provider: configured.Provider,
				APIKey:   configured.APIKey,
				BaseURL:  configured.BaseUrl,
			},
		)
	}

	effectiveKeys := chatprovider.MergeProviderAPIKeys(
		deploymentKeys,
		enabledConfiguredProviders,
	)
	return effectiveKeys.APIKey(provider.Provider) != ""
}

// @Summary Get PR insights
// @ID get-pr-insights
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param start_date query string true "Start date (RFC3339)"
// @Param end_date query string true "End date (RFC3339)"
// @Success 200 {object} codersdk.PRInsightsResponse
// @Router /chats/insights/pull-requests [get]
// @x-apidocgen {"skip": true}
func (api *API) prInsights(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Admin-only endpoint.
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	// Parse date range.
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -30)

	qp := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	startDate := p.Time(qp, defaultStart, "start_date", time.RFC3339)
	endDate := p.Time(qp, now, "end_date", time.RFC3339)
	p.ErrorExcessParams(qp)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}

	// Calculate previous period of equal length for trend comparison.
	duration := endDate.Sub(startDate)
	prevStart := startDate.Add(-duration)

	// No owner filter — admin sees all data.
	ownerID := uuid.NullUUID{}

	// Run all queries in parallel.
	var (
		currentSummary  database.GetPRInsightsSummaryRow
		previousSummary database.GetPRInsightsSummaryRow
		timeSeries      []database.GetPRInsightsTimeSeriesRow
		byModel         []database.GetPRInsightsPerModelRow
		recentPRs       []database.GetPRInsightsPullRequestsRow
	)

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(5)

	eg.Go(func() error {
		var err error
		currentSummary, err = api.Database.GetPRInsightsSummary(egCtx, database.GetPRInsightsSummaryParams{
			StartDate: startDate,
			EndDate:   endDate,
			OwnerID:   ownerID,
		})
		return err
	})

	eg.Go(func() error {
		var err error
		previousSummary, err = api.Database.GetPRInsightsSummary(egCtx, database.GetPRInsightsSummaryParams{
			StartDate: prevStart,
			EndDate:   startDate,
			OwnerID:   ownerID,
		})
		return err
	})

	eg.Go(func() error {
		var err error
		timeSeries, err = api.Database.GetPRInsightsTimeSeries(egCtx, database.GetPRInsightsTimeSeriesParams{
			StartDate: startDate,
			EndDate:   endDate,
			OwnerID:   ownerID,
		})
		return err
	})

	eg.Go(func() error {
		var err error
		byModel, err = api.Database.GetPRInsightsPerModel(egCtx, database.GetPRInsightsPerModelParams{
			StartDate: startDate,
			EndDate:   endDate,
			OwnerID:   ownerID,
		})
		return err
	})

	eg.Go(func() error {
		var err error
		recentPRs, err = api.Database.GetPRInsightsPullRequests(egCtx, database.GetPRInsightsPullRequestsParams{
			StartDate: startDate,
			EndDate:   endDate,
			OwnerID:   ownerID,
		})
		return err
	})

	if err := eg.Wait(); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Build summary with computed fields.
	summary := codersdk.PRInsightsSummary{
		TotalPRsCreated:     currentSummary.TotalPrsCreated,
		TotalPRsMerged:      currentSummary.TotalPrsMerged,
		TotalAdditions:      currentSummary.TotalAdditions,
		TotalDeletions:      currentSummary.TotalDeletions,
		TotalCostMicros:     currentSummary.TotalCostMicros,
		PrevTotalPRsCreated: previousSummary.TotalPrsCreated,
		PrevTotalPRsMerged:  previousSummary.TotalPrsMerged,
	}
	if summary.TotalPRsCreated > 0 {
		summary.MergeRate = float64(summary.TotalPRsMerged) / float64(summary.TotalPRsCreated)
	}
	if summary.TotalPRsMerged > 0 {
		summary.CostPerMergedPRMicros = currentSummary.MergedCostMicros / summary.TotalPRsMerged
	}
	if summary.PrevTotalPRsCreated > 0 {
		summary.PrevMergeRate = float64(summary.PrevTotalPRsMerged) / float64(summary.PrevTotalPRsCreated)
	}
	if summary.PrevTotalPRsMerged > 0 {
		summary.PrevCostPerMergedPRMicros = previousSummary.MergedCostMicros / summary.PrevTotalPRsMerged
	}

	// Convert time series.
	tsEntries := make([]codersdk.PRInsightsTimeSeriesEntry, 0, len(timeSeries))
	for _, ts := range timeSeries {
		tsEntries = append(tsEntries, codersdk.PRInsightsTimeSeriesEntry{
			Date:       ts.Date,
			PRsCreated: ts.PrsCreated,
			PRsMerged:  ts.PrsMerged,
			PRsClosed:  ts.PrsClosed,
		})
	}

	// Convert model breakdown.
	modelEntries := make([]codersdk.PRInsightsModelBreakdown, 0, len(byModel))
	for _, m := range byModel {
		entry := codersdk.PRInsightsModelBreakdown{
			ModelConfigID:   m.ModelConfigID.UUID,
			DisplayName:     m.DisplayName,
			Provider:        m.Provider,
			TotalPRs:        m.TotalPrs,
			MergedPRs:       m.MergedPrs,
			TotalAdditions:  m.TotalAdditions,
			TotalDeletions:  m.TotalDeletions,
			TotalCostMicros: m.TotalCostMicros,
		}
		if entry.TotalPRs > 0 {
			entry.MergeRate = float64(entry.MergedPRs) / float64(entry.TotalPRs)
		}
		if entry.MergedPRs > 0 {
			entry.CostPerMergedPRMicros = m.MergedCostMicros / entry.MergedPRs
		}
		modelEntries = append(modelEntries, entry)
	}

	// Convert recent PRs.
	prEntries := make([]codersdk.PRInsightsPullRequest, 0, len(recentPRs))
	for _, pr := range recentPRs {
		entry := codersdk.PRInsightsPullRequest{
			ChatID:           pr.ChatID,
			PRTitle:          pr.PrTitle,
			Draft:            pr.Draft,
			Additions:        pr.Additions,
			Deletions:        pr.Deletions,
			ChangedFiles:     pr.ChangedFiles,
			ChangesRequested: pr.ChangesRequested,
			BaseBranch:       pr.BaseBranch,
			ModelDisplayName: pr.ModelDisplayName,
			CostMicros:       pr.CostMicros,
			CreatedAt:        pr.CreatedAt,
		}
		if pr.PrUrl.Valid {
			entry.PRURL = &pr.PrUrl.String
		}
		if pr.PrNumber.Valid {
			entry.PRNumber = &pr.PrNumber.Int32
		}
		if pr.State.Valid {
			entry.State = pr.State.String
		}
		if pr.Commits.Valid {
			entry.Commits = &pr.Commits.Int32
		}
		if pr.Approved.Valid {
			entry.Approved = &pr.Approved.Bool
		}
		if pr.ReviewerCount.Valid {
			entry.ReviewerCount = &pr.ReviewerCount.Int32
		}
		if pr.AuthorLogin.Valid {
			entry.AuthorLogin = &pr.AuthorLogin.String
		}
		if pr.AuthorAvatarUrl.Valid {
			entry.AuthorAvatarURL = &pr.AuthorAvatarUrl.String
		}
		prEntries = append(prEntries, entry)
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.PRInsightsResponse{
		Summary:      summary,
		TimeSeries:   tsEntries,
		ByModel:      modelEntries,
		PullRequests: prEntries,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) postChatToolResults(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	apiKey := httpmw.APIKey(r)

	// Gate tool-result submission behind agents-access.
	// Submitting tool results resumes AI/LLM inference on
	// a chat in requires_action state.
	// See: https://github.com/coder/coder/issues/24250
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use.
	r.Body = http.MaxBytesReader(rw, r.Body, int64(2*maxSystemPromptLenBytes))
	var req codersdk.SubmitToolResultsRequest

	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if len(req.Results) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one tool result is required.",
		})
		return
	}

	// Fast-path check outside the transaction. The authoritative
	// check happens inside SubmitToolResults under a row lock.
	if chat.Status != database.ChatStatusRequiresAction {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Chat is not waiting for tool results.",
			Detail:  fmt.Sprintf("Chat status is %q, expected %q.", chat.Status, database.ChatStatusRequiresAction),
		})
		return
	}

	var dynamicTools json.RawMessage
	if chat.DynamicTools.Valid {
		dynamicTools = chat.DynamicTools.RawMessage
	}

	err := api.chatDaemon.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        apiKey.UserID,
		ModelConfigID: chat.LastModelConfigID,
		Results:       req.Results,
		DynamicTools:  dynamicTools,
	})
	if err != nil {
		var validationErr *chatd.ToolResultValidationError
		var conflictErr *chatd.ToolResultStatusConflictError
		switch {
		case errors.As(err, &conflictErr):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not waiting for tool results.",
				Detail:  err.Error(),
			})
		case errors.As(err, &validationErr):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: validationErr.Message,
				Detail:  validationErr.Detail,
			})
		default:
			api.Logger.Error(ctx, "tool results submission failed",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error submitting tool results.",
			})
		}
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// getChatDebugRuns returns a list of debug run summaries for a chat.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatDebugRuns(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	const maxDebugRuns = 100
	runs, err := api.Database.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   chat.ID,
		LimitVal: maxDebugRuns,
	})
	if err != nil {
		// The chat may have been deleted or access revoked between
		// middleware extraction and this query (dbauthz re-authorizes
		// on read). Surface those races as 404 to match the rest of
		// this API and avoid leaking backend details.
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching debug runs.",
			Detail:  err.Error(),
		})
		return
	}

	summaries := make([]codersdk.ChatDebugRunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, db2sdk.ChatDebugRunSummary(run))
	}
	httpapi.Write(ctx, rw, http.StatusOK, summaries)
}

// getChatDebugRun returns a single debug run with its steps.
// EXPERIMENTAL
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatDebugRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	runIDStr := chi.URLParam(r, "debugRun")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid debug run ID.",
			Detail:  err.Error(),
		})
		return
	}

	run, err := api.Database.GetChatDebugRunByID(ctx, runID)
	if err != nil {
		// Treat both not-found and authorization failures as 404 to
		// avoid leaking the existence of runs the caller cannot access.
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching debug run.",
			Detail:  err.Error(),
		})
		return
	}

	// Verify the run belongs to this chat.
	if run.ChatID != chat.ID {
		httpapi.ResourceNotFound(rw)
		return
	}

	steps, err := api.Database.GetChatDebugStepsByRunID(ctx, run.ID)
	if err != nil {
		// The run may have been deleted or access may have changed
		// between the two queries. Treat not-found/authz errors as
		// 404 for consistency with the run lookup above.
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching debug steps.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatDebugRunDetail(run, steps))
}
