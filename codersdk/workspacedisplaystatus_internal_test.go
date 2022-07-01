package codersdk

import "testing"

func TestWorkspaceDisplayStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		jobStatus  ProvisionerJobStatus
		transition WorkspaceTransition
		want       string
	}{
		{
			name:       "SucceededStatusWithStartTransition",
			jobStatus:  ProvisionerJobSucceeded,
			transition: WorkspaceTransitionStart,
			want:       "Started",
		},
		{
			name:       "SucceededStatusWithStopTransition",
			jobStatus:  ProvisionerJobSucceeded,
			transition: WorkspaceTransitionStop,
			want:       "Stopped",
		},
		{
			name:       "SucceededStatusWithDeleteTransition",
			jobStatus:  ProvisionerJobSucceeded,
			transition: WorkspaceTransitionDelete,
			want:       "Deleted",
		},
		{
			name:       "RunningStatusWithStartTransition",
			jobStatus:  ProvisionerJobRunning,
			transition: WorkspaceTransitionStart,
			want:       "Starting",
		},
		{
			name:       "RunningStatusWithStopTransition",
			jobStatus:  ProvisionerJobRunning,
			transition: WorkspaceTransitionStop,
			want:       "Stopping",
		},
		{
			name:       "RunningStatusWithDeleteTransition",
			jobStatus:  ProvisionerJobRunning,
			transition: WorkspaceTransitionDelete,
			want:       "Deleting",
		},
		{
			name:       "PendingStatusWithStartTransition",
			jobStatus:  ProvisionerJobPending,
			transition: WorkspaceTransitionStart,
			want:       "Queued",
		},
		{
			name:       "CancelingStatusWithStartTransition",
			jobStatus:  ProvisionerJobCanceling,
			transition: WorkspaceTransitionStart,
			want:       "Canceling action",
		},
		{
			name:       "CanceledStatusWithStartTransition",
			jobStatus:  ProvisionerJobCanceled,
			transition: WorkspaceTransitionStart,
			want:       "Canceled action",
		},
		{
			name:       "FailedStatusWithDeleteTransition",
			jobStatus:  ProvisionerJobFailed,
			transition: WorkspaceTransitionDelete,
			want:       "Failed",
		},
		{
			name:       "EmptyStatusWithDeleteTransition",
			jobStatus:  "",
			transition: WorkspaceTransitionDelete,
			want:       unknownStatus,
		},
		{
			name:       "RunningStatusWithEmptyTransition",
			jobStatus:  ProvisionerJobRunning,
			transition: "",
			want:       unknownStatus,
		},
		{
			name:       "SucceededStatusWithEmptyTransition",
			jobStatus:  ProvisionerJobSucceeded,
			transition: "",
			want:       unknownStatus,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WorkspaceDisplayStatus(tt.jobStatus, tt.transition); got != tt.want {
				t.Errorf("workspaceStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
