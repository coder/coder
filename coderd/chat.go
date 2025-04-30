package coderd

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/kylecarbs/aisdk-go"

	"github.com/coder/coder/v2/coderd/ai"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// postChats creates a new chat.
//
// @Summary Create a chat
// @ID create-a-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Success 201 {object} codersdk.Chat
// @Router /chats [post]
func (api *API) postChats(w http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	chat, err := api.Database.InsertChat(ctx, database.InsertChatParams{
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
		return
	}

	httpapi.Write(ctx, w, http.StatusOK, db2sdk.Chats(chats))
}

// chat returns a chat by ID.
//
// @Summary Get a chat
// @ID get-a-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chat
// @Param chat path string true "Chat ID"
// @Success 200 {object} codersdk.Chat
// @Router /chats/{chat} [get]
func (*API) chat(w http.ResponseWriter, r *http.Request) {
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
// @Param chat path string true "Chat ID"
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
		return
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
			return
		}
		messages[i] = msg
	}

	httpapi.Write(ctx, w, http.StatusOK, messages)
}

// postChatMessages creates a new chat message and streams the response.
//
// @Summary Create a chat message
// @ID create-a-chat-message
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chat
// @Param chat path string true "Chat ID"
// @Param request body codersdk.CreateChatMessageRequest true "Request body"
// @Success 200 {array} aisdk.DataStreamPart
// @Router /chats/{chat}/messages [post]
func (api *API) postChatMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	var req codersdk.CreateChatMessageRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to decode chat message",
			Detail:  err.Error(),
		})
		return
	}

	dbMessages, err := api.Database.GetChatMessagesByChatID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages",
			Detail:  err.Error(),
		})
		return
	}

	messages := make([]aisdk.Message, 0, len(dbMessages))
	for i, message := range dbMessages {
		err = json.Unmarshal(message.Content, &messages[i])
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal chat message",
				Detail:  err.Error(),
			})
			return
		}
	}
	messages = append(messages, req.Message)

	client := codersdk.New(api.AccessURL)
	client.SetSessionToken(httpmw.APITokenFromRequest(r))

	tools := make([]aisdk.Tool, len(toolsdk.All))
	handlers := map[string]toolsdk.GenericHandlerFunc{}
	for i, tool := range toolsdk.All {
		if tool.Tool.Schema.Required == nil {
			tool.Tool.Schema.Required = []string{}
		}
		tools[i] = tool.Tool
		handlers[tool.Tool.Name] = tool.Handler
	}

	provider, ok := api.LanguageModels[req.Model]
	if !ok {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Model not found",
		})
		return
	}

	// If it's the user's first message, generate a title for the chat.
	if len(messages) == 1 {
		var acc aisdk.DataStreamAccumulator
		stream, err := provider.StreamFunc(ctx, ai.StreamOptions{
			Model: req.Model,
			SystemPrompt: `- You will generate a short title based on the user's message.
- It should be maximum of 40 characters.
- Do not use quotes, colons, special characters, or emojis.`,
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create stream",
				Detail:  err.Error(),
			})
			return
		}
		stream = stream.WithAccumulator(&acc)
		err = stream.Pipe(io.Discard)
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to pipe stream",
				Detail:  err.Error(),
			})
			return
		}
		err = api.Database.UpdateChatByID(ctx, database.UpdateChatByIDParams{
			ID:        chat.ID,
			Title:     acc.Messages()[0].Content,
			UpdatedAt: time.Now(),
		})
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat title",
				Detail:  err.Error(),
			})
			return
		}
	}

	// Write headers for the data stream!
	aisdk.WriteDataStreamHeaders(w)

	// Insert the user-requested message into the database!
	raw, err := json.Marshal([]aisdk.Message{req.Message})
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal chat message",
			Detail:  err.Error(),
		})
		return
	}
	_, err = api.Database.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:    chat.ID,
		CreatedAt: dbtime.Now(),
		Model:     req.Model,
		Provider:  provider.Provider,
		Content:   raw,
	})
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to insert chat messages",
			Detail:  err.Error(),
		})
		return
	}

	deps, err := toolsdk.NewDeps(client)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create tool dependencies",
			Detail:  err.Error(),
		})
		return
	}

	for {
		var acc aisdk.DataStreamAccumulator
		stream, err := provider.StreamFunc(ctx, ai.StreamOptions{
			Model:    req.Model,
			Messages: messages,
			Tools:    tools,
			SystemPrompt: `You are a chat assistant for Coder - an open-source platform for creating and managing cloud development environments on any infrastructure. You are expected to be precise, concise, and helpful.

You are running as an agent - please keep going until the user's query is completely resolved, before ending your turn and yielding back to the user. Only terminate your turn when you are sure that the problem is solved. Do NOT guess or make up an answer.`,
		})
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create stream",
				Detail:  err.Error(),
			})
			return
		}
		stream = stream.WithToolCalling(func(toolCall aisdk.ToolCall) any {
			tool, ok := handlers[toolCall.Name]
			if !ok {
				return nil
			}
			toolArgs, err := json.Marshal(toolCall.Args)
			if err != nil {
				return nil
			}
			result, err := tool(ctx, deps, toolArgs)
			if err != nil {
				return map[string]any{
					"error": err.Error(),
				}
			}
			return result
		}).WithAccumulator(&acc)

		err = stream.Pipe(w)
		if err != nil {
			// The client disppeared!
			api.Logger.Error(ctx, "stream pipe error", "error", err)
			return
		}

		raw, err := json.Marshal(acc.Messages())
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to marshal chat message",
				Detail:  err.Error(),
			})
			return
		}
		messages = append(messages, acc.Messages()...)

		// Insert these messages into the database!
		_, err = api.Database.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:    chat.ID,
			CreatedAt: dbtime.Now(),
			Model:     req.Model,
			Provider:  provider.Provider,
			Content:   raw,
		})
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to insert chat messages",
				Detail:  err.Error(),
			})
			return
		}

		if acc.FinishReason() == aisdk.FinishReasonToolCalls {
			continue
		}

		break
	}
}
