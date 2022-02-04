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

// ProjectHistory is the JSON representation of Coder project version history.
type ProjectHistory struct {
	ID            uuid.UUID                     `json:"id"`
	ProjectID     uuid.UUID                     `json:"project_id"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Name          string                        `json:"name"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method"`
	Import        ProvisionerJob                `json:"import"`
}

// CreateProjectHistoryRequest enables callers to create a new Project Version.
type CreateProjectHistoryRequest struct {
	StorageMethod database.ProjectStorageMethod `json:"storage_method" validate:"oneof=inline-archive,required"`
	StorageSource []byte                        `json:"storage_source" validate:"max=1048576,required"`
}

// Lists history for a single project.
func (api *api) projectHistoryByOrganization(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	history, err := api.Database.GetProjectHistoryByProjectID(r.Context(), project.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project history: %s", err),
		})
		return
	}
	apiHistory := make([]ProjectHistory, 0)
	for _, version := range history {
		job, err := api.Database.GetProvisionerJobByID(r.Context(), version.ImportJobID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job: %s", err),
			})
			return
		}
		apiHistory = append(apiHistory, convertProjectHistory(version, job))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
}

// Return a single project history by organization and name.
func (api *api) projectHistoryByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	projectHistory := httpmw.ProjectHistoryParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectHistory.ImportJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectHistory(projectHistory, job))
}

// Creates a new version of the project. An import job is queued to parse
// the storage method provided. Once completed, the import job will specify
// the version as latest.
func (api *api) postProjectHistoryByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectHistoryRequest
	if !httpapi.Read(rw, r, &createProjectVersion) {
		return
	}

	switch createProjectVersion.StorageMethod {
	case database.ProjectStorageMethodInlineArchive:
		tarReader := tar.NewReader(bytes.NewReader(createProjectVersion.StorageSource))
		_, err := tarReader.Next()
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: "the archive must be a tar",
			})
			return
		}
	default:
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("unsupported storage method %s", createProjectVersion.StorageMethod),
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	project := httpmw.ProjectParam(r)

	var provisionerJob database.ProvisionerJob
	var projectHistory database.ProjectHistory
	err := api.Database.InTx(func(db database.Store) error {
		projectHistoryID := uuid.New()
		input, err := json.Marshal(projectImportJob{
			ProjectHistoryID: projectHistoryID,
		})
		if err != nil {
			return xerrors.Errorf("marshal import job: %w", err)
		}

		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
			InitiatorID: apiKey.UserID,
			Provisioner: project.Provisioner,
			Type:        database.ProvisionerJobTypeProjectImport,
			ProjectID:   project.ID,
			Input:       input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		projectHistory, err = api.Database.InsertProjectHistory(r.Context(), database.InsertProjectHistoryParams{
			ID:            projectHistoryID,
			ProjectID:     project.ID,
			CreatedAt:     database.Now(),
			UpdatedAt:     database.Now(),
			Name:          namesgenerator.GetRandomName(1),
			StorageMethod: createProjectVersion.StorageMethod,
			StorageSource: createProjectVersion.StorageSource,
			ImportJobID:   provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert project history: %s", err)
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
	render.JSON(rw, r, convertProjectHistory(projectHistory, provisionerJob))
}

func convertProjectHistory(history database.ProjectHistory, job database.ProvisionerJob) ProjectHistory {
	return ProjectHistory{
		ID:        history.ID,
		ProjectID: history.ProjectID,
		CreatedAt: history.CreatedAt,
		UpdatedAt: history.UpdatedAt,
		Name:      history.Name,
		Import:    convertProvisionerJob(job),
	}
}

func projectHistoryLogsChannel(projectHistoryID uuid.UUID) string {
	return fmt.Sprintf("project-history-logs:%s", projectHistoryID)
}
