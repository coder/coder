package coderd

import (
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat project summaries
// @ID list-chat-project-summaries
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProjectSummary
// @Router /api/experimental/chats/projects/summaries [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectSummaries(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// Summaries carry the caller's pins, which are personal data. dbauthz
	// refuses the read for an API key scoped without user:read_personal,
	// so answer 403 here instead of failing the query.
	if !api.Authorize(r, policy.ActionReadPersonal, rbac.ResourceUserObject(apiKey.UserID)) {
		httpapi.Forbidden(rw)
		return
	}

	rows, err := api.Database.GetChatProjectSummariesByUserID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat project summaries.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, db2sdk.ChatProjectSummary))
}

// @Summary List chat project member counts
// @ID list-chat-project-member-counts
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProjectMemberCount
// @Router /api/experimental/chats/projects/member-counts [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectMemberCounts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := api.Database.GetChatProjectMemberCountsByUserID(ctx, database.GetChatProjectMemberCountsByUserIDParams{
		UserID:             httpmw.APIKey(r).UserID,
		CountSharedMembers: api.chatProjectSharingAvailable(),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat project member counts.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, db2sdk.ChatProjectMemberCount))
}

// @Summary Pin chat project
// @ID pin-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
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

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		return tx.PinChatProject(ctx, database.PinChatProjectParams{
			UserID:    apiKey.UserID,
			ProjectID: project.ID,
			Source:    database.ChatProjectPinSourceUser,
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

// @Summary Reorder pinned chat projects
// @ID reorder-chat-project-pins
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param request body codersdk.ReorderChatProjectPinsRequest true "Reorder chat project pins request"
// @Success 204
// @Router /api/experimental/chats/projects/pins/order [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectPinOrder(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// The order is the caller's personal data, and the request names no
	// project to authorize. dbauthz refuses the write for an API key scoped
	// without user:update_personal, so answer 403 here instead of failing
	// the transaction.
	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceUserObject(apiKey.UserID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.ReorderChatProjectPinsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatProjectsOwnerLockID(apiKey.UserID)); err != nil {
			return xerrors.Errorf("lock chat project pins: %w", err)
		}
		return tx.ReorderChatProjectPins(ctx, database.ReorderChatProjectPinsParams{
			UserID:     apiKey.UserID,
			ProjectIDs: req.ProjectIDs,
		})
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to reorder chat project pins.",
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

	// Deleting one row cannot collide with another pin write, so unpinning
	// takes no lock.
	err := api.Database.UnpinChatProject(ctx, database.UnpinChatProjectParams{
		UserID:    apiKey.UserID,
		ProjectID: project.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unpin chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// chatProjectSharingAvailable reports whether member counts include the
// users a project is shared with. It matches the gate on the project ACL
// routes. Which projects are listed does not depend on it: as in GET
// /chats/projects, grant keys select candidates and RBAC decides.
func (api *API) chatProjectSharingAvailable() bool {
	return !api.chatSharingDisabled() && (buildinfo.IsDev() || api.Experiments.Enabled(codersdk.ExperimentChatProjectSharing))
}

// authorizeChatProjectPin writes a 404 and returns false unless the caller
// owns the project or holds a grant on it that RBAC honors.
// Summaries list only those projects, so role-based read access alone would
// leave the pin invisible.
func (api *API) authorizeChatProjectPin(rw http.ResponseWriter, r *http.Request, project database.ChatProject) bool {
	ctx := r.Context()
	userID := httpmw.APIKey(r).UserID
	// A pin is a personal preference, so the project only needs to be
	// readable.
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
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
	// The pin itself is the caller's personal data, which an API key scoped
	// without user:update_personal may not change. dbauthz refuses that
	// write, so answer 403 here instead of failing the transaction.
	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceUserObject(userID)) {
		httpapi.Forbidden(rw)
		return false
	}
	return true
}

// chatProjectsOwnerLockID serializes one user's project creates, pins and
// reorders. A new pin reads the last position and a reorder rewrites every
// position, so overlapping writes could otherwise share a position or
// deadlock.
func chatProjectsOwnerLockID(userID uuid.UUID) int64 {
	return database.GenLockID("chat-projects-owner:" + userID.String())
}
