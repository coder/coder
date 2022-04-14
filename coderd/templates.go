package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// Returns a single template.
func (api *api) template(rw http.ResponseWriter, r *http.Request) {
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
	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count))
}

func (api *api) deleteTemplate(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)

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

func (api *api) templateVersionsByTemplate(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)

	versions, err := api.Database.GetTemplateVersionsByTemplateID(r.Context(), template.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	jobIDs := make([]uuid.UUID, 0, len(versions))
	for _, version := range versions {
		jobIDs = append(jobIDs, version.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDs(r.Context(), jobIDs)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get jobs: %s", err),
		})
		return
	}
	jobByID := map[string]database.ProvisionerJob{}
	for _, job := range jobs {
		jobByID[job.ID.String()] = job
	}

	apiVersion := make([]codersdk.TemplateVersion, 0)
	for _, version := range versions {
		job, exists := jobByID[version.JobID.String()]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("job %q doesn't exist for version %q", version.JobID, version.ID),
			})
			return
		}
		apiVersion = append(apiVersion, convertTemplateVersion(version, convertProvisionerJob(job)))
	}

	httpapi.Write(rw, http.StatusOK, apiVersion)
}

func (api *api) templateVersionByName(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	templateVersionName := chi.URLParam(r, "templateversionname")
	templateVersion, err := api.Database.GetTemplateVersionByTemplateIDAndName(r.Context(), database.GetTemplateVersionByTemplateIDAndNameParams{
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		Name: templateVersionName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no template version found by name %q", templateVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version by name: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *api) patchActiveTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.UpdateActiveTemplateVersion
	if !httpapi.Read(rw, r, &req) {
		return
	}
	template := httpmw.TemplateParam(r)
	version, err := api.Database.GetTemplateVersionByID(r.Context(), req.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "template version not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	if version.TemplateID.UUID.String() != template.ID.String() {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "The provided template version doesn't belong to the specified template.",
		})
		return
	}
	err = api.Database.UpdateTemplateActiveVersionByID(r.Context(), database.UpdateTemplateActiveVersionByIDParams{
		ID:              template.ID,
		ActiveVersionID: req.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update active template version: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Updated the active template version!",
	})
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
		Provisioner:         template.Provisioner,
		ActiveVersionID:     template.ActiveVersionID,
		WorkspaceOwnerCount: workspaceOwnerCount,
	}
}
