package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/chats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

const chatMessagesPollInterval = 250 * time.Millisecond

// @Summary Create chat
// @ID create-chat
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param request body codersdk.CreateChatRequest true "Create chat request"
// @Success 201 {object} codersdk.Chat
// @Router /chats [post]
func (api *API) chatsCreate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := httpmw.APIKey(r)

	var req codersdk.CreateChatRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Provider == "" || req.Model == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "provider and model are required"})
		return
	}

	ownerID := key.UserID
	workspaceID := uuid.NullUUID{}
	var orgID uuid.UUID
	if req.WorkspaceID != nil {
		ws, err := api.Database.GetWorkspaceByID(ctx, *req.WorkspaceID)
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error fetching workspace.", Detail: err.Error()})
			return
		}
		workspaceID = uuid.NullUUID{UUID: ws.ID, Valid: true}
		orgID = ws.OrganizationID
		if ws.OwnerID != ownerID {
			httpapi.Forbidden(rw)
			return
		}
	} else {
		org, err := api.Database.GetDefaultOrganization(ctx)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error fetching default organization.", Detail: err.Error()})
			return
		}
		orgID = org.ID
	}

	id := uuid.New()
	now := time.Now().UTC()
	arg := database.InsertChatParams{
		ID:             id,
		CreatedAt:      now,
		UpdatedAt:      now,
		OrganizationID: orgID,
		OwnerID:        ownerID,
		WorkspaceID:    workspaceID,
		Title:          nullString(req.Title),
		Provider:       req.Provider,
		Model:          req.Model,
		Metadata:       req.Metadata,
	}

	chat, err := api.Database.InsertChat(ctx, arg)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error creating chat.", Detail: err.Error()})
		return
	}

	// Persist a system prompt as durable state so the agent loop can be restarted
	// solely from chat_messages.
	promptEnv := chats.SystemPromptEnvelope{Type: chats.EnvelopeTypeSystemPrompt, Content: chats.DefaultSystemPrompt}
	promptContent, err := chats.MarshalEnvelope(promptEnv)
	if err == nil {
		_, _ = api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:    chat.ID,
			CreatedAt: now,
			Role:      "system",
			Content:   promptContent,
		})
	}

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.Chat(chat))
}

// @Summary Get chat
// @ID get-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format:"uuid"
// @Success 200 {object} codersdk.Chat
// @Router /chats/{chat} [get]
func (api *API) chatGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = api
	chat := httpmw.ChatParam(r)
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(chat))
}

// @Summary List chat messages
// @ID list-chat-messages
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format:"uuid"
// @Success 200 {array} codersdk.ChatMessage
// @Router /chats/{chat}/messages [get]
func (api *API) chatMessages(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	rows, err := api.Database.ListChatMessages(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error listing chat messages.", Detail: err.Error()})
		return
	}

	out := make([]codersdk.ChatMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, db2sdk.ChatMessage(row))
	}
	//nolint:gosimple // Keep stable JSON shape.
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Create chat message and run agent loop
// @ID create-chat-message
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Chats
// @Param chat path string true "Chat ID" format:"uuid"
// @Param request body codersdk.CreateChatMessageRequest true "Create message request"
// @Success 201 {object} codersdk.CreateChatMessageResponse
// @Router /chats/{chat}/messages [post]
func (api *API) chatMessageCreateAndRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	var req codersdk.CreateChatMessageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Content == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "content is required"})
		return
	}

	runID := uuid.NewString()

	userMsg := aisdk.Message{
		Role: "user",
		Parts: []aisdk.Part{{
			Type: aisdk.PartTypeText,
			Text: req.Content,
		}},
	}
	msgEnv := chats.MessageEnvelope{Type: chats.EnvelopeTypeMessage, RunID: runID, Message: userMsg}
	content, err := chats.MarshalEnvelope(msgEnv)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error encoding chat message.", Detail: err.Error()})
		return
	}

	now := time.Now().UTC()
	row, err := api.Database.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:    chat.ID,
		CreatedAt: now,
		Role:      "user",
		Content:   content,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error creating chat message.", Detail: err.Error()})
		return
	}

	// Run the agentic loop in the background. All authoritative state is persisted
	// to chat_messages, so the client can watch the transcript (or re-fetch it) to
	// observe progress.
	actor, ok := dbauthz.ActorFromContext(ctx)
	if ok {
		//nolint:gocritic // We intentionally preserve the authenticated actor for background persistence.
		token := httpmw.APITokenFromRequest(r)
		chatRow := chat
		bg := dbauthz.As(api.ctx, actor)
		go func() {
			err := api.ChatRunner.Run(bg, chatRow, token, runID)
			if err != nil {
				api.Logger.Error(bg, "chat run failed to complete", slog.Error(err), slog.F("chat_id", chatRow.ID), slog.F("run_id", runID))
			}
		}()
	} else {
		api.Logger.Warn(ctx, "missing actor in request context; skipping chat run")
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateChatMessageResponse{RunID: runID, Message: db2sdk.ChatMessage(row)})
}

func (api *API) chatMessagesWatchWS(rw http.ResponseWriter, r *http.Request) {
	api.chatMessagesWatch(rw, r, httpapi.OneWayWebSocketEventSender(api.Logger.Named("chat_messages_watcher")))
}

func (api *API) chatMessagesWatch(
	rw http.ResponseWriter,
	r *http.Request,
	connect httpapi.EventSender,
) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)

	chat := httpmw.ChatParam(r)

	afterID := int64(0)
	if s := r.URL.Query().Get("after_id"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "invalid after_id"})
			return
		}
		afterID = v
	}

	sendEvent, closed, err := connect(rw, r)
	if err != nil {
		api.Logger.Warn(ctx, "chat watch failed to connect", slog.Error(err))
		return
	}

	// Helper to stream durable DB events.
	sendNewRows := func() error {
		rows, err := api.Database.ListChatMessagesAfter(ctx, database.ListChatMessagesAfterParams{
			ChatID: chat.ID,
			ID:     afterID,
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			afterID = row.ID
			err := sendEvent(codersdk.ServerSentEvent{Type: codersdk.ServerSentEventTypeData, Data: db2sdk.ChatMessage(row)})
			if err != nil {
				return err
			}
		}
		return nil
	}

	if err := sendNewRows(); err != nil {
		api.Logger.Warn(ctx, "chat watch failed to send initial rows", slog.Error(err))
		return
	}

	if api.ChatRunner == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "chat runner not configured"})
		return
	}

	sub, unsub := api.ChatRunner.Hub().Subscribe(ctx, chat.ID)
	defer unsub()

	ticker := time.NewTicker(chatMessagesPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-closed:
			return
		case <-ctx.Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}

			part, ok := ev.Part.(aisdk.DataStreamPart)
			if !ok {
				continue
			}

			payload := struct {
				RunID  string `json:"run_id"`
				TypeID byte   `json:"type_id"`
				Part   any    `json:"part"`
			}{
				RunID:  ev.RunID,
				TypeID: part.TypeID(),
				Part:   part,
			}
			if err := sendEvent(codersdk.ServerSentEvent{Type: codersdk.ServerSentEventTypeData, Data: payload}); err != nil {
				return
			}

		case <-ticker.C:
			if err := sendNewRows(); err != nil {
				api.Logger.Warn(ctx, "chat watch failed to poll", slog.Error(err))
				return
			}
		}
	}
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
