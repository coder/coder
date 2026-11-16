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
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Attach template to chat project
// @ID attach-template-to-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param template path string true "Template ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/templates/{template} [put]
// @x-apidocgen {"skip": true}
func (api *API) putChatProjectTemplate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionUpdate, project.RBACObject()) {
		return
	}
	templateID, ok := httpmw.ParseUUIDParam(rw, r, "template")
	if !ok {
		return
	}
	// A template the caller cannot read gets the same response as one that
	// does not exist, so attaching cannot probe for templates.
	if _, err := api.attachableChatProjectTemplates(ctx, project.OrganizationID, []uuid.UUID{templateID}); err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to attach template.")
		return
	}

	aReq, commitAudit := api.initChatProjectTemplatesAudit(rw, r, project)
	defer commitAudit()
	before, err := api.chatProjectTemplatesAudit(ctx, project)
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to attach template.")
		return
	}

	attached, err := api.Database.InsertChatProjectTemplates(ctx, database.InsertChatProjectTemplatesParams{
		ProjectID:   project.ID,
		TemplateIDs: []uuid.UUID{templateID},
		CreatedBy:   apiKey.UserID,
	})
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to attach template.")
		return
	}
	// An already attached template is left as it was, so nothing is audited.
	if len(attached) > 0 {
		aReq.Old = before
		aReq.New = before
		aReq.New.TemplateIDs = append(slices.Clone(before.TemplateIDs), templateID)
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Detach template from chat project
// @ID detach-template-from-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param template path string true "Template ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/templates/{template} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectTemplate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.authorizeChatProjectChange(rw, r, policy.ActionUpdate, project.RBACObject()) {
		return
	}
	templateID, ok := httpmw.ParseUUIDParam(rw, r, "template")
	if !ok {
		return
	}

	aReq, commitAudit := api.initChatProjectTemplatesAudit(rw, r, project)
	defer commitAudit()
	before, err := api.chatProjectTemplatesAudit(ctx, project)
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to detach template.")
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		// Setting the default template takes the same lock, so the default
		// cannot be pointed at this template while it is being detached.
		if _, err := tx.GetChatProjectByIDForUpdate(ctx, project.ID); err != nil {
			return err
		}
		_, err := tx.DeleteChatProjectTemplate(ctx, database.DeleteChatProjectTemplateParams{
			ProjectID:  project.ID,
			TemplateID: templateID,
		})
		return err
	}, nil)
	// A template the caller cannot read, one that does not exist, and one
	// that is not attached all get the same response.
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.writeChatProjectTemplateError(ctx, rw, err, "Failed to detach template.")
		return
	}
	aReq.Old = before
	aReq.New = before
	aReq.New.TemplateIDs = slices.DeleteFunc(slices.Clone(before.TemplateIDs), func(id uuid.UUID) bool { return id == templateID })
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) initChatProjectTemplatesAudit(rw http.ResponseWriter, r *http.Request, project database.ChatProject) (*audit.Request[database.AuditableChatProjectTemplates], func()) {
	return audit.InitRequest[database.AuditableChatProjectTemplates](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
}

// chatProjectTemplatesAudit returns the project's attached templates for the
// audit log. Audit logs are read by auditors, so the list is not limited to
// the templates the caller can access.
func (api *API) chatProjectTemplatesAudit(ctx context.Context, project database.ChatProject) (database.AuditableChatProjectTemplates, error) {
	rows, err := api.Database.GetChatProjectTemplatesByProjectIDs(ctx, []uuid.UUID{project.ID})
	if err != nil {
		return database.AuditableChatProjectTemplates{}, xerrors.Errorf("get chat project templates: %w", err)
	}
	return database.AuditableChatProjectTemplates{
		ProjectID:   project.ID,
		ProjectName: project.Name,
		TemplateIDs: slice.List(rows, func(row database.GetChatProjectTemplatesByProjectIDsRow) uuid.UUID {
			return row.ChatProjectTemplate.TemplateID
		}),
	}, nil
}

// chatProjectFieldError is a request validation failure on one field. It is
// an error so that validation inside a transaction can roll it back.
type chatProjectFieldError struct {
	field  string
	detail string
}

func (e chatProjectFieldError) Error() string { return e.detail }

// writeChatProjectTemplateError writes the response for a failed change to a
// project's templates or default template.
func (*API) writeChatProjectTemplateError(ctx context.Context, rw http.ResponseWriter, err error, message string) {
	var fieldErr chatProjectFieldError
	switch {
	case errors.As(err, &fieldErr):
		writeChatProjectFieldError(ctx, rw, fieldErr.field, fieldErr.detail)
	// The caller can read the project, so the denial is about a template
	// they can read but not use.
	case dbauthz.IsNotAuthorizedError(err):
		httpapi.Forbidden(rw)
	case httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
	default:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: message,
			Detail:  err.Error(),
		})
	}
}

// attachableChatProjectTemplates looks up the templates to attach to a
// project in organizationID as the requester. It returns a
// chatProjectFieldError when a template is unreadable, in another
// organization, deprecated, or closed to agents. The four cases share one
// message so the response does not reveal which one applies.
func (api *API) attachableChatProjectTemplates(ctx context.Context, organizationID uuid.UUID, templateIDs []uuid.UUID) ([]database.Template, error) {
	// An empty IDs filter matches every template.
	if len(templateIDs) == 0 {
		return nil, nil
	}
	templates, err := api.Database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		OrganizationID: organizationID,
		IDs:            templateIDs,
		Deprecated:     sql.NullBool{Bool: false, Valid: true},
		AgentsAllowed:  sql.NullBool{Bool: true, Valid: true},
	})
	if err != nil {
		return nil, xerrors.Errorf("get templates: %w", err)
	}
	for _, id := range templateIDs {
		if !slices.ContainsFunc(templates, func(t database.Template) bool { return t.ID == id }) {
			return nil, chatProjectFieldError{
				field:  "template_ids",
				detail: fmt.Sprintf("Template %s is not available to agents in this organization.", id),
			}
		}
	}
	return templates, nil
}

// chatProjectDefaultTemplate holds the validated database values of a
// project's default template.
type chatProjectDefaultTemplate struct {
	templateID uuid.NullUUID
	presetID   uuid.NullUUID
	parameters pqtype.NullRawMessage
}

// validateChatProjectDefaultTemplate checks a requested default template
// against attached, the project's templates as the requester sees them, so a
// template the requester cannot access is rejected the same way as one that
// is not attached. The preset and parameters are checked against the
// template's active version. It returns a chatProjectFieldError when the
// request is invalid.
func validateChatProjectDefaultTemplate(
	ctx context.Context,
	db database.Store,
	attached []database.Template,
	req *codersdk.ChatProjectDefaultTemplate,
) (chatProjectDefaultTemplate, error) {
	invalid := func(field, detail string) (chatProjectDefaultTemplate, error) {
		return chatProjectDefaultTemplate{}, chatProjectFieldError{field: "default_template." + field, detail: detail}
	}
	if req == nil {
		return chatProjectDefaultTemplate{}, nil
	}
	if req.TemplateID == uuid.Nil {
		if req.PresetID != nil || len(req.Parameters) > 0 {
			return invalid("template_id", "A default preset or default parameters require a default template.")
		}
		return chatProjectDefaultTemplate{}, nil
	}
	index := slices.IndexFunc(attached, func(t database.Template) bool { return t.ID == req.TemplateID })
	if index < 0 {
		return invalid("template_id", "The default template must be one of the project's templates.")
	}
	template := attached[index]
	fields := chatProjectDefaultTemplate{templateID: uuid.NullUUID{UUID: template.ID, Valid: true}}
	if req.PresetID != nil && len(req.Parameters) > 0 {
		return invalid("preset_id", "Choose a default preset or default parameters, not both.")
	}
	if req.PresetID != nil {
		presets, err := db.GetPresetsByTemplateVersionID(ctx, template.ActiveVersionID)
		if err != nil {
			return chatProjectDefaultTemplate{}, xerrors.Errorf("get presets: %w", err)
		}
		if !slices.ContainsFunc(presets, func(p database.TemplateVersionPreset) bool { return p.ID == *req.PresetID }) {
			return invalid("preset_id", "The default preset must belong to the default template's active version.")
		}
		fields.presetID = uuid.NullUUID{UUID: *req.PresetID, Valid: true}
	}
	if len(req.Parameters) == 0 {
		return fields, nil
	}
	// Dynamic parameters can appear or change based on other values, so the
	// stored parameter rows cannot check their names or values. Only the
	// classic flow is checked here; the build validates dynamic parameters
	// when a workspace is created.
	if template.UseClassicParameterFlow {
		parameters, err := db.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
		if err != nil {
			return chatProjectDefaultTemplate{}, xerrors.Errorf("get template version parameters: %w", err)
		}
		for _, name := range slices.Sorted(maps.Keys(req.Parameters)) {
			index := slices.IndexFunc(parameters, func(p database.TemplateVersionParameter) bool { return p.Name == name })
			if index < 0 {
				return invalid("parameters", fmt.Sprintf("Parameter %q does not exist on the default template's active version.", name))
			}
			parameter, err := db2sdk.TemplateVersionParameter(parameters[index])
			if err != nil {
				return chatProjectDefaultTemplate{}, xerrors.Errorf("convert template version parameter: %w", err)
			}
			value := codersdk.WorkspaceBuildParameter{Name: name, Value: req.Parameters[name]}
			if err := codersdk.ValidateWorkspaceBuildParameter(parameter, &value, nil); err != nil {
				reason := err
				if inner := errors.Unwrap(err); inner != nil {
					reason = inner
				}
				return invalid("parameters", fmt.Sprintf("Parameter %q has an invalid value: %s", name, reason))
			}
		}
	}
	raw, err := json.Marshal(req.Parameters)
	if err != nil {
		return chatProjectDefaultTemplate{}, xerrors.Errorf("marshal default parameters: %w", err)
	}
	fields.parameters = pqtype.NullRawMessage{RawMessage: raw, Valid: true}
	return fields, nil
}

// chatProjectAllowsTemplate reports whether a chat in the project may bind a
// workspace that uses the template. ctx must be authorized as the chat's
// owner, because the limit only admits attached templates the owner can
// access.
func (api *API) chatProjectAllowsTemplate(ctx context.Context, project database.ChatProject, templateID uuid.UUID) (bool, error) {
	if !project.LimitTemplates {
		return true, nil
	}
	templates, err := chattool.LoadProjectTemplates(ctx, api.Database, project)
	if err != nil {
		return false, err
	}
	return templates.Allows(templateID), nil
}
