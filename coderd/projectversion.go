package coderd

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID            uuid.UUID                     `json:"id"`
	ProjectID     uuid.UUID                     `json:"project_id"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Name          string                        `json:"name"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method"`
	Import        ProvisionerJob                `json:"import"`
}

// ProjectParameter represents a parameter parsed from project version source on creation.
type ProjectParameter struct {
	ID                       uuid.UUID                           `json:"id"`
	CreatedAt                time.Time                           `json:"created_at"`
	ProjectVersionID         uuid.UUID                           `json:"project_version_id"`
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
	StorageMethod database.ProjectStorageMethod `json:"storage_method" validate:"oneof=inline-archive,required"`
	StorageSource []byte                        `json:"storage_source" validate:"max=1048576,required"`
}

// Lists versions for a single project.
func (api *api) projectVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	version, err := api.Database.GetProjectVersionByProjectID(r.Context(), project.ID)
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
		job, err := api.Database.GetProvisionerJobByID(r.Context(), version.ImportJobID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job: %s", err),
			})
			return
		}
		apiVersion = append(apiVersion, convertProjectVersion(version, job))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiVersion)
}

// Return a single project version by organization and name.
func (api *api) projectVersionByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.ImportJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectVersion(projectVersion, job))
}

// Creates a new version of the project. An import job is queued to parse
// the storage method provided. Once completed, the import job will specify
// the version as latest.
func (api *api) postProjectVersionByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectVersionRequest
	if !httpapi.Read(rw, r, &createProjectVersion) {
		return
	}

	tarReader := tar.NewReader(bytes.NewReader(createProjectVersion.StorageSource))
	_, err := tarReader.Next()
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "the archive must be a tar",
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	project := httpmw.ProjectParam(r)

	var provisionerJob database.ProvisionerJob
	var projectVersion database.ProjectVersion
	err = api.Database.InTx(func(db database.Store) error {
		provisionerJobID := uuid.New()
		projectVersion, err = api.Database.InsertProjectVersion(r.Context(), database.InsertProjectVersionParams{
			ID:            uuid.New(),
			ProjectID:     project.ID,
			CreatedAt:     database.Now(),
			UpdatedAt:     database.Now(),
			Name:          namesgenerator.GetRandomName(1),
			StorageMethod: createProjectVersion.StorageMethod,
			StorageSource: createProjectVersion.StorageSource,
			ImportJobID:   provisionerJobID,
		})
		if err != nil {
			return xerrors.Errorf("insert project version: %s", err)
		}

		input, err := json.Marshal(projectImportJob{
			ProjectVersionID: projectVersion.ID,
		})
		if err != nil {
			return xerrors.Errorf("marshal import job: %w", err)
		}
		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:          provisionerJobID,
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
			InitiatorID: apiKey.UserID,
			Provisioner: project.Provisioner,
			Type:        database.ProvisionerJobTypeProjectImport,
			Input:       input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectVersion(projectVersion, provisionerJob))
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

	parameters, err := api.Database.GetProjectParametersByVersionID(r.Context(), projectVersion.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		parameters = []database.ProjectParameter{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project parameters: %s", err),
		})
		return
	}

	apiParameters := make([]ProjectParameter, 0, len(parameters))
	for _, parameter := range parameters {
		apiParameters = append(apiParameters, convertProjectParameter(parameter))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiParameters)
}

func convertProjectVersion(version database.ProjectVersion, job database.ProvisionerJob) ProjectVersion {
	return ProjectVersion{
		ID:            version.ID,
		ProjectID:     version.ProjectID,
		CreatedAt:     version.CreatedAt,
		UpdatedAt:     version.UpdatedAt,
		Name:          version.Name,
		StorageMethod: version.StorageMethod,
		Import:        convertProvisionerJob(job),
	}
}

func convertProjectParameter(parameter database.ProjectParameter) ProjectParameter {
	return ProjectParameter{
		ID:                       parameter.ID,
		CreatedAt:                parameter.CreatedAt,
		ProjectVersionID:         parameter.ProjectVersionID,
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
