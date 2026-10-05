package coderd

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get chat project instructions
// @ID get-chat-project-instructions
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProjectInstructions
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/instructions [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProjectInstructions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	row, err := api.Database.GetChatProjectInstructionsByProjectID(ctx, project.ID)
	if httpapi.Is404Error(err) {
		// A project without a row has no instructions. Report the unset
		// state rather than a 404 so clients can treat this as a singleton.
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatProjectInstructions{ProjectID: project.ID})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat project instructions.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectInstructions(row))
}

// @Summary Update chat project instructions
// @ID update-chat-project-instructions
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectInstructionsRequest true "Update chat project instructions request"
// @Success 200 {object} codersdk.ChatProjectInstructions
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/instructions [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectInstructions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Cap the raw request body to prevent excessive memory use from
	// payloads padded with invisible characters that sanitize away.
	var req codersdk.UpdateChatProjectInstructionsRequest
	if !httpapi.ReadLimit(ctx, rw, r, api.maxPromptRequestBodyBytes(), &req) {
		return
	}
	instructions := codersdk.SanitizePromptText(req.Instructions)
	if strings.TrimSpace(instructions) == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Instructions must not be blank.",
			Detail:  "Delete the instructions to clear them.",
			Validations: []codersdk.ValidationError{{
				Field:  "instructions",
				Detail: "Instructions must not be blank.",
			}},
		})
		return
	}
	// The same limit applies as for the deployment and user prompts.
	if len(instructions) > api.chatLimits.MaxPromptBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Instructions exceed maximum length.",
			Detail:  fmt.Sprintf("Maximum length is %d bytes, got %d.", api.chatLimits.MaxPromptBytes, len(instructions)),
		})
		return
	}

	// Write and read back in one transaction so a failed read rolls back
	// the write instead of returning an error for a change that was saved.
	var row database.GetChatProjectInstructionsByProjectIDRow
	err := api.Database.InTx(func(tx database.Store) error {
		_, err := tx.UpsertChatProjectInstructions(ctx, database.UpsertChatProjectInstructionsParams{
			ProjectID:      project.ID,
			OrganizationID: project.OrganizationID,
			Instructions:   instructions,
			UpdatedBy:      apiKey.UserID,
		})
		if err != nil {
			return xerrors.Errorf("upsert chat project instructions: %w", err)
		}
		// Read the row back to include the updating user's profile.
		row, err = tx.GetChatProjectInstructionsByProjectID(ctx, project.ID)
		if err != nil {
			return xerrors.Errorf("get chat project instructions: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat project instructions.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProjectInstructions(row))
}

// @Summary Delete chat project instructions
// @ID delete-chat-project-instructions
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/instructions [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectInstructions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// Deleting unset instructions succeeds, so the request is idempotent.
	if err := api.Database.DeleteChatProjectInstructionsByProjectID(ctx, project.ID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat project instructions.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}
