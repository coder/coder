package runner_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionerd/runner"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceStateCheckpoint(t *testing.T) {
	t.Parallel()
	const state = `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`
	for _, tc := range []struct {
		name, initError, planError, applyError, graphError string
		state, wantState                                   string
		authoritative, checkpoint                          bool
		foundation, cancelApply, success                   bool
	}{
		{name: "InitFailure", initError: "init failed", wantState: "inherited state"},
		{name: "PlanFailure", planError: "plan failed"},
		{name: "PartialApply", applyError: "apply failed", state: state, wantState: state, authoritative: true, checkpoint: true},
		{name: "ApplyWithoutCheckpoint", applyError: "apply failed"},
		{name: "InterruptedApply", applyError: "signal: killed", state: state, wantState: state},
		{name: "InvalidCheckpoint", applyError: "apply failed", state: "invalid", wantState: "invalid", authoritative: true},
		{name: "MissingCheckpoint", applyError: "apply failed", authoritative: true},
		{name: "GraphFailure", graphError: "graph failed", state: state, wantState: state, authoritative: true, checkpoint: true},
		{name: "GraphFailureWithoutAuthority", graphError: "graph failed", state: state, wantState: state},
		{name: "CanceledPartialApply", applyError: "apply failed", state: state, wantState: state, authoritative: true, cancelApply: true},
		{name: "CanceledSuccess", state: state, wantState: state, authoritative: true, cancelApply: true},
		{name: "FoundationSuccess", state: state, wantState: state, authoritative: true, foundation: true, success: true},
		{name: "FoundationWithoutAuthority", state: state, wantState: state, foundation: true},
		{name: "FoundationInvalidState", state: "invalid", wantState: "invalid", authoritative: true, foundation: true},
		{name: "UnboundSuccessWithoutAuthority", state: state, wantState: state, success: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			clientPipe, serverPipe := drpcsdk.MemTransportPipe()
			defer clientPipe.Close()
			defer serverPipe.Close()
			workDir := t.TempDir()
			serveDone := make(chan struct{})
			go func() {
				defer close(serveDone)
				_ = echo.Serve(ctx, &provisionersdk.ServeOptions{
					Listener: serverPipe, WorkDirectory: workDir, Logger: testutil.Logger(t),
				})
			}()
			defer func() {
				cancel()
				<-serveDone
			}()
			archive, err := echo.Tar(&echo.Responses{
				ProvisionInit: []*sdkproto.Response{{Type: &sdkproto.Response_Init{Init: &sdkproto.InitComplete{Error: tc.initError}}}},
				ProvisionPlan: []*sdkproto.Response{{Type: &sdkproto.Response_Plan{Plan: &sdkproto.PlanComplete{Error: tc.planError}}}},
				ProvisionApply: []*sdkproto.Response{{Type: &sdkproto.Response_Apply{Apply: &sdkproto.ApplyComplete{
					Error: tc.applyError, State: []byte(tc.state), StateIsAuthoritative: tc.authoritative,
				}}}},
				ProvisionGraph: []*sdkproto.Response{{Type: &sdkproto.Response_Graph{Graph: &sdkproto.GraphComplete{Error: tc.graphError}}}},
			})
			require.NoError(t, err)
			updater := &workspaceCheckpointUpdater{}
			job := &proto.AcquiredJob{
				JobId: uuid.NewString(), Provisioner: "echo", TemplateSourceArchive: archive,
				Type: &proto.AcquiredJob_WorkspaceBuild_{WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
					WorkspaceBuildId: uuid.NewString(), State: []byte("inherited state"),
					Metadata: &sdkproto.Metadata{WorkspaceTransition: sdkproto.WorkspaceTransition_START},
				}},
			}
			if tc.foundation {
				job.GetWorkspaceBuild().Metadata.FoundationId = uuid.NewString()
			}
			client := &workspaceCheckpointClient{DRPCProvisionerClient: sdkproto.NewDRPCProvisionerClient(clientPipe)}
			r := runner.New(ctx, job, runner.Options{
				Updater: updater, Provisioner: client,
				Logger: testutil.Logger(t), Tracer: noop.NewTracerProvider().Tracer(t.Name()),
				Metrics:        provisionerd.NewMetrics(prometheus.NewRegistry()).Runner,
				UpdateInterval: time.Hour, ForceCancelInterval: time.Hour, LogDebounceInterval: testutil.IntervalFast,
			})
			if tc.cancelApply {
				client.onApply = r.Cancel
			}
			runDone := make(chan struct{})
			go func() {
				defer close(runDone)
				r.Run()
			}()
			select {
			case <-runDone:
			case <-ctx.Done():
				t.Fatal("runner did not finish")
			}
			if tc.success {
				require.Nil(t, updater.failed)
				require.NotNil(t, updater.completed)
				require.Equal(t, tc.wantState, string(updater.completed.GetWorkspaceBuild().State))
				return
			}
			require.Nil(t, updater.completed)
			require.NotNil(t, updater.failed)
			result := updater.failed.GetWorkspaceBuild()
			require.NotNil(t, result)
			require.Equal(t, tc.checkpoint, result.StateIsCheckpoint)
			require.Equal(t, tc.wantState, string(result.State))
		})
	}
}

type workspaceCheckpointClient struct {
	sdkproto.DRPCProvisionerClient
	onApply func()
}

func (c *workspaceCheckpointClient) Session(ctx context.Context) (sdkproto.DRPCProvisioner_SessionClient, error) {
	sess, err := c.DRPCProvisionerClient.Session(ctx)
	if err != nil {
		return nil, err
	}
	return &workspaceCheckpointSession{DRPCProvisioner_SessionClient: sess, onApply: c.onApply}, nil
}

type workspaceCheckpointSession struct {
	sdkproto.DRPCProvisioner_SessionClient
	onApply func()
}

func (s *workspaceCheckpointSession) Recv() (*sdkproto.Response, error) {
	resp, err := s.DRPCProvisioner_SessionClient.Recv()
	if err == nil && resp.GetApply() != nil && s.onApply != nil {
		s.onApply()
	}
	return resp, err
}

type workspaceCheckpointUpdater struct {
	completed *proto.CompletedJob
	failed    *proto.FailedJob
}

func (*workspaceCheckpointUpdater) UpdateJob(context.Context, *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	return &proto.UpdateJobResponse{}, nil
}

func (u *workspaceCheckpointUpdater) CompleteJob(_ context.Context, job *proto.CompletedJob) error {
	u.completed = job
	return nil
}

func (u *workspaceCheckpointUpdater) FailJob(_ context.Context, job *proto.FailedJob) error {
	u.failed = job
	return nil
}
