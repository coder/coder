package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

type ProvisionerJobStatus string

// Completed returns whether the job is still processing.
func (p ProvisionerJobStatus) Completed() bool {
	return p == ProvisionerJobStatusSucceeded || p == ProvisionerJobStatusFailed || p == ProvisionerJobStatusCancelled
}

const (
	ProvisionerJobStatusPending   ProvisionerJobStatus = "pending"
	ProvisionerJobStatusRunning   ProvisionerJobStatus = "running"
	ProvisionerJobStatusSucceeded ProvisionerJobStatus = "succeeded"
	ProvisionerJobStatusCancelled ProvisionerJobStatus = "canceled"
	ProvisionerJobStatusFailed    ProvisionerJobStatus = "failed"
)

type ProvisionerJob struct {
	ID          uuid.UUID                `json:"id"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	StartedAt   *time.Time               `json:"started_at,omitempty"`
	CancelledAt *time.Time               `json:"canceled_at,omitempty"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
	Status      ProvisionerJobStatus     `json:"status"`
	Error       string                   `json:"error,omitempty"`
	Provisioner database.ProvisionerType `json:"provisioner"`
	WorkerID    *uuid.UUID               `json:"worker_id,omitempty"`
}

type CreateProjectImportJobRequest struct {
	StorageMethod   database.ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required"`
	StorageSource   string                            `json:"storage_source" validate:"required"`
	Provisioner     database.ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	ParameterValues []CreateParameterValueRequest     `json:"parameter_values"`
}

func (*api) provisionerJobByOrganization(rw http.ResponseWriter, r *http.Request) {
	job := httpmw.ProvisionerJobParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProvisionerJob(job))
}

func (api *api) postProvisionerImportJobByOrganization(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	var req CreateProjectImportJobRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}
	file, err := api.Database.GetFileByHash(r.Context(), req.StorageSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "file not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get file: %s", err),
		})
		return
	}

	input, err := json.Marshal(projectVersionImportJob{
		OrganizationID: organization.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("marshal job: %s", err),
		})
		return
	}

	jobID := uuid.New()
	for _, parameterValue := range req.ParameterValues {
		_, err = api.Database.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
			ID:                uuid.New(),
			Name:              parameterValue.Name,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			Scope:             database.ParameterScopeImportJob,
			ScopeID:           jobID.String(),
			SourceScheme:      parameterValue.SourceScheme,
			SourceValue:       parameterValue.SourceValue,
			DestinationScheme: parameterValue.DestinationScheme,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("insert parameter value: %s", err),
			})
			return
		}
	}

	job, err := api.Database.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: organization.ID,
		InitiatorID:    apiKey.UserID,
		Provisioner:    req.Provisioner,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		StorageSource:  file.Hash,
		Type:           database.ProvisionerJobTypeProjectVersionImport,
		Input:          input,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert provisioner job: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProvisionerJob(job))
}

// Return parsed parameter schemas for a job.
func (api *api) provisionerJobParameterSchemasByID(rw http.ResponseWriter, r *http.Request) {
	job := httpmw.ProvisionerJobParam(r)
	if convertProvisionerJob(job).Status != ProvisionerJobStatusSucceeded {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("Job is in state %q! Must be %q.", convertProvisionerJob(job).Status, ProvisionerJobStatusSucceeded),
		})
		return
	}

	schemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("list parameter schemas: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, schemas)
}

func (api *api) provisionerJobComputedParametersByID(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	job := httpmw.ProvisionerJobParam(r)
	if convertProvisionerJob(job).Status != ProvisionerJobStatusSucceeded {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("Job is in state %q! Must be %q.", convertProvisionerJob(job).Status, ProvisionerJobStatusSucceeded),
		})
		return
	}
	computedParametersForScope(rw, r, api.Database, parameter.ComputeScope{
		ProjectImportJobID: job.ID,
		OrganizationID:     job.OrganizationID,
		UserID:             apiKey.UserID,
	})
}

func convertProvisionerJob(provisionerJob database.ProvisionerJob) ProvisionerJob {
	job := ProvisionerJob{
		ID:          provisionerJob.ID,
		CreatedAt:   provisionerJob.CreatedAt,
		UpdatedAt:   provisionerJob.UpdatedAt,
		Error:       provisionerJob.Error.String,
		Provisioner: provisionerJob.Provisioner,
	}
	// Applying values optional to the struct.
	if provisionerJob.StartedAt.Valid {
		job.StartedAt = &provisionerJob.StartedAt.Time
	}
	if provisionerJob.CancelledAt.Valid {
		job.CancelledAt = &provisionerJob.CancelledAt.Time
	}
	if provisionerJob.CompletedAt.Valid {
		job.CompletedAt = &provisionerJob.CompletedAt.Time
	}
	if provisionerJob.WorkerID.Valid {
		job.WorkerID = &provisionerJob.WorkerID.UUID
	}

	switch {
	case provisionerJob.CancelledAt.Valid:
		job.Status = ProvisionerJobStatusCancelled
	case !provisionerJob.StartedAt.Valid:
		job.Status = ProvisionerJobStatusPending
	case provisionerJob.CompletedAt.Valid:
		job.Status = ProvisionerJobStatusSucceeded
	case database.Now().Sub(provisionerJob.UpdatedAt) > 30*time.Second:
		job.Status = ProvisionerJobStatusFailed
		job.Error = "Worker failed to update job in time."
	default:
		job.Status = ProvisionerJobStatusRunning
	}

	if !provisionerJob.CancelledAt.Valid && job.Error != "" {
		job.Status = ProvisionerJobStatusFailed
	}

	return job
}

func provisionerJobLogsChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}
