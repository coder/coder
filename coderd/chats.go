package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

const (
	chatDiffStatusTTL                = 120 * time.Second
	chatDiffBackgroundRefreshTimeout = 20 * time.Second
	githubAPIBaseURL                 = "https://api.github.com"
	chatStreamBatchSize              = 256

	chatContextLimitModelConfigKey                = "context_limit"
	chatContextCompressionThresholdModelConfigKey = "context_compression_threshold"
	defaultChatContextCompressionThreshold        = int32(70)
	minChatContextCompressionThreshold            = int32(0)
	maxChatContextCompressionThreshold            = int32(100)
)

// chatDiffRefreshBackoffSchedule defines the delays between successive
// background diff refresh attempts. The trigger fires when the agent
// obtains a GitHub token, which is typically right before a git push
// or PR creation. The backoff gives progressively more time for the
// push and any PR workflow to complete before querying the GitHub API.
var chatDiffRefreshBackoffSchedule = []time.Duration{
	1 * time.Second,
	3 * time.Second,
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
}

// chatGitRef holds the branch and remote origin reported by the
// workspace agent during a git operation.
type chatGitRef struct {
	Branch       string
	RemoteOrigin string
}

var (
	githubPullRequestPathPattern = regexp.MustCompile(
		`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([0-9]+)(?:[/?#].*)?$`,
	)
	githubRepositoryHTTPSPattern = regexp.MustCompile(
		`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
	)
	githubRepositorySSHPathPattern = regexp.MustCompile(
		`^(?:ssh://)?git@github\.com[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
	)
)

type githubPullRequestRef struct {
	Owner  string
	Repo   string
	Number int
}

type githubPullRequestStatus struct {
	PullRequestState string
	ChangesRequested bool
	Additions        int32
	Deletions        int32
	ChangedFiles     int32
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
				_ = sendEvent(codersdk.ServerSentEvent{
					Type: codersdk.ServerSentEventTypeData,
					Data: payload,
				})
			},
		))
	if err != nil {
		_ = sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeError,
			Data: codersdk.Response{
				Message: "Internal error subscribing to chat events.",
				Detail:  err.Error(),
			},
		})
		return
	}
	defer cancelSubscribe()

	// Send initial ping to signal the connection is ready.
	_ = sendEvent(codersdk.ServerSentEvent{
		Type: codersdk.ServerSentEventTypePing,
	})

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

	chats, err := api.Database.GetChatsByOwnerID(ctx, apiKey.UserID)
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

	contentBlocks, titleSource, inputError := createChatInputFromRequest(req)
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
		WorkspaceAgentID:   workspaceSelection.WorkspaceAgentID,
		Title:              title,
		ModelConfigID:      modelConfigID,
		SystemPrompt:       defaultChatSystemPrompt(),
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

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	messages, err := api.Database.GetChatMessagesByChatID(ctx, chatID)
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

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatWithMessages{
		Chat:           convertChat(chat, nil),
		Messages:       convertChatMessages(messages),
		QueuedMessages: convertChatQueuedMessages(queuedMessages),
	})
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) deleteChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	chatID := chat.ID

	var err error
	if api.chatDaemon != nil {
		err = api.chatDaemon.DeleteChat(ctx, chatID)
	} else {
		err = deleteChatTree(ctx, api.Database, chatID)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func deleteChatTree(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) error {
	// Child chats (sub-agent chats) reference their parent via
	// parent_chat_id with ON DELETE SET NULL, so without explicit
	// cleanup they would become orphaned root-level items.
	return store.InTx(func(tx database.Store) error {
		// Recursively collect all descendant chat IDs.
		var descendantIDs []uuid.UUID
		queue := []uuid.UUID{chatID}
		for len(queue) > 0 {
			parentID := queue[0]
			queue = queue[1:]
			children, err := tx.ListChildChatsByParentID(ctx, parentID)
			if err != nil {
				return xerrors.Errorf("list children of chat %s: %w", parentID, err)
			}
			for _, child := range children {
				descendantIDs = append(descendantIDs, child.ID)
				queue = append(queue, child.ID)
			}
		}

		// Delete descendants first. The FK is ON DELETE SET NULL so
		// order doesn't strictly matter, but deleting children before
		// parents is cleaner.
		for i := len(descendantIDs) - 1; i >= 0; i-- {
			if err := tx.DeleteChatByID(ctx, descendantIDs[i]); err != nil {
				return xerrors.Errorf("delete descendant chat %s: %w", descendantIDs[i], err)
			}
		}

		// Delete the target chat itself.
		if err := tx.DeleteChatByID(ctx, chatID); err != nil {
			return xerrors.Errorf("delete chat: %w", err)
		}

		return nil
	}, nil)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
func (api *API) postChatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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

	contentBlocks, _, inputError := createChatInputFromParts(req.Content, "content")
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

	contentBlocks, _, inputError := createChatInputFromParts(req.Content, "content")
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: inputError.Message,
			Detail:  inputError.Detail,
		})
		return
	}

	editResult, editErr := api.chatDaemon.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
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

	sendEvent, senderClosed, err := httpapi.OneWayWebSocketEventSender(api.Logger)(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to open chat stream.",
			Detail:  err.Error(),
		})
		return
	}
	defer func() {
		<-senderClosed
	}()

	snapshot, events, cancel, ok := api.chatDaemon.Subscribe(ctx, chatID, r.Header)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat streaming is not available.",
			Detail:  "Chat stream state is not configured.",
		})
		return
	}
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
		})
		if updateErr != nil {
			api.Logger.Error(ctx, "failed to mark chat as waiting",
				slog.F("chat_id", chatID), slog.Error(updateErr))
		} else {
			chat = updatedChat
		}
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
	return api.resolveChatDiffStatusWithOptions(ctx, chat, false)
}

//nolint:revive // Boolean forces cache refresh bypass.
func (api *API) resolveChatDiffStatusWithOptions(
	ctx context.Context,
	chat database.Chat,
	forceRefresh bool,
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
	if reference.PullRequestURL == "" {
		return &status, nil
	}
	if !shouldRefreshChatDiffStatus(status, now, forceRefresh) {
		return &status, nil
	}

	refreshed, err := api.refreshChatDiffStatus(
		ctx,
		chat.OwnerID,
		chat.ID,
		reference.PullRequestURL,
	)
	if err == nil {
		return &refreshed, nil
	}

	api.Logger.Warn(ctx, "failed to refresh chat diff status",
		slog.F("chat_id", chat.ID),
		slog.F("pull_request_url", reference.PullRequestURL),
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

//nolint:revive // Boolean forces cache refresh bypass.
func shouldRefreshChatDiffStatus(status database.ChatDiffStatus, now time.Time, forceRefresh bool) bool {
	if forceRefresh {
		return true
	}
	return chatDiffStatusIsStale(status, now)
}

func (api *API) triggerWorkspaceChatDiffStatusRefresh(workspace database.Workspace, gitRef chatGitRef) {
	if workspace.ID == uuid.Nil || workspace.OwnerID == uuid.Nil {
		return
	}

	go func(workspaceID, workspaceOwnerID uuid.UUID, gitRef chatGitRef) {
		ctx := api.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		//nolint:gocritic // Background goroutine for diff status refresh has no user context.
		ctx = dbauthz.AsSystemRestricted(ctx)

		// Always store the git ref so the data is persisted even
		// before a PR exists. The frontend can show branch info
		// and the refresh loop can resolve a PR later.
		api.storeChatGitRef(ctx, workspaceID, workspaceOwnerID, gitRef)

		for _, delay := range chatDiffRefreshBackoffSchedule {
			t := api.Clock.NewTimer(delay, "chat_diff_refresh")
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}

			// Refresh and publish status on every iteration.
			// Stop the loop once a PR is discovered — there's
			// nothing more to wait for after that.
			if api.refreshWorkspaceChatDiffStatuses(ctx, workspaceID, workspaceOwnerID) {
				return
			}
		}
	}(workspace.ID, workspace.OwnerID, gitRef)
}

// storeChatGitRef persists the git branch and remote origin reported
// by the workspace agent on all chats associated with the workspace.
func (api *API) storeChatGitRef(ctx context.Context, workspaceID, workspaceOwnerID uuid.UUID, gitRef chatGitRef) {
	chats, err := api.Database.GetChatsByOwnerID(ctx, workspaceOwnerID)
	if err != nil {
		api.Logger.Warn(ctx, "failed to list chats for git ref storage",
			slog.F("workspace_id", workspaceID),
			slog.Error(err),
		)
		return
	}

	for _, chat := range filterChatsByWorkspaceID(chats, workspaceID) {
		_, err := api.Database.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
			ChatID:          chat.ID,
			GitBranch:       gitRef.Branch,
			GitRemoteOrigin: gitRef.RemoteOrigin,
			StaleAt:         time.Now().UTC().Add(-time.Second),
			Url:             sql.NullString{},
		})
		if err != nil {
			api.Logger.Warn(ctx, "failed to store git ref on chat diff status",
				slog.F("chat_id", chat.ID),
				slog.F("workspace_id", workspaceID),
				slog.Error(err),
			)
		}
	}
}

// refreshWorkspaceChatDiffStatuses refreshes the diff status for all
// chats associated with the given workspace. It returns true when
// every chat has a PR URL resolved, signaling that the caller can
// stop polling.
func (api *API) refreshWorkspaceChatDiffStatuses(ctx context.Context, workspaceID, workspaceOwnerID uuid.UUID) bool {
	chats, err := api.Database.GetChatsByOwnerID(ctx, workspaceOwnerID)
	if err != nil {
		api.Logger.Warn(ctx, "failed to list workspace owner chats for diff refresh",
			slog.F("workspace_id", workspaceID),
			slog.F("workspace_owner_id", workspaceOwnerID),
			slog.Error(err),
		)
		return false
	}

	filtered := filterChatsByWorkspaceID(chats, workspaceID)
	if len(filtered) == 0 {
		return false
	}

	allHavePR := true
	for _, chat := range filtered {
		refreshCtx, cancel := context.WithTimeout(ctx, chatDiffBackgroundRefreshTimeout)
		status, err := api.resolveChatDiffStatusWithOptions(refreshCtx, chat, true)
		cancel()
		if err != nil {
			api.Logger.Warn(ctx, "failed to refresh chat diff status after workspace external auth",
				slog.F("workspace_id", workspaceID),
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
			allHavePR = false
		} else if status == nil || !status.Url.Valid || strings.TrimSpace(status.Url.String) == "" {
			allHavePR = false
		}

		api.publishChatStatusEvent(ctx, chat.ID)
	}

	return allHavePR
}

func filterChatsByWorkspaceID(chats []database.Chat, workspaceID uuid.UUID) []database.Chat {
	filteredChats := make([]database.Chat, 0, len(chats))
	for _, chat := range chats {
		if !chat.WorkspaceID.Valid || chat.WorkspaceID.UUID != workspaceID {
			continue
		}
		filteredChats = append(filteredChats, chat)
	}
	return filteredChats
}

func (api *API) publishChatStatusEvent(ctx context.Context, chatID uuid.UUID) {
	if api.chatDaemon == nil {
		return
	}

	if err := api.chatDaemon.RefreshStatus(ctx, chatID); err != nil {
		api.Logger.Debug(ctx, "failed to refresh published chat status",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
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
	if !strings.EqualFold(reference.RepositoryRef.Provider, string(codersdk.EnhancedExternalAuthProviderGitHub)) {
		return result, nil
	}

	token := api.resolveChatGitHubAccessToken(ctx, chat.OwnerID)

	if reference.PullRequestURL != "" {
		diff, err := api.fetchGitHubPullRequestDiff(ctx, reference.PullRequestURL, token)
		if err != nil {
			return result, err
		}
		result.Diff = diff
		return result, nil
	}

	diff, err := api.fetchGitHubCompareDiff(ctx, *reference.RepositoryRef, token)
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
	if reference.RepositoryRef != nil &&
		strings.EqualFold(reference.RepositoryRef.Provider, string(codersdk.EnhancedExternalAuthProviderGitHub)) {
		pullRequestURL, lookupErr := api.resolveGitHubPullRequestURLFromRepositoryRef(ctx, chat.OwnerID, *reference.RepositoryRef)
		if lookupErr != nil {
			api.Logger.Debug(ctx, "failed to resolve pull request from repository reference",
				slog.F("chat_id", chat.ID),
				slog.F("provider", reference.RepositoryRef.Provider),
				slog.F("remote_origin", reference.RepositoryRef.RemoteOrigin),
				slog.F("branch", reference.RepositoryRef.Branch),
				slog.Error(lookupErr),
			)
		} else if pullRequestURL != "" {
			reference.PullRequestURL = pullRequestURL
		}
	}

	reference.PullRequestURL = normalizeGitHubPullRequestURL(reference.PullRequestURL)

	// If we have a PR URL but no repo ref (e.g. the agent hasn't
	// reported branch/origin yet), derive a partial ref from the
	// PR URL so the caller can still show provider/owner/repo.
	if reference.RepositoryRef == nil && reference.PullRequestURL != "" {
		if parsed, ok := parseGitHubPullRequestURL(reference.PullRequestURL); ok {
			reference.RepositoryRef = &chatRepositoryRef{
				Provider:     string(codersdk.EnhancedExternalAuthProviderGitHub),
				RemoteOrigin: fmt.Sprintf("https://github.com/%s/%s", parsed.Owner, parsed.Repo),
				Owner:        parsed.Owner,
				Repo:         parsed.Repo,
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

	repoRef := &chatRepositoryRef{
		Provider:     strings.TrimSpace(api.resolveExternalAuthProviderType(origin)),
		RemoteOrigin: origin,
		Branch:       branch,
	}

	if owner, repo, normalizedOrigin, ok := parseGitHubRepositoryOrigin(repoRef.RemoteOrigin); ok {
		if repoRef.Provider == "" {
			repoRef.Provider = string(codersdk.EnhancedExternalAuthProviderGitHub)
		}
		repoRef.RemoteOrigin = normalizedOrigin
		repoRef.Owner = owner
		repoRef.Repo = repo
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

func (api *API) resolveExternalAuthProviderType(match string) string {
	match = strings.TrimSpace(match)
	if match == "" {
		return ""
	}

	for _, extAuth := range api.ExternalAuthConfigs {
		if extAuth.Regex == nil || !extAuth.Regex.MatchString(match) {
			continue
		}
		return strings.ToLower(strings.TrimSpace(extAuth.Type))
	}

	return ""
}

func parseGitHubRepositoryOrigin(raw string) (owner string, repo string, normalizedOrigin string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}

	matches := githubRepositoryHTTPSPattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		matches = githubRepositorySSHPathPattern.FindStringSubmatch(raw)
	}
	if len(matches) != 3 {
		return "", "", "", false
	}

	owner = strings.TrimSpace(matches[1])
	repo = strings.TrimSpace(matches[2])
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return "", "", "", false
	}

	return owner, repo, fmt.Sprintf("https://github.com/%s/%s", owner, repo), true
}

func buildGitHubBranchURL(owner string, repo string, branch string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return ""
	}

	return fmt.Sprintf(
		"https://github.com/%s/%s/tree/%s",
		owner,
		repo,
		url.PathEscape(branch),
	)
}

func chatDiffStatusIsStale(status database.ChatDiffStatus, now time.Time) bool {
	if !status.RefreshedAt.Valid {
		return true
	}
	return !status.StaleAt.After(now)
}

func (api *API) refreshChatDiffStatus(
	ctx context.Context,
	chatOwnerID uuid.UUID,
	chatID uuid.UUID,
	pullRequestURL string,
) (database.ChatDiffStatus, error) {
	status, err := api.fetchGitHubPullRequestStatus(
		ctx,
		pullRequestURL,
		api.resolveChatGitHubAccessToken(ctx, chatOwnerID),
	)
	if err != nil {
		return database.ChatDiffStatus{}, err
	}

	refreshedAt := time.Now().UTC()
	refreshedStatus, err := api.Database.UpsertChatDiffStatus(
		ctx,
		database.UpsertChatDiffStatusParams{
			ChatID: chatID,
			Url:    sql.NullString{String: pullRequestURL, Valid: true},
			PullRequestState: sql.NullString{
				String: status.PullRequestState,
				Valid:  status.PullRequestState != "",
			},
			ChangesRequested: status.ChangesRequested,
			Additions:        status.Additions,
			Deletions:        status.Deletions,
			ChangedFiles:     status.ChangedFiles,
			RefreshedAt:      refreshedAt,
			StaleAt:          refreshedAt.Add(chatDiffStatusTTL),
		},
	)
	if err != nil {
		return database.ChatDiffStatus{}, xerrors.Errorf("upsert chat diff status: %w", err)
	}
	return refreshedStatus, nil
}

func (api *API) resolveChatGitHubAccessToken(
	ctx context.Context,
	userID uuid.UUID,
) string {
	providerIDs := []string{"github"}
	for _, config := range api.ExternalAuthConfigs {
		if !strings.EqualFold(
			config.Type,
			string(codersdk.EnhancedExternalAuthProviderGitHub),
		) {
			continue
		}
		providerIDs = append(providerIDs, config.ID)
	}

	seen := map[string]struct{}{}
	for _, providerID := range providerIDs {
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}

		link, err := api.Database.GetExternalAuthLink(
			ctx,
			database.GetExternalAuthLinkParams{
				ProviderID: providerID,
				UserID:     userID,
			},
		)
		if err != nil {
			continue
		}

		token := strings.TrimSpace(link.OAuthAccessToken)
		if token != "" {
			return token
		}
	}

	return ""
}

func (api *API) resolveGitHubPullRequestURLFromRepositoryRef(
	ctx context.Context,
	userID uuid.UUID,
	repositoryRef chatRepositoryRef,
) (string, error) {
	if repositoryRef.Owner == "" || repositoryRef.Repo == "" || repositoryRef.Branch == "" {
		return "", nil
	}

	query := url.Values{}
	query.Set("state", "open")
	query.Set("head", fmt.Sprintf("%s:%s", repositoryRef.Owner, repositoryRef.Branch))
	query.Set("sort", "updated")
	query.Set("direction", "desc")
	query.Set("per_page", "1")

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/pulls?%s",
		githubAPIBaseURL,
		repositoryRef.Owner,
		repositoryRef.Repo,
		query.Encode(),
	)

	var pulls []struct {
		HTMLURL string `json:"html_url"`
	}

	token := api.resolveChatGitHubAccessToken(ctx, userID)
	if err := api.decodeGitHubJSON(ctx, requestURL, token, &pulls); err != nil {
		return "", err
	}
	if len(pulls) == 0 {
		return "", nil
	}

	return normalizeGitHubPullRequestURL(pulls[0].HTMLURL), nil
}

func (api *API) fetchGitHubPullRequestDiff(
	ctx context.Context,
	pullRequestURL string,
	token string,
) (string, error) {
	ref, ok := parseGitHubPullRequestURL(pullRequestURL)
	if !ok {
		return "", xerrors.Errorf("invalid GitHub pull request URL %q", pullRequestURL)
	}

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d",
		githubAPIBaseURL,
		ref.Owner,
		ref.Repo,
		ref.Number,
	)

	return api.fetchGitHubDiff(ctx, requestURL, token)
}

func (api *API) fetchGitHubCompareDiff(
	ctx context.Context,
	repositoryRef chatRepositoryRef,
	token string,
) (string, error) {
	if repositoryRef.Owner == "" || repositoryRef.Repo == "" || repositoryRef.Branch == "" {
		return "", nil
	}

	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}

	repositoryURL := fmt.Sprintf(
		"%s/repos/%s/%s",
		githubAPIBaseURL,
		repositoryRef.Owner,
		repositoryRef.Repo,
	)
	if err := api.decodeGitHubJSON(ctx, repositoryURL, token, &repository); err != nil {
		return "", err
	}
	defaultBranch := strings.TrimSpace(repository.DefaultBranch)
	if defaultBranch == "" {
		return "", xerrors.New("github repository default branch is empty")
	}

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/compare/%s...%s",
		githubAPIBaseURL,
		repositoryRef.Owner,
		repositoryRef.Repo,
		url.PathEscape(defaultBranch),
		url.PathEscape(repositoryRef.Branch),
	)

	return api.fetchGitHubDiff(ctx, requestURL, token)
}

func (api *API) fetchGitHubDiff(
	ctx context.Context,
	requestURL string,
	token string,
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", xerrors.Errorf("create github diff request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.diff")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "coder-chat-diff")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := api.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", xerrors.Errorf("execute github diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return "", xerrors.Errorf("github diff request failed with status %d", resp.StatusCode)
		}
		return "", xerrors.Errorf(
			"github diff request failed with status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	diff, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", xerrors.Errorf("read github diff response: %w", err)
	}
	return string(diff), nil
}

func (api *API) fetchGitHubPullRequestStatus(
	ctx context.Context,
	pullRequestURL string,
	token string,
) (githubPullRequestStatus, error) {
	ref, ok := parseGitHubPullRequestURL(pullRequestURL)
	if !ok {
		return githubPullRequestStatus{}, xerrors.Errorf(
			"invalid GitHub pull request URL %q",
			pullRequestURL,
		)
	}

	pullEndpoint := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d",
		githubAPIBaseURL,
		ref.Owner,
		ref.Repo,
		ref.Number,
	)

	var pull struct {
		State        string `json:"state"`
		Additions    int32  `json:"additions"`
		Deletions    int32  `json:"deletions"`
		ChangedFiles int32  `json:"changed_files"`
	}
	if err := api.decodeGitHubJSON(ctx, pullEndpoint, token, &pull); err != nil {
		return githubPullRequestStatus{}, err
	}

	var reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := api.decodeGitHubJSON(
		ctx,
		pullEndpoint+"/reviews?per_page=100",
		token,
		&reviews,
	); err != nil {
		return githubPullRequestStatus{}, err
	}

	return githubPullRequestStatus{
		PullRequestState: strings.ToLower(strings.TrimSpace(pull.State)),
		ChangesRequested: hasOutstandingGitHubChangesRequested(reviews),
		Additions:        pull.Additions,
		Deletions:        pull.Deletions,
		ChangedFiles:     pull.ChangedFiles,
	}, nil
}

func (api *API) decodeGitHubJSON(
	ctx context.Context,
	requestURL string,
	token string,
	dest any,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return xerrors.Errorf("create github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "coder-chat-diff-status")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := api.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return xerrors.Errorf("execute github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return xerrors.Errorf(
				"github request failed with status %d",
				resp.StatusCode,
			)
		}
		return xerrors.Errorf(
			"github request failed with status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return xerrors.Errorf("decode github response: %w", err)
	}
	return nil
}

func hasOutstandingGitHubChangesRequested(
	reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	},
) bool {
	type reviewerState struct {
		reviewID int64
		state    string
	}

	statesByReviewer := make(map[string]reviewerState)
	for _, review := range reviews {
		login := strings.ToLower(strings.TrimSpace(review.User.Login))
		if login == "" {
			continue
		}

		state := strings.ToUpper(strings.TrimSpace(review.State))
		switch state {
		case "CHANGES_REQUESTED", "APPROVED", "DISMISSED":
		default:
			continue
		}

		current, exists := statesByReviewer[login]
		if exists && current.reviewID > review.ID {
			continue
		}
		statesByReviewer[login] = reviewerState{
			reviewID: review.ID,
			state:    state,
		}
	}

	for _, state := range statesByReviewer {
		if state.state == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}

func normalizeGitHubPullRequestURL(raw string) string {
	ref, ok := parseGitHubPullRequestURL(strings.TrimRight(
		strings.TrimSpace(raw),
		"),.;",
	))
	if !ok {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", ref.Owner, ref.Repo, ref.Number)
}

func parseGitHubPullRequestURL(raw string) (githubPullRequestRef, bool) {
	matches := githubPullRequestPathPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 4 {
		return githubPullRequestRef{}, false
	}

	number, err := strconv.Atoi(matches[3])
	if err != nil {
		return githubPullRequestRef{}, false
	}

	return githubPullRequestRef{
		Owner:  matches[1],
		Repo:   matches[2],
		Number: number,
	}, true
}

type createChatWorkspaceSelection struct {
	WorkspaceID      uuid.NullUUID
	WorkspaceAgentID uuid.NullUUID
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

	workspaceAgents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		workspace.ID,
	)
	if err != nil {
		return selection, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to get workspace agents.",
			Detail:  err.Error(),
		}
	}
	if len(workspaceAgents) > 0 {
		selection.WorkspaceAgentID = uuid.NullUUID{
			UUID:  workspaceAgents[0].ID,
			Valid: true,
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

func defaultChatSystemPrompt() string {
	return chatd.DefaultSystemPrompt
}

func createChatInputFromRequest(req codersdk.CreateChatRequest) (
	[]fantasy.Content,
	string,
	*codersdk.Response,
) {
	return createChatInputFromParts(req.Content, "content")
}

func createChatInputFromParts(
	parts []codersdk.ChatInputPart,
	fieldName string,
) ([]fantasy.Content, string, *codersdk.Response) {
	if len(parts) == 0 {
		return nil, "", &codersdk.Response{
			Message: "Content is required.",
			Detail:  "Content cannot be empty.",
		}
	}

	content := make([]fantasy.Content, 0, len(parts))
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
			content = append(content, fantasy.TextContent{Text: text})
			textParts = append(textParts, text)
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

	titleSource := strings.TrimSpace(strings.Join(textParts, " "))
	if titleSource == "" {
		return nil, "", &codersdk.Response{
			Message: "Content is required.",
			Detail:  "Content must include at least one text part.",
		}
	}
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
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
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
	if c.WorkspaceAgentID.Valid {
		chat.WorkspaceAgentID = &c.WorkspaceAgentID.UUID
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
		owner, repo, _, ok := parseGitHubRepositoryOrigin(status.GitRemoteOrigin)
		if ok {
			branchURL := buildGitHubBranchURL(owner, repo, status.GitBranch)
			if branchURL != "" {
				result.URL = &branchURL
			}
		}
	}
	if status.PullRequestState.Valid {
		pullRequestState := strings.TrimSpace(status.PullRequestState.String)
		if pullRequestState != "" {
			result.PullRequestState = &pullRequestState
		}
	}
	result.ChangesRequested = status.ChangesRequested
	result.Additions = status.Additions
	result.Deletions = status.Deletions
	result.ChangedFiles = status.ChangedFiles
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
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	configs, err := api.Database.GetChatModelConfigs(ctx)
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

	encoded, err := json.Marshal(modelConfig)
	if err != nil {
		return nil, xerrors.Errorf("encode model config: %w", err)
	}
	return encoded, nil
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
		isZeroChatModelProviderOptions(config.ProviderOptions)
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
	return chatprovider.ProviderAPIKeys{
		OpenAI:    deploymentValues.AI.BridgeConfig.OpenAI.Key.Value(),
		Anthropic: deploymentValues.AI.BridgeConfig.Anthropic.Key.Value(),
		BaseURLByProvider: map[string]string{
			"openai":    deploymentValues.AI.BridgeConfig.OpenAI.BaseURL.Value(),
			"anthropic": deploymentValues.AI.BridgeConfig.Anthropic.BaseURL.Value(),
		},
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
