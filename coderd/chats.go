package coderd

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

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

	httpapi.Write(ctx, rw, http.StatusOK, convertChats(chats))
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

	// Set default title if empty.
	if req.Title == "" {
		req.Title = "New Chat"
	}

	// Marshal model config or use empty object.
	modelConfig := req.ModelConfig
	if modelConfig == nil {
		modelConfig = json.RawMessage("{}")
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
		Title:       req.Title,
		ModelConfig: modelConfig,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertChat(chat))
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
		Chat:     convertChat(chat),
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
	_, err = api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chatID,
		Role:      req.Role,
		Content:   req.Content,
		ToolCalls: req.ToolCalls,
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

func convertChat(c database.Chat) codersdk.Chat {
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
	return chat
}

func convertChats(chats []database.Chat) []codersdk.Chat {
	result := make([]codersdk.Chat, len(chats))
	for i, c := range chats {
		result[i] = convertChat(c)
	}
	return result
}

func convertChatMessage(m database.ChatMessage) codersdk.ChatMessage {
	msg := codersdk.ChatMessage{
		ID:        m.ID,
		ChatID:    m.ChatID,
		CreatedAt: m.CreatedAt,
		Role:      m.Role,
		Content:   m.Content,
		ToolCalls: m.ToolCalls,
		Hidden:    m.Hidden,
	}
	if m.ToolCallID.Valid {
		msg.ToolCallID = &m.ToolCallID.String
	}
	if m.Thinking.Valid {
		msg.Thinking = &m.Thinking.String
	}
	return msg
}

func convertChatMessages(messages []database.ChatMessage) []codersdk.ChatMessage {
	result := make([]codersdk.ChatMessage, len(messages))
	for i, m := range messages {
		result[i] = convertChatMessage(m)
	}
	return result
}
