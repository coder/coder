package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// Returns a single project.
func (api *api) project(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByProjectIDs(r.Context(), []uuid.UUID{project.ID})
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

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProject(project, count))
}

func (api *api) deleteProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	workspaces, err := api.Database.GetWorkspacesByProjectID(r.Context(), database.GetWorkspacesByProjectIDParams{
		ProjectID: project.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces by project id: %s", err),
		})
		return
	}
	if len(workspaces) > 0 {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "All workspaces must be deleted before a project can be removed.",
		})
		return
	}
	err = api.Database.UpdateProjectDeletedByID(r.Context(), database.UpdateProjectDeletedByIDParams{
		ID:      project.ID,
		Deleted: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update project deleted by id: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Project has been deleted!",
	})
}

func (api *api) projectVersionsByProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	versions, err := api.Database.GetProjectVersionsByProjectID(r.Context(), project.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
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

	apiVersion := make([]codersdk.ProjectVersion, 0)
	for _, version := range versions {
		job, exists := jobByID[version.JobID.String()]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("job %q doesn't exist for version %q", version.JobID, version.ID),
			})
			return
		}
		apiVersion = append(apiVersion, convertProjectVersion(version, convertProvisionerJob(job)))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiVersion)
}

func (api *api) projectVersionByName(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)
	projectVersionName := chi.URLParam(r, "projectversionname")
	projectVersion, err := api.Database.GetProjectVersionByProjectIDAndName(r.Context(), database.GetProjectVersionByProjectIDAndNameParams{
		ProjectID: uuid.NullUUID{
			UUID:  project.ID,
			Valid: true,
		},
		Name: projectVersionName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no project version found by name %q", projectVersionName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version by name: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectVersion(projectVersion, convertProvisionerJob(job)))
}

func (api *api) patchActiveProjectVersion(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.UpdateActiveProjectVersion
	if !httpapi.Read(rw, r, &req) {
		return
	}
	project := httpmw.ProjectParam(r)
	version, err := api.Database.GetProjectVersionByID(r.Context(), req.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "project version not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
		})
		return
	}
	if version.ProjectID.UUID.String() != project.ID.String() {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "The provided project version doesn't belong to the specified project.",
		})
		return
	}
	err = api.Database.UpdateProjectActiveVersionByID(r.Context(), database.UpdateProjectActiveVersionByIDParams{
		ID:              project.ID,
		ActiveVersionID: req.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update active project version: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Updated the active project version!",
	})
}

func convertProjects(projects []database.Project, workspaceCounts []database.GetWorkspaceOwnerCountsByProjectIDsRow) []codersdk.Project {
	apiProjects := make([]codersdk.Project, 0, len(projects))
	for _, project := range projects {
		found := false
		for _, workspaceCount := range workspaceCounts {
			if workspaceCount.ProjectID.String() != project.ID.String() {
				continue
			}
			apiProjects = append(apiProjects, convertProject(project, uint32(workspaceCount.Count)))
			found = true
			break
		}
		if !found {
			apiProjects = append(apiProjects, convertProject(project, uint32(0)))
		}
	}
	return apiProjects
}

func convertProject(project database.Project, workspaceOwnerCount uint32) codersdk.Project {
	return codersdk.Project{
		ID:                  project.ID,
		CreatedAt:           project.CreatedAt,
		UpdatedAt:           project.UpdatedAt,
		OrganizationID:      project.OrganizationID,
		Name:                project.Name,
		Provisioner:         project.Provisioner,
		ActiveVersionID:     project.ActiveVersionID,
		WorkspaceOwnerCount: workspaceOwnerCount,
	}
}
