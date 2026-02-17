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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	aiapi "go.jetify.com/ai/api"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

const (
	chatDiffStatusTTL                = 120 * time.Second
	chatDiffBackgroundRefreshTimeout = 20 * time.Second
	githubAPIBaseURL                 = "https://api.github.com"
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

// @Summary List chats
// @ID list-chats
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Success 200 {array} codersdk.Chat
// @Router /chats [get]
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

	statuses, err := api.Database.GetChatDiffStatusesByChatIDs(dbauthz.AsSystemRestricted(ctx), chatIDs)
	if err != nil {
		return nil, xerrors.Errorf("get chat diff statuses: %w", err)
	}

	statusesByChatID := make(map[uuid.UUID]database.ChatDiffStatus, len(statuses))
	for _, status := range statuses {
		statusesByChatID[status.ChatID] = status
	}
	return statusesByChatID, nil
}

// @Summary Create a chat
// @ID create-chat
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param request body codersdk.CreateChatRequest true "Create chat request"
// @Success 201 {object} codersdk.Chat
// @Router /chats [post]
func (api *API) createChat(rw http.ResponseWriter, r *http.Request) {
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

	systemPrompt := defaultChatSystemPrompt(api)
	if override := strings.TrimSpace(req.SystemPrompt); override != "" {
		if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
			httpapi.Forbidden(rw)
			return
		}
		systemPrompt = override
	}

	title := chatTitleFromMessage(titleSource)

	// Marshal model config or use empty object.
	modelConfig := req.ModelConfig
	if modelConfig == nil {
		modelConfig = json.RawMessage("{}")
	}
	if req.Model != "" {
		modelPayload, err := json.Marshal(map[string]string{"model": req.Model})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to encode model configuration.",
				Detail:  err.Error(),
			})
			return
		}
		modelConfig = modelPayload
	}

	chat, err := api.Database.InsertChat(ctx, database.InsertChatParams{
		OwnerID: apiKey.UserID,
		WorkspaceID: uuid.NullUUID{
			UUID:  uuidOrNil(req.WorkspaceID),
			Valid: req.WorkspaceID != nil,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  uuidOrNil(req.WorkspaceAgentID),
			Valid: req.WorkspaceAgentID != nil,
		},
		Title:       title,
		ModelConfig: modelConfig,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat.",
			Detail:  err.Error(),
		})
		return
	}
	if systemPrompt != "" {
		systemContent, err := json.Marshal(systemPrompt)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to encode system prompt.",
				Detail:  err.Error(),
			})
			return
		}

		_, err = api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID: chat.ID,
			Role:   "system",
			Content: pqtype.NullRawMessage{
				RawMessage: systemContent,
				Valid:      len(systemContent) > 0,
			},
			ToolCallID: sql.NullString{
				Valid: false,
			},
			Thinking: sql.NullString{
				Valid: false,
			},
			Hidden: false,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat message.",
				Detail:  err.Error(),
			})
			return
		}
	}

	content, err := json.Marshal(contentBlocks)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to encode chat message.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID: chat.ID,
		Role:   "user",
		Content: pqtype.NullRawMessage{
			RawMessage: content,
			Valid:      len(content) > 0,
		},
		ToolCallID: sql.NullString{
			Valid: false,
		},
		Thinking: sql.NullString{
			Valid: false,
		},
		Hidden: false,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat message.",
			Detail:  err.Error(),
		})
		return
	}

	updatedChat, err := api.Database.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to mark chat as pending",
			slog.F("chat_id", chat.ID), slog.Error(err))
	} else {
		chat = updatedChat
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertChat(chat, nil))
}

// @Summary List chat models
// @ID list-chat-models
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Success 200 {object} codersdk.ChatModelsResponse
// @Router /chats/models [get]
func (api *API) listChatModels(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if api.chatModelCatalog == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat model catalog unavailable.",
			Detail:  "Chat model catalog is not configured.",
		})
		return
	}

	configuredProviders, configuredModels, err := api.loadEnabledChatCatalogConfig(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load chat model configuration.",
			Detail:  err.Error(),
		})
		return
	}
	if response, ok := api.chatModelCatalog.ListConfiguredModels(configuredProviders, configuredModels); ok {
		httpapi.Write(ctx, rw, http.StatusOK, response)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.chatModelCatalog.ListConfiguredProviderAvailability(configuredProviders))
}

// @Summary Get a chat
// @ID get-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatWithMessages
// @Router /chats/{chat} [get]
func (api *API) getChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	chat, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

	messages, err := api.Database.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatWithMessages{
		Chat:     convertChat(chat, nil),
		Messages: convertChatMessages(messages),
	})
}

// @Summary Delete a chat
// @ID delete-chat
// @Security CoderSessionToken
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 204
// @Router /chats/{chat} [delete]
func (api *API) deleteChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	// Check that the chat exists and user has access.
	_, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.DeleteChatByID(ctx, chatID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Create a chat message
// @ID create-chat-message
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Param request body codersdk.CreateChatMessageRequest true "Create chat message request"
// @Success 200 {array} codersdk.ChatMessage
// @Router /chats/{chat}/messages [post]
func (api *API) createChatMessage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	var req codersdk.CreateChatMessageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate role.
	if req.Role != "user" && req.Role != "assistant" && req.Role != "system" && req.Role != "tool" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid role.",
			Detail:  "Role must be one of: user, assistant, system, tool",
		})
		return
	}

	// Check that the chat exists and user has access.
	_, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

	// Insert the message.
	message, err := api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID: chatID,
		Role:   req.Role,
		Content: pqtype.NullRawMessage{
			RawMessage: req.Content,
			Valid:      len(req.Content) > 0,
		},
		ToolCallID: sql.NullString{
			String: stringOrEmpty(req.ToolCallID),
			Valid:  req.ToolCallID != nil,
		},
		Thinking: sql.NullString{
			String: stringOrEmpty(req.Thinking),
			Valid:  req.Thinking != nil,
		},
		Hidden: false,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat message.",
			Detail:  err.Error(),
		})
		return
	}

	if api.chatStreamManager != nil {
		chatMessage := convertChatMessage(message)
		api.chatStreamManager.Publish(chatID, codersdk.ChatStreamEvent{
			Type:    codersdk.ChatStreamEventTypeMessage,
			ChatID:  chatID,
			Message: &chatMessage,
		})
	}

	// If this is a user message, mark the chat as pending so the processor picks
	// it up.
	if req.Role == "user" {
		updatedChat, err := api.Database.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        chatID,
			Status:    database.ChatStatusPending,
			WorkerID:  uuid.NullUUID{},
			StartedAt: sql.NullTime{},
		})
		if err != nil {
			// Log but don't fail - the message was saved.
			api.Logger.Error(ctx, "failed to mark chat as pending",
				slog.F("chat_id", chatID), slog.Error(err))
		} else if api.chatStreamManager != nil {
			api.chatStreamManager.Publish(chatID, codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				ChatID: chatID,
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(updatedChat.Status)},
			})
		}
	}

	// Return all messages for this chat.
	messages, err := api.Database.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChatMessages(messages))
}

// @Summary Stream chat updates
// @ID stream-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ServerSentEvent
// @Router /chats/{chat}/stream [get]
func (api *API) streamChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	// Check that the chat exists and user has access.
	_, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

	if api.chatStreamManager == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Chat streaming is not available.",
			Detail:  "Chat stream manager is not configured.",
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

	snapshot, events, cancel := api.chatStreamManager.Subscribe(chatID)
	defer cancel()

	for _, event := range snapshot {
		if err := sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: event,
		}); err != nil {
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
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := sendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeData,
				Data: event,
			}); err != nil {
				api.Logger.Debug(ctx, "failed to send chat stream event", slog.Error(err))
				return
			}
		}
	}
}

// @Summary Interrupt a chat
// @ID interrupt-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.Chat
// @Router /chats/{chat}/interrupt [post]
func (api *API) interruptChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	chat, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

	if api.chatProcessor != nil {
		api.chatProcessor.InterruptChat(chatID)
	}

	if api.chatStreamManager != nil {
		api.chatStreamManager.StopStream(chatID)
	}

	updatedChat, err := api.Database.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        chatID,
		Status:    database.ChatStatusWaiting,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to mark chat as waiting",
			slog.F("chat_id", chatID), slog.Error(err))
	} else {
		chat = updatedChat
	}

	if api.chatStreamManager != nil {
		api.chatStreamManager.Publish(chatID, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeStatus,
			ChatID: chatID,
			Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(chat.Status)},
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertChat(chat, nil))
}

// @Summary Get diff status for a chat
// @ID get-chat-diff-status
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatDiffStatus
// @Router /chats/{chat}/diff-status [get]
func (api *API) getChatDiffStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	chat, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

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

// @Summary Get diff contents for a chat
// @ID get-chat-diff
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {object} codersdk.ChatDiffContents
// @Router /chats/{chat}/diff [get]
func (api *API) getChatDiffContents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID, ok := parseChatID(rw, r)
	if !ok {
		return
	}

	chat, err := api.Database.GetChatByID(ctx, chatID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat.",
			Detail:  err.Error(),
		})
		return
	}

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

func (api *API) newChatWorkspaceCreator() chatd.WorkspaceCreator {
	return chatd.NewWorkspaceCreator(chatd.WorkspaceCreatorAdapterFuncs{
		PrepareWorkspaceCreateFunc: api.prepareChatWorkspaceCreate,
		AuthorizedTemplatesFunc:    api.authorizedChatWorkspaceTemplates,
		CreateWorkspaceFunc:        api.createChatWorkspace,
		DatabaseStore:              api.Database,
		PubsubStore:                api.Pubsub,
		LoggerStore:                api.Logger,
	})
}

func (api *API) prepareChatWorkspaceCreate(
	ctx context.Context,
	ownerID uuid.UUID,
) (context.Context, *http.Request, string, error) {
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, nil, "", xerrors.Errorf("load chat owner authorization: %w", err)
	}
	ctx = dbauthz.As(ctx, actor)

	accessURL := ""
	workspaceCreateURL := "http://localhost/api/v2/chats/workspace"
	if api.AccessURL != nil {
		accessURL = strings.TrimRight(api.AccessURL.String(), "/")
		workspaceCreateURL = accessURL + "/api/v2/chats/workspace"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, workspaceCreateURL, nil)
	if err != nil {
		return nil, nil, "", xerrors.Errorf("create synthetic workspace request: %w", err)
	}
	return ctx, req, accessURL, nil
}

func (api *API) authorizedChatWorkspaceTemplates(
	ctx context.Context,
	r *http.Request,
) ([]database.Template, error) {
	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, policy.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		return nil, xerrors.Errorf("prepare template authorization filter: %w", err)
	}

	templates, err := api.Database.GetAuthorizedTemplates(ctx, database.GetTemplatesWithFilterParams{
		Deleted: false,
		Deprecated: sql.NullBool{
			Bool:  false,
			Valid: true,
		},
	}, prepared)
	if err != nil {
		if err == sql.ErrNoRows {
			return []database.Template{}, nil
		}
		return nil, xerrors.Errorf("get authorized templates: %w", err)
	}
	return templates, nil
}

func (api *API) createChatWorkspace(
	ctx context.Context,
	r *http.Request,
	ownerID uuid.UUID,
	req codersdk.CreateWorkspaceRequest,
) (codersdk.Workspace, error) {
	ownerUser, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), ownerID)
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

	rw := httptest.NewRecorder()
	sw := &tracing.StatusWriter{ResponseWriter: rw}
	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](sw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})
	defer commitAudit()

	workspace, err := createWorkspace(ctx, aReq, ownerID, api, owner, req, r, nil)
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
		return nil, nil
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
// every chat has a PR URL resolved, signalling that the caller can
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

		api.publishChatStatusEvent(chat)
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

func (api *API) publishChatStatusEvent(chat database.Chat) {
	if api.chatStreamManager == nil {
		return
	}

	api.chatStreamManager.Publish(chat.ID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		ChatID: chat.ID,
		Status: &codersdk.ChatStreamStatus{
			Status: codersdk.ChatStatus(chat.Status),
		},
	})
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
			URL: sql.NullString{
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
			URL:    sql.NullString{String: pullRequestURL, Valid: true},
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

func parseChatID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	chatIDStr := chi.URLParam(r, "chat")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return chatID, true
}

func uuidOrNil(u *uuid.UUID) uuid.UUID {
	if u == nil {
		return uuid.Nil
	}
	return *u
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func defaultChatSystemPrompt(api *API) string {
	if api == nil || api.DeploymentValues == nil {
		return ""
	}
	return strings.TrimSpace(api.DeploymentValues.AI.Chat.SystemPrompt.Value())
}

func createChatInputFromRequest(req codersdk.CreateChatRequest) (
	[]aiapi.ContentBlock,
	string,
	*codersdk.Response,
) {
	if req.Input != nil {
		if len(req.Input.Parts) == 0 {
			return nil, "", &codersdk.Response{
				Message: "Input is required.",
				Detail:  "Input parts cannot be empty.",
			}
		}

		content := make([]aiapi.ContentBlock, 0, len(req.Input.Parts))
		textParts := make([]string, 0, len(req.Input.Parts))
		for i, part := range req.Input.Parts {
			switch strings.ToLower(strings.TrimSpace(string(part.Type))) {
			case string(codersdk.ChatInputPartTypeText):
				text := strings.TrimSpace(part.Text)
				if text == "" {
					return nil, "", &codersdk.Response{
						Message: "Invalid input part.",
						Detail:  fmt.Sprintf("input.parts[%d].text cannot be empty.", i),
					}
				}
				content = append(content, &aiapi.TextBlock{Text: text})
				textParts = append(textParts, text)
			default:
				return nil, "", &codersdk.Response{
					Message: "Invalid input part.",
					Detail: fmt.Sprintf(
						"input.parts[%d].type %q is not supported.",
						i,
						part.Type,
					),
				}
			}
		}

		titleSource := strings.TrimSpace(strings.Join(textParts, " "))
		if titleSource == "" {
			return nil, "", &codersdk.Response{
				Message: "Input is required.",
				Detail:  "Input must include at least one text part.",
			}
		}
		return content, titleSource, nil
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, "", &codersdk.Response{
			Message: "Input is required.",
			Detail:  "Provide input.parts or message.",
		}
	}

	return aiapi.ContentFromText(message), message, nil
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
	return chatd.TruncateRunes(title, maxRunes)
}

func convertChat(c database.Chat, diffStatus *database.ChatDiffStatus) codersdk.Chat {
	chat := codersdk.Chat{
		ID:          c.ID,
		OwnerID:     c.OwnerID,
		Title:       c.Title,
		Status:      codersdk.ChatStatus(c.Status),
		ModelConfig: c.ModelConfig,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
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

func convertChatMessage(m database.ChatMessage) codersdk.ChatMessage {
	return chatd.SDKChatMessage(m)
}

func convertChatMessages(messages []database.ChatMessage) []codersdk.ChatMessage {
	result := make([]codersdk.ChatMessage, len(messages))
	for i, m := range messages {
		result[i] = convertChatMessage(m)
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

	resp := make([]codersdk.ChatProviderConfig, 0, len(providers))
	for _, provider := range providers {
		resp = append(resp, convertChatProviderConfig(provider))
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) createChatProvider(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
			Detail:  "Provider must be one of: openai, anthropic.",
		})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	inserted, err := api.Database.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    provider,
		DisplayName: strings.TrimSpace(req.DisplayName),
		APIKey:      strings.TrimSpace(req.APIKey),
		ApiKeyKeyID: sql.NullString{},
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

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatProviderConfig(inserted))
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

	updated, err := api.Database.UpdateChatProvider(ctx, database.UpdateChatProviderParams{
		DisplayName: displayName,
		APIKey:      apiKey,
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

	httpapi.Write(ctx, rw, http.StatusOK, convertChatProviderConfig(updated))
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
			Detail:  "Provider must be one of: openai, anthropic.",
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

	inserted, err := api.Database.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:    provider,
		Model:       model,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Enabled:     enabled,
	})
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
				Detail:  "Provider must be one of: openai, anthropic.",
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

	updated, err := api.Database.UpdateChatModelConfig(ctx, database.UpdateChatModelConfigParams{
		Provider:    provider,
		Model:       model,
		DisplayName: displayName,
		Enabled:     enabled,
		ID:          existing.ID,
	})
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

	if err := api.Database.DeleteChatModelConfigByID(ctx, modelConfigID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat model config.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) loadEnabledChatCatalogConfig(ctx context.Context) ([]chatd.ConfiguredProvider, []chatd.ConfiguredModel, error) {
	ctx = dbauthz.AsSystemRestricted(ctx)

	enabledProviders, err := api.Database.GetEnabledChatProviders(ctx)
	if err != nil {
		return nil, nil, err
	}
	enabledModels, err := api.Database.GetEnabledChatModelConfigs(ctx)
	if err != nil {
		return nil, nil, err
	}

	providers := make([]chatd.ConfiguredProvider, 0, len(enabledProviders))
	for _, provider := range enabledProviders {
		providers = append(providers, chatd.ConfiguredProvider{
			Provider: provider.Provider,
			APIKey:   provider.APIKey,
		})
	}

	models := make([]chatd.ConfiguredModel, 0, len(enabledModels))
	for _, model := range enabledModels {
		models = append(models, chatd.ConfiguredModel{
			Provider:    model.Provider,
			Model:       model.Model,
			DisplayName: model.DisplayName,
		})
	}

	return providers, models, nil
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

func convertChatProviderConfig(provider database.ChatProvider) codersdk.ChatProviderConfig {
	return codersdk.ChatProviderConfig{
		ID:          provider.ID,
		Provider:    provider.Provider,
		DisplayName: provider.DisplayName,
		Enabled:     provider.Enabled,
		HasAPIKey:   strings.TrimSpace(provider.APIKey) != "",
		CreatedAt:   provider.CreatedAt,
		UpdatedAt:   provider.UpdatedAt,
	}
}

func convertChatModelConfig(config database.ChatModelConfig) codersdk.ChatModelConfig {
	return codersdk.ChatModelConfig{
		ID:          config.ID,
		Provider:    config.Provider,
		Model:       config.Model,
		DisplayName: config.DisplayName,
		Enabled:     config.Enabled,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}
}

func normalizeChatProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	default:
		return ""
	}
}
