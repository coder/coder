package runner_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
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

func TestNetworkPolicy(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		operation     proto.NetworkPolicyOperation
		initError     string
		planError     string
		applyError    string
		state         []byte
		interrupt     string
		interruptPlan bool
		wantError     string
		disposition   proto.StateDisposition
		timings       int
	}{
		{name: "Plan", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "Apply", operation: proto.NetworkPolicyOperation_APPLY, state: []byte("new state"), disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "InitError", operation: proto.NetworkPolicyOperation_APPLY, initError: "init failed", wantError: "init failed", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "PlanError", operation: proto.NetworkPolicyOperation_APPLY, planError: "plan failed", wantError: "plan failed", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "PartialApply", operation: proto.NetworkPolicyOperation_APPLY, applyError: "apply failed", state: []byte("partial state"), wantError: "apply failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		// Interrupted Terraform may return its pre-existing state file even
		// after changing infrastructure. Nonempty bytes do not prove certainty.
		{name: "KilledApplyWithStaleState", operation: proto.NetworkPolicyOperation_APPLY, applyError: "signal: killed", state: []byte("previous state"), wantError: "signal: killed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "CanceledApplyWithStaleState", operation: proto.NetworkPolicyOperation_APPLY, applyError: "context canceled", state: []byte("previous state"), wantError: "context canceled", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "MissingState", operation: proto.NetworkPolicyOperation_APPLY, wantError: "returned no state", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "ApplyErrorWithoutState", operation: proto.NetworkPolicyOperation_APPLY, applyError: "apply failed", wantError: "apply failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "CancelApply", operation: proto.NetworkPolicyOperation_APPLY, interrupt: "cancel", wantError: "canceled", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "TransportLoss", operation: proto.NetworkPolicyOperation_APPLY, interrupt: "disconnect", wantError: "recv provision", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "CancelPlan", interrupt: "cancel", interruptPlan: true, wantError: "canceled", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "PlanTransportLoss", interrupt: "disconnect", interruptPlan: true, wantError: "receive plan response", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "CancelBeforeApply", operation: proto.NetworkPolicyOperation_APPLY, interrupt: "cancel", interruptPlan: true, wantError: "canceled", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "Destroy", operation: proto.NetworkPolicyOperation_DESTROY, state: []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`), disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "DestroyPlanFailure", operation: proto.NetworkPolicyOperation_DESTROY, planError: "plan failed", wantError: "plan failed", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "DestroyInvalidState", operation: proto.NetworkPolicyOperation_DESTROY, state: []byte(`{}`), wantError: "complete Terraform", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "DestroyInterrupted", operation: proto.NetworkPolicyOperation_DESTROY, applyError: "canceled", state: []byte("checkpoint"), wantError: "canceled", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "DestroyTransportLoss", operation: proto.NetworkPolicyOperation_DESTROY, interrupt: "disconnect", wantError: "recv provision", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "UnknownOperation", operation: proto.NetworkPolicyOperation(42), wantError: "unknown network policy operation", disposition: proto.StateDisposition_UNCHANGED},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
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

			responses := &echo.Responses{
				ProvisionInit: []*sdkproto.Response{{Type: &sdkproto.Response_Init{Init: &sdkproto.InitComplete{
					Error: tt.initError, Timings: []*sdkproto.Timing{{Stage: "init"}},
				}}}},
				ProvisionPlan: []*sdkproto.Response{{Type: &sdkproto.Response_Plan{Plan: &sdkproto.PlanComplete{
					Error: tt.planError, Plan: []byte(`{"resource_changes":[]}`), Timings: []*sdkproto.Timing{{Stage: "plan"}},
				}}}},
				ProvisionApply: []*sdkproto.Response{
					{Type: &sdkproto.Response_Log{Log: &sdkproto.Log{Level: sdkproto.LogLevel_INFO, Output: "applying policy"}}},
					{Type: &sdkproto.Response_Apply{Apply: &sdkproto.ApplyComplete{
						Error: tt.applyError, State: tt.state, Timings: []*sdkproto.Timing{{Stage: "apply"}},
					}}},
				},
			}
			if tt.interrupt != "" {
				responses.ProvisionApply = responses.ProvisionApply[:1]
				if tt.interruptPlan {
					responses.ProvisionPlan = []*sdkproto.Response{{
						Type: &sdkproto.Response_Log{Log: &sdkproto.Log{Level: sdkproto.LogLevel_INFO, Output: "planning policy"}},
					}}
				}
			}
			archive, err := echo.Tar(responses)
			require.NoError(t, err)
			job := &proto.AcquiredJob{
				JobId: uuid.NewString(), Provisioner: "echo", TemplateSourceArchive: archive,
				Type: &proto.AcquiredJob_NetworkPolicy_{NetworkPolicy: &proto.AcquiredJob_NetworkPolicy{
					Operation: tt.operation, State: []byte("previous state"),
				}},
			}
			updater := &policyUpdater{t: t}
			metrics := provisionerd.NewMetrics(prometheus.NewRegistry()).Runner
			r := runner.New(ctx, job, runner.Options{
				Updater: updater, Provisioner: sdkproto.NewDRPCProvisionerClient(clientPipe),
				Logger: testutil.Logger(t), Tracer: noop.NewTracerProvider().Tracer(t.Name()), Metrics: metrics,
				UpdateInterval: time.Hour, ForceCancelInterval: time.Hour, LogDebounceInterval: testutil.IntervalFast,
			})
			var once sync.Once
			updater.onLog = func(log *proto.Log) {
				if log.Output == "applying policy" || (tt.interruptPlan && log.Output == "planning policy") {
					once.Do(func() {
						switch tt.interrupt {
						case "cancel":
							r.Cancel()
						case "disconnect":
							_ = clientPipe.Close()
						}
					})
				}
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
			if tt.wantError != "" {
				require.Nil(t, updater.completed)
				require.NotNil(t, updater.failed)
				require.Contains(t, updater.failed.Error, tt.wantError)
				result := updater.failed.GetNetworkPolicy()
				require.NotNil(t, result)
				require.Equal(t, tt.disposition, result.StateDisposition)
				require.Equal(t, tt.state, result.State)
				require.Len(t, result.Timings, tt.timings)
			} else {
				require.Nil(t, updater.failed)
				require.NotNil(t, updater.completed)
				result := updater.completed.GetNetworkPolicy()
				require.NotNil(t, result)
				require.Equal(t, tt.disposition, result.StateDisposition)
				require.Equal(t, tt.state, result.State)
				require.JSONEq(t, `{"resource_changes":[]}`, string(result.Plan))
				require.Len(t, result.Timings, tt.timings)
			}
			require.Equal(t, 0, promtestutil.CollectAndCount(metrics.WorkspaceBuilds))
			require.Equal(t, 0, promtestutil.CollectAndCount(metrics.WorkspaceBuildTimings))
		})
	}
}

type policyUpdater struct {
	t         *testing.T
	onLog     func(*proto.Log)
	completed *proto.CompletedJob
	failed    *proto.FailedJob
}

func (u *policyUpdater) UpdateJob(_ context.Context, req *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	assert.Empty(u.t, req.TemplateVariables)
	assert.Empty(u.t, req.UserVariableValues)
	assert.Empty(u.t, req.WorkspaceTags)
	assert.Empty(u.t, req.Readme)
	for _, log := range req.Logs {
		assert.False(u.t, strings.Contains(strings.ToLower(log.Stage), "workspace"))
		if u.onLog != nil {
			u.onLog(log)
		}
	}
	return &proto.UpdateJobResponse{}, nil
}

func (u *policyUpdater) CompleteJob(_ context.Context, job *proto.CompletedJob) error {
	u.completed = job
	return nil
}

func (u *policyUpdater) FailJob(_ context.Context, job *proto.FailedJob) error {
	u.failed = job
	return nil
}
