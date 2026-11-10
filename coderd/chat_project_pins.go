package coderd

import (
	"database/sql"
	"net/http"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
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

	// Sharing is available when the project ACL routes are, under the same
	// experiment gate.
	sharingEnabled := !api.chatSharingDisabled() && (buildinfo.IsDev() || api.Experiments.Enabled(codersdk.ExperimentChatProjectSharing))
	rows, err := api.Database.GetChatProjectSummariesByUserID(ctx, database.GetChatProjectSummariesByUserIDParams{
		UserID:         apiKey.UserID,
		SharingEnabled: sharingEnabled,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat project summaries.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, func(row database.GetChatProjectSummariesByUserIDRow) codersdk.ChatProjectSummary {
		return db2sdk.ChatProjectSummary(row, api.convertChatProject(r, row.ChatProject))
	}))
}

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
	// A pin is a personal preference, so reading the project is enough.
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.PinChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
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

	err := api.Database.PinChatProject(ctx, database.PinChatProjectParams{
		UserID:    apiKey.UserID,
		ProjectID: project.ID,
		PinOrder:  pinOrder,
	})
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
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

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
