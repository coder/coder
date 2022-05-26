package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// Returns a single template.
func (api *API) template(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, template) {
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count))
}

func (api *API) deleteTemplate(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(rw, r, rbac.ActionDelete, template) {
		return
	}

	workspaces, err := api.Database.GetWorkspacesByTemplateID(r.Context(), database.GetWorkspacesByTemplateIDParams{
		TemplateID: template.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces by template id: %s", err),
		})
		return
	}
	if len(workspaces) > 0 {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "All workspaces must be deleted before a template can be removed.",
		})
		return
	}
	err = api.Database.UpdateTemplateDeletedByID(r.Context(), database.UpdateTemplateDeletedByIDParams{
		ID:      template.ID,
		Deleted: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update template deleted by id: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Template has been deleted!",
	})
}

// Create a new template in an organization.
func (api *API) postTemplateByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createTemplate codersdk.CreateTemplateRequest
	organization := httpmw.OrganizationParam(r)
	if !api.Authorize(rw, r, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		return
	}

	if !httpapi.Read(rw, r, &createTemplate) {
		return
	}
	_, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createTemplate.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("template %q already exists", createTemplate.Name),
			Errors: []httpapi.Error{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template by name: %s", err),
		})
		return
	}
	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), createTemplate.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "template version does not exist",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version by id: %s", err),
		})
		return
	}
	importJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get import job by id: %s", err),
		})
		return
	}

	var template codersdk.Template
	err = api.Database.InTx(func(db database.Store) error {
		now := database.Now()
		dbTemplate, err := db.InsertTemplate(r.Context(), database.InsertTemplateParams{
			ID:              uuid.New(),
			CreatedAt:       now,
			UpdatedAt:       now,
			OrganizationID:  organization.ID,
			Name:            createTemplate.Name,
			Provisioner:     importJob.Provisioner,
			ActiveVersionID: templateVersion.ID,
			Description:     createTemplate.Description,
		})
		if err != nil {
			return xerrors.Errorf("insert template: %s", err)
		}

		err = db.UpdateTemplateVersionByID(r.Context(), database.UpdateTemplateVersionByIDParams{
			ID: templateVersion.ID,
			TemplateID: uuid.NullUUID{
				UUID:  dbTemplate.ID,
				Valid: true,
			},
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %s", err)
		}

		for _, parameterValue := range createTemplate.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeTemplate,
				ScopeID:           dbTemplate.ID,
				SourceScheme:      database.ParameterSourceScheme(parameterValue.SourceScheme),
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: database.ParameterDestinationScheme(parameterValue.DestinationScheme),
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		template = convertTemplate(dbTemplate, 0)
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, template)
}

func (api *API) templatesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templates, err := api.Database.GetTemplatesByOrganization(r.Context(), database.GetTemplatesByOrganizationParams{
		OrganizationID: organization.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get templates: %s", err.Error()),
		})
		return
	}

	// Filter templates based on rbac permissions
	templates = AuthorizeFilter(api, r, rbac.ActionRead, templates)

	templateIDs := make([]uuid.UUID, 0, len(templates))

	for _, template := range templates {
		templateIDs = append(templateIDs, template.ID)
	}
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), templateIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplates(templates, workspaceCounts))
}

func (api *API) templateByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")
	template, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: fmt.Sprintf("no template found by name %q in the %q organization", templateName, organization.Name),
			})
			return
		}

		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template by organization and name: %s", err),
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, template) {
		return
	}

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count))
}

func convertTemplates(templates []database.Template, workspaceCounts []database.GetWorkspaceOwnerCountsByTemplateIDsRow) []codersdk.Template {
	apiTemplates := make([]codersdk.Template, 0, len(templates))
	for _, template := range templates {
		found := false
		for _, workspaceCount := range workspaceCounts {
			if workspaceCount.TemplateID.String() != template.ID.String() {
				continue
			}
			apiTemplates = append(apiTemplates, convertTemplate(template, uint32(workspaceCount.Count)))
			found = true
			break
		}
		if !found {
			apiTemplates = append(apiTemplates, convertTemplate(template, uint32(0)))
		}
	}
	return apiTemplates
}

func convertTemplate(template database.Template, workspaceOwnerCount uint32) codersdk.Template {
	return codersdk.Template{
		ID:                  template.ID,
		CreatedAt:           template.CreatedAt,
		UpdatedAt:           template.UpdatedAt,
		OrganizationID:      template.OrganizationID,
		Name:                template.Name,
		Provisioner:         codersdk.ProvisionerType(template.Provisioner),
		ActiveVersionID:     template.ActiveVersionID,
		WorkspaceOwnerCount: workspaceOwnerCount,
		Description:         template.Description,
	}
}
