package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// Project is the JSON representation of a Coder project.
// This type matches the database object for now, but is
// abstracted for ease of change later on.
type Project struct {
	ID                  uuid.UUID                `json:"id"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
	OrganizationID      string                   `json:"organization_id"`
	Name                string                   `json:"name"`
	Provisioner         database.ProvisionerType `json:"provisioner"`
	ActiveVersionID     uuid.UUID                `json:"active_version_id"`
	WorkspaceOwnerCount uint32                   `json:"workspace_owner_count"`
}

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

	apiVersion := make([]ProjectVersion, 0)
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

func convertProjects(projects []database.Project, workspaceCounts []database.GetWorkspaceOwnerCountsByProjectIDsRow) []Project {
	apiProjects := make([]Project, 0, len(projects))
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

func convertProject(project database.Project, workspaceOwnerCount uint32) Project {
	return Project{
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
