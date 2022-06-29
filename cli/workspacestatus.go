package cli

import (
	"github.com/coder/coder/codersdk"
)

var inProgressToStatus = map[codersdk.WorkspaceTransition]string{
	codersdk.WorkspaceTransitionStart:  "Starting",
	codersdk.WorkspaceTransitionStop:   "Stopping",
	codersdk.WorkspaceTransitionDelete: "Deleting",
}
var succeededToStatus = map[codersdk.WorkspaceTransition]string{
	codersdk.WorkspaceTransitionStart:  "Started",
	codersdk.WorkspaceTransitionStop:   "Stopped",
	codersdk.WorkspaceTransitionDelete: "Deleted",
}

// workspaceStatus computes a status to display on CLI based on
// the workspace transition and the status of the provisioner job.
// This code is in sync with how we compute the status on frontend.
// Ref: site/src/util/workspace.ts (getWorkspaceStatus)
func workspaceStatus(jobStatus codersdk.ProvisionerJobStatus, transition codersdk.WorkspaceTransition) string {
	switch jobStatus {
	case codersdk.ProvisionerJobSucceeded:
		return succeededToStatus[transition]
	case codersdk.ProvisionerJobRunning:
		return inProgressToStatus[transition]
	case codersdk.ProvisionerJobPending:
		return "Queued"
	case codersdk.ProvisionerJobCanceling:
		return "Canceling action"
	case codersdk.ProvisionerJobCanceled:
		return "Canceled action"
	case codersdk.ProvisionerJobFailed:
		return "Failed"
	default:
		return "Loading..."
	}
}
