package codersdk

// Maps workspace transition to display status for Running job status
var runningStatusFromTransition = map[WorkspaceTransition]string{
	WorkspaceTransitionStart:  "Starting",
	WorkspaceTransitionStop:   "Stopping",
	WorkspaceTransitionDelete: "Deleting",
}

// Maps workspace transition to display status for Succeeded job status
var succeededStatusFromTransition = map[WorkspaceTransition]string{
	WorkspaceTransitionStart:  "Started",
	WorkspaceTransitionStop:   "Stopped",
	WorkspaceTransitionDelete: "Deleted",
}

// Maps workspace transition to display status for Failed job status
var failedStatusFromTransition = map[WorkspaceTransition]string{
	WorkspaceTransitionStart:  "Failed to start",
	WorkspaceTransitionStop:   "Failed to stop",
	WorkspaceTransitionDelete: "Failed to delete",
}

const unknownStatus = "Unknown"

// WorkspaceDisplayStatus computes a status to display on CLI/UI based on
// the workspace transition and the status of the provisioner job.
// This code is in sync with how we compute the status on frontend.
// Ref: site/src/util/workspace.ts (getWorkspaceStatus)
func WorkspaceDisplayStatus(jobStatus ProvisionerJobStatus, transition WorkspaceTransition) string {
	switch jobStatus {
	case ProvisionerJobSucceeded:
		status, ok := succeededStatusFromTransition[transition]
		if !ok {
			return unknownStatus
		}
		return status
	case ProvisionerJobRunning:
		status, ok := runningStatusFromTransition[transition]
		if !ok {
			return unknownStatus
		}
		return status
	case ProvisionerJobPending:
		return "Queued"
	case ProvisionerJobCanceling:
		return "Canceling"
	case ProvisionerJobCanceled:
		return "Canceled"
	case ProvisionerJobFailed:
		status, ok := failedStatusFromTransition[transition]
		if !ok {
			return "Failed"
		}
		return status
	default:
		return unknownStatus
	}
}
