package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

// maxPersonalSkillRequestBytes allows worst-case JSON string escaping for
// otherwise valid raw skill content.
const maxPersonalSkillRequestBytes = skills.MaxPersonalSkillSizeBytes*6 + 1024

// @Summary Create a user skill
// @ID create-a-user-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.CreateUserSkillRequest true "Create user skill request"
// @Success 201 {object} codersdk.UserSkill
// @Router /api/experimental/users/{user}/skills [post]
// @x-apidocgen {"skip": true}
func (api *API) postUserSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSkill](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	r.Body = http.MaxBytesReader(rw, r.Body, maxPersonalSkillRequestBytes)

	var req codersdk.CreateUserSkillRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	parsedSkill, err := skills.ValidatePersonalSkillMarkdown([]byte(req.Content))
	if err != nil {
		writeInvalidUserSkillContent(ctx, rw, err)
		return
	}

	params := database.InsertUserSkillParams{
		UserID:      user.ID,
		Name:        parsedSkill.Name,
		Description: parsedSkill.Description,
		Content:     req.Content,
	}
	skill, err := api.Database.InsertUserSkill(ctx, params)
	if err != nil {
		if database.IsCheckViolation(err, "user_skills_per_user_limit") {
			writeUserSkillLimitReached(ctx, rw)
			return
		}
		if database.IsUniqueViolation(err, database.UniqueUserSkillsUserIDNameIndex) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A skill with that name already exists.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = skill

	httpapi.Write(ctx, rw, http.StatusCreated, convertUserSkill(skill))
}

// @Summary List user skills
// @ID list-user-skills
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Success 200 {array} codersdk.UserSkillMetadata
// @Router /api/experimental/users/{user}/skills [get]
// @x-apidocgen {"skip": true}
func (api *API) getUserSkills(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)

	rows, err := api.Database.ListUserSkillMetadataByUserID(ctx, user.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertUserSkillMetadataList(rows))
}

// @Summary Get a user skill by name
// @ID get-a-user-skill-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param skillName path string true "Skill name"
// @Success 200 {object} codersdk.UserSkill
// @Router /api/experimental/users/{user}/skills/{skillName} [get]
// @x-apidocgen {"skip": true}
func (api *API) getUserSkill(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)
	name := chi.URLParam(r, "skillName")

	skill, err := api.Database.GetUserSkillByUserIDAndName(ctx, database.GetUserSkillByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertUserSkill(skill))
}

// @Summary Update a user skill
// @ID update-a-user-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param skillName path string true "Skill name"
// @Param request body codersdk.UpdateUserSkillRequest true "Update user skill request"
// @Success 200 {object} codersdk.UserSkill
// @Router /api/experimental/users/{user}/skills/{skillName} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchUserSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		name              = chi.URLParam(r, "skillName")
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSkill](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	r.Body = http.MaxBytesReader(rw, r.Body, maxPersonalSkillRequestBytes)

	var req codersdk.UpdateUserSkillRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	parsedSkill, err := skills.ValidatePersonalSkillMarkdown([]byte(req.Content))
	if err != nil {
		writeInvalidUserSkillContent(ctx, rw, err)
		return
	}
	if parsedSkill.Name != name {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "skill name in path does not match frontmatter name",
			Detail:  fmt.Sprintf("path has %q, frontmatter has %q", name, parsedSkill.Name),
		})
		return
	}

	params := database.UpdateUserSkillByUserIDAndNameParams{
		UserID:      user.ID,
		Name:        name,
		Description: parsedSkill.Description,
		Content:     req.Content,
	}

	var (
		skill    database.UserSkill
		oldSkill database.UserSkill
	)
	err = api.Database.InTx(func(tx database.Store) error {
		fetched, err := tx.GetUserSkillByUserIDAndName(ctx, database.GetUserSkillByUserIDAndNameParams{
			UserID: user.ID,
			Name:   name,
		})
		if err != nil {
			return xerrors.Errorf("fetch user skill: %w", err)
		}

		updated, err := tx.UpdateUserSkillByUserIDAndName(ctx, params)
		if err != nil {
			return xerrors.Errorf("update user skill: %w", err)
		}
		oldSkill = fetched
		skill = updated
		return nil
	}, nil)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	// Assign audit state after InTx returns so the audit log can never
	// claim a rolled-back update was committed.
	aReq.Old = oldSkill
	aReq.New = skill

	httpapi.Write(ctx, rw, http.StatusOK, convertUserSkill(skill))
}

// @Summary Delete a user skill
// @ID delete-a-user-skill
// @Security CoderSessionToken
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param skillName path string true "Skill name"
// @Success 204
// @Router /api/experimental/users/{user}/skills/{skillName} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteUserSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		name              = chi.URLParam(r, "skillName")
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSkill](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	deleted, err := api.Database.DeleteUserSkillByUserIDAndName(ctx, database.DeleteUserSkillByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = deleted

	rw.WriteHeader(http.StatusNoContent)
}

func writeUserSkillLimitReached(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "Personal skill limit reached.",
		Detail: fmt.Sprintf(
			"Each user can have at most %d personal skills.",
			skills.MaxPersonalSkillsPerUser,
		),
	})
}

func writeInvalidUserSkillContent(ctx context.Context, rw http.ResponseWriter, err error) {
	message := "Invalid skill content."
	switch {
	case xerrors.Is(err, skills.ErrInvalidSkillName):
		message = "Invalid skill name."
	case xerrors.Is(err, skills.ErrSkillBodyRequired):
		message = "Skill body is required."
	case xerrors.Is(err, skills.ErrSkillTooLarge):
		message = "Skill content is too large."
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: message,
		Detail:  err.Error(),
	})
}

func convertUserSkill(skill database.UserSkill) codersdk.UserSkill {
	return codersdk.UserSkill{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Content:     skill.Content,
		CreatedAt:   skill.CreatedAt,
		UpdatedAt:   skill.UpdatedAt,
	}
}

func convertUserSkillMetadata(skill database.ListUserSkillMetadataByUserIDRow) codersdk.UserSkillMetadata {
	return codersdk.UserSkillMetadata{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		CreatedAt:   skill.CreatedAt,
		UpdatedAt:   skill.UpdatedAt,
	}
}

func convertUserSkillMetadataList(rows []database.ListUserSkillMetadataByUserIDRow) []codersdk.UserSkillMetadata {
	metadata := make([]codersdk.UserSkillMetadata, 0, len(rows))
	for _, row := range rows {
		metadata = append(metadata, convertUserSkillMetadata(row))
	}
	return metadata
}
