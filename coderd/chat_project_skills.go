package coderd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

// chatProjectSkillCreateErrors maps the cap trigger's constraint, which dbgen
// does not emit, and the name index to conflicts. The trigger applies the
// personal skill limit to every owner kind.
var chatProjectSkillCreateErrors = skillWriteErrors{
	limit: "skills_per_project_limit",
	limitReached: codersdk.Response{
		Message: "Project skill limit reached.",
		Detail:  fmt.Sprintf("Each project can have at most %d skills.", skills.MaxPersonalSkillsPerUser),
	},
	nameIndex: database.UniqueSkillsProjectIDNameIndex,
}

// @Summary List chat project skills
// @ID list-chat-project-skills
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {array} codersdk.SkillMetadata
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/skills [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectSkills(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	rows, err := api.Database.ListProjectSkillMetadataByProjectID(ctx, project.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ProjectSkillMetadataList(rows))
}

// @Summary Create chat project skill
// @ID create-chat-project-skill
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.CreateSkillRequest true "Create chat project skill request"
// @Success 201 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/skills [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectSkill(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	aReq, commitAudit, ok := api.startChatProjectSkillWrite(rw, r, project, database.AuditActionCreate)
	if !ok {
		return
	}
	defer commitAudit()

	skill, ok := createSkill(ctx, rw, r, chatProjectSkillCreateErrors, skillOwnerProject, func(parsed skills.ParsedSkill, create skillCreate) (database.Skill, error) {
		return api.Database.InsertProjectSkill(ctx, database.InsertProjectSkillParams{
			ID:             uuid.New(),
			ProjectID:      project.ID,
			Name:           parsed.Name,
			Description:    parsed.Description,
			Content:        create.Content,
			Action:         create.Action,
			ModelInvocable: create.ModelInvocable,
		})
	})
	if !ok {
		return
	}
	aReq.New = database.AuditableChatProjectSkill{Skill: skill}
}

// @Summary Get chat project skill by name
// @ID get-chat-project-skill-by-name
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param skillName path string true "Skill name"
// @Success 200 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/skills/{skillName} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProjectSkill(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	skill, err := api.Database.GetProjectSkillByProjectIDAndName(ctx, database.GetProjectSkillByProjectIDAndNameParams{
		ProjectID: project.ID,
		Name:      chi.URLParam(r, "skillName"),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Skill(skill))
}

// @Summary Update chat project skill
// @ID update-chat-project-skill
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param skillName path string true "Skill name"
// @Param request body codersdk.UpdateSkillRequest true "Update chat project skill request"
// @Success 200 {object} codersdk.Skill
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/skills/{skillName} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProjectSkill(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	name := chi.URLParam(r, "skillName")
	aReq, commitAudit, ok := api.startChatProjectSkillWrite(rw, r, project, database.AuditActionWrite)
	if !ok {
		return
	}
	defer commitAudit()

	oldSkill, skill, ok := api.updateSkill(ctx, rw, r, name, skillWriteErrors{}, skillOwnerProject, func(tx database.Store, update skillUpdate) (database.Skill, database.Skill, error) {
		// Update on the project was checked above and the update below
		// rechecks it, so reads use a system context for update-only roles.
		//nolint:gocritic // See above.
		found, err := tx.GetProjectSkillByProjectIDAndName(dbauthz.AsSystemRestricted(ctx), database.GetProjectSkillByProjectIDAndNameParams{
			ProjectID: project.ID,
			Name:      name,
		})
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("fetch project skill: %w", err)
		}
		// Lock the row so the audit diff starts from the replaced value.
		//nolint:gocritic // See above.
		locked, err := tx.GetProjectSkillByIDForUpdate(dbauthz.AsSystemRestricted(ctx), found.ID)
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("lock project skill: %w", err)
		}
		updated, err := tx.UpdateProjectSkillByProjectIDAndName(ctx, database.UpdateProjectSkillByProjectIDAndNameParams{
			Description:    update.Description,
			Content:        update.Content,
			Enabled:        update.Enabled,
			Action:         update.Action,
			ModelInvocable: update.ModelInvocable,
			ProjectID:      project.ID,
			Name:           locked.Name,
		})
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("update project skill: %w", err)
		}
		return locked, updated, nil
	})
	if !ok {
		return
	}
	aReq.Old = database.AuditableChatProjectSkill{Skill: oldSkill}
	aReq.New = database.AuditableChatProjectSkill{Skill: skill}
}

// @Summary Delete chat project skill
// @ID delete-chat-project-skill
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param skillName path string true "Skill name"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/skills/{skillName} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectSkill(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	aReq, commitAudit, ok := api.startChatProjectSkillWrite(rw, r, project, database.AuditActionDelete)
	if !ok {
		return
	}
	defer commitAudit()

	deleted, err := api.Database.DeleteProjectSkillByProjectIDAndName(ctx, database.DeleteProjectSkillByProjectIDAndNameParams{
		ProjectID: project.ID,
		Name:      chi.URLParam(r, "skillName"),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = database.AuditableChatProjectSkill{Skill: deleted}
	rw.WriteHeader(http.StatusNoContent)
}

// startChatProjectSkillWrite checks update on the project before the request
// body is read, then starts the write's audit entry. Callers who cannot update
// the project get a 404, matching the project instructions routes.
func (api *API) startChatProjectSkillWrite(rw http.ResponseWriter, r *http.Request, project database.ChatProject, action database.AuditAction) (*audit.Request[database.AuditableChatProjectSkill], func(), bool) {
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return nil, nil, false
	}
	aReq, commitAudit := audit.InitRequest[database.AuditableChatProjectSkill](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         action,
		OrganizationID: project.OrganizationID,
	})
	return aReq, commitAudit, true
}
