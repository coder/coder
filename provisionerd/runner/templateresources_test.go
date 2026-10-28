package runner_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
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

func TestTemplateResources(t *testing.T) {
	t.Parallel()
	const emptyState = `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`
	const managedState = `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{}]}]}`
	const partialState = `{"version":4,"terraform_version":"1.15.0","serial":3,"lineage":"lineage","resources":[{"mode":"managed","instances":[{}]}]}`
	for _, tt := range []struct {
		name            string
		operation       proto.TemplateResourceOperation
		initError       string
		planError       string
		applyError      string
		unauthoritative bool
		state           string
		priorState      *string
		interrupt       string
		interruptPlan   bool
		wantError       string
		disposition     proto.StateDisposition
		timings         int
	}{
		{name: "Plan", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "InitialPlan", priorState: new(""), disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "Apply", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, state: managedState, disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "InitialApply", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, priorState: new(""), state: managedState, disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "InvalidPriorState", priorState: new("{}"), wantError: "prior state", disposition: proto.StateDisposition_UNCHANGED},
		{name: "InitError", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, initError: "init failed", wantError: "init failed", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "PlanError", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, planError: "plan failed", wantError: "plan failed", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "PartialApply", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, applyError: "apply failed", state: partialState, wantError: "apply failed", disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "PartialApplyInvalidState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, applyError: "apply failed", state: "checkpoint", wantError: "apply failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "PartialApplyWithoutAuthority", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, applyError: "apply failed", unauthoritative: true, state: managedState, wantError: "apply failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "KilledApplyWithStaleState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, applyError: "signal: killed", unauthoritative: true, state: managedState, wantError: "signal: killed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "SuccessWithoutAuthority", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, unauthoritative: true, state: managedState, wantError: "non-authoritative state", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "MissingState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, wantError: "returned no state", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "InvalidApplyState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, state: "{}", wantError: "complete Terraform", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "ApplyErrorWithoutState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, applyError: "apply failed", wantError: "apply failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "CancelApply", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, interrupt: "cancel", wantError: "canceled", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "TransportLoss", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, interrupt: "disconnect", wantError: "recv provision", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "CancelPlan", interrupt: "cancel", interruptPlan: true, wantError: "canceled", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "PlanTransportLoss", interrupt: "disconnect", interruptPlan: true, wantError: "receive plan response", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "CancelBeforeApply", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY, interrupt: "cancel", interruptPlan: true, wantError: "canceled", disposition: proto.StateDisposition_UNCHANGED, timings: 1},
		{name: "Destroy", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, state: emptyState, disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "DestroyNoPriorState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, priorState: new(""), wantError: "requires prior state", disposition: proto.StateDisposition_UNCHANGED},
		{name: "DestroyPlanFailure", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, planError: "plan failed", wantError: "plan failed", disposition: proto.StateDisposition_UNCHANGED, timings: 2},
		{name: "DestroyInvalidState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, state: "{}", wantError: "complete Terraform", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "DestroyManagedInstances", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, state: managedState, wantError: "still contains managed instances", disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "PartialDestroy", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, applyError: "destroy failed", state: partialState, wantError: "destroy failed", disposition: proto.StateDisposition_RETURNED, timings: 3},
		{name: "PartialDestroyInvalidState", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, applyError: "destroy failed", state: "{}", wantError: "destroy failed", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "DestroyInterrupted", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, applyError: "canceled", unauthoritative: true, state: managedState, wantError: "canceled", disposition: proto.StateDisposition_UNKNOWN, timings: 3},
		{name: "DestroyTransportLoss", operation: proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY, interrupt: "disconnect", wantError: "recv provision", disposition: proto.StateDisposition_UNKNOWN, timings: 2},
		{name: "UnknownOperation", operation: proto.TemplateResourceOperation(42), wantError: "unknown shared resource operation", disposition: proto.StateDisposition_UNCHANGED},
	} {
		t.Run(tt.name, func(t *testing.T) {
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

			responses := &echo.Responses{
				ProvisionInit: []*sdkproto.Response{{Type: &sdkproto.Response_Init{Init: &sdkproto.InitComplete{
					Error: tt.initError, Timings: []*sdkproto.Timing{{Stage: "init"}},
				}}}},
				ProvisionPlan: []*sdkproto.Response{{Type: &sdkproto.Response_Plan{Plan: &sdkproto.PlanComplete{
					Error: tt.planError, Plan: []byte(`{"resource_changes":[]}`), Timings: []*sdkproto.Timing{{Stage: "plan"}},
				}}}},
				ProvisionApply: []*sdkproto.Response{
					{Type: &sdkproto.Response_Log{Log: &sdkproto.Log{Level: sdkproto.LogLevel_INFO, Output: "applying resources"}}},
					{Type: &sdkproto.Response_Apply{Apply: &sdkproto.ApplyComplete{
						Error: tt.applyError, State: []byte(tt.state), StateIsAuthoritative: !tt.unauthoritative, Timings: []*sdkproto.Timing{{Stage: "apply"}},
					}}},
				},
			}
			if tt.interrupt != "" {
				responses.ProvisionApply = responses.ProvisionApply[:1]
				if tt.interruptPlan {
					responses.ProvisionPlan = []*sdkproto.Response{{
						Type: &sdkproto.Response_Log{Log: &sdkproto.Log{Level: sdkproto.LogLevel_INFO, Output: "planning resources"}},
					}}
				}
			}
			archive, err := echo.Tar(responses)
			require.NoError(t, err)
			priorState := managedState
			if tt.priorState != nil {
				priorState = *tt.priorState
			}
			job := &proto.AcquiredJob{
				JobId: uuid.NewString(), Provisioner: "echo", TemplateSourceArchive: archive,
				Type: &proto.AcquiredJob_TemplateResources_{TemplateResources: &proto.AcquiredJob_TemplateResources{
					Operation: tt.operation, State: []byte(priorState),
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
				if log.Output == "applying resources" || (tt.interruptPlan && log.Output == "planning resources") {
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
				result := updater.failed.GetTemplateResources()
				require.NotNil(t, result)
				require.Equal(t, tt.disposition, result.StateDisposition)
				require.Equal(t, tt.state, string(result.State))
				require.Len(t, result.Timings, tt.timings)
			} else {
				require.Nil(t, updater.failed)
				require.NotNil(t, updater.completed)
				result := updater.completed.GetTemplateResources()
				require.NotNil(t, result)
				require.Equal(t, tt.disposition, result.StateDisposition)
				require.Equal(t, tt.state, string(result.State))
				require.JSONEq(t, `{"resource_changes":[]}`, string(result.Plan))
				require.Len(t, result.Timings, tt.timings)
			}
			require.Equal(t, 0, promtestutil.CollectAndCount(metrics.WorkspaceBuilds))
			require.Equal(t, 0, promtestutil.CollectAndCount(metrics.WorkspaceBuildTimings))
		})
	}
}
