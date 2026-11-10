package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Pin chat project
// @ID pin-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.PinChatProjectRequest true "Pin chat project request"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/pin [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectPin(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectPin(rw, r, project) {
		return
	}

	// The body is optional because pin_order is. Decoding reports io.EOF
	// only for an empty body, which a chunked request cannot announce in
	// its Content-Length.
	var req codersdk.PinChatProjectRequest
	err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, httpapi.DefaultMaxRequestBodyBytes)).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}
	var pinOrder sql.NullInt32
	if req.PinOrder != nil {
		if *req.PinOrder < 1 {
			writeChatProjectFieldError(ctx, rw, "pin_order", "Pin order must be at least 1.")
			return
		}
		pinOrder = sql.NullInt32{Int32: *req.PinOrder, Valid: true}
	}

	err = api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		return tx.PinChatProject(ctx, database.PinChatProjectParams{
			UserID:     apiKey.UserID,
			ProjectID:  project.ID,
			ACLEnabled: !api.chatSharingDisabled(),
			PinOrder:   pinOrder,
		})
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to pin chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Unpin chat project
// @ID unpin-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/pin [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectPin(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectPin(rw, r, project) {
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		return tx.UnpinChatProject(ctx, database.UnpinChatProjectParams{
			UserID:     apiKey.UserID,
			ProjectID:  project.ID,
			ACLEnabled: !api.chatSharingDisabled(),
		})
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unpin chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// authorizeChatProjectPin writes a 404 and returns false unless the caller
// owns the project or holds a grant on it that RBAC honors.
// The project list shows only those projects, so role-based read access
// alone would leave the pin invisible.
func (api *API) authorizeChatProjectPin(rw http.ResponseWriter, r *http.Request, project database.ChatProject) bool {
	ctx := r.Context()
	userID := httpmw.APIKey(r).UserID
	// A pin is a personal preference, so reading the project is enough.
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) ||
		(project.OwnerID != userID && api.chatSharingDisabled()) {
		httpapi.ResourceNotFound(rw)
		return false
	}
	usable, err := chatd.ChatProjectUsableBy(ctx, api.Database, project, userID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error checking chat project access.",
			Detail:  err.Error(),
		})
		return false
	}
	if !usable {
		httpapi.ResourceNotFound(rw)
		return false
	}
	return true
}

// chatProjectsOwnerLockID serializes one user's project creates and pin
// writes. Pin writes renumber every pin of the user, so overlapping writes
// would otherwise deadlock or write positions onto rows another write just
// unpinned.
func chatProjectsOwnerLockID(userID uuid.UUID) int64 {
	return database.GenLockID("chat-projects-owner:" + userID.String())
}
