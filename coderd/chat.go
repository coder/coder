package coderd

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/google/uuid"
	"github.com/kylecarbs/aisdk-go"
)

// postChats creates a new chat.
//
// @Summary Create a chat
// @ID post-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 201 {object} codersdk.Chat
// @Router /chats [post]
func (api *API) postChats(w http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	chat, err := api.Database.InsertChat(ctx, database.InsertChatParams{
		ID:        uuid.New(),
		OwnerID:   apiKey.UserID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     "New Chat",
	})
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, w, http.StatusCreated, db2sdk.Chat(chat))
}

// listChats lists all chats for a user.
//
// @Summary List chats
// @ID list-chats
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 200 {array} codersdk.Chat
// @Router /chats [get]
func (api *API) listChats(w http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	chats, err := api.Database.GetChatsByOwnerID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chats",
			Detail:  err.Error(),
		})
	}

	httpapi.Write(ctx, w, http.StatusOK, db2sdk.Chats(chats))
}

// chat returns a chat by ID.
//
// @Summary Get a chat
// @ID get-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 200 {object} codersdk.Chat
// @Router /chats/{chat} [get]
func (api *API) chat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	httpapi.Write(ctx, w, http.StatusOK, db2sdk.Chat(chat))
}

// chatMessages returns the messages of a chat.
//
// @Summary Get chat messages
// @ID get-chat-messages
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 200 {array} aisdk.Message
// @Router /chats/{chat}/messages [get]
func (api *API) chatMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	rawMessages, err := api.Database.GetChatMessagesByChatID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages",
			Detail:  err.Error(),
		})
	}
	messages := make([]aisdk.Message, len(rawMessages))
	for i, message := range rawMessages {
		var msg aisdk.Message
		err = json.Unmarshal(message.Content, &msg)
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal chat message",
				Detail:  err.Error(),
			})
		}
		messages[i] = msg
	}

	httpapi.Write(ctx, w, http.StatusOK, messages)
}

func (api *API) postChatMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	var message aisdk.Message
	err := json.NewDecoder(r.Body).Decode(&message)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to decode chat message",
			Detail:  err.Error(),
		})
	}

	var stream aisdk.DataStream
	stream.WithToolCalling(func(toolCall aisdk.ToolCall) any {
		return nil
	})
}
