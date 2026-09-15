package coderd

import (
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get orchestrator chat
// @ID get-orchestrator-chat
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {object} codersdk.Chat
// @Failure 404 {object} codersdk.Response "The caller has no orchestrator chat yet"
// @Router /api/v2/chats/orchestrator [get]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getOrchestratorChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	chat, err := api.Database.GetOrchestratorChatByOwnerID(ctx, apiKey.UserID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch orchestrator chat.",
			Detail:  err.Error(),
		})
		return
	}

	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Chat(chat, nil, chatFiles))
}

// @Summary Create orchestrator chat
// @ID create-orchestrator-chat
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateOrchestratorChatRequest true "Create orchestrator chat request"
// @Success 201 {object} codersdk.Chat
// @Failure 409 {object} codersdk.Response "The caller already has an orchestrator chat"
// @Router /api/v2/chats/orchestrator [post]
func (api *API) postOrchestratorChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	var req codersdk.CreateOrchestratorChatRequest
	if !httpapi.Read(ctx, rw, r, &req) {
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
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String()).InOrg(req.OrganizationID)) {
		httpapi.Forbidden(rw)
		return
	}

	// Check before doing any work so the common duplicate case returns a
	// clear conflict. The deterministic chat ID still guards the race.
	if existing, err := api.Database.GetOrchestratorChatByOwnerID(ctx, apiKey.UserID); err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "An orchestrator chat already exists.",
			Detail:  existing.ID.String(),
		})
		return
	} else if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to check for an existing orchestrator chat.",
			Detail:  err.Error(),
		})
		return
	}

	// Reuse the regular create-chat validation for the shared fields.
	createReq := codersdk.CreateChatRequest{
		OrganizationID:  req.OrganizationID,
		Content:         req.Content,
		ModelConfigID:   req.ModelConfigID,
		ReasoningEffort: req.ReasoningEffort,
		MCPServerIDs:    req.MCPServerIDs,
		ClientType:      req.ClientType,
	}
	contentBlocks, _, inputError := createChatInputFromRequest(ctx, api.Database, createReq)
	if inputError != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *inputError)
		return
	}
	modelConfigID, personalOverrideEffort, modelConfigStatus, modelConfigError := api.resolveCreateChatModelConfigID(ctx, apiKey.UserID, createReq)
	if modelConfigError != nil {
		httpapi.Write(ctx, rw, modelConfigStatus, *modelConfigError)
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
	if len(invalidMCPServerIDs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, invalidChatMCPServerIDsResponse(invalidMCPServerIDs))
		return
	}
	if normalizedMCPServerIDs == nil {
		normalizedMCPServerIDs = []uuid.UUID{}
	}

	clientType := database.ChatClientTypeApi
	if req.ClientType != "" {
		clientType = database.ChatClientType(req.ClientType)
		if !clientType.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid client_type.",
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
		ID:              chatd.OrchestratorChatID(apiKey.UserID),
		OrganizationID:  req.OrganizationID,
		OwnerID:         apiKey.UserID,
		Title:           "Orchestrator",
		ModelConfigID:   modelConfigID,
		ReasoningEffort: reasoningEffort,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeOrchestrator,
			Valid:    true,
		},
		ClientType:         clientType,
		InitialUserContent: contentBlocks,
		MCPServerIDs:       normalizedMCPServerIDs,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "An orchestrator chat already exists.",
			})
			return
		}
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
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create orchestrator chat.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	chat, err = api.Database.GetChatByID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to read back chat after creation.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.Chat(chat, nil, chatFiles))
}

// isOrchestratorChat reports whether the chat is the owner's orchestrator.
func isOrchestratorChat(chat database.Chat) bool {
	return chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeOrchestrator
}
