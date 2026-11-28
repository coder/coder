package coderd

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/codersdk"
)

// requireChatAutomationsExperiment answers 404 when the chat-automations
// experiment is off for the caller. The evaluator fails closed, so an
// evaluation error is off too.
func (api *API) requireChatAutomationsExperiment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !api.ExperimentEvaluator.Enabled(r.Context(), httpmw.APIKey(r).UserID, codersdk.ExperimentChatAutomations) {
			httpapi.RouteNotFound(rw)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

// @Summary Create chat automation
// @ID create-chat-automation
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateChatAutomationRequest true "Create chat automation request"
// @Success 201 {object} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chats/automations [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	aReq, commitAudit := audit.InitRequest[database.ChatAutomation](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var req codersdk.CreateChatAutomationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	// One time for the first run and the listed runs, so they agree.
	now := dbtime.Now()
	automation, err := chatautomation.Create(ctx, api.Database, organization.ID, req, now)
	var invalid *chatautomation.InvalidRequestError
	switch {
	case errors.As(err, &invalid):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Validation failed.",
			Validations: invalid.Validations,
		})
		return
	case errors.Is(err, chatautomation.ErrLimitReached):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat automation limit reached.",
			Detail:  fmt.Sprintf("You can own at most %d chat automations across all organizations. Delete one to create another.", chatautomation.MaxAutomationsPerOwner),
		})
		return
	case errors.Is(err, chatautomation.ErrTargetChatForbidden):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the owner of a chat can make it the target of an automation.",
		})
		return
	case errors.Is(err, chatautomation.ErrCreateForbidden):
		httpapi.Forbidden(rw)
		return
	case errors.Is(err, chatautomation.ErrReferenceUnavailable):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "A chat, model configuration or project of this automation was deleted while it was created.",
		})
		return
	case err != nil:
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = automation

	// The row is committed, so a failed read of the owner and chats leaves
	// them out of the response instead of failing it.
	refs, err := chatautomation.LoadRefs(ctx, api.Database, []database.ChatAutomation{automation})
	if err != nil {
		api.Logger.Warn(ctx, "load chat automation references for the create response", slog.Error(err))
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatAutomation(automation, refs, now))
}

// @Summary List chat automations
// @ID list-chat-automations
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param owner query string false "Owner: me, a user ID or a username"
// @Success 200 {array} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chats/automations [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatAutomations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	callerID := httpmw.APIKey(r).UserID

	ownerID, found, err := api.chatAutomationOwnerFilter(ctx, callerID, r.URL.Query().Get("owner"))
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if !found {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.ChatAutomation{})
		return
	}
	// A caller who cannot read every automation of the organization reads
	// only its own. Naming the owner lets the query use the
	// (organization_id, owner_id) index, which the RBAC filter cannot use
	// because it compares owner_id as text. The check does not log: a denial
	// is the normal case for a member.
	if ownerID == uuid.Nil && api.Authorizer.Authorize(ctx, httpmw.UserAuthorization(ctx), policy.ActionRead, rbac.ResourceChatAutomation.InOrg(organization.ID)) != nil {
		ownerID = callerID
	}
	automations, err := api.Database.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
		OrganizationID: organization.ID,
		OwnerID:        ownerID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("list chat automations: %w", err))
		return
	}
	out, err := api.convertChatAutomations(ctx, automations)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// chatAutomationOwnerFilter resolves the owner query parameter to an owner
// ID. It returns the nil UUID, which the list query reads as every owner,
// when the parameter is empty. found is false when no user matches.
func (api *API) chatAutomationOwnerFilter(ctx context.Context, callerID uuid.UUID, owner string) (id uuid.UUID, found bool, err error) {
	switch owner {
	case "":
		return uuid.Nil, true, nil
	case codersdk.Me:
		return callerID, true, nil
	}
	// Only the canonical form with hyphens is an ID. uuid.Parse also accepts
	// 32 hexadecimal characters, which is a valid username.
	if parsed, err := uuid.Parse(owner); len(owner) == len(uuid.Nil.String()) && err == nil {
		// The nil UUID names no user and must not reach the query as every
		// owner.
		return parsed, parsed != uuid.Nil, nil
	}
	user, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{Username: owner})
	if httpapi.Is404Error(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, xerrors.Errorf("get owner: %w", err)
	}
	return user.ID, true, nil
}

// @Summary Get chat automation
// @ID get-chat-automation
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param automation path string true "Chat automation ID" format(uuid)
// @Success 200 {object} codersdk.ChatAutomation
// @Router /api/experimental/organizations/{organization}/chats/automations/{automation} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // get-return: revive assumes get* must be a getter, but this is an HTTP handler.
func (api *API) getChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseChatAutomationID(rw, r)
	if !ok {
		return
	}
	automation, err := api.Database.GetChatAutomationByID(ctx, id)
	if httpapi.Is404Error(err) || (err == nil && automation.OrganizationID != httpmw.OrganizationParam(r).ID) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get chat automation: %w", err))
		return
	}
	out, err := api.convertChatAutomations(ctx, []database.ChatAutomation{automation})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, out[0])
}

// @Summary Delete chat automation
// @ID delete-chat-automation
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param automation path string true "Chat automation ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/automations/{automation} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatAutomation(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	id, ok := parseChatAutomationID(rw, r)
	if !ok {
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatAutomation](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	deleted, err := chatautomation.Delete(ctx, api.Database, organization.ID, id)
	switch {
	case errors.Is(err, chatautomation.ErrDeleteForbidden):
		httpapi.Forbidden(rw)
	case httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
	case err != nil:
		httpapi.InternalServerError(rw, err)
	default:
		aReq.Old = deleted
		rw.WriteHeader(http.StatusNoContent)
	}
}

func parseChatAutomationID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "automation"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat automation ID.",
		})
		return uuid.Nil, false
	}
	return id, true
}

// convertChatAutomations converts rows to their API shape, with the chats
// that the caller can read.
func (api *API) convertChatAutomations(ctx context.Context, automations []database.ChatAutomation) ([]codersdk.ChatAutomation, error) {
	refs, err := chatautomation.LoadRefs(ctx, api.Database, automations)
	if err != nil {
		return nil, err
	}
	now := dbtime.Now()
	return slice.List(automations, func(a database.ChatAutomation) codersdk.ChatAutomation {
		return db2sdk.ChatAutomation(a, refs, now)
	}), nil
}
