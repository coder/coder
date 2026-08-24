package coderd

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/cryptokeys"
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
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
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

var allowedReasoningEffortValues = strings.Join(codersdk.ChatModelReasoningEffortValues(), ", ")

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

// Avoid returning raw dispatch errors, which may expose deployment internals.
func writeChatHookDispatchFailed(ctx context.Context, rw http.ResponseWriter, hookErr *dispatch.Error) {
	httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.ChatHookDispatchFailedResponse{
		Response: codersdk.Response{
			Message: "Chat lifecycle hook dispatch failed.",
			Detail:  fmt.Sprintf("Lifecycle hook dispatch %s failed (%s).", hookErr.DispatchID, hookErr.Class),
		},
		Kind: codersdk.ChatErrorKindHookDispatchFailed,
	})
}

// writeChatHookErr writes the response for lifecycle hook denials and
// dispatch failures, reporting whether it handled the error. The fallback
// message is used when the hook denies without a user message.
func writeChatHookErr(ctx context.Context, rw http.ResponseWriter, err error, deniedFallback string) bool {
	if denied, ok := errors.AsType[*chathooks.UserPromptDeniedError](err); ok {
		message := denied.UserMessage
		if message == "" {
			message = deniedFallback
		}
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.ChatHookDeniedResponse{
			Response: codersdk.Response{Message: message},
			Kind:     codersdk.ChatErrorKindHookDenied,
		})
		return true
	}
	if hookErr, ok := errors.AsType[*dispatch.Error](err); ok {
		writeChatHookDispatchFailed(ctx, rw, hookErr)
		return true
	}
	return false
}

// AI Gateway budget rejections and provider quota failures classify as usage
// limits; synchronous generation reports them as conflicts instead of 500s.
func maybeWriteChatUsageLimitError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	classified := chaterror.Classify(err)
	if classified.Kind != codersdk.ChatErrorKindUsageLimit {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
		Message: classified.Message,
	})
	return true
}

// statusClientClosedRequest is nginx's non-standard 499 status code,
// used here to distinguish a client-initiated cancel from a server-
// side failure when the manual title generation context is canceled.
const statusClientClosedRequest = 499

// maybeWriteManualTitleTimeoutErr translates context-cancel or
// title-timeout errors from the manual title pipeline into friendly
// 499/504 responses instead of a raw 500 that leaks the wrapped error
// chain. The errors bubble up wrapped, so match with errors.Is. Returns
// true when a response was written.
//
// The 499 branch additionally requires the request context itself to be
// canceled. A provider error can wrap context.Canceled (for example an
// upstream 401) while the caller context is still active; without the
// ctx.Err() guard such a provider failure would be misreported as a
// client-closed request instead of surfacing through the 500 path.
//
// The 504 branch keys off chatd.ErrManualTitleTimedOut, which chatd
// attaches only when a title deadline (per-attempt or overall walk
// budget) actually expired. A provider failure whose chain merely
// contains an unrelated transport deadline is not tagged and keeps its
// provider-failure surface.
func maybeWriteManualTitleTimeoutErr(ctx context.Context, rw http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, context.Canceled) && errors.Is(ctx.Err(), context.Canceled):
		httpapi.Write(ctx, rw, statusClientClosedRequest, codersdk.Response{
			Message: "Title generation was canceled.",
		})
		return true
	case errors.Is(err, chatd.ErrManualTitleTimedOut):
		httpapi.Write(ctx, rw, http.StatusGatewayTimeout, codersdk.Response{
			Message: "Title generation timed out. Try again or rename manually.",
		})
		return true
	}
	return false
}

// requireChatDaemon reports whether the chat daemon exists, writing a 503
// Service Unavailable with a remediation message when it does not. The
// daemon is nil when the in-memory AI Gateway is disabled by deployment
// config. Operations that depend on it (creating, mutating, or streaming a
// chat) must call this; pure reads (e.g. getChat) do not.
func (api *API) requireChatDaemon(ctx context.Context, rw http.ResponseWriter) bool {
	if api.chatDaemon != nil {
		return true
	}
	httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
		Message: "AI Gateway must be enabled for Coder Agents functionality. Please contact your deployment administrator.",
		Detail:  "Set CODER_AI_GATEWAY_ENABLED=true (or ai-gateway-enabled in deployment YAML) to enable.",
	})
	return false
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
//
// @Summary Watch chat events for a user via WebSockets
// @ID watch-chat-events-for-a-user-via-websockets
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {object} codersdk.ChatWatchEvent
// @Router /api/experimental/chats/watch [get]
// @Description Experimental: this endpoint is subject to change.
func (api *API) watchChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	logger := api.Logger.Named("chat_watcher")

	// Subscribe before accepting the websocket so the subscription
	// is active when the client's Dial returns.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		encoder      *json.Encoder
		encoderReady = make(chan struct{})
		// Capture before WebsocketNetConn reassigns ctx (data race).
		ctxDone = ctx.Done()
	)

	cancelSubscribe, err := api.Pubsub.SubscribeWithErr(pubsub.ChatWatchEventChannel(apiKey.UserID),
		pubsub.HandleChatWatchEvent(
			func(cbCtx context.Context, payload codersdk.ChatWatchEvent, err error) {
				if err != nil {
					logger.Error(cbCtx, "chat watch event subscription error", slog.Error(err))
					return
				}
				select {
				case <-encoderReady:
				case <-ctxDone:
					return
				case <-cbCtx.Done():
					return
				}

				// encoderReady may close with encoder still nil on error paths.
				if encoder == nil {
					return
				}
				// The encoder is only written from the pubsub delivery
				// goroutine, which processes messages serially. Do not
				// add a second write path without synchronization.
				if err := encoder.Encode(payload); err != nil {
					logger.Debug(cbCtx, "failed to send chat watch event", slog.Error(err))
					cancel()
					return
				}
			},
		))
	if err != nil {
		close(encoderReady)
		logger.Error(ctx, "failed to subscribe to chat watch events", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to subscribe to chat events.",
			Detail:  err.Error(),
		})
		return
	}
	defer cancelSubscribe()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		close(encoderReady)
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat watch stream.",
			Detail:  err.Error(),
		})
		return
	}

	_ = conn.CloseRead(context.Background())

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close()

	ctx = api.wsWatcher.Watch(ctx, logger, conn)

	encoder = json.NewEncoder(wsNetConn)
	close(encoderReady)

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
//     <at>Router /api/experimental/chats/by-workspace [post]
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
//
// @Summary List chats
// @ID list-chats
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param q query string false "Search query. Supports `title:<substring>` (case-insensitive, quote multi-word values), `archived:bool`, `has_unread:bool`, `pr_status:<draft\|open\|merged\|closed>` as repeated or comma-separated values, `source:<created_by_me\|shared_with_me>`, `diff_url:<url>` (quote values containing colons), `pr:<number>` (exact PR number match), `repo:<owner/repo>` (case-insensitive substring match against git remote origin or URL), `pr_title:<text>` (case-insensitive PR title substring), `search:<text>` (full-text search across chat titles, PR titles, PR numbers, and message bodies; quote multi-word values; cannot be combined with title, pr_title, or pr; a value that tokenizes to no searchable words returns an empty list). Bare terms are not supported; use `title:<value>` or `search:<value>`."
// @Param label query string false "Filter by label as key:value. Repeat for multiple (AND logic)."
// @Success 200 {array} codersdk.Chat
// @Router /api/experimental/chats [get]
// @Description Experimental: this endpoint is subject to change.
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

	var sharedWithGroupIDs []string
	if searchParams.SharedOnly {
		groups, err := api.Database.GetGroups(ctx, database.GetGroupsParams{HasMemberID: apiKey.UserID})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to list chats.",
				Detail:  err.Error(),
			})
			return
		}
		sharedWithGroupIDs = make([]string, 0, len(groups))
		for _, group := range groups {
			sharedWithGroupIDs = append(sharedWithGroupIDs, group.Group.ID.String())
		}
	}

	params := database.GetChatsParams{
		OwnedOnly:           searchParams.OwnedOnly,
		ViewerID:            apiKey.UserID,
		SharedOnly:          searchParams.SharedOnly,
		SharedWithUserID:    apiKey.UserID,
		SharedWithGroupIds:  sharedWithGroupIDs,
		Archived:            searchParams.Archived,
		AfterID:             paginationParams.AfterID,
		LabelFilter:         labelFilter,
		DiffURL:             searchParams.DiffURL,
		TitleQuery:          searchParams.TitleQuery,
		HasUnread:           searchParams.HasUnread,
		PullRequestStatuses: searchParams.PullRequestStatuses,
		PrNumber:            searchParams.PrNumber,
		RepoQuery:           searchParams.RepoQuery,
		PrTitleQuery:        searchParams.PrTitleQuery,
		Search:              searchParams.Search,
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

	sdkChats := db2sdk.ChatRowsWithChildren(chatRows, childRows, diffStatusesByChatID)
	api.enrichChatsWithMissingAgentIDs(ctx, sdkChats)
	httpapi.Write(ctx, rw, http.StatusOK, sdkChats)
}

// enrichChatsWithMissingAgentIDs skips existing bindings on list reads to avoid
// one authorization check per bound workspace.
func (api *API) enrichChatsWithMissingAgentIDs(ctx context.Context, chats []codersdk.Chat) {
	api.enrichChatAgentIDs(ctx, chats, func(chat *codersdk.Chat) bool {
		return chat.AgentID == nil
	})
}

// repairChatAgentIDs handles stale bindings left by workspace rebuilds. List
// reads skip this work to avoid authorization checks for bound workspaces.
func (api *API) repairChatAgentIDs(ctx context.Context, chats []codersdk.Chat) {
	api.enrichChatAgentIDs(ctx, chats, func(*codersdk.Chat) bool {
		return true
	})
}

// enrichChatAgentIDs performs best-effort response-only updates.
func (api *API) enrichChatAgentIDs(ctx context.Context, chats []codersdk.Chat, shouldEnrich func(*codersdk.Chat) bool) {
	candidateChats := make([]*codersdk.Chat, 0, len(chats))
	var workspaceIDs []uuid.UUID
	addCandidate := func(chat *codersdk.Chat) {
		if chat.WorkspaceID == nil || !shouldEnrich(chat) {
			return
		}
		candidateChats = append(candidateChats, chat)
		workspaceIDs = append(workspaceIDs, *chat.WorkspaceID)
	}
	for i := range chats {
		addCandidate(&chats[i])
		for j := range chats[i].Children {
			addCandidate(&chats[i].Children[j])
		}
	}
	if len(candidateChats) == 0 {
		return
	}

	slices.SortFunc(workspaceIDs, func(a, b uuid.UUID) int {
		return cmp.Compare(a.String(), b.String())
	})
	ids := slices.Compact(workspaceIDs)
	rows, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(ctx, ids)
	if err != nil {
		return
	}

	agentsByWorkspace := make(map[uuid.UUID][]database.WorkspaceAgent)
	latestBuildIDs := make(map[uuid.UUID]uuid.UUID)
	for _, row := range rows {
		agentsByWorkspace[row.WorkspaceID] = append(agentsByWorkspace[row.WorkspaceID], row.WorkspaceAgent)
		latestBuildIDs[row.WorkspaceID] = row.BuildID
	}
	agentIDs := make(map[uuid.UUID]uuid.UUID, len(agentsByWorkspace))
	for workspaceID, agents := range agentsByWorkspace {
		agent, err := agentselect.FindChatAgent(agents)
		if err != nil {
			api.Logger.Debug(ctx, "failed to select chat agent for enrichment", slog.F("workspace_id", workspaceID), slog.Error(err))
			continue
		}
		agentIDs[workspaceID] = agent.ID
	}

	for _, chat := range candidateChats {
		// Preserve bindings that still resolve in the latest build instead
		// of replacing them with the selected agent.
		if chat.AgentID != nil && slices.ContainsFunc(
			agentsByWorkspace[*chat.WorkspaceID],
			func(agent database.WorkspaceAgent) bool { return agent.ID == *chat.AgentID },
		) {
			continue
		}
		if agentID, ok := agentIDs[*chat.WorkspaceID]; ok {
			id := agentID
			chat.AgentID = &id
			// Pair the agent with its build so the response never mixes
			// the latest build's agent with a previous build's ID.
			buildID := latestBuildIDs[*chat.WorkspaceID]
			chat.BuildID = &buildID
		}
	}
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

func lookupEnabledChatModelConfigByID(
	ctx context.Context,
	db database.Store,
	id uuid.UUID,
) (database.ChatModelConfig, error) {
	return db.GetEnabledChatModelConfigByID(ctx, id)
}

func parseChatModelCallConfig(options json.RawMessage) (*codersdk.ChatModelCallConfig, error) {
	callConfig := &codersdk.ChatModelCallConfig{}
	if len(options) == 0 {
		return callConfig, nil
	}
	if err := json.Unmarshal(options, callConfig); err != nil {
		return nil, err
	}
	return callConfig, nil
}

func validateChatModelOverrideEffort(
	modelConfig database.ChatModelConfig,
	effort *string,
) (int, *codersdk.Response) {
	if effort == nil {
		return 0, nil
	}
	if !chatprovider.IsValidReasoningEffort(*effort) {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid reasoning_effort value.",
			Detail:  "Must be one of none, minimal, low, medium, high, xhigh, max.",
		}
	}
	callConfig, err := parseChatModelCallConfig(modelConfig.Options)
	if err != nil {
		return http.StatusInternalServerError, &codersdk.Response{
			Message: "Internal error validating reasoning effort.",
			Detail:  err.Error(),
		}
	}
	selectableEfforts := chatprovider.SelectableReasoningEfforts(callConfig.ReasoningEffort)
	if len(selectableEfforts) == 0 {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid reasoning_effort value.",
			Detail:  "This model does not support reasoning effort.",
		}
	}
	if !slices.Contains(selectableEfforts, *effort) {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid reasoning_effort value.",
			Detail:  "Must be one of " + strings.Join(selectableEfforts, ", ") + ".",
		}
	}
	return 0, nil
}

func validateChatModelOverride(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	id *uuid.UUID,
	effort *string,
) (int, *codersdk.Response) {
	if id == nil {
		if effort != nil {
			return http.StatusBadRequest, &codersdk.Response{
				Message: "reasoning_effort requires model_config_id.",
			}
		}
		return 0, nil
	}
	if *id == uuid.Nil {
		return http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id.",
		}
	}
	modelConfig, err := lookupEnabledChatModelConfigByID(ctx, db, *id)
	if err == nil && modelConfig.OrganizationID != organizationID {
		err = sql.ErrNoRows
	}
	if err != nil {
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
	return validateChatModelOverrideEffort(modelConfig, effort)
}

func parseChatModelOverrideContext(raw string) (codersdk.ChatModelOverrideContext, error) {
	overrideContext := codersdk.ChatModelOverrideContext(raw)
	if overrideContext.Valid() {
		return overrideContext, nil
	}
	return "", xerrors.Errorf("unknown chat model override context %q", raw)
}

var chatPersonalModelOverrideContexts = []codersdk.ChatPersonalModelOverrideContext{
	codersdk.ChatPersonalModelOverrideContextRoot,
	codersdk.ChatPersonalModelOverrideContextGeneral,
	codersdk.ChatPersonalModelOverrideContextExplore,
}

func parseChatPersonalModelOverrideContext(raw string) (codersdk.ChatPersonalModelOverrideContext, bool) {
	c := codersdk.ChatPersonalModelOverrideContext(raw)
	return c, slices.Contains(chatPersonalModelOverrideContexts, c)
}

func chatPersonalModelOverrideContextsJoined() string {
	values := make([]string, 0, len(chatPersonalModelOverrideContexts))
	for _, overrideContext := range chatPersonalModelOverrideContexts {
		values = append(values, string(overrideContext))
	}
	return strings.Join(values, ", ")
}

func defaultChatPersonalModelOverrideMode(
	overrideContext codersdk.ChatPersonalModelOverrideContext,
) codersdk.ChatPersonalModelOverrideMode {
	if overrideContext == codersdk.ChatPersonalModelOverrideContextRoot {
		return codersdk.ChatPersonalModelOverrideModeChatDefault
	}
	return codersdk.ChatPersonalModelOverrideModeDeploymentDefault
}

func chatPersonalModelOverrideResponse(
	overrideContext codersdk.ChatPersonalModelOverrideContext,
	row *database.ChatUserModelOverride,
) codersdk.ChatPersonalModelOverride {
	response := codersdk.ChatPersonalModelOverride{
		Context: overrideContext,
		Mode:    defaultChatPersonalModelOverrideMode(overrideContext),
	}
	if row == nil {
		return response
	}
	response.Mode = codersdk.ChatPersonalModelOverrideMode(row.Mode)
	response.IsSet = true
	if row.ModelConfigID.Valid {
		response.ModelConfigID = row.ModelConfigID.UUID.String()
	}
	if row.ReasoningEffort.Valid {
		response.ReasoningEffort = &row.ReasoningEffort.String
	}
	return response
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func chatOrganizationModelOverrideResponse(
	row database.ChatOrganizationModelOverride,
) codersdk.ChatModelOverrideResponse {
	response := codersdk.ChatModelOverrideResponse{
		Context:       codersdk.ChatModelOverrideContext(row.Context),
		ModelConfigID: row.ModelConfigID.String(),
	}
	if row.ReasoningEffort.Valid {
		response.ReasoningEffort = &row.ReasoningEffort.String
	}
	return response
}

func (api *API) chatPersonalModelOverrideDeploymentDefaultResponse(
	ctx context.Context,
	organizationID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (codersdk.ChatModelOverrideResponse, error) {
	row, err := api.Database.GetChatOrganizationModelOverride(ctx, database.GetChatOrganizationModelOverrideParams{
		OrganizationID: organizationID,
		Context:        string(overrideContext),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return codersdk.ChatModelOverrideResponse{Context: overrideContext}, nil
	}
	if err != nil {
		return codersdk.ChatModelOverrideResponse{}, err
	}
	return chatOrganizationModelOverrideResponse(row), nil
}

func (api *API) chatPersonalModelOverrideDeploymentDefaults(
	ctx context.Context,
	organizationID uuid.UUID,
) (codersdk.ChatPersonalModelOverrideDeploymentDefaults, error) {
	general, err := api.chatPersonalModelOverrideDeploymentDefaultResponse(
		ctx,
		organizationID,
		codersdk.ChatModelOverrideContextGeneral,
	)
	if err != nil {
		return codersdk.ChatPersonalModelOverrideDeploymentDefaults{}, err
	}
	explore, err := api.chatPersonalModelOverrideDeploymentDefaultResponse(
		ctx,
		organizationID,
		codersdk.ChatModelOverrideContextExplore,
	)
	if err != nil {
		return codersdk.ChatPersonalModelOverrideDeploymentDefaults{}, err
	}
	return codersdk.ChatPersonalModelOverrideDeploymentDefaults{
		General: general,
		Explore: explore,
	}, nil
}

type userChatModelAvailability struct {
	configuredProviders []chatprovider.ConfiguredProvider
	enabledModels       []database.ChatModelConfig
	providerStatusByID  map[uuid.UUID]chatprovider.ProviderAvailability
	enabledProviderIDs  map[uuid.UUID]struct{}
}

// chatModelConfigUnavailableReason reports why a model config cannot be used.
// The empty value means the model config is available. Callers must check the
// error returned by userCanUseChatModelConfig before interpreting this value.
type chatModelConfigUnavailableReason string

const (
	chatModelConfigAvailable                          chatModelConfigUnavailableReason = ""
	chatModelConfigUnavailableModelNotFoundOrDisabled chatModelConfigUnavailableReason = "model_not_found_or_disabled"
	chatModelConfigUnavailableProviderDisabled        chatModelConfigUnavailableReason = "provider_disabled"
	chatModelConfigUnavailableCredentialsMissing      chatModelConfigUnavailableReason = "credentials_missing"
	chatModelConfigUnavailableOutsideOrganization     chatModelConfigUnavailableReason = "outside_organization"
)

// getUserChatProviderAvailability returns the enabled chat providers and models
// the user can access in one organization. Provider configuration uses Chatd
// access. Model configs and user keys use the caller's authorization context.
func (api *API) getUserChatProviderAvailability(
	ctx context.Context,
	userID uuid.UUID,
	organizationID uuid.UUID,
) (userChatModelAvailability, error) {
	//nolint:gocritic // Chatd context is required to read enabled chat providers.
	chatdCtx := dbauthz.AsChatd(ctx)
	enabledProviders, err := api.Database.GetAIProviders(chatdCtx, database.GetAIProvidersParams{})
	if err != nil {
		return userChatModelAvailability{}, err
	}
	modelRows, err := api.Database.GetEnabledChatModelConfigsByOrganization(ctx, organizationID)
	if err != nil {
		return userChatModelAvailability{}, err
	}
	effectiveModels := database.DeriveEffectiveChatModelConfigs(modelRows)
	enabledModels := make([]database.ChatModelConfig, 0, len(effectiveModels.Configs))
	for _, row := range effectiveModels.Configs {
		enabledModels = append(enabledModels, row.ChatModelConfig)
	}

	configuredProviders, err := api.configuredProvidersFromAIProviders(chatdCtx, enabledProviders)
	if err != nil {
		return userChatModelAvailability{}, err
	}
	availability := userChatModelAvailability{
		configuredProviders: configuredProviders,
		enabledModels:       enabledModels,
		enabledProviderIDs:  make(map[uuid.UUID]struct{}, len(enabledProviders)),
		providerStatusByID:  make(map[uuid.UUID]chatprovider.ProviderAvailability, len(enabledProviders)),
	}
	for _, configuredProvider := range configuredProviders {
		if configuredProvider.ProviderID != uuid.Nil {
			availability.enabledProviderIDs[configuredProvider.ProviderID] = struct{}{}
		}
	}
	userKeys := []chatprovider.UserProviderKey{}
	if api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value() {
		userKeyStatus, err := api.userAIProviderKeyStatusByProviderID(ctx, userID)
		if err != nil {
			return userChatModelAvailability{}, err
		}
		userKeys = make([]chatprovider.UserProviderKey, 0, len(userKeyStatus))
		for providerID, configured := range userKeyStatus {
			if configured {
				userKeys = append(userKeys, chatprovider.UserProviderKey{ChatProviderID: providerID, APIKey: "configured"})
			}
		}
	}

	fallbackKeys := ChatProviderAPIKeysFromDeploymentValues(api.DeploymentValues)
	for _, configuredProvider := range configuredProviders {
		normalizedProvider := chatprovider.NormalizeProvider(configuredProvider.Provider)
		if normalizedProvider == "" {
			continue
		}
		_, providerStatus := chatprovider.ResolveUserProviderKeys(
			fallbackKeys,
			[]chatprovider.ConfiguredProvider{configuredProvider},
			userKeys,
		)
		status, ok := providerStatus[normalizedProvider]
		if ok && configuredProvider.ProviderID != uuid.Nil {
			availability.providerStatusByID[configuredProvider.ProviderID] = status
		}
	}
	return availability, nil
}

// userCanUseChatModelConfig returns chatModelConfigAvailable when the user can
// use the model config. If err is non-nil, callers must ignore the returned
// reason because it may be the zero-value availability sentinel.
func (api *API) userCanUseChatModelConfig(
	ctx context.Context,
	userID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, chatModelConfigUnavailableReason, error) {
	if modelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, chatModelConfigUnavailableModelNotFoundOrDisabled, nil
	}
	model, err := api.Database.GetChatModelConfigByID(ctx, modelConfigID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
			return database.ChatModelConfig{}, chatModelConfigUnavailableModelNotFoundOrDisabled, nil
		}
		return database.ChatModelConfig{}, chatModelConfigAvailable, err
	}
	if model.OrganizationID != organizationID {
		return database.ChatModelConfig{}, chatModelConfigUnavailableOutsideOrganization, nil
	}
	if !model.Enabled {
		return database.ChatModelConfig{}, chatModelConfigUnavailableModelNotFoundOrDisabled, nil
	}

	availability, err := api.getUserChatProviderAvailability(ctx, userID, organizationID)
	if err != nil {
		return database.ChatModelConfig{}, chatModelConfigAvailable, err
	}
	if model.AIProviderID.Valid {
		providerID := model.AIProviderID.UUID
		if _, ok := availability.enabledProviderIDs[providerID]; !ok {
			return database.ChatModelConfig{}, chatModelConfigUnavailableProviderDisabled, nil
		}
		providerStatus, ok := availability.providerStatusByID[providerID]
		if !ok {
			return database.ChatModelConfig{}, chatModelConfigUnavailableProviderDisabled, nil
		}
		if !providerStatus.Available {
			return database.ChatModelConfig{}, chatModelConfigUnavailableCredentialsMissing, nil
		}
		return model, chatModelConfigAvailable, nil
	}
	// Active configs always carry a provider FK (CHECK
	// chat_model_configs_ai_provider_required_when_active), so an unset FK
	// means the config is not usable.
	return database.ChatModelConfig{}, chatModelConfigUnavailableModelNotFoundOrDisabled, nil
}

func validateUserChatModelConfigAvailability(
	modelConfig database.ChatModelConfig,
	reason chatModelConfigUnavailableReason,
) (database.ChatModelConfig, int, *codersdk.Response) {
	switch reason {
	case chatModelConfigAvailable:
		return modelConfig, 0, nil
	case chatModelConfigUnavailableModelNotFoundOrDisabled,
		chatModelConfigUnavailableOutsideOrganization:
		return database.ChatModelConfig{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id: model config not found or disabled.",
		}
	case chatModelConfigUnavailableCredentialsMissing:
		return database.ChatModelConfig{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id: provider credentials unavailable for this model.",
		}
	case chatModelConfigUnavailableProviderDisabled:
		return database.ChatModelConfig{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id: provider is not enabled for this model.",
		}
	default:
		return database.ChatModelConfig{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Invalid model_config_id.",
		}
	}
}

func (api *API) validateUserChatModelConfigAvailable(
	ctx context.Context,
	userID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, int, *codersdk.Response) {
	modelConfig, reason, err := api.userCanUseChatModelConfig(ctx, userID, organizationID, modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, http.StatusInternalServerError, &codersdk.Response{
			Message: "Internal error validating model config override.",
			Detail:  err.Error(),
		}
	}
	if reason != chatModelConfigAvailable &&
		reason != chatModelConfigUnavailableModelNotFoundOrDisabled &&
		reason != chatModelConfigUnavailableCredentialsMissing &&
		reason != chatModelConfigUnavailableProviderDisabled &&
		reason != chatModelConfigUnavailableOutsideOrganization {
		api.Logger.Warn(ctx,
			"unknown chat model config availability reason",
			slog.F("user_id", userID),
			slog.F("model_config_id", modelConfigID),
			slog.F("reason", reason),
		)
	}
	return validateUserChatModelConfigAvailability(modelConfig, reason)
}

// validateExplicitChatModelConfigAvailable validates a caller-supplied
// model config ID. A nil ID keeps the chat's current model and is
// validated by the daemon's fallback resolution instead.
func (api *API) validateExplicitChatModelConfigAvailable(
	ctx context.Context,
	userID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) (int, *codersdk.Response) {
	if modelConfigID == uuid.Nil {
		return 0, nil
	}
	_, status, resp := api.validateUserChatModelConfigAvailable(ctx, userID, organizationID, modelConfigID)
	return status, resp
}

func validateChatMCPServerIDs(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	ids []uuid.UUID,
) (normalized []uuid.UUID, invalid []uuid.UUID, err error) {
	unique := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return unique, nil, nil
	}

	configs, err := db.GetEnabledMCPServerConfigsByOrganizationAndIDs(ctx, database.GetEnabledMCPServerConfigsByOrganizationAndIDsParams{
		OrganizationID: organizationID,
		IDs:            unique,
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("get enabled MCP server configs for organization: %w", err)
	}

	valid := make(map[uuid.UUID]struct{}, len(configs))
	for _, config := range configs {
		valid[config.ID] = struct{}{}
	}
	invalid = make([]uuid.UUID, 0, len(unique)-len(valid))
	for _, id := range unique {
		if _, ok := valid[id]; !ok {
			invalid = append(invalid, id)
		}
	}
	return unique, invalid, nil
}

// normalizeRequestedChatMCPServerIDs validates a request's MCP server
// selection for an existing chat. When requested is nil there is no
// change to make. IDs already persisted on the chat are exempt from the
// enabled-in-organization check: a server that is disabled or revoked
// after selection must not block sends. The generation path skips
// servers the chat can no longer use, and keeping the ID preserves the
// selection if the server is re-enabled. A non-nil response indicates
// the caller must write it with the returned status and stop.
func (api *API) normalizeRequestedChatMCPServerIDs(ctx context.Context, chat database.Chat, requested *[]uuid.UUID) (*[]uuid.UUID, int, *codersdk.Response) {
	if requested == nil {
		return nil, 0, nil
	}
	normalized, invalid, err := validateChatMCPServerIDs(ctx, api.Database, chat.OrganizationID, *requested)
	if err != nil {
		return nil, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to validate MCP server IDs.",
			Detail:  err.Error(),
		}
	}
	persisted := make(map[uuid.UUID]struct{}, len(chat.MCPServerIDs))
	for _, id := range chat.MCPServerIDs {
		persisted[id] = struct{}{}
	}
	newlyInvalid := make([]uuid.UUID, 0, len(invalid))
	for _, id := range invalid {
		if _, ok := persisted[id]; !ok {
			newlyInvalid = append(newlyInvalid, id)
		}
	}
	if len(newlyInvalid) > 0 {
		resp := invalidChatMCPServerIDsResponse(newlyInvalid)
		return nil, http.StatusBadRequest, &resp
	}
	return &normalized, 0, nil
}

func invalidChatMCPServerIDsResponse(ids []uuid.UUID) codersdk.Response {
	invalid := make([]string, 0, len(ids))
	for _, id := range ids {
		invalid = append(invalid, id.String())
	}
	return codersdk.Response{
		Message: "One or more MCP server IDs are invalid or disabled.",
		Detail:  fmt.Sprintf("Invalid IDs: %s", strings.Join(invalid, ", ")),
	}
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Create chat
// @ID create-chat
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateChatRequest true "Create chat request"
// @Success 201 {object} codersdk.Chat
// @Failure 413 {object} codersdk.Response "Request body exceeds 256 KiB"
// @Router /api/experimental/chats [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) postChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Limit memory used to decode dynamic tool schemas.
	var req codersdk.CreateChatRequest
	if !httpapi.ReadLimit(ctx, rw, r, int64(2*maxSystemPromptLenBytes), &req) {
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
	// NOTE: This authorize check is intentionally placed after request
	// parsing because we need req.OrganizationID to scope the RBAC check
	// to the correct org. The request body is bounded by the ReadLimit above,
	// limiting the cost of parsing before rejection.
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String()).InOrg(req.OrganizationID)) {
		httpapi.Forbidden(rw)
		return
	}

	contentBlocks, titleSource, inputError := createChatInputFromRequest(ctx, api.Database, req)
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *inputError)
		return
	}

	workspaceSelection, validationStatus, validationError := api.validateCreateChatWorkspaceSelection(ctx, r, req)
	if validationError != nil {
		httpapi.Write(ctx, rw, validationStatus, *validationError)
		return
	}

	title := chatprompt.FallbackTitle(titleSource)

	modelConfigID, personalOverrideEffort, modelConfigStatus, modelConfigError := api.resolveCreateChatModelConfigID(ctx, apiKey.UserID, req)
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

	normalizedMCPServerIDs, invalidMCPServerIDs, err := validateChatMCPServerIDs(ctx, api.Database, req.OrganizationID, req.MCPServerIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to validate MCP server IDs.",
			Detail:  err.Error(),
		})
		return
	}
	req.MCPServerIDs = normalizedMCPServerIDs
	if len(invalidMCPServerIDs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, invalidChatMCPServerIDsResponse(invalidMCPServerIDs))
		return
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

	reasoningEffort := req.ReasoningEffort
	if reasoningEffort == nil {
		reasoningEffort = personalOverrideEffort
	}
	if reasoningEffort != nil && !chatprovider.IsValidReasoningEffort(*reasoningEffort) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, invalidReasoningEffortResponse(*reasoningEffort))
		return
	}

	chat, err := api.chatDaemon.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:          req.OrganizationID,
		OwnerID:                 apiKey.UserID,
		WorkspaceID:             workspaceSelection.WorkspaceID,
		Title:                   title,
		TitleDerivedFromContent: true,
		ModelConfigID:           modelConfigID,
		ReasoningEffort:         reasoningEffort,
		PlanMode:                planModeToNullChatPlanMode(req.PlanMode),
		ClientType:              clientType,
		SystemPrompt:            req.SystemPrompt,
		InitialUserContent:      contentBlocks,
		MCPServerIDs:            mcpServerIDs,
		Labels:                  labels,
		DynamicTools:            dynamicToolsJSON,
		// IMPORTANT: users can only create root chats at the time of writing.
		ParentChatID: uuid.NullUUID{},
	})
	if err != nil {
		if writeChatHookErr(ctx, rw, err, "Chat creation denied by lifecycle hook.") {
			return
		}
		if writeChatFileError(ctx, rw, err) {
			return
		}
		if xerrors.Is(err, chatd.ErrInvalidModelConfigID) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config ID.",
				Detail:  err.Error(),
			})
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

	chat, err = api.Database.GetChatByID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to read back chat after creation.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	// Kick off best-effort automatic title generation now that the
	// chat and its initial user message are persisted. It runs
	// detached so it never blocks the create response, and only acts
	// on the first user turn.
	api.chatDaemon.GenerateChatTitleAsync(ctx, chat)

	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)
	response := db2sdk.Chat(chat, nil, chatFiles)
	httpapi.Write(ctx, rw, http.StatusCreated, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get chat by ID
// @ID get-chat-by-id
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat} [get]
// @Description Experimental: this endpoint is subject to change.
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

	if api.chatDaemon != nil {
		queued, err := api.chatDaemon.ChatQueuedForCapacity(ctx, chat)
		if err != nil {
			api.Logger.Error(ctx, "failed to derive chat queued-for-capacity state",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
		} else {
			sdkChat.QueuedForCapacity = queued
		}
	}

	// Enrich the lightweight context summary with the chat's pinned
	// resources (metadata only). This detail is computed on read and only
	// attached on the single-chat GET; list and watch payloads stay
	// lightweight. A failure here is non-fatal: the chat is still usable
	// without the detail, so we log and return the rest of the response.
	if sdkChat.Context != nil && api.chatDaemon != nil {
		resources, err := api.chatDaemon.ContextResources(ctx, chat)
		if err != nil {
			api.Logger.Error(ctx, "failed to compute chat context resources",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
		} else {
			sdkChat.Context.Resources = resources
		}
	}

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

	enriched := []codersdk.Chat{sdkChat}
	api.repairChatAgentIDs(ctx, enriched)
	sdkChat = enriched[0]

	httpapi.Write(ctx, rw, http.StatusOK, sdkChat)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List chat messages
// @ID list-chat-messages
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Param before_id query int false "Return messages with id < before_id"
// @Param after_id query int false "Return messages with id > after_id"
// @Param limit query int false "Page size, 1 to 200. Defaults to 50."
// @Success 200 {object} codersdk.ChatMessagesResponse
// @Router /api/experimental/chats/{chat}/messages [get]
// @Description Experimental: this endpoint is subject to change.
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
	afterID := parser.PositiveInt64(queryParams, 0, "after_id")
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
	// Reject transposed or equal cursors so an empty open range is loud,
	// not silently indistinguishable from "no messages in this range."
	if beforeID > 0 && afterID > 0 && afterID >= beforeID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "after_id must be less than before_id.",
		})
		return
	}

	// Polling with only after_id uses ASC so the cursor advances
	// monotonically; a DESC limit would drop rows when a burst larger
	// than `limit` lands between polls. Fetch limit+1 in both paths to
	// detect whether more pages exist.
	var messages []database.ChatMessage
	var err error
	switch {
	case afterID > 0 && beforeID == 0:
		messages, err = api.Database.GetChatMessagesByChatIDAscPaginated(ctx, database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chatID,
			AfterID:  afterID,
			LimitVal: limit + 1,
		})
	default:
		messages, err = api.Database.GetChatMessagesByChatIDDescPaginated(ctx, database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chatID,
			BeforeID: beforeID,
			AfterID:  afterID,
			LimitVal: limit + 1,
		})
	}
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

	// Queued messages are only meaningful for the initial top-of-history
	// load. Suppress them whenever any cursor is set so polling callers do
	// not receive the snapshot on every page fetch.
	var queuedMessages []database.ChatQueuedMessage
	if beforeID == 0 && afterID == 0 {
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get chat cost
// @ID get-chat-cost
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatCost
// @Router /api/experimental/chats/{chat}/cost [get]
// @Description Experimental: this endpoint is subject to change.
// @Description
// @Description Cost covers the whole chat tree: the root chat plus every
// @Description subagent chat beneath it. Requesting cost for a subagent chat
// @Description returns that same total.
// @Description
// @Description Cost is derived from AI Gateway data, which is subject to its
// @Description own retention period, 60 days by default, configured
// @Description independently of chat retention. Spend for requests older than
// @Description that period is no longer reported, so a chat whose requests
// @Description have all been purged reports zero cost.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatCost(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	// AI Gateway attributes a subagent's requests to the chat that spawned
	// it, so cost is only meaningful for a whole chat tree. Resolve the root
	// chat and report the tree total, including for subagent chats. Fall back
	// to the parent when root_chat_id is NULL, matching the
	// COALESCE(root_chat_id, parent_chat_id) resolution the chat queries use:
	// both columns are ON DELETE SET NULL, so deleting a root leaves
	// descendants with only a parent.
	rootChatID := chat.ID
	switch {
	case chat.RootChatID.Valid:
		rootChatID = chat.RootChatID.UUID
	case chat.ParentChatID.Valid:
		rootChatID = chat.ParentChatID.UUID
	}

	row, err := api.Database.GetAIBridgeChatCost(ctx, rootChatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat cost.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatCost{
		ChatID:               chat.ID,
		TotalCostMicros:      row.TotalCostMicros,
		RequestCount:         row.RequestCount,
		UnpricedRequestCount: row.UnpricedRequestCount,
	})
}

// @Summary List chat user prompts
// @ID list-chat-user-prompts
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Param limit query int false "Page size, 0 to 2000. 0 (the default) means the server-side default of 500."
// @Success 200 {object} codersdk.ChatPromptsResponse
// @Router /api/experimental/chats/{chat}/prompts [get]
// @Description Experimental: this endpoint is subject to change.
// @Description
// @Description Returns the user-authored prompts in a chat, newest first,
// @Description with each prompt's text parts concatenated in the order they
// @Description were authored. Used by the composer to power the up/down
// @Description arrow prompt-history cycle without paging through every
// @Description message in the chat.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatUserPrompts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	queryParams := r.URL.Query()
	parser := httpapi.NewQueryParamParser()
	// Default 0 sentinel; the SQL query treats 0 as "use the built-in
	// default of 500" via COALESCE(NULLIF(@limit_val, 0), 500). The
	// SDK guards opts.Limit > 0 so callers using the typed client only
	// reach here with an explicit value; raw HTTP callers can omit the
	// parameter (or pass 0) to opt into the default.
	limit := parser.PositiveInt32(queryParams, 0, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	// PositiveInt32 already rejects negatives via parser.Errors above,
	// so we only need to cap the upper bound here.
	if limit > 2000 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid limit parameter (0-2000).",
		})
		return
	}

	rows, err := api.Database.GetChatUserPromptsByChatID(ctx, database.GetChatUserPromptsByChatIDParams{
		ChatID:   chatID,
		LimitVal: limit,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat user prompts.",
			Detail:  err.Error(),
		})
		return
	}

	prompts := make([]codersdk.ChatPrompt, 0, len(rows))
	for _, row := range rows {
		prompts = append(prompts, codersdk.ChatPrompt{
			ID:   row.ID,
			Text: row.Text,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatPromptsResponse{
		Prompts: prompts,
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
			Message: codersdk.ChatGitWatchWorkspaceNotFoundMessage,
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
// @Summary Watch chat workspace git state via WebSockets
// @ID watch-chat-workspace-git-state-via-websockets
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceAgentGitServerMessage
// @Router /api/experimental/chats/{chat}/stream/git [get]
// @Description Experimental: this endpoint is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) watchChatGit(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		chat   = httpmw.ChatParam(r)
		logger = api.Logger.Named("chat_git_watcher").With(slog.F("chat_id", chat.ID))
	)

	if _, ok := api.authorizeChatWorkspaceExec(rw, r, chat, codersdk.ChatGitWatchNoWorkspaceMessage); !ok {
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
	agent, err := agentselect.FindChatAgent(agents)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: codersdk.ChatGitWatchNoEligibleAgentMessage,
			Detail:  err.Error(),
		})
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		agent,
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
			Message: codersdk.ChatGitWatchAgentStateMessage(apiAgent.Status),
		})
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()

	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, agent.ID)
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
	ctx = api.wsWatcher.Watch(ctx, logger, clientConn)

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
// @Summary Connect to chat workspace desktop via WebSockets
// @ID connect-to-chat-workspace-desktop-via-websockets
// @Security CoderSessionToken
// @Tags Chats
// @Produce application/octet-stream
// @Param chat path string true "Chat ID" format(uuid)
// @Success 101
// @Router /api/experimental/chats/{chat}/stream/desktop [get]
// @Description Raw binary WebSocket stream of the chat workspace desktop.
// @Description Experimental: this endpoint is subject to change.
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
	agent, err := agentselect.FindChatAgent(agents)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: codersdk.ChatGitWatchNoEligibleAgentMessage,
			Detail:  err.Error(),
		})
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		agent,
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

	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, agent.ID)
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

	ctx = api.wsWatcher.Watch(ctx, logger, conn)

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

	updatedChat, wrote, err := api.chatDaemon.RenameChatTitle(ctx, chat, trimmedTitle)
	if err != nil {
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
		api.chatDaemon.PublishTitleChange(updatedChat)
	}
	return updatedChat, false
}

// refreshChatContext re-pins a chat to its agent's latest context snapshot
// and clears the dirty marker.
//
// @Summary Refresh chat context
// @ID refresh-chat-context
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat}/context [put]
// @Description Experimental: this endpoint is subject to change.
func (api *API) refreshChatContext(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	updated, err := api.chatDaemon.RefreshChatContext(ctx, chat)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error refreshing chat context.",
			Detail:  err.Error(),
		})
		return
	}

	sdkChat := db2sdk.Chat(updated, nil, nil)

	// Enrich the context summary with the freshly pinned resources so the
	// client reflects the refresh immediately, without a full reload. This
	// mirrors getChat; we pass the re-pinned chat so the detail reflects the
	// post-refresh state. A failure here is non-fatal: the refresh already
	// succeeded, so we log and return the rest of the response.
	if sdkChat.Context != nil && api.chatDaemon != nil {
		resources, err := api.chatDaemon.ContextResources(ctx, updated)
		if err != nil {
			api.Logger.Error(ctx, "failed to compute chat context resources after refresh",
				slog.F("chat_id", updated.ID),
				slog.Error(err),
			)
		} else {
			sdkChat.Context.Resources = resources
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, sdkChat)
}

// patchChat updates a chat resource. Supports updating labels,
// workspace binding, archiving, pinning, and pinned-chat ordering.
//
// @Summary Update chat
// @ID update-chat
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param chat path string true "Chat ID" format(uuid)
// @Param request body codersdk.UpdateChatRequest true "Update chat request"
// @Success 204
// @Router /api/experimental/chats/{chat} [patch]
// @Description Experimental: this endpoint is subject to change.
func (api *API) patchChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

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

		// Archive invariant is one-way: parent archived implies
		// child archived. Archive state changes target the root
		// chat and cascade atomically across the family; child
		// chats cannot be archived or unarchived independently.
		// This check precedes the no-op check so any child attempt
		// surfaces the root-only error regardless of the chat's
		// current archived value.
		if chat.ParentChatID.Valid {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Chat archive state can only be changed on the root chat.",
			})
			return
		}

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

		var err error
		if archived {
			err = api.chatDaemon.ArchiveChat(ctx, chat)
		} else {
			err = api.chatDaemon.UnarchiveChat(ctx, chat)
		}
		if err != nil {
			if errors.Is(err, chatd.ErrArchiveRequiresRootChat) || errors.Is(err, chatstate.ErrChatNotRoot) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Chat archive state can only be changed on the root chat.",
				})
				return
			}
			if writeChatInvalidState(ctx, rw, err) {
				return
			}
			if errors.Is(err, chatstate.ErrTransitionNotAllowed) {
				// Archive only succeeds from idle / error execution
				// states (W, E0, E1) per the chatd RFC; active
				// chats refuse archive instead of being silently
				// transitioned to waiting first.
				httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
					Message: "Cannot archive an active chat. Interrupt or wait for the chat to finish first.",
					Detail:  err.Error(),
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

		if pinOrder > 0 && chat.ParentChatID.Valid {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot pin a child chat.",
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
			switch {
			case database.IsCheckViolation(err, database.CheckChatsPinOrderParentCheck):
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Cannot pin a child chat.",
				})
			case database.IsCheckViolation(err, database.CheckChatsPinOrderArchivedCheck):
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Cannot pin an archived chat.",
				})
			default:
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: errMsg,
					Detail:  err.Error(),
				})
			}
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

// writeChatInvalidState writes the shared invalid-state response for
// chatstate.ErrInvalidState across every chat mutation endpoint.
// Returns true when a response has been written.
func writeChatInvalidState(ctx context.Context, rw http.ResponseWriter, err error) bool {
	if !errors.Is(err, chatstate.ErrInvalidState) {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
		Message: "Chat is in an invalid state.",
	})
	return true
}

func noLocalChatModelResponse() *codersdk.Response {
	return &codersdk.Response{
		Message: "No chat model is available in this organization.",
		Detail:  "Ask an organization administrator to configure and enable a chat model.",
	}
}

func writeNoLocalChatModelResponse(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, *noLocalChatModelResponse())
}

// writeCommonChatMutationError writes responses shared by chat
// mutation endpoints. Returns true when a response has been written.
func writeCommonChatMutationError(ctx context.Context, rw http.ResponseWriter, err error, archivedMessage string) bool {
	switch {
	case xerrors.Is(err, chatd.ErrChatArchived):
		if archivedMessage == "" {
			archivedMessage = "Cannot mutate an archived chat."
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: archivedMessage,
		})
	case writeChatInvalidState(ctx, rw, err):
		// response already written
	case errors.Is(err, chatstate.ErrChatNotFound), httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
	default:
		return false
	}
	return true
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Send chat message
// @ID send-chat-message
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Param request body codersdk.CreateChatMessageRequest true "Create chat message request"
// @Success 200 {object} codersdk.CreateChatMessageResponse
// @Router /api/experimental/chats/{chat}/messages [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) postChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Sending a message triggers LLM inference, requiring update
	// permission on the org-scoped chat resource.
	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may send messages. Org admins pass the
	// RBAC check above (org-level ActionUpdate), but chat
	// processing forwards the *owner's* credentials (OIDC tokens,
	// provider API keys) to external services. Allowing a
	// non-owner to trigger processing would leak the owner's
	// tokens to MCP servers the caller controls.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may send messages.",
		})
		return
	}

	if chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot send messages to an archived chat.",
		})
		return
	}

	var req codersdk.CreateChatMessageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	contentBlocks, _, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
		})
		return
	}

	normalizedMCPServerIDs, status, mcpResp := api.normalizeRequestedChatMCPServerIDs(ctx, chat, req.MCPServerIDs)
	if mcpResp != nil {
		httpapi.Write(ctx, rw, status, *mcpResp)
		return
	}
	req.MCPServerIDs = normalizedMCPServerIDs

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

	modelConfigID := uuid.Nil
	if req.ModelConfigID != nil {
		modelConfigID = *req.ModelConfigID
	}
	if status, resp := api.validateExplicitChatModelConfigAvailable(ctx, apiKey.UserID, chat.OrganizationID, modelConfigID); resp != nil {
		httpapi.Write(ctx, rw, status, *resp)
		return
	}

	reasoningEffort := req.ReasoningEffort
	if reasoningEffort != nil && !chatprovider.IsValidReasoningEffort(*reasoningEffort) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, invalidReasoningEffortResponse(*reasoningEffort))
		return
	}

	sendResult, sendErr := api.chatDaemon.SendMessage(
		ctx,
		chatd.SendMessageOptions{
			ChatID:          chatID,
			CreatedBy:       apiKey.UserID,
			Content:         contentBlocks,
			ModelConfigID:   modelConfigID,
			ReasoningEffort: reasoningEffort,
			BusyBehavior:    busyBehavior,
			PlanMode:        sendPlanMode,
			MCPServerIDs:    req.MCPServerIDs,
		},
	)
	if sendErr != nil {
		if writeChatHookErr(ctx, rw, sendErr, "Chat message denied by lifecycle hook.") {
			return
		}
		if writeChatFileError(ctx, rw, sendErr) {
			return
		}
		if xerrors.Is(sendErr, chatd.ErrChatArchived) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot send messages to an archived chat.",
			})
			return
		}
		if xerrors.Is(sendErr, chatstate.ErrMessageQueueFull) {
			var queueFull *chatstate.MessageQueueFullError
			detail := ""
			if errors.As(sendErr, &queueFull) {
				detail = fmt.Sprintf("Maximum %d messages can be queued.", queueFull.Max)
			}
			httpapi.Write(ctx, rw, http.StatusTooManyRequests, codersdk.Response{
				Message: "Message queue is full.",
				Detail:  detail,
			})
			return
		}
		if xerrors.Is(sendErr, chatd.ErrInvalidModelConfigID) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config ID.",
			})
			return
		}
		if xerrors.Is(sendErr, chatd.ErrNoDefaultChatModelConfig) {
			writeNoLocalChatModelResponse(ctx, rw)
			return
		}
		if errors.Is(sendErr, chatstate.ErrChatNotFound) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if writeChatInvalidState(ctx, rw, sendErr) {
			return
		}
		if errors.Is(sendErr, chatstate.ErrTransitionNotAllowed) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not in a state that accepts new messages.",
				Detail:  sendErr.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat message.",
			Detail:  chaterror.FormatDiagnosticDetail(sendErr),
		})
		return
	}

	response := codersdk.CreateChatMessageResponse{Queued: sendResult.Queued}
	if sendResult.Queued {
		if sendResult.QueuedMessage != nil {
			response.QueuedMessage = convertChatQueuedMessagePtr(*sendResult.QueuedMessage)
		}
	} else {
		message := convertChatMessage(sendResult.Message)
		response.Message = &message
	}
	// Return the full user-visible inserted batch. A queued send on an errored
	// chat can promote the previous queue head, which clients must cache.
	for _, inserted := range sendResult.InsertedMessages {
		if inserted.Visibility == database.ChatMessageVisibilityModel {
			continue
		}
		response.Messages = append(response.Messages, convertChatMessage(inserted))
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Edit chat message
// @ID edit-chat-message
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Param message path int true "Message ID"
// @Param request body codersdk.EditChatMessageRequest true "Edit chat message request"
// @Success 200 {object} codersdk.EditChatMessageResponse
// @Router /api/experimental/chats/{chat}/messages/{message} [patch]
// @Description Experimental: this endpoint is subject to change.
func (api *API) patchChatMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may edit messages. See postChatMessages
	// for the security rationale.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may edit messages.",
		})
		return
	}

	if chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot edit messages in an archived chat.",
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

	contentBlocks, _, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
		})
		return
	}

	editModelConfigID := uuid.Nil
	if req.ModelConfigID != nil {
		editModelConfigID = *req.ModelConfigID
	}
	if status, resp := api.validateExplicitChatModelConfigAvailable(ctx, apiKey.UserID, chat.OrganizationID, editModelConfigID); resp != nil {
		httpapi.Write(ctx, rw, status, *resp)
		return
	}

	editReasoningEffort := req.ReasoningEffort
	if editReasoningEffort != nil && !chatprovider.IsValidReasoningEffort(*editReasoningEffort) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, invalidReasoningEffortResponse(*editReasoningEffort))
		return
	}

	editMCPServerIDs, status, mcpResp := api.normalizeRequestedChatMCPServerIDs(ctx, chat, req.MCPServerIDs)
	if mcpResp != nil {
		httpapi.Write(ctx, rw, status, *mcpResp)
		return
	}

	editResult, editErr := api.chatDaemon.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		CreatedBy:       apiKey.UserID,
		EditedMessageID: messageID,
		Content:         contentBlocks,
		ModelConfigID:   editModelConfigID,
		ReasoningEffort: editReasoningEffort,
		MCPServerIDs:    editMCPServerIDs,
	})
	if editErr != nil {
		if writeChatHookErr(ctx, rw, editErr, "Chat message denied by lifecycle hook.") {
			return
		}
		if writeChatFileError(ctx, rw, editErr) {
			return
		}

		switch {
		case xerrors.Is(editErr, chatd.ErrChatArchived):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot edit messages in an archived chat.",
			})
		case xerrors.Is(editErr, chatd.ErrEditedMessageNotFound):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Chat message not found.",
				Detail:  "Message does not belong to this chat.",
			})
		case xerrors.Is(editErr, chatd.ErrEditedMessageNotUser):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Only user messages can be edited.",
			})
		case xerrors.Is(editErr, chatd.ErrInvalidModelConfigID):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config ID.",
			})
		case xerrors.Is(editErr, chatd.ErrNoDefaultChatModelConfig):
			writeNoLocalChatModelResponse(ctx, rw)
		case errors.Is(editErr, chatstate.ErrChatNotFound):
			httpapi.ResourceNotFound(rw)
		case writeChatInvalidState(ctx, rw, editErr):
			// response already written
		case errors.Is(editErr, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not in a state that accepts message edits.",
				Detail:  editErr.Error(),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to edit chat message.",
				Detail:  editErr.Error(),
			})
		}
		return
	}

	response := codersdk.EditChatMessageResponse{Message: convertChatMessage(editResult.Message)}
	// Synthetic cancellations precede the replacement with lower IDs;
	// clients that seed their transcript cache from this response need
	// all user-visible inserted rows, or a stream reconnect with
	// after_id set to the replacement would skip the earlier ones.
	for _, inserted := range editResult.InsertedMessages {
		if inserted.Visibility == database.ChatMessageVisibilityModel {
			continue
		}
		response.Messages = append(response.Messages, convertChatMessage(inserted))
	}
	response.DeletedMessageIDs = editResult.DeletedMessageIDs
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteChatQueuedMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
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

	err = api.chatDaemon.DeleteQueued(ctx, chatID, queuedMessageID)
	if err != nil {
		switch {
		case xerrors.Is(err, chatstate.ErrQueuedMessageNotFound), xerrors.Is(err, sql.ErrNoRows):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Queued message not found.",
			})
		case errors.Is(err, chatstate.ErrChatNotFound):
			httpapi.ResourceNotFound(rw)
		case writeChatInvalidState(ctx, rw, err):
			// response already written
		case errors.Is(err, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat has no queued messages to delete.",
				Detail:  err.Error(),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to delete queued message.",
				Detail:  err.Error(),
			})
		}
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

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Promoting a queued message triggers LLM inference,
	// requiring update permission on the org-scoped chat resource.
	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may promote messages. See
	// postChatMessages for the security rationale.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may promote queued messages.",
		})
		return
	}

	if chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot promote queued messages in an archived chat.",
		})
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

	_, txErr := api.chatDaemon.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chatID,
		CreatedBy:       apiKey.UserID,
		QueuedMessageID: queuedMessageID,
	})

	if txErr != nil {
		switch {
		case xerrors.Is(txErr, chatd.ErrChatArchived):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot promote queued messages in an archived chat.",
			})
		case xerrors.Is(txErr, chatstate.ErrQueuedMessageNotFound):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Queued message not found.",
			})
		case xerrors.Is(txErr, chatd.ErrNoDefaultChatModelConfig):
			writeNoLocalChatModelResponse(ctx, rw)
		case errors.Is(txErr, chatstate.ErrChatNotFound):
			httpapi.ResourceNotFound(rw)
		case writeChatInvalidState(ctx, rw, txErr):
			// response already written
		case errors.Is(txErr, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat has no queued messages to promote.",
				Detail:  txErr.Error(),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to promote queued message.",
				Detail:  txErr.Error(),
			})
		}
		return
	}

	httpapi.Write(ctx, rw, http.StatusAccepted, codersdk.Response{
		Message: "Queued message promotion accepted.",
	})
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
//
// @Summary Stream chat events via WebSockets
// @ID stream-chat-events-via-websockets
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatStreamEvent
// @Router /api/experimental/chats/{chat}/stream [get]
// @Description Experimental: this endpoint is subject to change.
func (api *API) streamChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_streamer").With(slog.F("chat_id", chatID))

	if !api.requireChatDaemon(ctx, rw) {
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
	snapshot, events, cancelSub, ok := api.chatDaemon.SubscribeAuthorized(ctx, chat, r.Header, afterMessageID)
	// Defensive against future SubscribeAuthorized failure modes.
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

	ctx = api.wsWatcher.Watch(ctx, logger, conn)

	// The last_read_message_id field is owner-scoped. Shared readers
	// intentionally lack chat update permission, so their streams must not
	// update it.
	if chat.OwnerID == httpmw.APIKey(r).UserID {
		api.markChatAsRead(ctx, chatID)
		defer api.markChatAsRead(context.WithoutCancel(ctx), chatID)
	}

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
//
// @Summary Interrupt chat
// @ID interrupt-chat
// @Security CoderSessionToken
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Produce json
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat}/interrupt [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) interruptChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_interrupt").With(slog.F("chat_id", chatID))

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	updated, err := api.chatDaemon.InterruptChat(ctx, chat)
	if err != nil {
		if writeCommonChatMutationError(ctx, rw, err, "Cannot interrupt an archived chat.") {
			return
		}
		switch {
		case errors.Is(err, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not in an interruptible state.",
				Detail:  err.Error(),
			})
		default:
			logger.Error(ctx, "failed to interrupt chat", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to interrupt chat.",
				Detail:  err.Error(),
			})
		}
		return
	}
	chat = updated

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(chat, nil, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Compact chat
// @ID compact-chat
// @Security CoderSessionToken
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Produce json
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat}/compact [post]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
// @Description Requests a manual context compaction on an idle or errored
// @Description chat, clearing any stored error. The compaction runs
// @Description asynchronously through the chat worker and bypasses the
// @Description automatic usage threshold.
func (api *API) compactChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_compact").With(slog.F("chat_id", chatID))

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Compaction triggers LLM inference, requiring update permission
	// on the org-scoped chat resource.
	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may trigger compaction. Org admins pass the
	// RBAC check above (org-level ActionUpdate), but compaction runs
	// inference with the owner's delegated credentials.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may compact the chat.",
		})
		return
	}

	updated, err := api.chatDaemon.CompactChat(ctx, chat)
	if err != nil {
		if writeCommonChatMutationError(ctx, rw, err, "Cannot compact an archived chat.") {
			return
		}
		switch {
		case errors.Is(err, chatd.ErrNothingToCompact):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Nothing to compact.",
				Detail:  "The chat has no conversation to summarize after the latest compaction.",
			})
		case errors.Is(err, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Cannot compact the chat in its current state.",
				Detail:  "Compaction is not available while the chat is generating.",
			})
		default:
			logger.Error(ctx, "failed to compact chat", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to compact chat.",
				Detail:  err.Error(),
			})
		}
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(updated, nil, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Reconcile invalid chat state
// @ID reconcile-invalid-chat-state
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat}/reconcile-invalid [post]
// @Description Experimental: this endpoint is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) reconcileInvalidChatState(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID
	logger := api.Logger.Named("chat_reconcile_invalid").With(slog.F("chat_id", chatID))

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	updated, err := api.chatDaemon.ReconcileInvalidStateChat(ctx, chat)
	if err != nil {
		if writeCommonChatMutationError(ctx, rw, err, "") {
			return
		}
		switch {
		case errors.Is(err, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not in an invalid state.",
				Detail:  err.Error(),
			})
		default:
			logger.Error(ctx, "failed to reconcile invalid chat state", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to reconcile chat state.",
				Detail:  err.Error(),
			})
		}
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(updated, nil, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Regenerate chat title
// @ID regenerate-chat-title
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.Chat
// @Router /api/experimental/chats/{chat}/title/regenerate [post]
// @Description Experimental: this endpoint is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) regenerateChatTitle(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may regenerate titles. See
	// postChatMessages for the security rationale.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may regenerate the title.",
		})
		return
	}

	updatedChat, err := api.chatDaemon.RegenerateChatTitle(ctx, chat)
	if err != nil {
		if errors.Is(err, chatd.ErrNoDefaultChatModelConfig) {
			writeNoLocalChatModelResponse(ctx, rw)
			return
		}
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if maybeWriteChatUsageLimitError(ctx, rw, err) {
			return
		}
		if maybeWriteManualTitleTimeoutErr(ctx, rw, err) {
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
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may propose titles. See
	// postChatMessages for the security rationale.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may propose a title.",
		})
		return
	}

	title, err := api.chatDaemon.ProposeChatTitle(ctx, chat)
	if err != nil {
		if errors.Is(err, chatd.ErrNoDefaultChatModelConfig) {
			writeNoLocalChatModelResponse(ctx, rw)
			return
		}
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if maybeWriteChatUsageLimitError(ctx, rw, err) {
			return
		}
		if maybeWriteManualTitleTimeoutErr(ctx, rw, err) {
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
// @Summary Get chat diff contents
// @ID get-chat-diff-contents
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatDiffContents
// @Router /api/experimental/chats/{chat}/diff [get]
// @Description Experimental: this endpoint is subject to change.
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
//
// Aliased as ChatStartWorkspace in coderd/export_test.go so external
// tests in the coderd_test package can drive the auto-update path
// end-to-end. The proper fix is to extract the request building into
// a pure function; tracked in CODAGT-292.
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

// chatStopWorkspace stops a workspace by creating a new build with the
// "stop" transition. It mirrors chatStartWorkspace, without start-only
// active-version behavior.
func (api *API) chatStopWorkspace(
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

	req.Transition = codersdk.WorkspaceTransitionStop

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

	gp := api.resolveGitProvider(ctx, reference.RepositoryRef.RemoteOrigin)
	if gp == nil {
		return result, nil
	}

	token, err := api.resolveChatGitAccessToken(ctx, chat.OwnerID, reference.RepositoryRef.RemoteOrigin)
	if errors.Is(err, gitsync.ErrNoTokenAvailable) || token == nil {
		// No token available; return metadata without fetching diff.
		return result, nil
	} else if err != nil {
		return result, xerrors.Errorf("resolve git access token: %w", err)
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
	reference.RepositoryRef = api.buildChatRepositoryRefFromStatus(ctx, status)

	// If we have a repo ref with a branch, try to resolve the
	// current open PR. This picks up new PRs after the previous
	// one was closed.
	if reference.RepositoryRef != nil && reference.RepositoryRef.Owner != "" {
		gp := api.resolveGitProvider(ctx, reference.RepositoryRef.RemoteOrigin)
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
			gp, err := extAuth.Git()
			if err != nil || gp == nil {
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
func (api *API) buildChatRepositoryRefFromStatus(ctx context.Context, status database.ChatDiffStatus) *chatRepositoryRef {
	branch := strings.TrimSpace(status.GitBranch)
	origin := strings.TrimSpace(status.GitRemoteOrigin)
	if branch == "" || origin == "" {
		return nil
	}

	providerType, gp := api.resolveExternalAuth(ctx, origin)
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
// if no matching config is found or no provider could be constructed.
func (api *API) resolveExternalAuth(ctx context.Context, origin string) (providerType string, gp gitprovider.Provider) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", nil
	}
	for _, extAuth := range api.ExternalAuthConfigs {
		if extAuth.Regex == nil || !extAuth.Regex.MatchString(origin) {
			continue
		}
		p, err := extAuth.Git()
		if err != nil {
			api.Logger.Warn(ctx, "failed to construct git provider",
				slog.F("provider_id", extAuth.ID),
				slog.F("provider_type", extAuth.Type),
				slog.Error(err),
			)
			continue
		}
		if p == nil {
			continue
		}
		return strings.ToLower(strings.TrimSpace(extAuth.Type)), p
	}
	return "", nil
}

// resolveGitProvider finds the external auth config matching the
// given remote origin URL and returns its git provider. Returns
// nil if no matching git provider is configured.
func (api *API) resolveGitProvider(ctx context.Context, origin string) gitprovider.Provider {
	_, gp := api.resolveExternalAuth(ctx, origin)
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
	userID uuid.UUID,
	req codersdk.CreateChatRequest,
) (uuid.UUID, *string, int, *codersdk.Response) {
	if req.ModelConfigID != nil {
		if *req.ModelConfigID == uuid.Nil {
			return uuid.Nil, nil, http.StatusBadRequest, &codersdk.Response{
				Message: "Invalid model config ID.",
			}
		}
		if _, status, resp := api.validateUserChatModelConfigAvailable(ctx, userID, req.OrganizationID, *req.ModelConfigID); resp != nil {
			return uuid.Nil, nil, status, resp
		}
		return *req.ModelConfigID, nil, 0, nil
	}

	personalOverridesEnabled, err := api.Database.GetChatPersonalModelOverridesEnabled(ctx)
	if err != nil {
		return uuid.Nil, nil, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat model config.",
			Detail:  err.Error(),
		}
	}
	if !personalOverridesEnabled {
		id, status, resp := api.defaultCreateChatModelConfigID(ctx, req.OrganizationID)
		return id, nil, status, resp
	}

	override, err := api.Database.GetChatUserModelOverride(ctx, database.GetChatUserModelOverrideParams{
		UserID:         userID,
		OrganizationID: req.OrganizationID,
		Context:        string(codersdk.ChatPersonalModelOverrideContextRoot),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat model config.",
			Detail:  err.Error(),
		}
	}
	if err == nil {
		switch codersdk.ChatPersonalModelOverrideMode(override.Mode) {
		case codersdk.ChatPersonalModelOverrideModeChatDefault:
		case codersdk.ChatPersonalModelOverrideModeModel:
			if override.ModelConfigID.Valid {
				_, reason, err := api.userCanUseChatModelConfig(
					ctx,
					userID,
					req.OrganizationID,
					override.ModelConfigID.UUID,
				)
				if err != nil {
					return uuid.Nil, nil, http.StatusInternalServerError, &codersdk.Response{
						Message: "Failed to resolve chat model config.",
						Detail:  err.Error(),
					}
				}
				if reason == chatModelConfigAvailable {
					var effort *string
					if override.ReasoningEffort.Valid {
						effort = &override.ReasoningEffort.String
					}
					return override.ModelConfigID.UUID, effort, 0, nil
				}
				api.Logger.Debug(
					ctx,
					"personal root model override is unavailable, using default model",
					slog.F("user_id", userID),
					slog.F("model_config_id", override.ModelConfigID.UUID),
					slog.F("reason", reason),
				)
			}
		default:
			api.Logger.Warn(
				ctx,
				"unsupported personal root model override mode, using default model",
				slog.F("user_id", userID),
				slog.F("mode", override.Mode),
			)
		}
	}

	id, status, resp := api.defaultCreateChatModelConfigID(ctx, req.OrganizationID)
	return id, nil, status, resp
}

func (api *API) defaultCreateChatModelConfigID(
	ctx context.Context,
	organizationID uuid.UUID,
) (uuid.UUID, int, *codersdk.Response) {
	rows, err := api.Database.GetEnabledChatModelConfigsByOrganization(ctx, organizationID)
	if err != nil {
		return uuid.Nil, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat model config.",
			Detail:  err.Error(),
		}
	}
	effective := database.DeriveEffectiveChatModelConfigs(rows)
	if effective.DefaultConfig.ID != uuid.Nil {
		return effective.DefaultConfig.ID, 0, nil
	}

	return uuid.Nil, http.StatusBadRequest, noLocalChatModelResponse()
}

// validateChatCompressionThreshold enforces the chat_model_configs CHECK
// constraint range.
func validateChatCompressionThreshold(threshold int32) error {
	if threshold < minChatContextCompressionThreshold ||
		threshold > maxChatContextCompressionThreshold {
		return xerrors.Errorf(
			"context_compression_threshold must be between %d and %d",
			minChatContextCompressionThreshold,
			maxChatContextCompressionThreshold,
		)
	}
	return nil
}

func normalizeChatCompressionThreshold(
	requested *int32,
	fallback int32,
) (int32, error) {
	threshold := fallback
	if requested != nil {
		threshold = *requested
	}

	if err := validateChatCompressionThreshold(threshold); err != nil {
		return 0, err
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

// chatInstructionSettingsLockTimeout bounds how long a request waits for the
// per-setting advisory lock. The only other holders are sibling requests
// holding it for a single upsert.
const chatInstructionSettingsLockTimeout = 5 * time.Second

func (api *API) putChatSystemPrompt(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Identity is assigned before the authorization check so a denied PUT
	// records the attempt with status 403 and an empty diff. The body is
	// never read before authorization, so no request content reaches that
	// row.
	aReq, commitAudit := audit.InitRequestWithCancel[database.ChatInstructionSettings](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit(true)
	aReq.Old = database.ChatInstructionSettings{
		ID:   audit.ChatInstructionSystemPromptID,
		Name: audit.ChatInstructionSystemPromptName,
	}
	aReq.New = aReq.Old

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	var req codersdk.UpdateChatSystemPromptRequest
	if !httpapi.ReadLimit(ctx, rw, r, int64(2*maxSystemPromptLenBytes), &req) {
		return
	}
	sanitizedPrompt := codersdk.SanitizePromptText(req.SystemPrompt)
	// 128 KiB is generous for a system prompt while still
	// preventing abuse or accidental pastes of large content.
	if len(sanitizedPrompt) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "System prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(sanitizedPrompt)),
		})
		return
	}

	var noChange bool
	// The per-setting advisory lock serializes the audit change-detection
	// with the write: two concurrent identical PUTs both still succeed,
	// but the second transaction's comparison sees the first's committed
	// state and reports no change. The lock wait is bounded so a waiter
	// cannot hang past the client's deadline.
	lockCtx, lockCancel := context.WithTimeout(ctx, chatInstructionSettingsLockTimeout)
	defer lockCancel()
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(lockCtx, database.LockIDChatInstructionSystemPrompt); err != nil {
			return xerrors.Errorf("acquire chat instruction setting write lock: %w", err)
		}

		oldConfig, err := tx.GetChatSystemPromptConfig(ctx)
		if err != nil {
			return err
		}
		aReq.Old.SystemPrompt = oldConfig.ChatSystemPrompt
		aReq.Old.IncludeDefaultSystemPromptSet = oldConfig.IncludeDefaultSystemPromptSet
		aReq.Old.IncludeDefaultSystemPrompt = oldConfig.IncludeDefaultSystemPrompt

		if err := tx.UpsertChatSystemPrompt(ctx, sanitizedPrompt); err != nil {
			return err
		}
		// Only update the include-default flag when the caller explicitly
		// provides it. Omitting the field preserves whatever is currently
		// stored (or the schema-level default for new deployments),
		// avoiding a backward-compatibility regression for older clients
		// that only send system_prompt.
		if req.IncludeDefaultSystemPrompt != nil {
			if err := tx.UpsertChatIncludeDefaultSystemPrompt(ctx, *req.IncludeDefaultSystemPrompt); err != nil {
				return err
			}
		}

		// Derive New from what was written rather than re-reading: the
		// upserts store $1 verbatim, so the stored prompt is the request
		// value, and the effective include-default flag follows
		// GetChatSystemPromptConfig's rule from the written toggle row
		// and the written prompt. A post-write read would fail on
		// query-local context cancellation even after a successful
		// commit, silently discarding a change the client made.
		newIncludeSet := oldConfig.IncludeDefaultSystemPromptSet
		newIncludeValue := oldConfig.IncludeDefaultSystemPrompt
		if req.IncludeDefaultSystemPrompt != nil {
			newIncludeSet = true
			newIncludeValue = *req.IncludeDefaultSystemPrompt
		}
		if !newIncludeSet {
			// Legacy fallback: a non-empty custom prompt implies opting
			// out; otherwise the setting defaults to true.
			newIncludeValue = sanitizedPrompt == ""
		}
		aReq.New.SystemPrompt = sanitizedPrompt
		aReq.New.IncludeDefaultSystemPromptSet = newIncludeSet
		aReq.New.IncludeDefaultSystemPrompt = newIncludeValue
		noChange = aReq.New.SystemPrompt == aReq.Old.SystemPrompt &&
			aReq.New.IncludeDefaultSystemPromptSet == aReq.Old.IncludeDefaultSystemPromptSet &&
			aReq.New.IncludeDefaultSystemPrompt == aReq.Old.IncludeDefaultSystemPrompt
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat system prompt configuration.",
			Detail:  err.Error(),
		})
		return
	}
	if noChange {
		// Stage the no-op decision until after the transaction commits,
		// so a commit failure cannot suppress an attempt row.
		commitAudit(false)
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

	// Identity is assigned before the authorization check so a denied PUT
	// records the attempt with status 403 and an empty diff. The body is
	// never read before authorization, so no request content reaches that
	// row.
	aReq, commitAudit := audit.InitRequestWithCancel[database.ChatInstructionSettings](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit(true)
	aReq.Old = database.ChatInstructionSettings{
		ID:   audit.ChatInstructionPlanModeID,
		Name: audit.ChatInstructionPlanModeName,
	}
	aReq.New = aReq.Old

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	var req codersdk.UpdateChatPlanModeInstructionsRequest
	if !httpapi.ReadLimit(ctx, rw, r, int64(2*maxSystemPromptLenBytes), &req) {
		return
	}

	sanitizedInstructions := codersdk.SanitizePromptText(req.PlanModeInstructions)
	if len(sanitizedInstructions) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Plan mode instructions exceed maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(sanitizedInstructions)),
		})
		return
	}

	var noChange bool
	lockCtx, lockCancel := context.WithTimeout(ctx, chatInstructionSettingsLockTimeout)
	defer lockCancel()
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(lockCtx, database.LockIDChatInstructionPlanMode); err != nil {
			return xerrors.Errorf("acquire chat instruction setting write lock: %w", err)
		}

		oldInstructions, err := tx.GetChatPlanModeInstructions(ctx)
		if err != nil {
			return err
		}
		aReq.Old.PlanModeInstructions = oldInstructions

		if err := tx.UpsertChatPlanModeInstructions(ctx, sanitizedInstructions); err != nil {
			return err
		}
		aReq.New.PlanModeInstructions = sanitizedInstructions
		noChange = aReq.New.PlanModeInstructions == aReq.Old.PlanModeInstructions
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating plan mode instructions.",
			Detail:  err.Error(),
		})
		return
	}
	if noChange {
		// Stage the no-op decision until after the transaction commits,
		// so a commit failure cannot suppress an attempt row.
		commitAudit(false)
	}
	rw.WriteHeader(http.StatusNoContent)
}

func readChatModelOverrideContext(
	rw http.ResponseWriter,
	r *http.Request,
) (codersdk.ChatModelOverrideContext, bool) {
	ctx := r.Context()
	rawContext := chi.URLParam(r, "context")
	overrideContext, err := parseChatModelOverrideContext(rawContext)
	if err == nil {
		return overrideContext, true
	}
	validContextValues := make(
		[]string,
		0,
		len(codersdk.AllChatModelOverrideContexts()),
	)
	for _, overrideContext := range codersdk.AllChatModelOverrideContexts() {
		validContextValues = append(validContextValues, string(overrideContext))
	}
	validContexts := strings.Join(validContextValues, ", ")
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Invalid chat model override context.",
		Detail: fmt.Sprintf(
			"Expected one of %s. Got %q.",
			validContexts,
			rawContext,
		),
	})
	return "", false
}

// @Summary List organization chat model overrides
// @ID list-organization-chat-model-overrides
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Success 200 {object} codersdk.ChatModelOverridesResponse
// @Router /api/experimental/organizations/{organization}/chats/model-overrides [get]
// @x-apidocgen {"skip": true}
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getOrganizationChatModelOverrides(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	rows, err := api.Database.GetChatOrganizationModelOverrides(ctx, organization.ID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching model overrides.",
			Detail:  err.Error(),
		})
		return
	}
	response := codersdk.ChatModelOverridesResponse{
		Overrides: make([]codersdk.ChatModelOverrideResponse, 0, len(rows)),
	}
	for _, row := range rows {
		response.Overrides = append(response.Overrides, chatOrganizationModelOverrideResponse(row))
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Update organization chat model override
// @ID update-organization-chat-model-override
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param context path string true "Override context" Enums(general,explore,title_generation,compaction,advisor)
// @Param request body codersdk.UpdateChatModelOverrideRequest true "Model override"
// @Success 200 {object} codersdk.ChatModelOverrideResponse
// @Router /api/experimental/organizations/{organization}/chats/model-overrides/{context} [put]
// @x-apidocgen {"skip": true}
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putOrganizationChatModelOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceChatModelConfig.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	overrideContext, ok := readChatModelOverrideContext(rw, r)
	if !ok {
		return
	}
	if overrideContext == codersdk.ChatModelOverrideContextAdvisor &&
		!api.Experiments.Enabled(codersdk.ExperimentChatAdvisor) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateChatModelOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	response := codersdk.ChatModelOverrideResponse{Context: overrideContext}
	trimmedModelConfigID := strings.TrimSpace(req.ModelConfigID)
	if trimmedModelConfigID == "" {
		if req.ReasoningEffort != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "reasoning_effort requires model_config_id.",
			})
			return
		}
		err := api.Database.DeleteChatOrganizationModelOverride(ctx, database.DeleteChatOrganizationModelOverrideParams{
			OrganizationID: organization.ID,
			Context:        string(overrideContext),
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error clearing %s model override.", overrideContext),
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, response)
		return
	}

	modelConfigID, err := uuid.Parse(trimmedModelConfigID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid model_config_id.",
			Detail:  fmt.Sprintf("Value %q is not a valid UUID.", req.ModelConfigID),
		})
		return
	}
	// The explicit ActionUpdate authorization above gates this endpoint. Look
	// the referenced model up under a system context so a custom role that
	// grants update without read can still use the endpoint; organization
	// ownership is enforced inside the validation.
	//nolint:gocritic // See above.
	status, validationResponse := validateChatModelOverride(
		dbauthz.AsSystemRestricted(ctx),
		api.Database,
		organization.ID,
		&modelConfigID,
		req.ReasoningEffort,
	)
	if validationResponse != nil {
		httpapi.Write(ctx, rw, status, *validationResponse)
		return
	}

	row := database.ChatOrganizationModelOverride{
		OrganizationID:  organization.ID,
		Context:         string(overrideContext),
		ModelConfigID:   modelConfigID,
		ReasoningEffort: sql.NullString{String: derefOrEmpty(req.ReasoningEffort), Valid: req.ReasoningEffort != nil},
	}
	err = api.Database.UpsertChatOrganizationModelOverride(ctx, database.UpsertChatOrganizationModelOverrideParams{
		OrganizationID:  row.OrganizationID,
		Context:         row.Context,
		ModelConfigID:   row.ModelConfigID,
		ReasoningEffort: row.ReasoningEffort,
	})
	if database.IsForeignKeyViolation(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid model_config_id."})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error updating %s model override.", overrideContext),
			Detail:  err.Error(),
		})
		return
	}
	response = chatOrganizationModelOverrideResponse(row)
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

func readChatPersonalModelOverrideContext(
	rw http.ResponseWriter,
	r *http.Request,
) (codersdk.ChatPersonalModelOverrideContext, bool) {
	ctx := r.Context()
	rawContext := chi.URLParam(r, "context")
	overrideContext, ok := parseChatPersonalModelOverrideContext(rawContext)
	if ok {
		return overrideContext, true
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Invalid chat personal model override context.",
		Detail: fmt.Sprintf(
			"Expected one of %s. Got %q.",
			chatPersonalModelOverrideContextsJoined(),
			rawContext,
		),
	})
	return "", false
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatPersonalModelOverridesAdminSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.ResourceNotFound(rw)
		return
	}

	enabled, err := api.Database.GetChatPersonalModelOverridesEnabled(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal model override setting.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatPersonalModelOverridesAdminSettings{
		AllowUsers: enabled,
	})
}

const chatOperationalSettingsLockTimeout = 5 * time.Second

type chatOperationalSetting string

const (
	chatOperationalSettingChatRetentionDays             chatOperationalSetting = "agents_chat_retention_days"
	chatOperationalSettingChatDebugRetentionDays        chatOperationalSetting = "agents_chat_debug_retention_days"
	chatOperationalSettingChatAutoArchiveDays           chatOperationalSetting = "agents_chat_auto_archive_days"
	chatOperationalSettingWorkspaceTTL                  chatOperationalSetting = "agents_workspace_ttl"
	chatOperationalSettingComputerUseProvider           chatOperationalSetting = "agents_computer_use_provider"
	chatOperationalSettingDebugLoggingAllowUsers        chatOperationalSetting = "agents_chat_debug_logging_allow_users"
	chatOperationalSettingPersonalModelOverridesEnabled chatOperationalSetting = "agents_chat_personal_model_overrides_enabled"
)

func (s chatOperationalSetting) defaultValue() string {
	switch s {
	case chatOperationalSettingChatRetentionDays:
		return "30"
	case chatOperationalSettingChatDebugRetentionDays:
		return strconv.FormatInt(int64(codersdk.DefaultChatDebugRetentionDays), 10)
	case chatOperationalSettingChatAutoArchiveDays:
		return strconv.FormatInt(int64(codersdk.DefaultChatAutoArchiveDays), 10)
	case chatOperationalSettingWorkspaceTTL:
		return "0s"
	case chatOperationalSettingComputerUseProvider:
		return string(chattool.DefaultComputerUseProvider(""))
	case chatOperationalSettingDebugLoggingAllowUsers,
		chatOperationalSettingPersonalModelOverridesEnabled:
		return "false"
	default:
		panic(fmt.Sprintf("unknown chat operational setting %q", s))
	}
}

func (s chatOperationalSetting) auditValue(value string, id uuid.UUID) database.ChatOperationalSettings {
	settings := database.ChatOperationalSettings{ID: id}
	switch s {
	case chatOperationalSettingChatRetentionDays:
		settings.ChatRetentionDays = value
	case chatOperationalSettingChatDebugRetentionDays:
		settings.ChatDebugRetentionDays = value
	case chatOperationalSettingChatAutoArchiveDays:
		settings.ChatAutoArchiveDays = value
	case chatOperationalSettingWorkspaceTTL:
		settings.WorkspaceTTL = value
	case chatOperationalSettingComputerUseProvider:
		settings.ComputerUseProvider = value
	case chatOperationalSettingDebugLoggingAllowUsers:
		settings.DebugLoggingAllowUsers = value
	case chatOperationalSettingPersonalModelOverridesEnabled:
		settings.PersonalModelOverridesEnabled = value
	default:
		panic(fmt.Sprintf("unknown chat operational setting %q", s))
	}
	return settings
}

func (api *API) initChatOperationalSettingsAudit(
	rw http.ResponseWriter,
	r *http.Request,
) (*audit.Request[database.ChatOperationalSettings], func(bool)) {
	aReq, commitAudit := audit.InitRequestWithCancel[database.ChatOperationalSettings](rw, &audit.RequestParams{
		Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionWrite,
	})
	aReq.New.ID = uuid.New()
	return aReq, commitAudit
}

// auditedChatOperationalSettingWrite captures the effective old value and
// performs the write in one transaction. It suppresses the audit entry when
// the effective value does not change.
func (api *API) auditedChatOperationalSettingWrite(
	ctx context.Context,
	aReq *audit.Request[database.ChatOperationalSettings],
	commitAudit func(bool),
	setting chatOperationalSetting,
	newValue string,
	write func(database.Store) error,
) error {
	var noChange bool
	lockCtx, lockCancel := context.WithTimeout(ctx, chatOperationalSettingsLockTimeout)
	defer lockCancel()

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(lockCtx, database.GenLockID(string(setting))); err != nil {
			return xerrors.Errorf("acquire chat operational setting write lock: %w", err)
		}

		old, err := tx.GetChatSiteConfigValue(ctx, string(setting))
		if err != nil {
			return err
		}
		oldValue := old.Value
		if !old.Exists {
			oldValue = setting.defaultValue()
		}
		if oldValue == newValue {
			noChange = true
			return nil
		}
		if err := write(tx); err != nil {
			return err
		}

		aReq.Old = setting.auditValue(oldValue, uuid.Nil)
		aReq.New = setting.auditValue(newValue, aReq.New.ID)
		return nil
	}, nil)
	if err != nil {
		return err
	}
	if noChange {
		commitAudit(false)
	}
	return nil
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatPersonalModelOverridesAdminSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingPersonalModelOverridesEnabled,
		strconv.FormatBool(req.AllowUsers),
		func(tx database.Store) error { return tx.UpsertChatPersonalModelOverridesEnabled(ctx, req.AllowUsers) },
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating personal model override setting.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get organization member chat model overrides
// @ID get-organization-member-chat-model-overrides
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param user path string true "User name, ID, or me"
// @Success 200 {object} codersdk.UserChatPersonalModelOverridesResponse
// @Router /api/experimental/organizations/{organization}/members/{user}/chats/model-overrides [get]
// @x-apidocgen {"skip": true}
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getUserChatPersonalModelOverrides(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	member := httpmw.OrganizationMemberParam(r)

	enabled, err := api.Database.GetChatPersonalModelOverridesEnabled(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal model override setting.",
			Detail:  err.Error(),
		})
		return
	}

	rows, err := api.Database.GetChatUserModelOverrides(ctx, database.GetChatUserModelOverridesParams{
		UserID:         member.UserID,
		OrganizationID: organization.ID,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user personal model overrides.",
			Detail:  err.Error(),
		})
		return
	}
	byContext := make(map[string]database.ChatUserModelOverride, len(rows))
	for _, row := range rows {
		byContext[row.Context] = row
	}

	deploymentDefaults, err := api.chatPersonalModelOverrideDeploymentDefaults(ctx, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organization model defaults.",
			Detail:  err.Error(),
		})
		return
	}

	response := codersdk.UserChatPersonalModelOverridesResponse{
		Enabled:            enabled,
		DeploymentDefaults: deploymentDefaults,
	}
	for _, overrideContext := range chatPersonalModelOverrideContexts {
		var row *database.ChatUserModelOverride
		if contextRow, ok := byContext[string(overrideContext)]; ok {
			row = &contextRow
		}
		override := chatPersonalModelOverrideResponse(overrideContext, row)
		switch overrideContext {
		case codersdk.ChatPersonalModelOverrideContextRoot:
			response.Root = override
		case codersdk.ChatPersonalModelOverrideContextGeneral:
			response.General = override
		case codersdk.ChatPersonalModelOverrideContextExplore:
			response.Explore = override
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Update organization member chat model override
// @ID update-organization-member-chat-model-override
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization name or ID"
// @Param user path string true "User name, ID, or me"
// @Param context path string true "Override context" Enums(root,general,explore)
// @Param request body codersdk.UpdateUserChatPersonalModelOverrideRequest true "Personal model override"
// @Success 204
// @Router /api/experimental/organizations/{organization}/members/{user}/chats/model-overrides/{context} [put]
// @x-apidocgen {"skip": true}
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putUserChatPersonalModelOverride(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	member := httpmw.OrganizationMemberParam(r)

	enabled, err := api.Database.GetChatPersonalModelOverridesEnabled(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal model override setting.",
			Detail:  err.Error(),
		})
		return
	}
	if !enabled && apiKey.UserID == member.UserID &&
		!api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "An administrator has not enabled user personal model overrides.",
		})
		return
	}

	overrideContext, ok := readChatPersonalModelOverrideContext(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateUserChatPersonalModelOverrideRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	modelConfigID := uuid.NullUUID{}
	reasoningEffort := req.ReasoningEffort
	rawModelConfigID := strings.TrimSpace(req.ModelConfigID)
	switch req.Mode {
	case codersdk.ChatPersonalModelOverrideModeChatDefault:
		if rawModelConfigID != "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "model_config_id must be empty unless mode is model."})
			return
		}
		if reasoningEffort != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "reasoning_effort requires mode model."})
			return
		}
	case codersdk.ChatPersonalModelOverrideModeDeploymentDefault:
		if overrideContext == codersdk.ChatPersonalModelOverrideContextRoot {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "deployment_default is not supported for root personal model overrides."})
			return
		}
		if rawModelConfigID != "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "model_config_id must be empty unless mode is model."})
			return
		}
		if reasoningEffort != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "reasoning_effort requires mode model."})
			return
		}
	case codersdk.ChatPersonalModelOverrideModeModel:
		if rawModelConfigID == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "model_config_id is required when mode is model."})
			return
		}
		parsedModelConfigID, err := uuid.Parse(rawModelConfigID)
		if err != nil || parsedModelConfigID == uuid.Nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model_config_id.",
				Detail:  fmt.Sprintf("Value %q is not a valid UUID.", req.ModelConfigID),
			})
			return
		}
		// Validate with the target member's ACLs so a model visible only to
		// an administrator caller is rejected instead of persisted; chatd
		// resolves the override under the member's actor at runtime.
		validateCtx := ctx
		if apiKey.UserID != member.UserID {
			memberSubject, _, err := httpmw.UserRBACSubject(ctx, api.Database, member.UserID, rbac.ScopeAll)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error validating model config override.",
					Detail:  err.Error(),
				})
				return
			}
			validateCtx = dbauthz.As(ctx, memberSubject)
		}
		modelConfig, status, validationResponse := api.validateUserChatModelConfigAvailable(
			validateCtx, member.UserID, organization.ID, parsedModelConfigID,
		)
		if validationResponse != nil {
			httpapi.Write(ctx, rw, status, *validationResponse)
			return
		}
		status, validationResponse = validateChatModelOverrideEffort(modelConfig, reasoningEffort)
		if validationResponse != nil {
			httpapi.Write(ctx, rw, status, *validationResponse)
			return
		}
		modelConfigID = uuid.NullUUID{UUID: parsedModelConfigID, Valid: true}
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid personal model override mode."})
		return
	}

	err = api.Database.UpsertChatUserModelOverride(ctx, database.UpsertChatUserModelOverrideParams{
		UserID:          member.UserID,
		OrganizationID:  organization.ID,
		Context:         string(overrideContext),
		Mode:            string(req.Mode),
		ModelConfigID:   modelConfigID,
		ReasoningEffort: sql.NullString{String: derefOrEmpty(reasoningEffort), Valid: reasoningEffort != nil},
	})
	if database.IsForeignKeyViolation(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid model_config_id: model config not found or disabled.",
		})
		return
	}
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user personal model override.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatComputerUseProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider, err := api.Database.GetChatComputerUseProvider(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching computer use provider.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatComputerUseProviderResponse{
		Provider: chattool.DefaultComputerUseProvider(codersdk.ChatComputerUseProvider(provider)),
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatComputerUseProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatComputerUseProviderRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if !req.Provider.Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid computer use provider.",
			Detail: fmt.Sprintf(
				"Expected one of: %s. Got %q.",
				strings.Join(chattool.SupportedComputerUseProviders(), ", "),
				req.Provider,
			),
		})
		return
	}

	value := string(req.Provider)
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingComputerUseProvider, value,
		func(tx database.Store) error { return tx.UpsertChatComputerUseProvider(ctx, value) },
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating computer use provider.",
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
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatDebugLoggingAllowUsersRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingDebugLoggingAllowUsers,
		strconv.FormatBool(req.AllowUsers),
		func(tx database.Store) error { return tx.UpsertChatDebugLoggingAllowUsers(ctx, req.AllowUsers) },
	)
	if err != nil {
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
func (api *API) getChatAdvisorConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw, err := api.Database.GetChatAdvisorConfig(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching advisor configuration.",
			Detail:  err.Error(),
		})
		return
	}

	var resp codersdk.AdvisorConfig
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Stored advisor configuration is invalid.",
			Detail:  err.Error(),
		})
		return
	}
	resp.MaxUsesPerRun = max(resp.MaxUsesPerRun, 0)
	resp.MaxOutputTokens = max(resp.MaxOutputTokens, 0)
	resp.Enabled = api.Experiments.Enabled(codersdk.ExperimentChatAdvisor)

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putChatAdvisorConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateAdvisorConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.DeprecatedModelConfigID != nil || req.DeprecatedReasoningEffort != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Advisor model settings moved to PUT /api/experimental/organizations/{organization}/chats/model-overrides/advisor.",
		})
		return
	}
	if req.MaxUsesPerRun < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("max_uses_per_run %d must be non-negative.", req.MaxUsesPerRun),
		})
		return
	}
	if req.MaxOutputTokens < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("max_output_tokens %d must be non-negative.", req.MaxOutputTokens),
		})
		return
	}

	runtimeConfig := codersdk.AdvisorConfig{
		MaxUsesPerRun:   req.MaxUsesPerRun,
		MaxOutputTokens: req.MaxOutputTokens,
	}

	raw, err := json.Marshal(runtimeConfig)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error encoding advisor configuration.",
			Detail:  err.Error(),
		})
		return
	}
	if err := api.Database.UpsertChatAdvisorConfig(ctx, string(raw)); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating advisor configuration.",
			Detail:  err.Error(),
		})
		return
	}

	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventAdvisorConfig, uuid.Nil)

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
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

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

	value := d.String()
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingWorkspaceTTL, value,
		func(tx database.Store) error { return tx.UpsertChatWorkspaceTTL(ctx, value) },
	)
	if httpapi.Is404Error(err) {
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
// @Router /api/experimental/chats/config/retention-days [get]
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
// @Router /api/experimental/chats/config/retention-days [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatRetentionDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

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
	value := strconv.FormatInt(int64(req.RetentionDays), 10)
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingChatRetentionDays, value,
		func(tx database.Store) error { return tx.UpsertChatRetentionDays(ctx, req.RetentionDays) },
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat retention days.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// getChatDebugRetentionDays returns the deployment-wide chat debug run
// retention window. Any authenticated user can read it; writes require admin.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatDebugRetentionDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	retentionDays, err := api.Database.GetChatDebugRetentionDays(ctx, codersdk.DefaultChatDebugRetentionDays)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat debug retention days.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatDebugRetentionDaysResponse{
		DebugRetentionDays: retentionDays,
	})
}

// Keep in sync with the validation schema in
// site/src/pages/AgentsPage/components/DebugRetentionSettings.tsx.
const chatDebugRetentionDaysMaximum = 3650 // ~10 years

// putChatDebugRetentionDays updates the deployment-wide chat debug run
// retention window. Admin-only.
func (api *API) putChatDebugRetentionDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatDebugRetentionDaysRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.DebugRetentionDays < 0 || req.DebugRetentionDays > chatDebugRetentionDaysMaximum {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Chat debug retention days must be between 0 and %d.", chatDebugRetentionDaysMaximum),
		})
		return
	}
	value := strconv.FormatInt(int64(req.DebugRetentionDays), 10)
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingChatDebugRetentionDays, value,
		func(tx database.Store) error { return tx.UpsertChatDebugRetentionDays(ctx, req.DebugRetentionDays) },
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat debug retention days.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// getChatAutoArchiveDays returns the deployment-wide auto-archive
// window. Any authenticated user can read it (same as retention
// days); writes require admin.
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatAutoArchiveDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	autoArchiveDays, err := api.Database.GetChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat auto-archive days.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatAutoArchiveDaysResponse{
		AutoArchiveDays: autoArchiveDays,
	})
}

// Upper bound for the auto-archive window. Keep in sync with
// the validation schema in site/src/pages/AgentsPage/components/AutoArchiveSettings.tsx.
const autoArchiveDaysMaximum = 3650 // ~10 years

// putChatAutoArchiveDays updates the deployment-wide auto-archive
// window. Admin-only; documented in docs/ai-coder/agents/chats-api.md.
func (api *API) putChatAutoArchiveDays(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	aReq, commitAudit := api.initChatOperationalSettingsAudit(rw, r)
	defer commitAudit(true)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateChatAutoArchiveDaysRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.AutoArchiveDays < 0 || req.AutoArchiveDays > autoArchiveDaysMaximum {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Auto-archive days must be between 0 and %d.", autoArchiveDaysMaximum),
		})
		return
	}
	value := strconv.FormatInt(int64(req.AutoArchiveDays), 10)
	err := api.auditedChatOperationalSettingWrite(
		ctx, aReq, commitAudit, chatOperationalSettingChatAutoArchiveDays, value,
		func(tx database.Store) error { return tx.UpsertChatAutoArchiveDays(ctx, req.AutoArchiveDays) },
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat auto-archive days.",
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
	var params codersdk.UserChatCustomPrompt
	if !httpapi.ReadLimit(ctx, rw, r, int64(2*maxSystemPromptLenBytes), &params) {
		return
	}

	sanitizedPrompt := codersdk.SanitizePromptText(params.CustomPrompt)
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

	// The preference is personal, so model existence checks must not depend on
	// whether the user can read the organization model configuration.
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Upload chat file
// @ID upload-chat-file
// @Security CoderSessionToken
// @Tags Chats
// @Accept image/png,image/jpeg,image/gif,image/webp,text/plain,text/markdown,text/csv,application/json,application/pdf
// @Produce json
// @Param organization query string true "Organization ID" format(uuid)
// @Success 201 {object} codersdk.UploadChatFileResponse
// @Failure 413 {object} codersdk.Response "Request body exceeds 10 MiB"
// @Router /api/experimental/chats/files [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) postChatFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

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
	// NOTE: This authorize check is intentionally placed after query
	// parameter parsing because we need orgID to scope the RBAC check
	// to the correct org.
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String()).InOrg(orgID)) {
		httpapi.Forbidden(rw)
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
	// application/octet-stream means the client could not classify the file
	// ahead of time, so we defer to byte classification below.
	if contentType != "application/octet-stream" && !chatfiles.IsAllowedPromptInputMediaType(contentType) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unsupported file type.",
			Detail:  fmt.Sprintf("Allowed types: %s.", chatfiles.AllowedPromptInputMediaTypesString()),
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

	r.Body = http.MaxBytesReader(rw, r.Body, codersdk.MaxChatFileSizeBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httpapi.RecordRequestBodyLimit(ctx, codersdk.MaxChatFileSizeBytes)
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "File too large.",
				Detail:  fmt.Sprintf("Maximum file size is %d bytes.", codersdk.MaxChatFileSizeBytes),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read file from request.",
			Detail:  err.Error(),
		})
		return
	}

	// Classify the actual content before applying the upload policy so
	// a client cannot spoof Content-Type to serve active content.
	filename, detected, err := chatfiles.PrepareStoredFile(filename, filename, data)
	if err != nil {
		switch {
		case errors.Is(err, chatfiles.ErrStoredFileNameRequired):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Filename is required.",
				Detail:  "Provide a filename in the Content-Disposition header.",
			})
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid file.",
				Detail:  err.Error(),
			})
		}
		return
	}
	if !chatfiles.IsAllowedPromptInputMediaType(detected) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unsupported file type.",
			Detail:  fmt.Sprintf("Allowed types: %s.", chatfiles.AllowedPromptInputMediaTypesString()),
		})
		return
	}
	// The compatibility check below is security-critical: it keeps exact
	// media-type matching by default while allowing application/
	// octet-stream uploads to defer to byte classification, and letting
	// text/plain refine to safe text subtypes such as JSON, CSV, and
	// Markdown. Combined with the X-Content-Type-Options: nosniff header
	// applied globally, this still prevents clients from smuggling binary
	// or active content under a safer declared Content-Type.
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

// ChatFileDownloadClaims are the signed claims embedded in a chat file
// download URL token.
type ChatFileDownloadClaims struct {
	jwtutils.RegisteredClaims
	FileID uuid.UUID `json:"file_id"`
	UserID uuid.UUID `json:"user_id"`
}

func (c ChatFileDownloadClaims) Validate(expected jwt.Expected) error {
	if c.FileID == uuid.Nil {
		return xerrors.New("file ID is required")
	}
	if c.UserID == uuid.Nil {
		return xerrors.New("user ID is required")
	}
	return c.RegisteredClaims.Validate(expected)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Create chat file download URL
// @ID create-chat-file-download-url
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param file path string true "File ID" format(uuid)
// @Success 200 {object} codersdk.ChatFileDownloadURLResponse
// @Router /api/experimental/chats/files/{file}/download-url [post]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
func (api *API) postChatFileDownloadURL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID, err := uuid.Parse(chi.URLParam(r, "file"))
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

	now := time.Now()
	// Truncate to whole seconds so the advertised expiry matches the JWT
	// exp claim, which jwt.NewNumericDate stores at second precision.
	expiresAt := now.Add(cryptokeys.ChatFilesTokenDuration).Truncate(time.Second)
	claims := ChatFileDownloadClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Expiry:   jwt.NewNumericDate(expiresAt),
			IssuedAt: jwt.NewNumericDate(now),
		},
		FileID: fileID,
		UserID: httpmw.APIKey(r).UserID,
	}
	token, err := jwtutils.Sign(ctx, api.ChatFileTokenKeyCache, claims)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat file download URL.",
			Detail:  err.Error(),
		})
		return
	}

	downloadURL := api.AccessURL.JoinPath("api", "experimental", "chats", "files", fileID.String(), "download")
	downloadURL.RawQuery = url.Values{"token": {token}}.Encode()
	digest := sha256.Sum256(chatFile.Data)
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatFileDownloadURLResponse{
		URL:       downloadURL.String(),
		ExpiresAt: expiresAt,
		SHA256:    hex.EncodeToString(digest[:]),
		SizeBytes: int64(len(chatFile.Data)),
		Name:      chatFile.Name,
		MimeType:  chatFile.Mimetype,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Download chat file with signed token
// @ID download-chat-file
// @Tags Chats
// @Produce image/png,image/jpeg,image/gif,image/webp,text/plain,text/markdown,text/csv,application/json,application/pdf
// @Param file path string true "File ID" format(uuid)
// @Param token query string true "Signed download token"
// @Success 200
// @Router /api/experimental/chats/files/{file}/download [get]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
func (api *API) downloadChatFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fileID, err := uuid.Parse(chi.URLParam(r, "file"))
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	var claims ChatFileDownloadClaims
	if err := jwtutils.Verify(ctx, api.ChatFileTokenKeyCache, r.URL.Query().Get("token"), &claims); err != nil || claims.FileID != fileID {
		httpapi.ResourceNotFound(rw)
		return
	}

	subject, status, err := httpmw.UserRBACSubject(ctx, api.Database, claims.UserID, rbac.ScopeAll)
	if err != nil || status != database.UserStatusActive {
		httpapi.ResourceNotFound(rw)
		return
	}

	chatFile, err := api.Database.GetChatFileByID(dbauthz.As(ctx, subject), fileID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		// This endpoint is reachable without a session token, so internal
		// error details stay in the logs rather than the response body.
		api.Logger.Error(ctx, "failed to get chat file for signed download", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat file.",
		})
		return
	}
	// Never let private caches replay a signed URL past its expiry;
	// revocation is re-checked only when the request reaches coderd.
	rw.Header().Set("Cache-Control", "no-store")
	api.serveChatFile(ctx, rw, chatFile)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get chat file
// @ID get-chat-file
// @Security CoderSessionToken
// @Tags Chats
// @Produce image/png,image/jpeg,image/gif,image/webp,text/plain,text/markdown,text/csv,application/json,application/pdf
// @Param file path string true "File ID" format(uuid)
// @Success 200
// @Router /api/experimental/chats/files/{file} [get]
// @Description Experimental: this endpoint is subject to change.
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

	rw.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	api.serveChatFile(r.Context(), rw, chatFile)
}

// serveChatFile writes the file body and content headers; callers set
// Cache-Control because signed and session-authenticated downloads have
// different caching requirements.
func (api *API) serveChatFile(ctx context.Context, rw http.ResponseWriter, chatFile database.ChatFile) {
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
	rw.Header().Set("Content-Length", strconv.Itoa(len(chatFile.Data)))
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(chatFile.Data); err != nil {
		api.Logger.Debug(ctx, "failed to write chat file response", slog.Error(err))
	}
}

func createChatInputFromRequest(ctx context.Context, db database.Store, req codersdk.CreateChatRequest) (
	[]codersdk.ChatMessagePart,
	string,
	*codersdk.Response,
) {
	content, pasteData, inputError := createChatInputFromParts(ctx, db, req.Content, "content")
	if inputError != nil {
		return nil, "", inputError
	}
	// Derive titleSource through the same chatprompt.TitleText used at
	// generation time; auto-titling gates on that equality. Paste blobs
	// are copied only when text and file-reference parts yield nothing.
	titleSource := chatprompt.TitleText(content, nil)
	if titleSource == "" && len(pasteData) > 0 {
		pasteText := make(map[uuid.UUID]string, len(pasteData))
		for id, data := range pasteData {
			pasteText[id] = chatprompt.TitlePasteText(data)
		}
		titleSource = chatprompt.TitleText(content, pasteText)
	}
	return content, titleSource, nil
}

// createChatInputFromParts validates input parts and converts them to
// message content. The returned map holds pasted-text blob references
// by file ID; the create path derives a title from it, message send
// and edit discard it without copying blob data.
func createChatInputFromParts(
	ctx context.Context,
	db database.Store,
	parts []codersdk.ChatInputPart,
	fieldName string,
) ([]codersdk.ChatMessagePart, map[uuid.UUID][]byte, *codersdk.Response) {
	if len(parts) == 0 {
		return nil, nil, &codersdk.Response{
			Message: "Content is required.",
			Detail:  "Content cannot be empty.",
		}
	}

	content := make([]codersdk.ChatMessagePart, 0, len(parts))
	var pasteData map[uuid.UUID][]byte
	for i, part := range parts {
		switch strings.ToLower(strings.TrimSpace(string(part.Type))) {
		case string(codersdk.ChatInputPartTypeText):
			text := strings.TrimSpace(part.Text)
			if text == "" {
				return nil, nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].text cannot be empty.", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageText(text))
		case string(codersdk.ChatInputPartTypeFile):
			if part.FileID == uuid.Nil {
				return nil, nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].file_id is required for file parts.", fieldName, i),
				}
			}
			// Validate that the file exists and get its media type.
			// The loaded file data is only retained for synthetic
			// pastes below; LLM dispatch re-resolves file content via
			// chatFileResolver.
			chatFile, err := db.GetChatFileByID(ctx, part.FileID)
			if err != nil {
				if httpapi.Is404Error(err) {
					return nil, nil, &codersdk.Response{
						Message: "Invalid input part.",
						Detail:  fmt.Sprintf("%s[%d].file_id references a file that does not exist.", fieldName, i),
					}
				}
				return nil, nil, &codersdk.Response{
					Message: "Internal error.",
					Detail:  fmt.Sprintf("Failed to retrieve file for %s[%d].", fieldName, i),
				}
			}
			if !chatfiles.IsAllowedPromptInputMediaType(chatFile.Mimetype) {
				return nil, nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].file_id references a file type that cannot be used as prompt input. Allowed types: %s.", fieldName, i, chatfiles.AllowedPromptInputMediaTypesString()),
				}
			}
			content = append(content, codersdk.ChatMessageFile(part.FileID, chatFile.Mimetype, chatFile.Name))
			// Retain blob references for create-time title derivation;
			// send and edit paths discard the map.
			if chatprompt.IsSyntheticPaste(chatFile.Name, chatFile.Mimetype) {
				if pasteData == nil {
					pasteData = make(map[uuid.UUID][]byte)
				}
				pasteData[part.FileID] = chatFile.Data
			}
		// file-reference parts carry inline code snippets, not uploaded
		// files. They have no FileID and are excluded from file tracking.
		case string(codersdk.ChatInputPartTypeFileReference):
			if part.FileName == "" {
				return nil, nil, &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].file_name cannot be empty for file-reference.", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageFileReference(part.FileName, part.StartLine, part.EndLine, part.Content))
		default:
			return nil, nil, &codersdk.Response{
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

	if len(content) == 0 {
		return nil, nil, &codersdk.Response{
			Message: "Content is required.",
			Detail:  fmt.Sprintf("%s must include at least one text or file part.", fieldName),
		}
	}
	return content, pasteData, nil
}

func writeChatFileError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, chatstate.ErrChatFileCapExceeded):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat attachment limit reached.",
			Detail:  fmt.Sprintf("A chat can reference at most %d attachments. Remove some attachments or start a new chat.", codersdk.MaxChatFileIDs),
		})
	case errors.Is(err, chatstate.ErrChatFileUnavailable):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat attachment unavailable.",
			Detail:  "An attachment is no longer available. Upload it again and retry.",
		})
	default:
		return false
	}
	return true
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

func parseUserAIProviderID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "aiProvider"))
}

func convertAIProviderSummary(provider database.AIProvider) codersdk.AIProviderSummary {
	displayName := provider.Name
	if provider.DisplayName.Valid && provider.DisplayName.String != "" {
		displayName = provider.DisplayName.String
	}
	return codersdk.AIProviderSummary{
		ID:          provider.ID,
		Type:        codersdk.AIProviderType(provider.Type),
		Name:        provider.Name,
		DisplayName: displayName,
		Icon:        provider.Icon,
		Enabled:     provider.Enabled,
		Deleted:     provider.Deleted,
	}
}

func (api *API) listUserAIProviderKeyConfigs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser := httpmw.UserParam(r)
	//nolint:gocritic // Users can list limited provider metadata to manage their own AI provider keys.
	metadataCtx := dbauthz.AsAIProviderMetadataReader(ctx)
	providers, err := api.Database.GetAIProviders(metadataCtx, database.GetAIProvidersParams{IncludeDisabled: true})
	if err != nil {
		api.Logger.Error(ctx, "failed to list user AI provider configs", slog.Error(err), slog.F("user_id", targetUser.ID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list AI providers."})
		return
	}
	keys, err := api.Database.GetUserAIProviderKeysByUserID(ctx, targetUser.ID)
	if err != nil {
		api.Logger.Error(ctx, "failed to list user AI provider keys", slog.Error(err), slog.F("user_id", targetUser.ID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list user AI provider keys."})
		return
	}

	keysByProviderID := make(map[uuid.UUID]struct{}, len(keys))
	for _, key := range keys {
		keysByProviderID[key.AIProviderID] = struct{}{}
	}

	visibleProviders := make([]database.AIProvider, 0, len(providers))
	visibleProviderIDs := make([]uuid.UUID, 0, len(providers))
	for _, provider := range providers {
		_, hasUserKey := keysByProviderID[provider.ID]
		if !provider.Enabled && !hasUserKey {
			continue
		}
		visibleProviders = append(visibleProviders, provider)
		visibleProviderIDs = append(visibleProviderIDs, provider.ID)
	}

	providerKeysByProviderID := make(map[uuid.UUID]struct{}, len(visibleProviderIDs))
	if len(visibleProviderIDs) > 0 {
		providerKeyIDs, err := api.Database.GetAIProviderKeyPresence(metadataCtx, visibleProviderIDs)
		if err != nil {
			api.Logger.Error(ctx, "failed to list AI provider key presence", slog.Error(err), slog.F("user_id", targetUser.ID))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list AI provider keys."})
			return
		}
		for _, providerID := range providerKeyIDs {
			providerKeysByProviderID[providerID] = struct{}{}
		}
	}

	byokEnabled := api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value()
	configs := make([]codersdk.UserAIProviderKeyConfig, 0, len(visibleProviders))
	for _, provider := range visibleProviders {
		_, hasUserKey := keysByProviderID[provider.ID]
		_, hasProviderKey := providerKeysByProviderID[provider.ID]
		configs = append(configs, codersdk.UserAIProviderKeyConfig{
			Provider:          convertAIProviderSummary(provider),
			HasUserAPIKey:     hasUserKey,
			HasProviderAPIKey: hasProviderKey,
			BYOKEnabled:       byokEnabled,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, configs)
}

func (api *API) upsertUserAIProviderKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value() {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "BYOK is disabled."})
		return
	}
	targetUser := httpmw.UserParam(r)
	providerID, err := parseUserAIProviderID(r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid AI provider ID."})
		return
	}
	//nolint:gocritic // Users can attach their own key to an enabled provider without AI provider admin permissions.
	metadataCtx := dbauthz.AsAIProviderMetadataReader(ctx)
	provider, err := api.Database.GetAIProviderByID(metadataCtx, providerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: "AI provider not found."})
			return
		}
		api.Logger.Error(ctx, "failed to get AI provider", slog.Error(err), slog.F("ai_provider_id", providerID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to get AI provider."})
		return
	}
	if !provider.Enabled {
		writeChatProviderPreconditionError(ctx, rw, errChatProviderDisabled)
		return
	}
	var req codersdk.CreateUserAIProviderKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if err := validateChatProviderAPIKeySize(req.APIKey); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "API key too large.",
			Detail:  err.Error(),
		})
		return
	}
	if req.APIKey == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "API key is required."})
		return
	}
	if strings.TrimSpace(req.APIKey) != req.APIKey {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "API key must not contain leading or trailing whitespace."})
		return
	}
	providerKeys, err := api.Database.GetAIProviderKeyPresence(metadataCtx, []uuid.UUID{providerID})
	if err != nil {
		api.Logger.Error(ctx, "failed to list AI provider key presence", slog.Error(err), slog.F("ai_provider_id", providerID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list AI provider keys."})
		return
	}
	now := api.Clock.Now()
	_, err = api.Database.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       targetUser.ID,
		AIProviderID: providerID,
		APIKey:       req.APIKey,
		ApiKeyKeyID:  sql.NullString{},
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to update user AI provider key", slog.Error(err), slog.F("user_id", targetUser.ID), slog.F("ai_provider_id", providerID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to update user AI provider key."})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserAIProviderKeyConfig{
		Provider:          convertAIProviderSummary(provider),
		HasUserAPIKey:     true,
		HasProviderAPIKey: len(providerKeys) > 0,
		BYOKEnabled:       true,
	})
}

func (api *API) deleteUserAIProviderKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser := httpmw.UserParam(r)
	providerID, err := parseUserAIProviderID(r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid AI provider ID."})
		return
	}
	if err := api.Database.DeleteUserAIProviderKey(ctx, database.DeleteUserAIProviderKeyParams{UserID: targetUser.ID, AIProviderID: providerID}); err != nil {
		api.Logger.Error(ctx, "failed to delete user AI provider key", slog.Error(err), slog.F("user_id", targetUser.ID), slog.F("ai_provider_id", providerID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to delete user AI provider key."})
		return
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// userAIProviderKeyStatusByProviderID returns only credential presence for the
// requesting user. The privileged lookup is contained here so callers never
// receive key material or require user:read_personal.
func (api *API) userAIProviderKeyStatusByProviderID(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	//nolint:gocritic // The result is reduced to credential presence for exactly the requesting user.
	rows, err := api.Database.GetUserAIProviderKeysByUserID(dbauthz.AsChatd(ctx), userID)
	if err != nil {
		return nil, err
	}
	statusByProviderID := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		statusByProviderID[row.AIProviderID] = strings.TrimSpace(row.APIKey) != ""
	}
	return statusByProviderID, nil
}

func (api *API) configuredProvidersFromAIProviders(ctx context.Context, providers []database.AIProvider) ([]chatprovider.ConfiguredProvider, error) {
	if len(providers) == 0 {
		return nil, nil
	}
	providerIDs := make([]uuid.UUID, 0, len(providers))
	for _, provider := range providers {
		providerIDs = append(providerIDs, provider.ID)
	}
	keys, err := api.Database.GetAIProviderKeysByProviderIDs(ctx, providerIDs)
	if err != nil {
		return nil, xerrors.Errorf("get AI provider keys: %w", err)
	}
	keysByProviderID := make(map[uuid.UUID][]database.AIProviderKey, len(providers))
	for _, key := range keys {
		keysByProviderID[key.ProviderID] = append(keysByProviderID[key.ProviderID], key)
	}
	configuredProviders := make([]chatprovider.ConfiguredProvider, 0, len(providers))
	for _, provider := range providers {
		configuredProviders = append(configuredProviders, api.configuredProviderFromAIProviderKeys(provider, keysByProviderID[provider.ID]))
	}
	return configuredProviders, nil
}

func (api *API) configuredProviderFromAIProviderKeys(provider database.AIProvider, keys []database.AIProviderKey) chatprovider.ConfiguredProvider {
	apiKey := ""
	for _, key := range keys {
		if key.APIKey != "" {
			apiKey = key.APIKey
			break
		}
	}
	return chatprovider.ConfiguredProvider{
		ProviderID:                 provider.ID,
		Provider:                   string(provider.Type),
		APIKey:                     apiKey,
		BaseURL:                    provider.BaseUrl,
		CentralAPIKeyEnabled:       true,
		AllowUserAPIKey:            api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value(),
		AllowCentralAPIKeyFallback: true,
	}
}

func writeLegacyChatProviderGone(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusGone, codersdk.Response{
		Message: "Legacy chat provider APIs were removed. Use AI provider APIs instead.",
		Detail:  "See https://coder.com/docs/ai-coder/agents/models#providers for AI provider configuration.",
	})
}

func (*API) listChatProviders(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) createChatProvider(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) updateChatProvider(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) deleteChatProvider(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) listUserChatProviderConfigs(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) upsertUserChatProviderKey(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (*API) deleteUserChatProviderKey(rw http.ResponseWriter, r *http.Request) {
	writeLegacyChatProviderGone(rw, r)
}

func (api *API) listDefaultOrganizationChatModelConfigs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	apiKey := httpmw.APIKey(r)

	if !chatModelConfigReadScope(apiKey.Scopes) {
		httpapi.Forbidden(rw)
		return
	}

	configs, err := api.Database.GetChatModelConfigs(ctx, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat model configs.",
			Detail:  err.Error(),
		})
		return
	}

	resp := make([]codersdk.ChatModel, 0, len(configs))
	for _, config := range configs {
		resp = append(resp, convertChatModelConfig(config))
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary List AI models and provider descriptors in an organization
// @ID list-ai-models-by-organization
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Success 200 {object} codersdk.OrganizationChatModelsResponse
// @Router /api/experimental/organizations/{organization}/chats/models [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatModelConfigsByOrganization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	apiKey := httpmw.APIKey(r)

	// Keep the token scope gate separate from the authorized database filter.
	// The filter applies each config ACL and the organization predicate.
	if !chatModelConfigReadScope(apiKey.Scopes) {
		httpapi.Forbidden(rw)
		return
	}
	configs, visible, err := api.readableChatModelsInOrganization(ctx, r, organization)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if !visible {
		httpapi.ResourceNotFound(rw)
		return
	}

	availability, err := api.getUserChatProviderAvailability(ctx, apiKey.UserID, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load chat model availability.",
			Detail:  err.Error(),
		})
		return
	}

	providers, err := api.chatModelProviderDescriptors(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list AI providers.",
			Detail:  err.Error(),
		})
		return
	}

	for i := range providers {
		if status, ok := availability.providerStatusByID[providers[i].ID]; ok {
			providers[i].Available = status.Available
			if !status.Available {
				providers[i].UnavailableReason = status.UnavailableReason
			}
		}
	}
	resp := codersdk.OrganizationChatModelsResponse{
		Models:               make([]codersdk.ChatModel, 0, len(configs)),
		Providers:            providers,
		UnsupportedProviders: chatprovider.UnsupportedProviders(availability.configuredProviders),
	}
	for _, config := range configs {
		resp.Models = append(resp.Models, convertChatModelConfig(config))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) readableChatModelsInOrganization(
	ctx context.Context,
	r *http.Request,
	organization database.Organization,
) ([]database.ChatModelConfig, bool, error) {
	configs, err := api.Database.GetChatModelConfigs(ctx, organization.ID)
	if err != nil {
		return nil, false, err
	}
	if len(configs) > 0 {
		return configs, true, nil
	}
	return configs, api.Authorize(r, policy.ActionRead, organization.RBACObject()), nil
}

func chatModelConfigReadScope(scopes database.APIKeyScopes) bool {
	return scopes.Has(database.ApiKeyScopeCoderAll) ||
		scopes.Has(database.ApiKeyScopeChatModelConfigRead)
}

// chatModelProviderDescriptors assembles the redacted provider descriptors
// for the org model collection. The caller already passed the org's model
// read gate; providers are deployment-scoped and an org admin cannot read
// them directly, so the fetch runs under a narrow AsChatd context scoped to
// exactly these two reads and the result is projected to the fixed redacted
// fields (no key material, base URLs, or headers). Disclosure matches what
// /api/experimental/chats/models already shows any authenticated caller.
func (api *API) chatModelProviderDescriptors(
	ctx context.Context,
	userID uuid.UUID,
) ([]codersdk.ChatModelProviderDescriptor, error) {
	//nolint:gocritic // Fixed redacted projection under the model read gate; see function doc.
	providers, err := api.Database.GetAIProviders(dbauthz.AsChatd(ctx), database.GetAIProvidersParams{IncludeDisabled: true})
	if err != nil {
		return nil, err
	}

	//nolint:gocritic // Key presence is boolean metadata, not key material; same redaction as the deployment endpoint.
	keysByProvider, err := loadAIProviderKeysByProvider(dbauthz.AsChatd(ctx), api.Database)
	if err != nil {
		return nil, err
	}

	userKeyStatus := make(map[uuid.UUID]bool)
	if api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value() {
		userKeyStatus, err = api.userAIProviderKeyStatusByProviderID(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	out := make([]codersdk.ChatModelProviderDescriptor, 0, len(providers))
	for _, provider := range providers {
		display := provider.Name
		if provider.DisplayName.Valid && provider.DisplayName.String != "" {
			display = provider.DisplayName.String
		}
		hasKey := false
		for _, key := range keysByProvider[provider.ID] {
			if key.APIKey != "" {
				hasKey = true
				break
			}
		}
		hasUserKey := userKeyStatus[provider.ID]
		out = append(out, codersdk.ChatModelProviderDescriptor{
			ID:                 provider.ID,
			Type:               string(provider.Type),
			DisplayName:        display,
			Icon:               provider.Icon,
			Enabled:            provider.Enabled,
			HasAPIKey:          hasKey,
			HasUserAPIKey:      hasUserKey,
			HasEffectiveAPIKey: hasKey || hasUserKey || provider.Type == database.AIProviderTypeBedrock,
			AllowUserAPIKey:    api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value(),
		})
	}
	return out, nil
}

func chatModelConfigRBACObject(config database.ChatModelConfig) rbac.Object {
	return rbac.ResourceChatModelConfig.
		WithID(config.ID).
		InOrg(config.OrganizationID).
		WithACLUserList(config.UserACL.RBACACL()).
		WithGroupACL(config.GroupACL.RBACACL())
}

// getChatModelConfig returns one chat model config after the organization and
// model identities have been resolved by route middleware.
// @Summary Get an AI model
// @ID get-ai-model
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID"
// @Success 200 {object} codersdk.ChatModel
// @Router /api/experimental/organizations/{organization}/chats/models/{model} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionRead, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, convertChatModelConfig(config))
}

type chatModelConfigProviderModelError struct {
	Response codersdk.Response
}

func (e *chatModelConfigProviderModelError) Error() string {
	return e.Response.Message
}

func validateChatModelConfigProviderModel(aiProvider database.AIProvider, model string) *chatModelConfigProviderModelError {
	if err := chatd.ValidateAIGatewayProviderModel(aiProvider, model); err != nil {
		return &chatModelConfigProviderModelError{
			Response: codersdk.Response{
				Message: "OpenRouter-like provider configured as type openai does not support slash-namespaced models.",
				Detail:  "Change the AI provider type to openrouter or openai-compat. Slash-namespaced model IDs on OpenRouter-like gateways require one of those provider types.",
			},
		}
	}
	return nil
}

// inChatModelConfigWriteTx runs fn in a transaction that holds the advisory
// lock serializing chat model config writes for one organization. All writes
// to the table must go through this helper so concurrent writers cannot act
// on stale reads, e.g. two creates on an empty organization both
// self-promoting to default and violating the
// idx_chat_model_configs_single_default unique index.
//
// The transaction must run at ReadCommitted. A snapshot isolation level takes
// the snapshot at the lock statement, so a re-read inside fn would observe
// pre-lock state and reintroduce the lost update this helper prevents.
func (api *API) inChatModelConfigWriteTx(
	ctx context.Context,
	organizationID uuid.UUID,
	fn func(tx database.Store) error,
) error {
	return api.Database.InTx(func(tx database.Store) error {
		// organization_id is immutable. UpdateChatModelConfig cannot change it,
		// so a lock key from a pre-lock read stays correct.
		lockID := database.GenLockID("chat_model_config_writes:" + organizationID.String())
		if err := tx.AcquireLock(ctx, lockID); err != nil {
			return xerrors.Errorf("acquire chat model config write lock: %w", err)
		}
		return fn(tx)
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted})
}

type chatModelConfigAuditTransition struct {
	Old database.ChatModelConfig
	New database.ChatModelConfig
}

func (api *API) auditChatModelConfigTransitions(
	ctx context.Context,
	r *http.Request,
	userID uuid.UUID,
	status int,
	transitions []chatModelConfigAuditTransition,
) {
	if len(transitions) == 0 {
		return
	}
	auditor := api.Auditor.Load()
	auditCtx := context.WithoutCancel(ctx)
	requestID := httpmw.RequestID(r)
	for _, transition := range transitions {
		audit.BackgroundAudit(auditCtx, &audit.BackgroundAuditParams[database.ChatModelConfig]{
			Audit:          *auditor,
			Log:            api.Logger,
			UserID:         userID,
			RequestID:      requestID,
			Status:         status,
			IP:             r.RemoteAddr,
			UserAgent:      r.UserAgent(),
			Action:         database.AuditActionWrite,
			OrganizationID: transition.New.OrganizationID,
			Old:            transition.Old,
			New:            transition.New,
		})
	}
}

// @Summary Create an AI model in an organization
// @ID create-ai-model
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param request body codersdk.CreateChatModelRequest true "Model"
// @Success 201 {object} codersdk.ChatModel
// @Router /api/experimental/organizations/{organization}/chats/models [post]
// @x-apidocgen {"skip": true}
func (api *API) createChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatModelConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var req struct {
		codersdk.CreateChatModelRequest
		GroupACL json.RawMessage `json:"group_acl"`
		UserACL  json.RawMessage `json:"user_acl"`
	}
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.GroupACL != nil || req.UserACL != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Model ACLs cannot be set here. Use the nested /acl endpoint after creating the model.",
		})
		return
	}

	if req.AIProviderID == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "AI provider ID is required."})
		return
	}
	aiProviderID := uuid.NullUUID{UUID: *req.AIProviderID, Valid: true}

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

	// Seed the everyone-in-org read entry so the config stays visible to
	// members once ACLs are enforced; the Everyone group shares the
	// organization's ID.
	everyoneReadACL := database.ChatACL{
		organization.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
	}

	insertParams := database.InsertChatModelConfigParams{
		Model:                model,
		DisplayName:          strings.TrimSpace(req.DisplayName),
		Enabled:              enabled,
		IsDefault:            isDefault,
		ContextLimit:         contextLimit,
		CompressionThreshold: compressionThreshold,
		Options:              modelConfigRaw,
		AIProviderID:         aiProviderID,
		CreatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		UpdatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		OrganizationID:       organization.ID,
		GroupACL:             everyoneReadACL,
		UserACL:              database.ChatACL{},
	}

	var (
		inserted         database.ChatModelConfig
		auditTransitions []chatModelConfigAuditTransition
	)
	err := api.inChatModelConfigWriteTx(ctx, insertParams.OrganizationID, func(tx database.Store) error {
		currentDefault, err := tx.GetDefaultChatModelConfig(ctx, insertParams.OrganizationID)
		defaultExists := err == nil
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get default model config: %w", err)
		}
		insertAsDefault := isDefault || !defaultExists

		if insertAsDefault && defaultExists {
			if err := tx.UnsetDefaultChatModelConfigs(ctx, insertParams.OrganizationID); err != nil {
				return xerrors.Errorf("unset default model configs: %w", err)
			}
			//nolint:gocritic // The create write owns authorization for this transition.
			demoted, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), currentDefault.ID)
			if err != nil {
				return xerrors.Errorf("refresh demoted default chat model config: %w", err)
			}
			auditTransitions = append(auditTransitions, chatModelConfigAuditTransition{Old: currentDefault, New: demoted})
		}
		insertParams.IsDefault = insertAsDefault

		config, err := tx.InsertChatModelConfig(ctx, insertParams)
		if err != nil {
			if database.IsForeignKeyViolation(err, database.ForeignKeyChatModelConfigsAIProviderID) {
				return errChatProviderMissing
			}
			return err
		}
		inserted = config

		//nolint:gocritic // The provider fetch only reads the redacted descriptor fields.
		lockedAIProvider, err := tx.GetAIProviderByIDForReferenceLock(dbauthz.AsChatd(ctx), insertParams.AIProviderID.UUID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return errChatProviderMissing
			}
			return xerrors.Errorf("get AI provider for create: %w", err)
		}
		if !lockedAIProvider.Enabled {
			return errChatProviderDisabled
		}
		if err := validateChatModelConfigProviderModel(lockedAIProvider, insertParams.Model); err != nil {
			return err
		}

		transition, err := ensureDefaultChatModelConfig(ctx, tx, insertParams.OrganizationID)
		if err != nil {
			return err
		}
		if transition != nil {
			auditTransitions = append(auditTransitions, *transition)
		}

		refreshedConfig, err := tx.GetChatModelConfigByID(ctx, inserted.ID)
		if err != nil {
			return xerrors.Errorf("refresh inserted chat model config: %w", err)
		}
		inserted = refreshedConfig
		return nil
	})
	if err != nil {
		if writeChatProviderPreconditionError(ctx, rw, err) {
			return
		}
		var providerModelErr *chatModelConfigProviderModelError
		switch {
		case errors.As(err, &providerModelErr):
			httpapi.Write(ctx, rw, http.StatusBadRequest, providerModelErr.Response)
			return
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat model config already exists.",
				Detail:  err.Error(),
			})
			return
		case dbauthz.IsNotAuthorizedError(err):
			// The dbauthz create-in-org check is the write access boundary;
			// surface its denial as 403, not a concealed 404 or a 500.
			httpapi.Forbidden(rw)
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat model config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	aReq.New = inserted
	api.auditChatModelConfigTransitions(ctx, r, apiKey.UserID, http.StatusCreated, auditTransitions)
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, inserted.ID)

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatModelConfig(inserted))
}

// @Summary Update an AI model
// @ID update-ai-model
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID"
// @Param request body codersdk.UpdateChatModelRequest true "Model updates"
// @Success 200 {object} codersdk.ChatModel
// @Router /api/experimental/organizations/{organization}/chats/models/{model} [patch]
// @x-apidocgen {"skip": true}
func (api *API) updateChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	existing := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionUpdate, chatModelConfigRBACObject(existing)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatModelConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: existing.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = existing

	var req struct {
		codersdk.UpdateChatModelRequest
		GroupACL json.RawMessage `json:"group_acl"`
		UserACL  json.RawMessage `json:"user_acl"`
	}
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.GroupACL != nil || req.UserACL != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Model ACLs cannot be updated here. Use the nested /acl endpoint.",
		})
		return
	}

	if req.ContextLimit != nil && *req.ContextLimit <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Context limit must be greater than zero.",
		})
		return
	}

	// A PATCH that omits the field keeps the stored value, which the
	// chat_model_configs CHECK constraint already bounds to 0..100.
	var requestedCompressionThreshold *int32
	if req.CompressionThreshold != nil {
		if thresholdErr := validateChatCompressionThreshold(*req.CompressionThreshold); thresholdErr != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid compression threshold.",
				Detail:  thresholdErr.Error(),
			})
			return
		}
		requestedCompressionThreshold = req.CompressionThreshold
	}

	var requestedModelConfig json.RawMessage
	if req.ModelConfig != nil {
		encodedModelConfig, modelConfigErr := marshalChatModelCallConfig(req.ModelConfig)
		if modelConfigErr != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid model config.",
				Detail:  modelConfigErr.Error(),
			})
			return
		}
		requestedModelConfig = encodedModelConfig
	}

	var (
		updated          database.ChatModelConfig
		auditTransitions []chatModelConfigAuditTransition
	)
	err := api.inChatModelConfigWriteTx(ctx, existing.OrganizationID, func(tx database.Store) error {
		// The middleware lookup above only rejects unknown IDs; a concurrent
		// writer can change or delete the row between the two reads, so merge
		// against this copy.
		//nolint:gocritic // The model update below reauthorizes the locked row for update.
		lockedExisting, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), existing.ID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return errChatModelConfigNotFound
			}
			return xerrors.Errorf("get chat model config for update: %w", err)
		}

		aReq.Old = lockedExisting

		model := lockedExisting.Model
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			model = trimmed
		}
		displayName := lockedExisting.DisplayName
		if trimmed := strings.TrimSpace(req.DisplayName); trimmed != "" {
			displayName = trimmed
		}
		enabled := lockedExisting.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		isDefault := lockedExisting.IsDefault
		if req.IsDefault != nil {
			isDefault = *req.IsDefault
		}
		contextLimit := lockedExisting.ContextLimit
		if req.ContextLimit != nil {
			contextLimit = *req.ContextLimit
		}
		compressionThreshold := lockedExisting.CompressionThreshold
		if requestedCompressionThreshold != nil {
			compressionThreshold = *requestedCompressionThreshold
		}
		modelConfigRaw := lockedExisting.Options
		if requestedModelConfig != nil {
			modelConfigRaw = requestedModelConfig
		}
		aiProviderID := lockedExisting.AIProviderID
		if req.AIProviderID != nil {
			aiProviderID = uuid.NullUUID{UUID: *req.AIProviderID, Valid: true}
		}

		updateParams := database.UpdateChatModelConfigParams{
			Model:                model,
			DisplayName:          displayName,
			Enabled:              enabled,
			IsDefault:            isDefault,
			ContextLimit:         contextLimit,
			CompressionThreshold: compressionThreshold,
			Options:              modelConfigRaw,
			AIProviderID:         aiProviderID,
			UpdatedBy:            uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
			ID:                   lockedExisting.ID,
		}

		// An update that touches neither the provider nor the model cannot
		// invalidate the stored provider/model pair.
		revalidateProviderModel := updateParams.AIProviderID.Valid && (req.AIProviderID != nil || strings.TrimSpace(req.Model) != "")
		if revalidateProviderModel {
			//nolint:gocritic // The provider fetch only reads the redacted descriptor fields.
			aiProvider, err := tx.GetAIProviderByIDForReferenceLock(dbauthz.AsChatd(ctx), updateParams.AIProviderID.UUID)
			if err != nil {
				if xerrors.Is(err, sql.ErrNoRows) {
					return errChatProviderMissing
				}
				return xerrors.Errorf("get AI provider for update: %w", err)
			}
			if !aiProvider.Enabled {
				return errChatProviderDisabled
			}
			if err := validateChatModelConfigProviderModel(aiProvider, updateParams.Model); err != nil {
				return err
			}
		}

		setAsDefault := updateParams.IsDefault && !lockedExisting.IsDefault
		if setAsDefault {
			//nolint:gocritic // The target update owns authorization for the sibling transition.
			currentDefault, err := tx.GetDefaultChatModelConfig(dbauthz.AsSystemRestricted(ctx), lockedExisting.OrganizationID)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("get default model config before update: %w", err)
			}
			if err := tx.UnsetDefaultChatModelConfigs(ctx, lockedExisting.OrganizationID); err != nil {
				return xerrors.Errorf("unset default model configs: %w", err)
			}
			if err == nil {
				//nolint:gocritic // The target update owns authorization for the sibling transition.
				demoted, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), currentDefault.ID)
				if err != nil {
					return xerrors.Errorf("refresh demoted default chat model config: %w", err)
				}
				auditTransitions = append(auditTransitions, chatModelConfigAuditTransition{Old: currentDefault, New: demoted})
			}
		}

		updated, err = tx.UpdateChatModelConfig(ctx, updateParams)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return errChatModelConfigNotFound
			}
			return err
		}

		excludeConfigID := uuid.Nil
		if lockedExisting.IsDefault && req.IsDefault != nil && !*req.IsDefault {
			excludeConfigID = lockedExisting.ID
		}

		transition, err := ensureDefaultChatModelConfig(
			ctx,
			tx,
			lockedExisting.OrganizationID,
			excludeConfigID,
		)
		if err != nil {
			return err
		}
		if transition != nil {
			if transition.New.ID == lockedExisting.ID {
				updated = transition.New
			} else {
				auditTransitions = append(auditTransitions, *transition)
			}
		}
		return nil
	})
	if err != nil {
		if writeChatProviderPreconditionError(ctx, rw, err) {
			return
		}
		var providerModelErr *chatModelConfigProviderModelError
		switch {
		case errors.As(err, &providerModelErr):
			httpapi.Write(ctx, rw, http.StatusBadRequest, providerModelErr.Response)
			return
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat model config already exists.",
				Detail:  err.Error(),
			})
			return
		case dbauthz.IsNotAuthorizedError(err):
			// The dbauthz object update check is the write access boundary;
			// surface its denial as 403, not a concealed 404 or a 500.
			httpapi.Forbidden(rw)
			return
		case xerrors.Is(err, errChatModelConfigNotFound), httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
			return
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat model config.",
				Detail:  err.Error(),
			})
			return
		}
	}

	aReq.New = updated
	api.auditChatModelConfigTransitions(ctx, r, apiKey.UserID, http.StatusOK, auditTransitions)
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, updated.ID)

	httpapi.Write(ctx, rw, http.StatusOK, convertChatModelConfig(updated))
}

// @Summary Delete an AI model
// @ID delete-ai-model
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/models/{model} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatModelConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	existing := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionDelete, chatModelConfigRBACObject(existing)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatModelConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: existing.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = existing

	var auditTransitions []chatModelConfigAuditTransition
	if err := api.inChatModelConfigWriteTx(ctx, existing.OrganizationID, func(tx database.Store) error {
		//nolint:gocritic // The delete write below reauthorizes the locked row.
		current, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), existing.ID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return errChatModelConfigNotFound
			}
			return xerrors.Errorf("get chat model config for delete: %w", err)
		}
		aReq.Old = current
		if _, err := tx.DeleteChatModelConfigByID(ctx, current.ID); err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return errChatModelConfigNotFound
			}
			return err
		}
		transition, err := ensureDefaultChatModelConfig(ctx, tx, current.OrganizationID)
		if err != nil {
			return err
		}
		if transition != nil {
			auditTransitions = append(auditTransitions, *transition)
		}
		return nil
	}); err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			// The dbauthz object delete check is the write access boundary;
			// surface its denial as 403, not a concealed 404 or a 500.
			httpapi.Forbidden(rw)
			return
		}
		if xerrors.Is(err, errChatModelConfigNotFound) || httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat model config.",
			Detail:  err.Error(),
		})
		return
	}

	api.auditChatModelConfigTransitions(ctx, r, httpmw.APIKey(r).UserID, http.StatusNoContent, auditTransitions)
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, existing.ID)

	rw.WriteHeader(http.StatusNoContent)
}

// ensureDefaultChatModelConfig preserves one default for every organization
// that owns at least one chat model config.
func ensureDefaultChatModelConfig(
	ctx context.Context,
	tx database.Store,
	organizationID uuid.UUID,
	excludedConfigIDs ...uuid.UUID,
) (*chatModelConfigAuditTransition, error) {
	//nolint:gocritic // Default election is contained by action-authorized writes.
	_, err := tx.GetDefaultChatModelConfig(dbauthz.AsSystemRestricted(ctx), organizationID)
	switch {
	case err == nil:
		return nil, nil //nolint:nilnil // A nil transition means no default changed.
	case !xerrors.Is(err, sql.ErrNoRows):
		return nil, xerrors.Errorf("get default model config: %w", err)
	}

	orgModelConfigs, err := tx.GetChatModelConfigsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, xerrors.Errorf("list default chat model config candidates: %w", err)
	}
	if len(orgModelConfigs) == 0 {
		return nil, nil //nolint:nilnil // No model remains to promote.
	}

	// Prefer a config that can actually serve requests (enabled, under an
	// enabled provider) so the promoted default does not reject
	// omitted-model chat creation. Fall back to any non-excluded config
	// when no usable candidate exists.
	enabledRows, err := tx.GetEnabledChatModelConfigsByOrganization(ctx, organizationID)
	if err != nil {
		return nil, xerrors.Errorf("list enabled chat model configs: %w", err)
	}
	usable := make(map[uuid.UUID]struct{}, len(enabledRows))
	for _, row := range enabledRows {
		usable[row.ChatModelConfig.ID] = struct{}{}
	}

	excluded := make(map[uuid.UUID]struct{}, len(excludedConfigIDs))
	for _, configID := range excludedConfigIDs {
		if configID == uuid.Nil {
			continue
		}
		excluded[configID] = struct{}{}
	}

	var selected *database.ChatModelConfig
	for i := range orgModelConfigs {
		config := &orgModelConfigs[i]
		if _, skip := excluded[config.ID]; skip {
			continue
		}
		if selected == nil {
			selected = config
		}
		if _, ok := usable[config.ID]; ok {
			selected = config
			break
		}
	}
	if selected == nil {
		// Re-promoting the excluded sole candidate preserves the required default.
		selected = &orgModelConfigs[0]
	}
	candidateConfig := *selected

	if err := tx.UnsetDefaultChatModelConfigs(ctx, organizationID); err != nil {
		return nil, xerrors.Errorf("unset default model configs: %w", err)
	}

	params := chatModelConfigToUpdateParams(candidateConfig)
	params.IsDefault = true
	promoted, err := tx.UpdateChatModelConfig(ctx, params)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// Do not wrap with %w. Callers map target misses to 404, but a
			// default-candidate race is an internal retryable failure.
			return nil, xerrors.Errorf("set default model config: %v", err)
		}
		return nil, xerrors.Errorf("set default model config: %w", err)
	}
	return &chatModelConfigAuditTransition{Old: candidateConfig, New: promoted}, nil
}

func chatModelConfigToUpdateParams(
	config database.ChatModelConfig,
) database.UpdateChatModelConfigParams {
	return database.UpdateChatModelConfigParams{
		Model:                config.Model,
		DisplayName:          config.DisplayName,
		Enabled:              config.Enabled,
		IsDefault:            config.IsDefault,
		ContextLimit:         config.ContextLimit,
		CompressionThreshold: config.CompressionThreshold,
		Options:              config.Options,
		AIProviderID:         config.AIProviderID,
		UpdatedBy:            uuid.NullUUID{},
		ID:                   config.ID,
	}
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

func convertChatModelConfig(config database.ChatModelConfig) codersdk.ChatModel {
	modelConfig := unmarshalChatModelCallConfig(config.Options)
	var reasoningEffortConfig *codersdk.ChatModelReasoningEffortConfig
	if modelConfig != nil {
		reasoningEffortConfig = modelConfig.ReasoningEffort
	}

	// Active configs always carry a non-null ai_provider_id (CHECK
	// chat_model_configs_ai_provider_required_when_active).
	return codersdk.ChatModel{
		ID:                   config.ID,
		OrganizationID:       config.OrganizationID,
		AIProviderID:         config.AIProviderID.UUID,
		Model:                config.Model,
		DisplayName:          config.DisplayName,
		Enabled:              config.Enabled,
		IsDefault:            config.IsDefault,
		ContextLimit:         config.ContextLimit,
		CompressionThreshold: config.CompressionThreshold,
		ModelConfig:          modelConfig,
		ReasoningEfforts:     chatprovider.SelectableReasoningEfforts(reasoningEffortConfig),
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}
}

func marshalChatModelCallConfig(modelConfig *codersdk.ChatModelCallConfig) (json.RawMessage, error) {
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

func invalidReasoningEffortResponse(value string) codersdk.Response {
	return codersdk.Response{
		Message: "Invalid reasoning_effort value.",
		Detail:  fmt.Sprintf("Invalid value %q, must be one of %s", value, allowedReasoningEffortValues),
	}
}

func validateChatModelCallConfig(modelConfig *codersdk.ChatModelCallConfig) error {
	if modelConfig == nil {
		return nil
	}

	if err := validateChatModelReasoningEffortConfig(modelConfig); err != nil {
		return err
	}

	return validateChatModelProviderOptions(modelConfig.ProviderOptions)
}

// validateChatModelReasoningEffortConfig validates the reasoning_effort
// config. Values must exactly match the global effort scale, and default
// must not exceed max.
func validateChatModelReasoningEffortConfig(modelConfig *codersdk.ChatModelCallConfig) error {
	config := modelConfig.ReasoningEffort
	if config == nil {
		return nil
	}
	if config.Default == nil || config.Max == nil {
		return xerrors.New("reasoning_effort.default and reasoning_effort.max must both be set")
	}
	if !chatprovider.IsValidReasoningEffort(*config.Default) {
		return xerrors.Errorf("reasoning_effort.default %q must be one of %s", *config.Default, allowedReasoningEffortValues)
	}
	if !chatprovider.IsValidReasoningEffort(*config.Max) {
		return xerrors.Errorf("reasoning_effort.max %q must be one of %s", *config.Max, allowedReasoningEffortValues)
	}
	if !chatprovider.ReasoningEffortLessOrEqual(*config.Default, *config.Max) {
		return xerrors.New("reasoning_effort.default must not exceed reasoning_effort.max")
	}
	return nil
}

func validateChatModelProviderOptions(options *codersdk.ChatModelProviderOptions) error {
	if options == nil {
		return nil
	}

	if options.Anthropic != nil && options.Anthropic.ThinkingDisplay != nil &&
		strings.TrimSpace(*options.Anthropic.ThinkingDisplay) != "" &&
		chatprovider.AnthropicThinkingDisplayFromChat(options.Anthropic.ThinkingDisplay) == nil {
		return xerrors.Errorf("provider_options.anthropic.thinking_display must be one of summarized, omitted")
	}

	if options.Google != nil && options.Google.ThinkingConfig != nil &&
		options.Google.ThinkingConfig.ThinkingLevel != nil &&
		strings.TrimSpace(*options.Google.ThinkingConfig.ThinkingLevel) != "" {
		if chatprovider.GoogleThinkingLevelFromChat(options.Google.ThinkingConfig.ThinkingLevel) == nil {
			return xerrors.Errorf("provider_options.google.thinking_config.thinking_level must be one of minimal, low, medium, high")
		}
		if options.Google.ThinkingConfig.ThinkingBudget != nil {
			return xerrors.Errorf("provider_options.google.thinking_config.thinking_level cannot be combined with thinking_budget")
		}
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
		config.ReasoningEffort == nil &&
		isZeroChatModelOpenAIConfig(config.OpenAIConfig) &&
		isZeroChatModelProviderOptions(config.ProviderOptions)
}

func isZeroChatModelOpenAIConfig(config *codersdk.ChatModelOpenAIConfig) bool {
	return config == nil || config.UseResponsesAPI == nil
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

const maxChatProviderAPIKeySize = 10240 // 10 KB

func validateChatProviderAPIKeySize(apiKey string) error {
	if len(apiKey) > maxChatProviderAPIKeySize {
		return xerrors.Errorf("API key exceeds maximum size of 10 KB (%d bytes)", maxChatProviderAPIKeySize)
	}
	return nil
}

func writeChatProviderPreconditionError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	var message string
	switch {
	case xerrors.Is(err, errChatProviderMissing):
		message = "AI provider is not configured."
	case xerrors.Is(err, errChatProviderDisabled):
		message = "AI provider is disabled."
	default:
		return false
	}
	httpapi.Write(ctx, rw, http.StatusPreconditionFailed, codersdk.Response{Message: message})
	return true
}

var (
	errChatProviderDisabled    = xerrors.New("AI provider is disabled")
	errChatProviderMissing     = xerrors.New("AI provider is not configured")
	errChatModelConfigNotFound = xerrors.New("chat model config not found")
)

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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) postChatToolResults(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	apiKey := httpmw.APIKey(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Submitting tool results resumes LLM inference,
	// requiring update permission on the org-scoped chat resource.
	if !api.Authorize(r, policy.ActionUpdate, chat.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Only the chat owner may submit tool results. See
	// postChatMessages for the security rationale.
	if apiKey.UserID != chat.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the chat owner may submit tool results.",
		})
		return
	}

	if chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot submit tool results to an archived chat.",
		})
		return
	}

	// Cap the raw request body to prevent excessive memory use.
	var req codersdk.SubmitToolResultsRequest

	if !httpapi.ReadLimit(ctx, rw, r, int64(2*maxSystemPromptLenBytes), &req) {
		return
	}

	if len(req.Results) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one tool result is required.",
		})
		return
	}

	// The authoritative status check happens inside SubmitToolResults
	// under the row lock; that path also surfaces the shared
	// invalid-state response for chats that are not in a valid
	// execution state at all.

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
		if hookErr, ok := errors.AsType[*dispatch.Error](err); ok {
			writeChatHookDispatchFailed(ctx, rw, hookErr)
			return
		}
		var validationErr *chatd.ToolResultValidationError
		var conflictErr *chatd.ToolResultStatusConflictError
		switch {
		case xerrors.Is(err, chatd.ErrChatArchived):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot submit tool results to an archived chat.",
			})
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
		case errors.Is(err, chatstate.ErrChatNotFound):
			httpapi.ResourceNotFound(rw)
		case writeChatInvalidState(ctx, rw, err):
			// response already written
		case errors.Is(err, chatstate.ErrTransitionNotAllowed):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Chat is not waiting for tool results.",
				Detail:  err.Error(),
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Stream chat parts via WebSockets
// @ID stream-chat-parts-via-websockets
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatStreamEvent
// @Router /api/experimental/chats/{chat}/stream/parts [get]
// @x-apidocgen {"skip": true}
// @Description Experimental: this endpoint is subject to change.
func (api *API) streamChatParts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	if !api.requireChatDaemon(ctx, rw) {
		return
	}
	if err := api.chatDaemon.ServeStreamPartsAuthorized(rw, r, chat); err != nil {
		api.Logger.Named("chat_stream_parts").Debug(ctx, "chat stream parts closed", slog.Error(err))
	}
}
