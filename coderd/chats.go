package coderd

import (
	"bufio"
	"bytes"
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/gitsync"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const (
	chatDiffStatusTTL   = gitsync.DiffStatusTTL
	chatStreamBatchSize = 256

	chatContextLimitModelConfigKey                = "context_limit"
	chatContextCompressionThresholdModelConfigKey = "context_compression_threshold"
	defaultChatContextCompressionThreshold        = int32(70)
	minChatContextCompressionThreshold            = int32(0)
	maxChatContextCompressionThreshold            = int32(100)
	maxSystemPromptLenBytes                       = 131072 // 128 KiB
)

// chatGitRef holds the branch and remote origin reported by the
// workspace agent during a git operation.
type chatGitRef struct {
	Branch       string
	RemoteOrigin string
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) watchChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	sendEvent, senderClosed, err := httpapi.OneWayWebSocketEventSender(api.Logger)(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat watch stream.",
			Detail:  err.Error(),
		})
		return
	}
	defer func() {
		<-senderClosed
	}()

	cancelSubscribe, err := api.Pubsub.SubscribeWithErr(pubsub.ChatEventChannel(apiKey.UserID),
		pubsub.HandleChatEvent(
			func(ctx context.Context, payload pubsub.ChatEvent, err error) {
				if err != nil {
					api.Logger.Error(ctx, "chat event subscription error", slog.Error(err))
					return
				}
				if err := sendEvent(codersdk.ServerSentEvent{
					Type: codersdk.ServerSentEventTypeData,
					Data: payload,
				}); err != nil {
					api.Logger.Debug(ctx, "failed to send chat event", slog.Error(err))
				}
			},
		))
	if err != nil {
		if err := sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeError,
			Data: codersdk.Response{
				Message: "Internal error subscribing to chat events.",
				Detail:  err.Error(),
			},
		}); err != nil {
			api.Logger.Debug(ctx, "failed to send chat subscribe error event", slog.Error(err))
		}
		return
	}
	defer cancelSubscribe()

	// Send initial ping to signal the connection is ready.
	if err := sendEvent(codersdk.ServerSentEvent{
		Type: codersdk.ServerSentEventTypePing,
	}); err != nil {
		api.Logger.Debug(ctx, "failed to send chat ping event", slog.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-senderClosed:
			return
		}
	}
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

	params := database.GetChatsByOwnerIDParams{
		OwnerID:  apiKey.UserID,
		Archived: searchParams.Archived,
		AfterID:  paginationParams.AfterID,
		// #nosec G115 - Pagination offsets are small and fit in int32
		OffsetOpt: int32(paginationParams.Offset),
		// #nosec G115 - Pagination limits are small and fit in int32
		LimitOpt: int32(paginationParams.Limit),
	}

	chats, err := api.Database.GetChatsByOwnerID(ctx, params)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chats.",
			Detail:  err.Error(),
		})
		return
	}

	diffStatusesByChatID, err := api.getChatDiffStatusesByChatID(ctx, chats)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chats.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChats(chats, diffStatusesByChatID))
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) postChats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateChatRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	contentBlocks, titleSource, inputError := createChatInputFromRequest(ctx, api.Database, req)
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *inputError)
		return
	}

	workspaceSelection, validationStatus, validationError := api.validateCreateChatWorkspaceSelection(ctx, req)
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

	chat, err := api.chatDaemon.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            apiKey.UserID,
		WorkspaceID:        workspaceSelection.WorkspaceID,
		Title:              title,
		ModelConfigID:      modelConfigID,
		SystemPrompt:       api.resolvedChatSystemPrompt(ctx),
		InitialUserContent: contentBlocks,
	})
	if err != nil {
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
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertChat(chat, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) listChatModels(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	for _, provider := range enabledProviders {
		configuredProviders = append(
			configuredProviders, chatprovider.ConfiguredProvider{
				Provider: provider.Provider,
				APIKey:   provider.APIKey,
				BaseURL:  provider.BaseUrl,
			},
		)
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

	keys := chatprovider.MergeProviderAPIKeys(
		chatProviderAPIKeysFromDeploymentValues(api.DeploymentValues),
		configuredProviders,
	)
	catalog := chatprovider.NewModelCatalog(keys)
	var response codersdk.ChatModelsResponse
	if configured, ok := catalog.ListConfiguredModels(
		configuredProviders, configuredModels,
	); ok {
		response = configured
	} else {
		response = catalog.ListConfiguredProviderAvailability(configuredProviders)
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
		httpapi.InternalServerError(rw, err)
		return
	}

	byModel, err := api.Database.GetChatCostPerModel(ctx, database.GetChatCostPerModelParams{
		OwnerID:   targetUser.ID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	byChat, err := api.Database.GetChatCostPerChat(ctx, database.GetChatCostPerChatParams{
		OwnerID:   targetUser.ID,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
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

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatCostSummary{
		StartDate:            startDate,
		EndDate:              endDate,
		TotalCostMicros:      summary.TotalCostMicros,
		PricedMessageCount:   summary.PricedMessageCount,
		UnpricedMessageCount: summary.UnpricedMessageCount,
		TotalInputTokens:     summary.TotalInputTokens,
		TotalOutputTokens:    summary.TotalOutputTokens,
		ByModel:              modelBreakdowns,
		ByChat:               chatBreakdowns,
	})
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	httpapi.Write(ctx, rw, http.StatusOK, convertChat(chat, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	messages, err := api.Database.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages.",
			Detail:  err.Error(),
		})
		return
	}

	queuedMessages, err := api.Database.GetChatQueuedMessages(ctx, chatID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get queued messages.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatMessagesResponse{
		Messages:       convertChatMessages(messages),
		QueuedMessages: convertChatQueuedMessages(queuedMessages),
	})
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

	if !chat.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat has no workspace to watch.",
		})
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

// @Summary Watch chat desktop
// @ID watch-chat-desktop
// @Security CoderSessionToken
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 101
// @Router /chats/{chat}/desktop [get]
//
// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) watchChatDesktop(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		chat   = httpmw.ChatParam(r)
		logger = api.Logger.Named("chat_desktop").With(slog.F("chat_id", chat.ID))
	)

	if !chat.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat has no workspace.",
		})
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

// @Summary Archive a chat
// @ID archive-chat
// @Tags Chats
// @Success 204
// @Router /chats/{chat}/archive [post]
func (api *API) archiveChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat is already archived.",
		})
		return
	}

	var err error
	// Use chatDaemon when available so it can notify
	// active subscribers. Fall back to direct DB for the
	// simple archive flag — no streaming state is involved.
	if api.chatDaemon != nil {
		err = api.chatDaemon.ArchiveChat(ctx, chat.ID)
	} else {
		err = api.Database.ArchiveChatByID(ctx, chat.ID)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to archive chat.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) unarchiveChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !chat.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat is not archived.",
		})
		return
	}

	var err error
	// Use chatDaemon when available so it can notify
	// active subscribers. Fall back to direct DB for the
	// simple unarchive flag — no streaming state is involved.
	if api.chatDaemon != nil {
		err = api.chatDaemon.UnarchiveChat(ctx, chat.ID)
	} else {
		err = api.Database.UnarchiveChatByID(ctx, chat.ID)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unarchive chat.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) postChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

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

	contentBlocks, _, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
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
			BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
		},
	)
	if sendErr != nil {
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

	response := codersdk.CreateChatMessageResponse{Queued: sendResult.Queued}
	if sendResult.Queued {
		if sendResult.QueuedMessage != nil {
			response.QueuedMessage = convertChatQueuedMessagePtr(*sendResult.QueuedMessage)
		}
	} else {
		message := convertChatMessage(sendResult.Message)
		response.Message = &message
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

	contentBlocks, _, inputError := createChatInputFromParts(ctx, api.Database, req.Content, "content")
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

	message := convertChatMessage(editResult.Message)
	httpapi.Write(ctx, rw, http.StatusOK, message)
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
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to promote queued message.",
			Detail:  txErr.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChatMessage(promoteResult.PromotedMessage))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) streamChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

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

	sendEvent, senderClosed, err := httpapi.OneWayWebSocketEventSender(api.Logger)(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat stream.",
			Detail:  err.Error(),
		})
		return
	}
	snapshot, events, cancel, ok := api.chatDaemon.Subscribe(ctx, chatID, r.Header, afterMessageID)
	if !ok {
		if err := sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeError,
			Data: codersdk.Response{
				Message: "Chat streaming is not available.",
				Detail:  "Chat stream state is not configured.",
			},
		}); err != nil {
			api.Logger.Debug(ctx, "failed to send chat stream unavailable event", slog.Error(err))
		}
		// Ensure the WebSocket is closed so senderClosed
		// completes and the handler can return.
		<-senderClosed
		return
	}
	defer func() {
		<-senderClosed
	}()
	defer cancel()

	sendChatStreamBatch := func(batch []codersdk.ChatStreamEvent) error {
		if len(batch) == 0 {
			return nil
		}
		return sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: batch,
		})
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
			api.Logger.Debug(ctx, "failed to send chat stream snapshot", slog.Error(err))
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-senderClosed:
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
				api.Logger.Debug(ctx, "failed to send chat stream event", slog.Error(err))
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
			api.Logger.Error(ctx, "failed to mark chat as waiting",
				slog.F("chat_id", chatID), slog.Error(updateErr))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to interrupt chat.",
				Detail:  updateErr.Error(),
			})
			return
		}
		chat = updatedChat
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChat(chat, nil))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatDiffStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	status, err := api.resolveChatDiffStatus(ctx, chat)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat diff status.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChatDiffStatus(chatID, status))
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

func chatWorkspaceAuditStatus(err error) int {
	if responder, ok := httperror.IsResponder(err); ok {
		status, _ := responder.Response()
		return status
	}
	return http.StatusInternalServerError
}

func (api *API) resolveChatDiffStatus(
	ctx context.Context,
	chat database.Chat,
) (*database.ChatDiffStatus, error) {
	status, found, err := api.getCachedChatDiffStatus(ctx, chat.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	reference, err := api.resolveChatDiffReference(ctx, chat, found, status)
	if err != nil {
		return nil, err
	}
	if reference.PullRequestURL != "" {
		if !found || !strings.EqualFold(strings.TrimSpace(status.Url.String), reference.PullRequestURL) {
			status, err = api.upsertChatDiffStatusReference(ctx, chat.ID, reference.PullRequestURL, now.Add(-time.Second))
			if err != nil {
				return nil, err
			}
			found = true
		}
	}

	if !found {
		return nil, nil //nolint:nilnil // Callers handle nil status explicitly.
	}
	if !chatDiffStatusIsStale(status, now) {
		return &status, nil
	}

	// Use the same refresh pipeline as the background worker
	// so both paths share identical provider/token resolution.
	refreshed, err := api.gitSyncWorker.RefreshChat(
		ctx, status, chat.OwnerID,
	)
	if err == nil && refreshed != nil {
		return refreshed, nil
	}
	if err == nil {
		// No PR exists yet; return what we have.
		return &status, nil
	}

	api.Logger.Warn(ctx, "failed to refresh chat diff status",
		slog.F("chat_id", chat.ID),
		slog.Error(err),
	)

	backoffStatus, backoffErr := api.upsertChatDiffStatusReference(ctx, chat.ID, reference.PullRequestURL, now.Add(chatDiffStatusTTL))
	if backoffErr != nil {
		api.Logger.Warn(ctx, "failed to extend chat diff status stale timestamp",
			slog.F("chat_id", chat.ID),
			slog.Error(backoffErr),
		)
		return &status, nil
	}

	return &backoffStatus, nil
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

func chatDiffStatusIsStale(status database.ChatDiffStatus, now time.Time) bool {
	if !status.RefreshedAt.Valid {
		return true
	}
	return !status.StaleAt.After(now)
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

func (api *API) validateCreateChatWorkspaceSelection(
	ctx context.Context,
	req codersdk.CreateChatRequest,
) (
	createChatWorkspaceSelection,
	int,
	*codersdk.Response,
) {
	selection := createChatWorkspaceSelection{}
	if req.WorkspaceID == nil {
		return selection, 0, nil
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, *req.WorkspaceID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return selection, http.StatusBadRequest, &codersdk.Response{
				Message: "Workspace not found or you do not have access to this resource",
			}
		}
		return selection, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		}
	}
	selection.WorkspaceID = uuid.NullUUID{
		UUID:  workspace.ID,
		Valid: true,
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

const (
	// maxChatFileSize is the maximum size of a chat file upload (10 MB).
	maxChatFileSize = 10 << 20
	// maxChatFileName is the maximum length of an uploaded file name.
	maxChatFileName = 255
)

// allowedChatFileMIMETypes lists the content types accepted for chat
// file uploads. SVG is explicitly excluded because it can contain scripts.
var allowedChatFileMIMETypes = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": false, // SVG can contain scripts.
}

var (
	webpMagicRIFF = []byte("RIFF")
	webpMagicWEBP = []byte("WEBP")
)

// detectChatFileType detects the MIME type of the given data.
// It extends http.DetectContentType with support for WebP, which
// Go's standard sniffer does not recognize.
func detectChatFileType(data []byte) string {
	if len(data) >= 12 &&
		bytes.Equal(data[0:4], webpMagicRIFF) &&
		bytes.Equal(data[8:12], webpMagicWEBP) {
		return "image/webp"
	}
	return http.DetectContentType(data)
}

//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatSystemPrompt(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prompt, err := api.Database.GetChatSystemPrompt(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat system prompt.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatSystemPromptResponse{
		SystemPrompt: prompt,
	})
}

func (api *API) putChatSystemPrompt(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.UpdateChatSystemPromptRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	trimmedPrompt := strings.TrimSpace(req.SystemPrompt)
	// 128 KiB is generous for a system prompt while still
	// preventing abuse or accidental pastes of large content.
	if len(trimmedPrompt) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "System prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(trimmedPrompt)),
		})
		return
	}
	err := api.Database.UpsertChatSystemPrompt(ctx, trimmedPrompt)
	if httpapi.Is404Error(err) { // also catches authz error
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat system prompt.",
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

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatCustomPromptResponse{
		CustomPrompt: customPrompt,
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) putUserChatCustomPrompt(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)
	)

	var params codersdk.UpdateUserChatCustomPromptRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	trimmedPrompt := strings.TrimSpace(params.CustomPrompt)
	// Apply the same 128 KiB limit as the deployment system prompt.
	if len(trimmedPrompt) > maxSystemPromptLenBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Custom prompt exceeds maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", maxSystemPromptLenBytes, len(trimmedPrompt)),
		})
		return
	}

	updatedConfig, err := api.Database.UpdateUserChatCustomPrompt(ctx, database.UpdateUserChatCustomPromptParams{
		UserID:           apiKey.UserID,
		ChatCustomPrompt: trimmedPrompt,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error updating user chat custom prompt.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserChatCustomPromptResponse{
		CustomPrompt: updatedConfig.Value,
	})
}

func (api *API) resolvedChatSystemPrompt(ctx context.Context) string {
	custom, err := api.Database.GetChatSystemPrompt(ctx)
	if err != nil {
		// Log but don't fail chat creation — fall back to the
		// built-in default so the user isn't blocked.
		api.Logger.Error(ctx, "failed to fetch custom chat system prompt, using default", slog.Error(err))
		return chatd.DefaultSystemPrompt
	}
	if strings.TrimSpace(custom) != "" {
		return custom
	}
	return chatd.DefaultSystemPrompt
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

	if allowed, ok := allowedChatFileMIMETypes[contentType]; !ok || !allowed {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unsupported file type.",
			Detail:  "Allowed types: image/png, image/jpeg, image/gif, image/webp.",
		})
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, maxChatFileSize)
	br := bufio.NewReader(r.Body)

	// Peek at the leading bytes to sniff the real content type
	// before reading the entire body.
	peek, peekErr := br.Peek(512)
	if peekErr != nil && !errors.Is(peekErr, io.EOF) && !errors.Is(peekErr, bufio.ErrBufferFull) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read file from request.",
			Detail:  peekErr.Error(),
		})
		return
	}

	// Verify the actual content matches a safe image type so that
	// a client cannot spoof Content-Type to serve active content.
	detected := detectChatFileType(peek)
	if allowed, ok := allowedChatFileMIMETypes[detected]; !ok || !allowed {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unsupported file type.",
			Detail:  "Allowed types: image/png, image/jpeg, image/gif, image/webp.",
		})
		return
	}

	// Read the full body now that we know the type is valid.
	data, err := io.ReadAll(br)
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

	// Extract filename from Content-Disposition header if provided.
	var filename string
	if cd := r.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			filename = params["filename"]
			if len(filename) > maxChatFileName {
				// Truncate at rune boundary to avoid splitting
				// multi-byte UTF-8 characters.
				var truncated []byte
				for _, r := range filename {
					encoded := []byte(string(r))
					if len(truncated)+len(encoded) > maxChatFileName {
						break
					}
					truncated = append(truncated, encoded...)
				}
				filename = string(truncated)
			}
		}
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
	if chatFile.Name != "" {
		rw.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": chatFile.Name}))
	} else {
		rw.Header().Set("Content-Disposition", "inline")
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
	*codersdk.Response,
) {
	return createChatInputFromParts(ctx, db, req.Content, "content")
}

func createChatInputFromParts(
	ctx context.Context,
	db database.Store,
	parts []codersdk.ChatInputPart,
	fieldName string,
) ([]codersdk.ChatMessagePart, string, *codersdk.Response) {
	if len(parts) == 0 {
		return nil, "", &codersdk.Response{
			Message: "Content is required.",
			Detail:  "Content cannot be empty.",
		}
	}

	content := make([]codersdk.ChatMessagePart, 0, len(parts))
	textParts := make([]string, 0, len(parts))
	for i, part := range parts {
		switch strings.ToLower(strings.TrimSpace(string(part.Type))) {
		case string(codersdk.ChatInputPartTypeText):
			text := strings.TrimSpace(part.Text)
			if text == "" {
				return nil, "", &codersdk.Response{
					Message: "Invalid input part.",
					Detail:  fmt.Sprintf("%s[%d].text cannot be empty.", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageText(text))
			textParts = append(textParts, text)
		case string(codersdk.ChatInputPartTypeFile):
			if part.FileID == uuid.Nil {
				return nil, "", &codersdk.Response{
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
					return nil, "", &codersdk.Response{
						Message: "Invalid input part.",
						Detail:  fmt.Sprintf("%s[%d].file_id references a file that does not exist.", fieldName, i),
					}
				}
				return nil, "", &codersdk.Response{
					Message: "Internal error.",
					Detail:  fmt.Sprintf("Failed to retrieve file for %s[%d].", fieldName, i),
				}
			}
			content = append(content, codersdk.ChatMessageFile(part.FileID, chatFile.Mimetype))
		case string(codersdk.ChatInputPartTypeFileReference):
			if part.FileName == "" {
				return nil, "", &codersdk.Response{
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
			return nil, "", &codersdk.Response{
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
		return nil, "", &codersdk.Response{
			Message: "Content is required.",
			Detail:  fmt.Sprintf("%s must include at least one text or file part.", fieldName),
		}
	}
	titleSource := strings.TrimSpace(strings.Join(textParts, " "))
	return content, titleSource, nil
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

func convertChat(c database.Chat, diffStatus *database.ChatDiffStatus) codersdk.Chat {
	chat := codersdk.Chat{
		ID:                c.ID,
		OwnerID:           c.OwnerID,
		LastModelConfigID: c.LastModelConfigID,
		Title:             c.Title,
		Status:            codersdk.ChatStatus(c.Status),
		Archived:          c.Archived,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
	if c.LastError.Valid {
		chat.LastError = &c.LastError.String
	}
	if c.ParentChatID.Valid {
		parentChatID := c.ParentChatID.UUID
		chat.ParentChatID = &parentChatID
	}
	switch {
	case c.RootChatID.Valid:
		rootChatID := c.RootChatID.UUID
		chat.RootChatID = &rootChatID
	case c.ParentChatID.Valid:
		rootChatID := c.ParentChatID.UUID
		chat.RootChatID = &rootChatID
	default:
		rootChatID := c.ID
		chat.RootChatID = &rootChatID
	}
	if c.WorkspaceID.Valid {
		chat.WorkspaceID = &c.WorkspaceID.UUID
	}
	if diffStatus != nil {
		convertedDiffStatus := convertChatDiffStatus(c.ID, diffStatus)
		chat.DiffStatus = &convertedDiffStatus
	}
	return chat
}

func convertChats(chats []database.Chat, diffStatusesByChatID map[uuid.UUID]database.ChatDiffStatus) []codersdk.Chat {
	result := make([]codersdk.Chat, len(chats))
	for i, c := range chats {
		diffStatus, ok := diffStatusesByChatID[c.ID]
		if ok {
			result[i] = convertChat(c, &diffStatus)
			continue
		}

		result[i] = convertChat(c, nil)
		if diffStatusesByChatID != nil {
			emptyDiffStatus := convertChatDiffStatus(c.ID, nil)
			result[i].DiffStatus = &emptyDiffStatus
		}
	}
	return result
}

func convertChatCostModelBreakdown(model database.GetChatCostPerModelRow) codersdk.ChatCostModelBreakdown {
	displayName := strings.TrimSpace(model.DisplayName)
	if displayName == "" {
		displayName = model.Model
	}
	return codersdk.ChatCostModelBreakdown{
		ModelConfigID:     model.ModelConfigID,
		DisplayName:       displayName,
		Provider:          model.Provider,
		Model:             model.Model,
		TotalCostMicros:   model.TotalCostMicros,
		MessageCount:      model.MessageCount,
		TotalInputTokens:  model.TotalInputTokens,
		TotalOutputTokens: model.TotalOutputTokens,
	}
}

func convertChatCostChatBreakdown(chat database.GetChatCostPerChatRow) codersdk.ChatCostChatBreakdown {
	return codersdk.ChatCostChatBreakdown{
		RootChatID:        chat.RootChatID,
		ChatTitle:         chat.ChatTitle,
		TotalCostMicros:   chat.TotalCostMicros,
		MessageCount:      chat.MessageCount,
		TotalInputTokens:  chat.TotalInputTokens,
		TotalOutputTokens: chat.TotalOutputTokens,
	}
}

func convertChatCostUserRollup(user database.GetChatCostPerUserRow) codersdk.ChatCostUserRollup {
	return codersdk.ChatCostUserRollup{
		UserID:            user.UserID,
		Username:          user.Username,
		Name:              user.Name,
		AvatarURL:         user.AvatarURL,
		TotalCostMicros:   user.TotalCostMicros,
		MessageCount:      user.MessageCount,
		ChatCount:         user.ChatCount,
		TotalInputTokens:  user.TotalInputTokens,
		TotalOutputTokens: user.TotalOutputTokens,
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

func convertChatDiffStatus(chatID uuid.UUID, status *database.ChatDiffStatus) codersdk.ChatDiffStatus {
	result := codersdk.ChatDiffStatus{
		ChatID: chatID,
	}
	if status == nil {
		return result
	}

	result.ChatID = status.ChatID
	if status.Url.Valid {
		u := strings.TrimSpace(status.Url.String)
		if u != "" {
			result.URL = &u
		}
	}
	if result.URL == nil {
		// Try to build a branch URL from the stored origin.
		// Since convertChatDiffStatus does not have access to
		// the API instance, we construct a GitHub provider
		// directly as a best-effort fallback.
		// TODO: This uses the default github.com API base URL,
		// so branch URLs for GitHub Enterprise instances will
		// be incorrect. To fix this, convertChatDiffStatus
		// would need access to the external auth configs.
		gp := gitprovider.New("github", "", nil)
		if gp != nil {
			if owner, repo, _, ok := gp.ParseRepositoryOrigin(status.GitRemoteOrigin); ok {
				branchURL := gp.BuildBranchURL(owner, repo, status.GitBranch)
				if branchURL != "" {
					result.URL = &branchURL
				}
			}
		}
	}
	if status.PullRequestState.Valid {
		pullRequestState := strings.TrimSpace(status.PullRequestState.String)
		if pullRequestState != "" {
			result.PullRequestState = &pullRequestState
		}
	}
	result.PullRequestTitle = status.PullRequestTitle
	result.PullRequestDraft = status.PullRequestDraft
	result.ChangesRequested = status.ChangesRequested
	result.Additions = status.Additions
	result.Deletions = status.Deletions
	result.ChangedFiles = status.ChangedFiles
	if status.AuthorLogin.Valid {
		result.AuthorLogin = &status.AuthorLogin.String
	}
	if status.AuthorAvatarUrl.Valid {
		result.AuthorAvatarURL = &status.AuthorAvatarUrl.String
	}
	if status.BaseBranch.Valid {
		result.BaseBranch = &status.BaseBranch.String
	}
	if status.PrNumber.Valid {
		result.PRNumber = &status.PrNumber.Int32
	}
	if status.Commits.Valid {
		result.Commits = &status.Commits.Int32
	}
	if status.Approved.Valid {
		result.Approved = &status.Approved.Bool
	}
	if status.ReviewerCount.Valid {
		result.ReviewerCount = &status.ReviewerCount.Int32
	}
	if status.RefreshedAt.Valid {
		refreshedAt := status.RefreshedAt.Time
		result.RefreshedAt = &refreshedAt
	}
	staleAt := status.StaleAt
	result.StaleAt = &staleAt

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
		enabledConfiguredProviders = append(
			enabledConfiguredProviders, chatprovider.ConfiguredProvider{
				Provider: provider.Provider,
				APIKey:   provider.APIKey,
				BaseURL:  provider.BaseUrl,
			},
		)
	}

	effectiveKeys := chatprovider.MergeProviderAPIKeys(
		chatProviderAPIKeysFromDeploymentValues(api.DeploymentValues),
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
					effectiveKeys.APIKey(provider) != "",
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
			ID:          uuid.Nil,
			Provider:    provider,
			DisplayName: chatprovider.ProviderDisplayName(provider),
			Enabled:     enabled,
			HasAPIKey:   hasAPIKey,
			BaseURL:     effectiveKeys.BaseURL(provider),
			Source:      source,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) createChatProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
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

	inserted, err := api.Database.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    provider,
		DisplayName: strings.TrimSpace(req.DisplayName),
		APIKey:      strings.TrimSpace(req.APIKey),
		BaseUrl:     baseURL,
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		Enabled:     enabled,
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
		apiKey = strings.TrimSpace(*req.APIKey)
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

	updated, err := api.Database.UpdateChatProvider(ctx, database.UpdateChatProviderParams{
		DisplayName: displayName,
		APIKey:      apiKey,
		BaseUrl:     baseURL,
		ApiKeyKeyID: apiKeyKeyID,
		Enabled:     enabled,
		ID:          existing.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat provider.",
			Detail:  err.Error(),
		})
		return
	}

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
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat provider.",
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
		ID:          provider.ID,
		Provider:    provider.Provider,
		DisplayName: displayName,
		Enabled:     provider.Enabled,
		HasAPIKey:   hasAPIKey,
		BaseURL:     strings.TrimSpace(provider.BaseUrl),
		Source:      source,
		CreatedAt:   provider.CreatedAt,
		UpdatedAt:   provider.UpdatedAt,
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

func chatProviderAPIKeysFromDeploymentValues(
	deploymentValues *codersdk.DeploymentValues,
) chatprovider.ProviderAPIKeys {
	_ = deploymentValues
	// For now, we'll just manage configs in the UI.
	// We should probably not be reusing the AI bridge configs anyways.
	return chatprovider.ProviderAPIKeys{
		// OpenAI:    deploymentValues.AI.BridgeConfig.OpenAI.Key.Value(),
		// Anthropic: deploymentValues.AI.BridgeConfig.Anthropic.Key.Value(),
		// BaseURLByProvider: map[string]string{
		// 	"openai":    deploymentValues.AI.BridgeConfig.OpenAI.BaseURL.Value(),
		// 	"anthropic": deploymentValues.AI.BridgeConfig.Anthropic.BaseURL.Value(),
		// },
	}
}

func (api *API) hasEffectiveProviderAPIKey(ctx context.Context, provider database.ChatProvider) bool {
	if strings.TrimSpace(provider.APIKey) != "" {
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
		enabledConfiguredProviders = append(
			enabledConfiguredProviders, chatprovider.ConfiguredProvider{
				Provider: configured.Provider,
				APIKey:   configured.APIKey,
				BaseURL:  configured.BaseUrl,
			},
		)
	}

	effectiveKeys := chatprovider.MergeProviderAPIKeys(
		chatProviderAPIKeysFromDeploymentValues(api.DeploymentValues),
		enabledConfiguredProviders,
	)
	return effectiveKeys.APIKey(provider.Provider) != ""
}
