package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// skillJSONEscapeExpansion is the maximum expansion for one byte in a JSON string.
	skillJSONEscapeExpansion = 6
	// skillRequestEnvelopeBytes leaves room for the surrounding JSON object.
	skillRequestEnvelopeBytes = 1024
	// maxSkillRequestBytes allows worst-case JSON string escaping for
	// otherwise valid raw skill content.
	maxSkillRequestBytes = skills.MaxPersonalSkillSizeBytes*skillJSONEscapeExpansion + skillRequestEnvelopeBytes

	// These names are raised by trigger functions with USING CONSTRAINT.
	// They are not table CHECK constraints, so dbgen does not emit them in
	// check_constraint.go.
	skillsPerUserLimitConstraint   database.CheckConstraint = "skills_per_user_limit"
	userSkillUserDeletedConstraint database.CheckConstraint = "user_skill_user_deleted"
)

// @Summary Create a user skill
// @ID create-a-user-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.CreateSkillRequest true "Create user skill request"
// @Success 201 {object} codersdk.Skill
// @Router /api/experimental/users/{user}/skills [post]
// @x-apidocgen {"skip": true}
func (api *API) postUserSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Skill](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	skill, ok := createSkill(ctx, rw, r, userSkillCreateErrors, func(parsed skills.ParsedSkill, content string) (database.Skill, error) {
		return api.Database.InsertUserSkill(ctx, database.InsertUserSkillParams{
			ID:          uuid.New(),
			UserID:      user.ID,
			Name:        parsed.Name,
			Description: parsed.Description,
			Content:     content,
		})
	})
	if !ok {
		return
	}
	aReq.New = skill
}

// @Summary List user skills
// @ID list-user-skills
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Success 200 {array} codersdk.SkillMetadata
// @Router /api/experimental/users/{user}/skills [get]
// @x-apidocgen {"skip": true}
func (api *API) getUserSkills(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)

	rows, err := api.Database.ListUserSkillMetadataByUserID(ctx, user.ID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSkillMetadataList(rows))
}

// @Summary Get a user skill by name
// @ID get-a-user-skill-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param skillName path string true "Skill name"
// @Success 200 {object} codersdk.Skill
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

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Skill(skill))
}

// @Summary Update a user skill
// @ID update-a-user-skill
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param skillName path string true "Skill name"
// @Param request body codersdk.UpdateSkillRequest true "Update user skill request"
// @Success 200 {object} codersdk.Skill
// @Router /api/experimental/users/{user}/skills/{skillName} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchUserSkill(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		name              = chi.URLParam(r, "skillName")
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Skill](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	oldSkill, skill, ok := api.updateSkill(ctx, rw, r, name, userSkillUpdateErrors, func(tx database.Store, update skillUpdate) (database.Skill, database.Skill, error) {
		fetched, err := tx.GetUserSkillByUserIDAndName(ctx, database.GetUserSkillByUserIDAndNameParams{
			UserID: user.ID,
			Name:   name,
		})
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("fetch user skill: %w", err)
		}
		updated, err := tx.UpdateUserSkillByUserIDAndName(ctx, database.UpdateUserSkillByUserIDAndNameParams{
			UserID:      user.ID,
			Name:        name,
			Description: update.Description,
			Content:     update.Content,
			Enabled:     update.Enabled,
		})
		if err != nil {
			return database.Skill{}, database.Skill{}, xerrors.Errorf("update user skill: %w", err)
		}
		return fetched, updated, nil
	})
	if !ok {
		return
	}
	aReq.Old = oldSkill
	aReq.New = skill
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
		aReq, commitAudit = audit.InitRequest[database.Skill](rw, &audit.RequestParams{
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

var (
	userSkillCreateErrors = skillWriteErrors{
		ownerDeleted: userDeletedSkillResponse("Cannot create skills for deleted users."),
		limit:        skillsPerUserLimitConstraint,
		limitReached: codersdk.Response{
			Message: "Personal skill limit reached.",
			Detail:  fmt.Sprintf("Each user can have at most %d personal skills.", skills.MaxPersonalSkillsPerUser),
		},
		nameIndex: database.UniqueSkillsUserIDNameIndex,
	}
	userSkillUpdateErrors = skillWriteErrors{
		ownerDeleted: userDeletedSkillResponse("Cannot modify skills for deleted users."),
	}
)

func userDeletedSkillResponse(message string) codersdk.Response {
	return codersdk.Response{Message: message, Detail: "This user has been deleted and cannot be modified."}
}

// skillWriteErrors holds the owner-specific responses for errors from a
// skill insert or update. Empty responses leave their error to the generic
// mapping, and an empty forbidden response uses httpapi.Forbidden.
type skillWriteErrors struct {
	forbidden    codersdk.Response
	ownerDeleted codersdk.Response
	limit        database.CheckConstraint
	limitReached codersdk.Response
	nameIndex    database.UniqueConstraint
}

func (e skillWriteErrors) write(ctx context.Context, rw http.ResponseWriter, err error) {
	switch {
	case httpapi.IsUnauthorizedError(err) && e.forbidden.Message == "":
		httpapi.Forbidden(rw)
	case httpapi.IsUnauthorizedError(err):
		httpapi.Write(ctx, rw, http.StatusForbidden, e.forbidden)
	case e.ownerDeleted.Message != "" && database.IsCheckViolation(err, userSkillUserDeletedConstraint):
		httpapi.Write(ctx, rw, http.StatusConflict, e.ownerDeleted)
	case httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
	case e.limitReached.Message != "" && database.IsCheckViolation(err, e.limit):
		httpapi.Write(ctx, rw, http.StatusConflict, e.limitReached)
	case e.nameIndex != "" && database.IsUniqueViolation(err, e.nameIndex):
		writeSkillNameConflict(ctx, rw)
	default:
		httpapi.InternalServerError(rw, err)
	}
}

// createSkill parses a skill create request, stores the skill with insert,
// and writes the response. It returns false when it wrote an error.
func createSkill(
	ctx context.Context,
	rw http.ResponseWriter,
	r *http.Request,
	errs skillWriteErrors,
	insert func(parsed skills.ParsedSkill, content string) (database.Skill, error),
) (database.Skill, bool) {
	var req codersdk.CreateSkillRequest
	if !httpapi.ReadLimit(ctx, rw, r, maxSkillRequestBytes, &req) {
		return database.Skill{}, false
	}
	parsed, err := skills.ParsePersonalSkillMarkdown([]byte(req.Content))
	if err != nil {
		writeInvalidSkillContent(ctx, rw, err)
		return database.Skill{}, false
	}
	skill, err := insert(parsed, req.Content)
	if err != nil {
		errs.write(ctx, rw, err)
		return database.Skill{}, false
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.Skill(skill))
	return skill, true
}

// updateSkill applies a skill PATCH request for the skill named name and
// writes the response. apply runs in a transaction and returns the rows
// before and after the update. Callers assign audit state only after it
// returns true, so the audit log never claims a rolled-back update.
func (api *API) updateSkill(
	ctx context.Context,
	rw http.ResponseWriter,
	r *http.Request,
	name string,
	errs skillWriteErrors,
	apply func(tx database.Store, update skillUpdate) (old database.Skill, updated database.Skill, err error),
) (old database.Skill, updated database.Skill, ok bool) {
	update, ok := readSkillUpdate(ctx, rw, r, name)
	if !ok {
		return database.Skill{}, database.Skill{}, false
	}
	err := api.Database.InTx(func(tx database.Store) error {
		var err error
		old, updated, err = apply(tx, update)
		return err
	}, nil)
	if err != nil {
		errs.write(ctx, rw, err)
		return database.Skill{}, database.Skill{}, false
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Skill(updated))
	return old, updated, true
}

// skillUpdate holds the optional columns a skill PATCH writes.
type skillUpdate struct {
	Description sql.NullString
	Content     sql.NullString
	Enabled     sql.NullBool
}

// readSkillUpdate reads and validates a PATCH request for the skill named
// name. It writes the error response and returns false when the request is
// invalid.
func readSkillUpdate(ctx context.Context, rw http.ResponseWriter, r *http.Request, name string) (skillUpdate, bool) {
	var req codersdk.UpdateSkillRequest
	if !httpapi.ReadLimit(ctx, rw, r, maxSkillRequestBytes, &req) {
		return skillUpdate{}, false
	}
	if req.Content == nil && req.Enabled == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No skill fields to update.",
			Detail:  "Set content, enabled, or both.",
		})
		return skillUpdate{}, false
	}

	var update skillUpdate
	if req.Enabled != nil {
		update.Enabled = sql.NullBool{Bool: *req.Enabled, Valid: true}
	}
	if req.Content == nil {
		return update, true
	}
	parsed, err := skills.ParsePersonalSkillMarkdown([]byte(*req.Content))
	if err != nil {
		writeInvalidSkillContent(ctx, rw, err)
		return skillUpdate{}, false
	}
	if parsed.Name != name {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Skill name in path does not match frontmatter name.",
			Detail:  fmt.Sprintf("path has %q, frontmatter has %q", name, parsed.Name),
		})
		return skillUpdate{}, false
	}
	update.Description = sql.NullString{String: parsed.Description, Valid: true}
	update.Content = sql.NullString{String: *req.Content, Valid: true}
	return update, true
}

func writeSkillNameConflict(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
		Message: "A skill with that name already exists.",
		Detail:  "Choose a different name, or edit the existing skill.",
	})
}

func writeInvalidSkillContent(ctx context.Context, rw http.ResponseWriter, err error) {
	message := "Invalid skill content."
	switch {
	case errors.Is(err, skills.ErrInvalidSkillName):
		message = "Invalid skill name."
	case errors.Is(err, skills.ErrSkillBodyRequired):
		message = "Skill body is required."
	case errors.Is(err, skills.ErrSkillTooLarge):
		message = "Skill content is too large."
	case errors.Is(err, skills.ErrSkillDescriptionTooLarge):
		message = "Skill description is too large."
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: message,
		Detail:  err.Error(),
	})
}
