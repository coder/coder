package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/chats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

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
	workspaceName := ""
	var orgID uuid.UUID
	id := uuid.New()
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
		workspaceName = ws.Name
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

		ws, err := api.createChatWorkspace(ctx, rw, r, ownerID, orgID, id)
		if err != nil {
			httperror.WriteResponseError(ctx, rw, err)
			return
		}
		workspaceID = uuid.NullUUID{UUID: ws.ID, Valid: true}
		workspaceName = ws.Name
		orgID = ws.OrganizationID
	}
	now := time.Now().UTC()
	metadata := req.Metadata
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}

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
		Metadata:       metadata,
	}

	chat, err := api.Database.InsertChat(ctx, arg)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error creating chat.", Detail: err.Error()})
		return
	}

	// Persist a system prompt as durable state so the agent loop can be restarted
	// solely from chat_messages.
	systemPrompt := chats.DefaultSystemPrompt
	if workspaceID.Valid {
		name := workspaceName
		if name == "" {
			// Fetch workspace details to build a context-aware prompt.
			ws, wsErr := api.Database.GetWorkspaceByID(ctx, workspaceID.UUID)
			if wsErr == nil {
				name = ws.Name
			}
		}
		if name != "" {
			systemPrompt = buildWorkspaceSystemPrompt(name)
		}
	}
	promptEnv := chats.SystemPromptEnvelope{Type: chats.EnvelopeTypeSystemPrompt, Content: systemPrompt}
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

// @Summary List chats
// @ID list-chats
// @Security CoderSessionToken
// @Produce json
// @Tags Chats
// @Success 200 {array} codersdk.Chat
// @Router /chats [get]
func (api *API) chatsList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := httpmw.APIKey(r)

	rows, err := api.Database.ListChatsByOwner(ctx, key.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Internal error listing chats.", Detail: err.Error()})
		return
	}

	out := make([]codersdk.Chat, 0, len(rows))
	for _, row := range rows {
		out = append(out, db2sdk.Chat(row))
	}
	//nolint:gosimple // Keep stable JSON shape.
	httpapi.Write(ctx, rw, http.StatusOK, out)
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

	if api.ChatRunner != nil {
		api.ChatRunner.Hub().PublishMessage(chat.ID, row)
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

	if api.ChatRunner == nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "chat runner not configured"})
		return
	}

	sub, unsub := api.ChatRunner.Hub().Subscribe(ctx, chat.ID)
	defer unsub()

	if err := sendNewRows(); err != nil {
		api.Logger.Warn(ctx, "chat watch failed to send initial rows", slog.Error(err))
		return
	}

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

			if ev.Message != nil {
				if ev.Message.ID <= afterID {
					continue
				}
				afterID = ev.Message.ID
				if err := sendEvent(codersdk.ServerSentEvent{Type: codersdk.ServerSentEventTypeData, Data: db2sdk.ChatMessage(*ev.Message)}); err != nil {
					return
				}
				continue
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

		}
	}
}

func (api *API) createChatWorkspace(
	ctx context.Context,
	rw http.ResponseWriter,
	r *http.Request,
	ownerID uuid.UUID,
	orgID uuid.UUID,
	chatID uuid.UUID,
) (codersdk.Workspace, error) {
	owner, err := api.Database.GetUserByID(ctx, ownerID)
	if err != nil {
		return codersdk.Workspace{}, err
	}

	template, err := selectChatTemplate(ctx, api.Database, orgID)
	if err != nil {
		return codersdk.Workspace{}, err
	}

	shortID := strings.SplitN(chatID.String(), "-", 2)[0]
	workspaceName := fmt.Sprintf("chat-%s", shortID)

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
		OrganizationID: orgID,
	})
	defer commitAudit()

	return createWorkspace(ctx, aReq, ownerID, api, workspaceOwner{
		ID:        owner.ID,
		Username:  owner.Username,
		AvatarURL: owner.AvatarURL,
	}, codersdk.CreateWorkspaceRequest{
		TemplateID: template.ID,
		Name:       workspaceName,
	}, r, nil)
}

func selectChatTemplate(ctx context.Context, db database.Store, orgID uuid.UUID) (database.Template, error) {
	templates, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		Deleted:        false,
		OrganizationID: orgID,
		HasAITask:      sql.NullBool{Bool: true, Valid: true},
	})
	if err != nil {
		return database.Template{}, err
	}
	if template, ok, pickErr := pickChatTemplate(ctx, db, templates); pickErr != nil {
		return database.Template{}, pickErr
	} else if ok {
		return template, nil
	}

	templates, err = db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		Deleted:        false,
		OrganizationID: orgID,
	})
	if err != nil {
		return database.Template{}, err
	}
	if template, ok, pickErr := pickChatTemplate(ctx, db, templates); pickErr != nil {
		return database.Template{}, pickErr
	} else if ok {
		return template, nil
	}
	if len(templates) == 0 {
		return database.Template{}, httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "No templates available to create a chat workspace.",
			Detail:  "Create a template or pass workspace_id when creating a chat.",
		})
	}
	return database.Template{}, httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
		Message: "No templates with a successfully imported version are available to create a chat workspace.",
		Detail:  "Wait for a template version to finish importing or pass workspace_id when creating a chat.",
	})
}

func pickChatTemplate(
	ctx context.Context,
	db database.Store,
	templates []database.Template,
) (database.Template, bool, error) {
	for _, template := range templates {
		if template.ActiveVersionID == uuid.Nil {
			continue
		}
		version, err := db.GetTemplateVersionByID(ctx, template.ActiveVersionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return database.Template{}, false, err
		}
		job, err := db.GetProvisionerJobByID(ctx, version.JobID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return database.Template{}, false, err
		}
		if job.JobStatus == database.ProvisionerJobStatusSucceeded {
			return template, true, nil
		}
	}
	return database.Template{}, false, nil
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func buildWorkspaceSystemPrompt(workspaceName string) string {
	return fmt.Sprintf(`You are a helpful coding assistant with access to a Coder workspace named %q.

When you need to execute commands, read files, or perform other operations in the workspace, use the available tools with workspace=%q. Refer to the tool descriptions for details about each tool.

Be concise and helpful. When executing commands, explain what you're doing and interpret the results.`, workspaceName, workspaceName)
}
