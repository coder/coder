package coderd

import (
	"archive/tar"
	"bytes"
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

// ProjectHistory is the JSON representation of Coder project version history.
type ProjectHistory struct {
	ID            uuid.UUID                     `json:"id"`
	ProjectID     uuid.UUID                     `json:"project_id"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Name          string                        `json:"name"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method"`
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
		apiHistory = append(apiHistory, convertProjectHistory(version))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
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

	project := httpmw.ProjectParam(r)
	history, err := api.Database.InsertProjectHistory(r.Context(), database.InsertProjectHistoryParams{
		ID:            uuid.New(),
		ProjectID:     project.ID,
		CreatedAt:     database.Now(),
		UpdatedAt:     database.Now(),
		Name:          namesgenerator.GetRandomName(1),
		StorageMethod: createProjectVersion.StorageMethod,
		StorageSource: createProjectVersion.StorageSource,
		// TODO: Make this do something!
		ImportJobID: uuid.New(),
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project history: %s", err),
		})
		return
	}

	// TODO: A job to process the new version should occur here.

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectHistory(history))
}

func convertProjectHistory(history database.ProjectHistory) ProjectHistory {
	return ProjectHistory{
		ID:        history.ID,
		ProjectID: history.ProjectID,
		CreatedAt: history.CreatedAt,
		UpdatedAt: history.UpdatedAt,
		Name:      history.Name,
	}
}
