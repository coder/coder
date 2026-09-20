package coderd

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// chatTreeEnabled reports whether the chat-tree experiment is on. It does
// not use the dev-build bypass so the behavior is identical in dev builds.
func (api *API) chatTreeEnabled() bool {
	return api.Experiments.Enabled(codersdk.ExperimentChatTree)
}

// chatTreeRootModelResolver resolves the model config a lazily created
// tree root uses, following the same rules as creating a chat without an
// explicit model config. A client-class response (no usable model) is
// returned as [chatd.ErrChatTreeRootUnavailable]; server failures are
// returned as ordinary errors.
func (api *API) chatTreeRootModelResolver(userID, organizationID uuid.UUID) func(context.Context) (uuid.UUID, error) {
	return func(ctx context.Context) (uuid.UUID, error) {
		modelConfigID, _, status, resp := api.resolveCreateChatModelConfigID(ctx, userID, codersdk.CreateChatRequest{
			OrganizationID: organizationID,
		})
		if resp != nil {
			err := xerrors.Errorf("%s: %s", resp.Message, resp.Detail)
			if status >= 400 && status < 500 {
				return uuid.Nil, errors.Join(chatd.ErrChatTreeRootUnavailable, err)
			}
			return uuid.Nil, err
		}
		return modelConfigID, nil
	}
}

// auditChatTreeRootCreated records the creation of a tree root that
// happened as a side effect of another request.
func (api *API) auditChatTreeRootCreated(ctx context.Context, r *http.Request, root database.Chat) {
	audit.BackgroundAudit(context.WithoutCancel(ctx), &audit.BackgroundAuditParams[database.Chat]{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		UserID:         root.OwnerID,
		RequestID:      httpmw.RequestID(r),
		Status:         http.StatusCreated,
		IP:             r.RemoteAddr,
		UserAgent:      r.UserAgent(),
		Action:         database.AuditActionCreate,
		OrganizationID: root.OrganizationID,
		New:            root,
	})
}

// ensureChatTreeRoot materializes the caller's tree root and adopts the
// caller's parentless chats. It returns the root, or the nil UUID and no
// error when no model config is available for the root. Other resolver
// failures are returned as errors.
func (api *API) ensureChatTreeRoot(ctx context.Context, userID, organizationID uuid.UUID) (database.Chat, bool, error) {
	root, created, err := api.chatDaemon.EnsureChatTreeRoot(ctx, chatd.EnsureChatTreeRootOptions{
		OwnerID:              userID,
		OrganizationID:       organizationID,
		ResolveModelConfigID: api.chatTreeRootModelResolver(userID, organizationID),
	})
	if errors.Is(err, chatd.ErrChatTreeRootUnavailable) {
		return database.Chat{}, false, nil
	}
	return root, created, err
}

// @Summary Get chat tree
// @ID get-chat-tree
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param archived query bool false "Return archived chats instead of unarchived ones. The root is always returned."
// @Success 200 {object} codersdk.ChatTreeResponse
// @Router /api/v2/organizations/{organization}/chats/tree [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatTree(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.chatTreeEnabled() {
		httpapi.ResourceNotFound(rw)
		return
	}
	if !api.requireChatDaemon(ctx, rw) {
		return
	}
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	isMember, err := httpmw.UserAuthorization(ctx).HasOrganizationMembership(organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if !isMember {
		httpapi.ResourceNotFound(rw)
		return
	}
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceChat.WithOwner(apiKey.UserID.String()).InOrg(organization.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	parser := httpapi.NewQueryParamParser()
	archived := parser.Boolean(r.URL.Query(), false, "archived")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	root, created, err := api.ensureChatTreeRoot(ctx, apiKey.UserID, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to prepare the chat tree.",
			Detail:  err.Error(),
		})
		return
	}
	if created {
		aReq.New = root
	}

	rows, err := api.Database.GetChatTreeByOwnerAndOrganization(ctx, database.GetChatTreeByOwnerAndOrganizationParams{
		OwnerID:        apiKey.UserID,
		OrganizationID: organization.ID,
		Archived:       archived,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to load the chat tree.",
			Detail:  err.Error(),
		})
		return
	}

	resp := codersdk.ChatTreeResponse{
		Chats: db2sdk.ChatTreeRows(rows),
	}
	if root.ID != uuid.Nil {
		rootID := root.ID
		resp.RootChatID = &rootID
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// writeChatTreeParentError maps parent validation errors from chat
// creation to 400 responses. It reports whether a response was written.
func writeChatTreeParentError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	var message string
	switch {
	case errors.Is(err, chatd.ErrChatTreeParentMismatch):
		message = "Parent chat not found."
	case errors.Is(err, chatd.ErrChatTreeParentUnanchored):
		message = "Parent chat is not attached to a chat tree."
	case errors.Is(err, chatd.ErrChatTreeParentIsSubagent):
		message = "Subagent chats cannot have child chats."
	case errors.Is(err, chatd.ErrChatTreeParentArchived):
		message = "Cannot create a chat under an archived parent."
	case errors.Is(err, chatd.ErrChatTreeDepthExceeded):
		message = "Chat tree depth limit exceeded."
	default:
		return false
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: message,
		Detail:  err.Error(),
	})
	return true
}

// attachChatTreePosition sets Depth and ChildChatCount on a single chat
// response for root and chat kind rows attached to a tree root.
func (api *API) attachChatTreePosition(ctx context.Context, chat database.Chat, sdkChat *codersdk.Chat) error {
	if chat.Kind == database.ChatKindSubagent {
		return nil
	}
	depth, err := api.Database.GetChatTreeDepthByID(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("get chat tree depth: %w", err)
	}
	if depth == 0 {
		return nil
	}
	count, err := api.Database.CountChatChildrenByParentID(ctx, database.CountChatChildrenByParentIDParams{
		ParentChatID: chat.ID,
		Kinds:        []database.ChatKind{database.ChatKindChat},
	})
	if err != nil {
		return xerrors.Errorf("count child chats: %w", err)
	}
	db2sdk.SetChatTreePosition(sdkChat, depth, count)
	return nil
}
