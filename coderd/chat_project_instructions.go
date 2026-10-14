package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
	// Update is checked before read, unlike authorizeChatProjectChange, so
	// update-only roles can still write.
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		// Readers learn nothing new from a 403.
		if api.Authorize(r, policy.ActionRead, project.RBACObject()) {
			httpapi.Forbidden(rw)
		} else {
			httpapi.ResourceNotFound(rw)
		}
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

	aReq, commitAudit := audit.InitRequestWithCancel[database.AuditableChatProjectInstructions](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit(true)
	aReq.Old = database.ChatProjectInstruction{ProjectID: project.ID, OrganizationID: project.OrganizationID}.Auditable(project.Name)
	aReq.New = aReq.Old

	// Write and read back in one transaction so a failed read rolls back
	// the write instead of returning an error for a change that was saved.
	// The audit diff is applied only after the transaction commits, so a
	// rolled-back write is audited without one.
	var (
		row               database.GetChatProjectInstructionsByProjectIDRow
		previous, updated database.ChatProjectInstruction
	)
	err := api.Database.InTx(func(tx database.Store) error {
		previous = aReq.Old.ChatProjectInstruction
		// The per-project lock serializes reading the previous value for
		// the audit diff with the write.
		if err := tx.AcquireLock(ctx, chatProjectInstructionsLockID(project.ID)); err != nil {
			return xerrors.Errorf("acquire chat project instructions lock: %w", err)
		}
		// The ActionUpdate check above gates this endpoint, so the reads
		// below run under a system context: a role that grants update
		// without read can still save.
		//nolint:gocritic // See above.
		old, err := tx.GetChatProjectInstructionsByProjectID(dbauthz.AsSystemRestricted(ctx), project.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get previous chat project instructions: %w", err)
		}
		if err == nil {
			previous = old.ChatProjectInstruction
		}
		updated, err = tx.UpsertChatProjectInstructions(ctx, database.UpsertChatProjectInstructionsParams{
			ProjectID:    project.ID,
			Instructions: instructions,
			UpdatedBy:    apiKey.UserID,
		})
		if err != nil {
			return xerrors.Errorf("upsert chat project instructions: %w", err)
		}
		// Read the row back to include the updating user's profile.
		//nolint:gocritic // See above.
		row, err = tx.GetChatProjectInstructionsByProjectID(dbauthz.AsSystemRestricted(ctx), project.ID)
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
	// Saving unchanged instructions is not audited.
	if updated.Instructions == previous.Instructions {
		commitAudit(false)
	}
	aReq.Old = previous.Auditable(project.Name)
	aReq.New = updated.Auditable(project.Name)
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
	// Update is checked before read, unlike authorizeChatProjectChange, so
	// update-only roles can still write.
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		// Readers learn nothing new from a 403.
		if api.Authorize(r, policy.ActionRead, project.RBACObject()) {
			httpapi.Forbidden(rw)
		} else {
			httpapi.ResourceNotFound(rw)
		}
		return
	}
	aReq, commitAudit := audit.InitRequestWithCancel[database.AuditableChatProjectInstructions](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit(true)
	aReq.Old = database.ChatProjectInstruction{ProjectID: project.ID, OrganizationID: project.OrganizationID}.Auditable(project.Name)

	// Deleting unset instructions succeeds, so the request is idempotent.
	// The audit diff is applied only after the transaction commits, so a
	// rolled-back delete is audited without one.
	var (
		deleted  bool
		previous database.ChatProjectInstruction
	)
	err := api.Database.InTx(func(tx database.Store) error {
		deleted = false
		// The per-project lock serializes reading the deleted value for the
		// audit diff with the delete.
		if err := tx.AcquireLock(ctx, chatProjectInstructionsLockID(project.ID)); err != nil {
			return xerrors.Errorf("acquire chat project instructions lock: %w", err)
		}
		// The ActionUpdate check above gates this endpoint, so read the
		// value being deleted under a system context: a role that grants
		// update without read can still delete.
		//nolint:gocritic // See above.
		old, err := tx.GetChatProjectInstructionsByProjectID(dbauthz.AsSystemRestricted(ctx), project.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("get chat project instructions: %w", err)
		}
		previous = old.ChatProjectInstruction
		if err := tx.DeleteChatProjectInstructionsByProjectID(ctx, project.ID); err != nil {
			return xerrors.Errorf("delete chat project instructions: %w", err)
		}
		deleted = true
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat project instructions.",
			Detail:  err.Error(),
		})
		return
	}
	// Deleting unset instructions changes nothing, so it is not audited.
	if !deleted {
		commitAudit(false)
	}
	aReq.Old = previous.Auditable(project.Name)
	rw.WriteHeader(http.StatusNoContent)
}

// chatProjectInstructionsLockID returns the advisory lock ID that serializes
// writes to one project's instructions.
func chatProjectInstructionsLockID(projectID uuid.UUID) int64 {
	return database.GenLockID("chat-project-instructions:" + projectID.String())
}
