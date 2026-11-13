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
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/codersdk"
)

// requireChatAutomationsExperiment answers 404 when the chat-automations
// experiment is off for the caller. The evaluator fails closed, so an
// evaluation error is off too.
func (api *API) requireChatAutomationsExperiment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		apiKey := httpmw.APIKey(r)
		if !api.ExperimentEvaluator.Enabled(r.Context(), apiKey.UserID, codersdk.ExperimentChatAutomations) {
			httpapi.RouteNotFound(rw)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

// writeChatAutomationError answers 500 with no detail. The request logger
// logs the body of a 5xx response, so the cause goes to a log field instead.
func (api *API) writeChatAutomationError(ctx context.Context, rw http.ResponseWriter, message string, err error) {
	api.Logger.Error(ctx, message, slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: message})
}

// @Summary Create chat automation
// @ID create-chat-automation
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateChatAutomationRequest true "Create chat automation request"
// @Success 201 {object} codersdk.CreateChatAutomationResponse
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

	// codersdk.CreateChatAutomationRequest has no validate tags, because the
	// reader echoes the value of a field that fails one.
	var req codersdk.CreateChatAutomationRequest
	if !httpapi.ReadLimit(ctx, rw, r, api.maxPromptRequestBodyBytes(), &req) {
		return
	}

	result, err := api.chatAutomationCreator.Create(ctx, httpmw.UserAuthorization(ctx), chatautomation.CreateParams{
		OrganizationID: organization.ID,
		Request:        req,
		Now:            dbtime.Now(),
	})
	var invalid *chatautomation.InvalidRequestError
	var limit *chatautomation.LimitError
	switch {
	case errors.As(err, &invalid):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Validation failed.",
			Validations: invalid.Validations,
		})
		return
	case errors.As(err, &limit):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("You can have at most %d chat automations across all organizations. Delete one to create another.", limit.Max),
		})
		return
	case errors.Is(err, chatautomation.ErrOwnerUnavailable):
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "This user cannot own chat automations.",
		})
		return
	case errors.Is(err, chatautomation.ErrReferenceUnavailable):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "A chat, model configuration or project of this automation was deleted while it was created. Check them and try again.",
		})
		return
	case errors.Is(err, chatautomation.ErrCreateForbidden):
		httpapi.Forbidden(rw)
		return
	case err != nil:
		api.writeChatAutomationError(ctx, rw, "Internal error creating chat automation.", err)
		return
	}
	aReq.New = result.Automation

	// The row is committed and the generated secret exists only in this
	// response, so a failed read of the owner and chats must not fail it.
	automations := []database.ChatAutomation{result.Automation}
	refs, err := api.loadChatAutomationRefs(ctx, automations)
	if err != nil {
		api.Logger.Warn(ctx, "load chat automation references for the create response", slog.Error(err))
		refs = db2sdk.ChatAutomationRefs{}
	}
	rw.Header().Set("Cache-Control", "no-store")
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateChatAutomationResponse{
		Automation:    db2sdk.ChatAutomation(result.Automation, refs, dbtime.Now()),
		WebhookSecret: result.GeneratedSecret,
	})
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

	ownerID, found, err := api.chatAutomationOwnerFilter(ctx, httpmw.APIKey(r).UserID, r.URL.Query().Get("owner"))
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to list chat automations.", err)
		return
	}
	if !found {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.ChatAutomation{})
		return
	}

	rows, err := api.Database.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
		OrganizationID: organization.ID,
		OwnerID:        ownerID,
	})
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to list chat automations.", err)
		return
	}
	automations := slice.List(rows, db2sdk.ChatAutomationFromListRow)
	refs, err := api.loadChatAutomationRefs(ctx, automations)
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to list chat automations.", err)
		return
	}
	now := dbtime.Now()
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(automations, func(a database.ChatAutomation) codersdk.ChatAutomation {
		return db2sdk.ChatAutomation(a, refs, now)
	}))
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
	// Only the canonical form with hyphens is an ID. uuid.Parse also accepts 32
	// hexadecimal characters, which is a valid username, and the braces and
	// urn:uuid: forms.
	if parsed, err := uuid.Parse(owner); len(owner) == len(uuid.Nil.String()) && err == nil {
		// The nil UUID is no user, and must not reach a query that reads it
		// as every owner.
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
	organization := httpmw.OrganizationParam(r)
	id, ok := parseChatAutomationID(rw, r)
	if !ok {
		return
	}

	// This row holds the decrypted webhook secret. The converter never
	// copies it.
	automation, err := api.Database.GetChatAutomationByID(ctx, id)
	if httpapi.Is404Error(err) || (err == nil && automation.OrganizationID != organization.ID) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to get chat automation.", err)
		return
	}
	refs, err := api.loadChatAutomationRefs(ctx, []database.ChatAutomation{automation})
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to get chat automation.", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatAutomation(automation, refs, dbtime.Now()))
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

	deleted, err := chatautomation.Delete(ctx, api.Database, httpmw.UserAuthorization(ctx), organization.ID, id)
	if errors.Is(err, chatautomation.ErrDeleteForbidden) {
		httpapi.Forbidden(rw)
		return
	}
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.writeChatAutomationError(ctx, rw, "Failed to delete chat automation.", err)
		return
	}
	aReq.Old = deleted
	rw.WriteHeader(http.StatusNoContent)
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

// loadChatAutomationRefs reads the owners and chats of the rows. It reads the
// chats in one query. It reads the owners in one query too, and one by one when
// the caller cannot read every owner at once.
func (api *API) loadChatAutomationRefs(ctx context.Context, automations []database.ChatAutomation) (db2sdk.ChatAutomationRefs, error) {
	refs := db2sdk.ChatAutomationRefs{
		Owners: make(map[uuid.UUID]database.User),
		Chats:  make(map[uuid.UUID]database.Chat),
	}
	ownerIDs := make(map[uuid.UUID]struct{})
	chatIDs := make(map[uuid.UUID]struct{})
	for _, a := range automations {
		ownerIDs[a.OwnerID] = struct{}{}
		for _, id := range []uuid.NullUUID{a.TargetChatID, a.CreatedByChatID} {
			if id.Valid {
				chatIDs[id.UUID] = struct{}{}
			}
		}
	}
	if len(ownerIDs) > 0 {
		users, err := api.Database.GetUsersByIDs(ctx, mapKeys(ownerIDs))
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			// The batch read is all or nothing. Read the owners one by one and
			// leave out each one that the caller cannot read.
			for id := range ownerIDs {
				user, err := api.Database.GetUserByID(ctx, id)
				if httpapi.Is404Error(err) {
					continue
				}
				if err != nil {
					return refs, xerrors.Errorf("get owner: %w", err)
				}
				refs.Owners[id] = user
			}
		case err != nil:
			return refs, xerrors.Errorf("get owners: %w", err)
		default:
			for _, u := range users {
				refs.Owners[u.ID] = u
			}
		}
	}
	if len(chatIDs) > 0 {
		chats, err := api.Database.GetReadableChatsByIDs(ctx, mapKeys(chatIDs))
		if err != nil {
			return refs, xerrors.Errorf("get chats: %w", err)
		}
		for _, c := range chats {
			refs.Chats[c.ID] = c
		}
	}
	return refs, nil
}

func mapKeys(set map[uuid.UUID]struct{}) []uuid.UUID {
	keys := make([]uuid.UUID, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	return keys
}
