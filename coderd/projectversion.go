package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
	ImportJobID uuid.UUID `json:"import_job_id"`
}

// ProjectVersionParameter represents a parameter parsed from project version source on creation.
type ProjectVersionParameter struct {
	ID                       uuid.UUID                           `json:"id"`
	CreatedAt                time.Time                           `json:"created_at"`
	Name                     string                              `json:"name"`
	Description              string                              `json:"description,omitempty"`
	DefaultSourceScheme      database.ParameterSourceScheme      `json:"default_source_scheme,omitempty"`
	DefaultSourceValue       string                              `json:"default_source_value,omitempty"`
	AllowOverrideSource      bool                                `json:"allow_override_source"`
	DefaultDestinationScheme database.ParameterDestinationScheme `json:"default_destination_scheme,omitempty"`
	DefaultDestinationValue  string                              `json:"default_destination_value,omitempty"`
	AllowOverrideDestination bool                                `json:"allow_override_destination"`
	DefaultRefresh           string                              `json:"default_refresh"`
	RedisplayValue           bool                                `json:"redisplay_value"`
	ValidationError          string                              `json:"validation_error,omitempty"`
	ValidationCondition      string                              `json:"validation_condition,omitempty"`
	ValidationTypeSystem     database.ParameterTypeSystem        `json:"validation_type_system,omitempty"`
	ValidationValueType      string                              `json:"validation_value_type,omitempty"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	ImportJobID uuid.UUID `json:"import_job_id" validate:"required"`
}

// Lists versions for a single project.
func (api *api) projectVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	version, err := api.Database.GetProjectVersionsByProjectID(r.Context(), project.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
		})
		return
	}
	apiVersion := make([]ProjectVersion, 0)
	for _, version := range version {
		apiVersion = append(apiVersion, convertProjectVersion(version))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiVersion)
}

// Return a single project version by organization and name.
func (*api) projectVersionByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectVersion(projectVersion))
}

// Creates a new version of the project. An import job is queued to parse
// the storage method provided. Once completed, the import job will specify
// the version as latest.
func (api *api) postProjectVersionByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectVersionRequest
	if !httpapi.Read(rw, r, &createProjectVersion) {
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), createProjectVersion.ImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "job not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	project := httpmw.ProjectParam(r)
	projectVersion, err := api.Database.InsertProjectVersion(r.Context(), database.InsertProjectVersionParams{
		ID:          uuid.New(),
		ProjectID:   project.ID,
		CreatedAt:   database.Now(),
		UpdatedAt:   database.Now(),
		Name:        namesgenerator.GetRandomName(1),
		ImportJobID: job.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project version: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectVersion(projectVersion))
}

func (api *api) projectVersionParametersByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.ImportJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	apiJob := convertProvisionerJob(job)
	if !apiJob.Status.Completed() {
		httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
			Message: fmt.Sprintf("import job hasn't completed: %s", apiJob.Status),
		})
		return
	}
	if apiJob.Status != ProvisionerJobStatusSucceeded {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "import job wasn't successful. no parameters were parsed",
		})
		return
	}

	parameters, err := api.Database.GetParameterSchemasByJobID(r.Context(), projectVersion.ImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		parameters = []database.ParameterSchema{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project parameters: %s", err),
		})
		return
	}

	apiParameters := make([]ProjectVersionParameter, 0, len(parameters))
	for _, parameter := range parameters {
		apiParameters = append(apiParameters, convertProjectParameter(parameter))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiParameters)
}

func convertProjectVersion(version database.ProjectVersion) ProjectVersion {
	return ProjectVersion{
		ID:          version.ID,
		ProjectID:   version.ProjectID,
		CreatedAt:   version.CreatedAt,
		UpdatedAt:   version.UpdatedAt,
		Name:        version.Name,
		ImportJobID: version.ImportJobID,
	}
}

func convertProjectParameter(parameter database.ParameterSchema) ProjectVersionParameter {
	return ProjectVersionParameter{
		ID:                       parameter.ID,
		CreatedAt:                parameter.CreatedAt,
		Name:                     parameter.Name,
		Description:              parameter.Description,
		DefaultSourceScheme:      parameter.DefaultSourceScheme,
		DefaultSourceValue:       parameter.DefaultSourceValue.String,
		AllowOverrideSource:      parameter.AllowOverrideSource,
		DefaultDestinationScheme: parameter.DefaultDestinationScheme,
		DefaultDestinationValue:  parameter.DefaultDestinationValue.String,
		AllowOverrideDestination: parameter.AllowOverrideDestination,
		DefaultRefresh:           parameter.DefaultRefresh,
		RedisplayValue:           parameter.RedisplayValue,
		ValidationError:          parameter.ValidationError,
		ValidationCondition:      parameter.ValidationCondition,
		ValidationTypeSystem:     parameter.ValidationTypeSystem,
		ValidationValueType:      parameter.ValidationValueType,
	}
}
