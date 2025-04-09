package coderd

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/ai"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	codermcp "github.com/coder/coder/v2/mcp"
	"github.com/google/uuid"
	"github.com/kylecarbs/aisdk-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
	}

	dbMessages, err := api.Database.GetChatMessagesByChatID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat messages",
			Detail:  err.Error(),
		})
	}

	messages := make([]aisdk.Message, len(dbMessages))
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

	toolMap := codermcp.AllTools()
	toolsByName := make(map[string]server.ToolHandlerFunc)
	client := codersdk.New(api.AccessURL)
	client.SetSessionToken(httpmw.APITokenFromRequest(r))
	toolDeps := codermcp.ToolDeps{
		Client: client,
		Logger: &api.Logger,
	}
	for _, tool := range toolMap {
		toolsByName[tool.Tool.Name] = tool.MakeHandler(toolDeps)
	}
	convertedTools := make([]aisdk.Tool, len(toolMap))
	for i, tool := range toolMap {
		schema := aisdk.Schema{
			Required:   tool.Tool.InputSchema.Required,
			Properties: tool.Tool.InputSchema.Properties,
		}
		if tool.Tool.InputSchema.Required == nil {
			schema.Required = []string{}
		}
		convertedTools[i] = aisdk.Tool{
			Name:        tool.Tool.Name,
			Description: tool.Tool.Description,
			Schema:      schema,
		}
	}

	provider, ok := api.LanguageModels[req.Model]
	if !ok {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Model not found",
		})
		return
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

	for {
		var acc aisdk.DataStreamAccumulator
		stream, err := provider.StreamFunc(ctx, ai.StreamOptions{
			Model:    req.Model,
			Messages: messages,
			Tools:    convertedTools,
		})
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create stream",
				Detail:  err.Error(),
			})
			return
		}
		stream = stream.WithToolCalling(func(toolCall aisdk.ToolCall) any {
			tool, ok := toolsByName[toolCall.Name]
			if !ok {
				return nil
			}
			result, err := tool(ctx, mcp.CallToolRequest{
				Params: struct {
					Name      string                 "json:\"name\""
					Arguments map[string]interface{} "json:\"arguments,omitempty\""
					Meta      *struct {
						ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
					} "json:\"_meta,omitempty\""
				}{
					Name:      toolCall.Name,
					Arguments: toolCall.Args,
				},
			})
			if err != nil {
				return map[string]any{
					"error": err.Error(),
				}
			}
			return result.Content
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
