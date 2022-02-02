package coderd

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/database"
)

type ProvisionerJobStatus string

// Completed returns whether the job is still processing.
func (p ProvisionerJobStatus) Completed() bool {
	return p == ProvisionerJobStatusSucceeded || p == ProvisionerJobStatusFailed
}

const (
	ProvisionerJobStatusPending   ProvisionerJobStatus = "pending"
	ProvisionerJobStatusRunning   ProvisionerJobStatus = "running"
	ProvisionerJobStatusSucceeded ProvisionerJobStatus = "succeeded"
	ProvisionerJobStatusCancelled ProvisionerJobStatus = "canceled"
	ProvisionerJobStatusFailed    ProvisionerJobStatus = "failed"
)

type ProvisionerJob struct {
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

func convertProvisionerJob(provisionerJob database.ProvisionerJob) ProvisionerJob {
	job := ProvisionerJob{
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

	if job.Error != "" {
		job.Status = ProvisionerJobStatusFailed
	}

	return job
}
