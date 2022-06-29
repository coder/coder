package cli

import (
	"testing"

	"github.com/coder/coder/codersdk"
)

func Test_workspaceStatus(t *testing.T) {
	t.Parallel()
	type args struct {
		jobStatus  codersdk.ProvisionerJobStatus
		transition codersdk.WorkspaceTransition
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "SucceededStatusWithStartTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobSucceeded,
				transition: codersdk.WorkspaceTransitionStart,
			},
			want: "Started",
		},
		{
			name: "SucceededStatusWithStopTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobSucceeded,
				transition: codersdk.WorkspaceTransitionStop,
			},
			want: "Stopped",
		},
		{
			name: "SucceededStatusWithDeleteTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobSucceeded,
				transition: codersdk.WorkspaceTransitionDelete,
			},
			want: "Deleted",
		},
		{
			name: "RunningStatusWithStartTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobRunning,
				transition: codersdk.WorkspaceTransitionStart,
			},
			want: "Starting",
		},
		{
			name: "RunningStatusWithStopTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobRunning,
				transition: codersdk.WorkspaceTransitionStop,
			},
			want: "Stopping",
		},
		{
			name: "RunningStatusWithDeleteTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobRunning,
				transition: codersdk.WorkspaceTransitionDelete,
			},
			want: "Deleting",
		},
		{
			name: "PendingStatusWithStartTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobPending,
				transition: codersdk.WorkspaceTransitionStart,
			},
			want: "Queued",
		},
		{
			name: "CancelingStatusWithStartTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobCanceling,
				transition: codersdk.WorkspaceTransitionStart,
			},
			want: "Canceling action",
		},
		{
			name: "CanceledStatusWithStartTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobCanceled,
				transition: codersdk.WorkspaceTransitionStart,
			},
			want: "Canceled action",
		},
		{
			name: "FailedStatusWithDeleteTransition",
			args: args{
				jobStatus:  codersdk.ProvisionerJobFailed,
				transition: codersdk.WorkspaceTransitionDelete,
			},
			want: "Failed",
		},
		{
			name: "DefaultStatusWithDeleteTransition",
			args: args{
				jobStatus:  "",
				transition: codersdk.WorkspaceTransitionDelete,
			},
			want: "Loading...",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := workspaceStatus(tt.args.jobStatus, tt.args.transition); got != tt.want {
				t.Errorf("workspaceStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
