package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects
// @ID list-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	projects, err := api.Database.GetChatProjectsByOwnerID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(projects, db2sdk.ChatProject))
}

// @Summary Create chat project
// @ID create-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateChatProjectRequest true "Create chat project request"
// @Success 201 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	var req codersdk.CreateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Icon = strings.TrimSpace(req.Icon)
	if req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name is required.")
		return
	}
	if !validateChatProjectFields(ctx, rw, req.Name, req.Description, req.Icon) {
		return
	}
	if !api.Authorize(r, policy.ActionCreate, database.ChatProject{
		OrganizationID: organization.ID,
		OwnerID:        apiKey.UserID,
	}.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}
	templates, ok := api.chatProjectTemplateFieldsOrWriteError(ctx, rw, organization.ID, req.Templates)
	if !ok {
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var project database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock serializes a user's creates so concurrent requests cannot
		// both observe room under the cap.
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-projects-owner:"+apiKey.UserID.String())); err != nil {
			return xerrors.Errorf("lock chat projects: %w", err)
		}
		// The count spans the user's projects in every organization, and the
		// caller was already authorized to create one.
		//nolint:gocritic // See above.
		count, err := tx.CountChatProjectsByOwnerID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
		if err != nil {
			return xerrors.Errorf("count chat projects: %w", err)
		}
		if count >= maxChatProjectsPerOwner {
			return errChatProjectLimit
		}
		project, err = tx.InsertChatProject(ctx, database.InsertChatProjectParams{
			ID:             uuid.NullUUID{},
			OrganizationID: organization.ID,
			OwnerID:        apiKey.UserID,
			Name:           req.Name,
			Description:    req.Description,
			Icon:           req.Icon,

			TemplateIDs:                    templates.templateIDs,
			DefaultTemplateID:              templates.defaultTemplateID,
			DefaultTemplateVersionPresetID: templates.defaultPresetID,
			DefaultParameters:              pqtype.NullRawMessage{RawMessage: templates.defaultParameters, Valid: true},
		})
		return err
	}, nil)
	if errors.Is(err, errChatProjectLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("You can have at most %d chat projects. Delete a project to create another.", maxChatProjectsPerOwner),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = project
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.ChatProject(project))
}

// @Summary Get chat project
// @ID get-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProject(project))
}

// @Summary Update chat project
// @ID update-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectRequest true "Update chat project request"
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	// Only the owner can update a project today, including its templates.
	// Project editor roles will extend this check.
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	var req codersdk.UpdateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
	}
	if req.Icon != nil {
		*req.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Name != nil && *req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name must not be blank.")
		return
	}
	name := ptr.NilToDefault(req.Name, project.Name)
	description := ptr.NilToDefault(req.Description, project.Description)
	icon := ptr.NilToDefault(req.Icon, project.Icon)
	if !validateChatProjectFields(ctx, rw, name, description, icon) {
		return
	}
	templates := storedChatProjectTemplateFields(project)
	if req.Templates != nil {
		var ok bool
		templates, ok = api.chatProjectTemplateFieldsOrWriteError(ctx, rw, project.OrganizationID, req.Templates)
		if !ok {
			return
		}
	}

	updated, err := api.Database.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:          project.ID,
		Name:        name,
		Description: description,
		Icon:        icon,

		TemplateIDs:                    templates.templateIDs,
		DefaultTemplateID:              templates.defaultTemplateID,
		DefaultTemplateVersionPresetID: templates.defaultPresetID,
		DefaultParameters:              templates.defaultParameters,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.ChatProject(updated))
}

// @Summary Delete chat project
// @ID delete-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionDelete, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	err := api.Database.DeleteChatProjectByID(ctx, project.ID)
	if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat project.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// maxChatProjectsPerOwner caps how many projects one user owns across all
// organizations, which bounds the project list every agents page loads.
const maxChatProjectsPerOwner = 100

var errChatProjectLimit = xerrors.New("chat project limit reached")

const (
	// The project name is embedded in every generation's system prompt and
	// memory tool descriptions, so it is kept short.
	chatProjectNameMaxChars        = 64
	chatProjectDescriptionMaxChars = 1024
	chatProjectIconMaxChars        = 256
)

// validateChatProjectFields writes a 400 response and returns false when a
// sanitized field exceeds its limit.
func validateChatProjectFields(ctx context.Context, rw http.ResponseWriter, name, description, icon string) bool {
	if utf8.RuneCountInString(name) > chatProjectNameMaxChars {
		writeChatProjectFieldError(ctx, rw, "name", fmt.Sprintf("Name must be at most %d characters.", chatProjectNameMaxChars))
		return false
	}
	if utf8.RuneCountInString(description) > chatProjectDescriptionMaxChars {
		writeChatProjectFieldError(ctx, rw, "description", fmt.Sprintf("Description must be at most %d characters.", chatProjectDescriptionMaxChars))
		return false
	}
	if utf8.RuneCountInString(icon) > chatProjectIconMaxChars {
		writeChatProjectFieldError(ctx, rw, "icon", fmt.Sprintf("Icon must be at most %d characters.", chatProjectIconMaxChars))
		return false
	}
	return true
}

// chatProjectTemplateFieldsOrWriteError validates requested templates and
// writes the error response when they are invalid or cannot be checked.
func (api *API) chatProjectTemplateFieldsOrWriteError(
	ctx context.Context,
	rw http.ResponseWriter,
	organizationID uuid.UUID,
	req *codersdk.ChatProjectTemplates,
) (chatProjectTemplateFields, bool) {
	fields, validation, err := api.validateChatProjectTemplates(ctx, organizationID, req)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to validate chat project templates.",
			Detail:  err.Error(),
		})
		return chatProjectTemplateFields{}, false
	}
	if validation != nil {
		writeChatProjectFieldError(ctx, rw, validation.Field, validation.Detail)
		return chatProjectTemplateFields{}, false
	}
	return fields, true
}

func writeChatProjectFieldError(ctx context.Context, rw http.ResponseWriter, field, detail string) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     detail,
		Validations: []codersdk.ValidationError{{Field: field, Detail: detail}},
	})
}

// chatProjectTemplateFields are the validated database values for a
// project's templates and defaults.
type chatProjectTemplateFields struct {
	templateIDs       []uuid.UUID
	defaultTemplateID uuid.NullUUID
	defaultPresetID   uuid.NullUUID
	defaultParameters json.RawMessage
}

// storedChatProjectTemplateFields returns a project's current template
// fields so an update that omits templates keeps them.
func storedChatProjectTemplateFields(project database.ChatProject) chatProjectTemplateFields {
	return chatProjectTemplateFields{
		templateIDs:       project.TemplateIDs,
		defaultTemplateID: project.DefaultTemplateID,
		defaultPresetID:   project.DefaultTemplateVersionPresetID,
		defaultParameters: project.DefaultParameters,
	}
}

// validateChatProjectTemplates checks a project's requested templates and
// defaults. Templates are looked up as the requester, so templates they
// cannot read are reported as unavailable. A non-nil validation error means
// the request is invalid; a non-nil error means the lookup failed.
func (api *API) validateChatProjectTemplates(
	ctx context.Context,
	organizationID uuid.UUID,
	req *codersdk.ChatProjectTemplates,
) (chatProjectTemplateFields, *codersdk.ValidationError, error) {
	fields := chatProjectTemplateFields{templateIDs: []uuid.UUID{}, defaultParameters: json.RawMessage("{}")}
	if req == nil {
		return fields, nil, nil
	}
	invalid := func(field, detail string) (chatProjectTemplateFields, *codersdk.ValidationError, error) {
		return chatProjectTemplateFields{}, &codersdk.ValidationError{Field: "templates." + field, Detail: detail}, nil
	}

	fields.templateIDs = slice.Unique(req.TemplateIDs)
	activeVersions := make(map[uuid.UUID]uuid.UUID, len(fields.templateIDs))
	if len(fields.templateIDs) > 0 {
		templates, err := api.Database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
			OrganizationID: organizationID,
			IDs:            fields.templateIDs,
			Deprecated:     sql.NullBool{Bool: false, Valid: true},
			AgentsAllowed:  sql.NullBool{Bool: true, Valid: true},
		})
		if err != nil {
			return chatProjectTemplateFields{}, nil, xerrors.Errorf("get templates: %w", err)
		}
		for _, template := range templates {
			activeVersions[template.ID] = template.ActiveVersionID
		}
	}
	for _, id := range fields.templateIDs {
		if _, ok := activeVersions[id]; !ok {
			return invalid("template_ids", fmt.Sprintf("Template %s is not available to agents in this organization.", id))
		}
	}

	if req.DefaultTemplateID == nil {
		if req.DefaultPresetID != nil || len(req.DefaultParameters) > 0 {
			return invalid("default_template_id", "Default preset and parameters require a default template.")
		}
		return fields, nil, nil
	}
	activeVersionID, ok := activeVersions[*req.DefaultTemplateID]
	if !ok {
		return invalid("default_template_id", "The default template must be one of the project's templates.")
	}
	field, detail, err := api.validateChatProjectDefaults(ctx, activeVersionID, req)
	if err != nil {
		return chatProjectTemplateFields{}, nil, err
	}
	if field != "" {
		return invalid(field, detail)
	}

	fields.defaultTemplateID = uuid.NullUUID{UUID: *req.DefaultTemplateID, Valid: true}
	if req.DefaultPresetID != nil {
		fields.defaultPresetID = uuid.NullUUID{UUID: *req.DefaultPresetID, Valid: true}
	}
	if len(req.DefaultParameters) > 0 {
		raw, err := json.Marshal(req.DefaultParameters)
		if err != nil {
			return chatProjectTemplateFields{}, nil, xerrors.Errorf("marshal default parameters: %w", err)
		}
		fields.defaultParameters = raw
	}
	return fields, nil, nil
}

// validateChatProjectDefaults checks the default preset and parameters
// against the default template's active version. It returns the invalid
// field and its detail, or an empty field when the defaults are valid.
func (api *API) validateChatProjectDefaults(
	ctx context.Context,
	activeVersionID uuid.UUID,
	req *codersdk.ChatProjectTemplates,
) (field string, detail string, err error) {
	if req.DefaultPresetID != nil && len(req.DefaultParameters) > 0 {
		return "default_preset_id", "Choose a default preset or default parameters, not both.", nil
	}
	if req.DefaultPresetID != nil {
		presets, err := api.Database.GetPresetsByTemplateVersionID(ctx, activeVersionID)
		if err != nil {
			return "", "", xerrors.Errorf("get presets: %w", err)
		}
		if !slices.ContainsFunc(presets, func(p database.TemplateVersionPreset) bool { return p.ID == *req.DefaultPresetID }) {
			return "default_preset_id", "The default preset must belong to the default template's active version.", nil
		}
	}
	if len(req.DefaultParameters) > 0 {
		parameters, err := api.Database.GetTemplateVersionParameters(ctx, activeVersionID)
		if err != nil {
			return "", "", xerrors.Errorf("get template version parameters: %w", err)
		}
		for _, name := range slices.Sorted(maps.Keys(req.DefaultParameters)) {
			if !slices.ContainsFunc(parameters, func(p database.TemplateVersionParameter) bool { return p.Name == name }) {
				return "default_parameters", fmt.Sprintf("Parameter %q does not exist on the default template's active version.", name), nil
			}
		}
	}
	return "", "", nil
}
