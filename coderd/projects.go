package coderd

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

// Project is the JSON representation of a Coder project.
// This type matches the database object for now, but is
// abstracted for ease of change later on.
type Project database.Project

// ProjectHistory is the JSON representation of Coder project version history.
type ProjectHistory struct {
	ID            uuid.UUID                     `json:"id"`
	ProjectID     uuid.UUID                     `json:"project_id"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Name          string                        `json:"name"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method"`
}

type ProjectHistoryLog struct {
	ID        uuid.UUID
	CreatedAt time.Time          `json:"created_at"`
	Source    database.LogSource `json:"log_source"`
	Level     database.LogLevel  `json:"log_level"`
	Output    string             `json:"output"`
}

// CreateProjectRequest enables callers to create a new Project.
type CreateProjectRequest struct {
	Name        string                   `json:"name" validate:"username,required"`
	Provisioner database.ProvisionerType `json:"provisioner" validate:"oneof=terraform cdr-basic,required"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	StorageMethod database.ProjectStorageMethod `json:"storage_method" validate:"oneof=inline-archive,required"`
	StorageSource []byte                        `json:"storage_source" validate:"max=1048576,required"`
}

type projects struct {
	Database database.Store
	Pubsub   database.Pubsub
}

// Lists all projects the authenticated user has access to.
func (p *projects) allProjects(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organizations, err := p.Database.GetOrganizationsByUserID(r.Context(), apiKey.UserID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organizations: %s", err.Error()),
		})
		return
	}
	organizationIDs := make([]string, 0, len(organizations))
	for _, organization := range organizations {
		organizationIDs = append(organizationIDs, organization.ID)
	}
	projects, err := p.Database.GetProjectsByOrganizationIDs(r.Context(), organizationIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get projects: %s", err.Error()),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, projects)
}

// Lists all projects in an organization.
func (p *projects) allProjectsForOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	projects, err := p.Database.GetProjectsByOrganizationIDs(r.Context(), []string{organization.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get projects: %s", err.Error()),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, projects)
}

// Creates a new project in an organization.
func (p *projects) createProject(rw http.ResponseWriter, r *http.Request) {
	var createProject CreateProjectRequest
	if !httpapi.Read(rw, r, &createProject) {
		return
	}
	organization := httpmw.OrganizationParam(r)
	_, err := p.Database.GetProjectByOrganizationAndName(r.Context(), database.GetProjectByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createProject.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("project %q already exists", createProject.Name),
			Errors: []httpapi.Error{{
				Field: "name",
				Code:  "exists",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project by name: %s", err.Error()),
		})
		return
	}

	project, err := p.Database.InsertProject(r.Context(), database.InsertProjectParams{
		ID:             uuid.New(),
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: organization.ID,
		Name:           createProject.Name,
		Provisioner:    createProject.Provisioner,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project: %s", err),
		})
		return
	}
	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, project)
}

// Returns a single project.
func (*projects) project(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, project)
}

// Lists history for a single project.
func (p *projects) allProjectHistory(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	history, err := p.Database.GetProjectHistoryByProjectID(r.Context(), project.ID)
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
func (p *projects) createProjectHistory(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectVersionRequest
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
	history, err := p.Database.InsertProjectHistory(r.Context(), database.InsertProjectHistoryParams{
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

func (p *projects) projectHistoryLogs(rw http.ResponseWriter, r *http.Request) {
	projectHistory := httpmw.ProjectHistoryParam(r)
	follow := r.URL.Query().Has("follow")

	if !follow {
		// If we're not attempting to follow logs,
		// we can exit immediately!
		logs, err := p.Database.GetProjectHistoryLogsByIDBefore(r.Context(), database.GetProjectHistoryLogsByIDBeforeParams{
			ProjectHistoryID: projectHistory.ID,
			CreatedAt:        time.Now(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get project history logs: %s", err),
			})
			return
		}
		render.Status(r, http.StatusOK)
		render.JSON(rw, r, logs)
		return
	}

	// We only want to fetch messages before subscribe, so that
	// there aren't any duplicates.
	timeBeforeSubscribe := database.Now()
	// Start subscribing immediately, otherwise we could miss messages
	// that occur during the database read.
	newLogNotify := make(chan ProjectHistoryLog, 128)
	cancelNewLogNotify, err := p.Pubsub.Subscribe(projectHistoryLogsChannel(projectHistory.ID), func(ctx context.Context, message []byte) {
		var logs []database.ProjectHistoryLog
		err := json.Unmarshal(message, &logs)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("parse logs from publish: %s", err),
			})
			return
		}
		for _, log := range logs {
			// If many logs are sent during our database query, this channel
			// could overflow. The Go scheduler would decide the order to send
			// logs in at that point, which is an unfortunate (but not fatal)
			// flaw of this approach.
			//
			// This is an extremely unlikely outcome given reasonable database
			// query times.
			newLogNotify <- convertProjectHistoryLog(log)
		}
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("listen for new logs: %s", err),
		})
		return
	}
	defer cancelNewLogNotify()

	// In-between here logs could be missed!
	projectHistoryLogs, err := p.Database.GetProjectHistoryLogsByIDBefore(r.Context(), database.GetProjectHistoryLogsByIDBeforeParams{
		ProjectHistoryID: projectHistory.ID,
		CreatedAt:        timeBeforeSubscribe,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project history logs: %s", err),
		})
		return
	}

	// "follow" uses the ndjson format to stream data.
	// See: https://canjs.com/doc/can-ndjson-stream.html
	rw.Header().Set("Content-Type", "application/stream+json")
	rw.WriteHeader(http.StatusOK)
	rw.(http.Flusher).Flush()

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(rw)
	for _, projectHistoryLog := range projectHistoryLogs {
		// JSON separated by a newline
		err = encoder.Encode(convertProjectHistoryLog(projectHistoryLog))
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("marshal: %s", err),
			})
			return
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case log := <-newLogNotify:
			err = encoder.Encode(log)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("marshal follow: %s", err),
				})
				return
			}
		}
	}
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

func convertProjectHistoryLog(log database.ProjectHistoryLog) ProjectHistoryLog {
	return ProjectHistoryLog{
		ID:        log.ID,
		CreatedAt: log.CreatedAt,
		Source:    log.Source,
		Level:     log.Level,
		Output:    log.Output,
	}
}

func projectHistoryLogsChannel(projectHistoryID uuid.UUID) string {
	return fmt.Sprintf("project-history-logs:%s", projectHistoryID)
}
