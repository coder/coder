package conversion

import (
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func ProvisionerJobStatus(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJobStatus {
	switch {
	case provisionerJob.CanceledAt.Valid:
		if !provisionerJob.CompletedAt.Valid {
			return codersdk.ProvisionerJobCanceling
		}
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobCanceled
		}
		return codersdk.ProvisionerJobFailed
	case !provisionerJob.StartedAt.Valid:
		return codersdk.ProvisionerJobPending
	case provisionerJob.CompletedAt.Valid:
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobSucceeded
		}
		return codersdk.ProvisionerJobFailed
	case database.Now().Sub(provisionerJob.UpdatedAt) > 30*time.Second:
		provisionerJob.Error.String = "Worker failed to update job in time."
		return codersdk.ProvisionerJobFailed
	default:
		return codersdk.ProvisionerJobRunning
	}
}
